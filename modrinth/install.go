package modrinth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/evictedcucumber/packwiz/cmdshared"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "add [URL]",
	Short: "Add a project from a Modrinth URL",
	Long: `Add a project from a Modrinth URL.

If the project is already in the pack, it isn't added again: the version you asked for (by default the latest) is
compared with the one the pack has, and if they differ you are asked whether to update it. A pinned project is not updated.`,
	Aliases: []string{"install", "get"},
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pack, err := core.LoadPack()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		index, err := pack.LoadIndex()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		// If project/version IDs/version file name is provided in command line, use those
		var projectID, versionID, versionFilename string
		if projectIDFlag != "" {
			projectID = projectIDFlag
			if len(args) != 0 {
				ui.Error.Println("--project-id cannot be used with a separately specified URL")
				os.Exit(1)
			}
		}
		if versionIDFlag != "" {
			versionID = versionIDFlag
			if len(args) != 0 {
				ui.Error.Println("--version-id cannot be used with a separately specified URL")
				os.Exit(1)
			}
		}
		if versionFilenameFlag != "" {
			versionFilename = versionFilenameFlag
		}

		if releaseTypeFlag != "" && !core.IsValidReleaseType(releaseTypeFlag) {
			ui.Error.Printf("Invalid --release-type %q; must be one of: release, beta, alpha\n", releaseTypeFlag)
			os.Exit(1)
		}

		// A version ID is enough: it belongs to a project, which is looked up from it
		if (len(args) == 0 || len(args[0]) == 0) && projectID == "" && versionID == "" {
			ui.Error.Println("You must specify a project; with the ID flags, or by passing a Modrinth URL directly.")
			os.Exit(1)
		}

		var version string
		if projectID == "" && versionID == "" && len(args) == 1 {
			// Interpret the argument as a project/version/CDN URL
			err = parseUrl(args[0], &projectID, &version, &versionID, &versionFilename)
			if err != nil {
				ui.Error.Printf("Failed to parse URL: %v\n", err)
				os.Exit(1)
			}
		}

		// Modrinth transparently handles slugs/project IDs in their API; we don't have to detect which one it is.
		project, versionData, err := resolveTarget(pack, &index, projectID, version, versionID, releaseTypeFlag)
		if err != nil {
			ui.Error.Printf("Failed to add project: %s\n", err)
			os.Exit(1)
		}
		if err := installVersion(project, versionData, versionFilename, pack, &index, releaseTypeFlag); err != nil {
			ui.Error.Printf("Failed to add project: %s\n", err)
			os.Exit(1)
		}
	},
}

// resolveTarget finds the project that is to be added, and the version of it: the one with the given ID, if a version ID
// is given (it belongs to a project, which is looked up from it); else the one with the given number, if there is one; else
// the latest that suits the pack. A project can be given by its ID or its slug.
func resolveTarget(pack core.Pack, index *core.Index, projectID, versionNumber, versionID, releaseType string) (*modrinthApi.Project, *modrinthApi.Version, error) {
	if versionID != "" {
		version, err := mrDefaultClient.Versions.Get(versionID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch version %s: %v", versionID, err)
		}
		project, err := mrDefaultClient.Projects.Get(*version.ProjectID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch project %s: %v", *version.ProjectID, err)
		}
		return project, version, nil
	}

	project, err := mrDefaultClient.Projects.Get(projectID)
	if err != nil {
		return nil, nil, err
	}
	if versionNumber != "" {
		// Try to look up version number
		version, err := resolveVersion(project, versionNumber)
		return project, version, err
	}

	// No version specified; find latest
	version, err := latestVersionOfProject(project, pack, index, releaseType)
	return project, version, err
}

func installProject(project *modrinthApi.Project, versionFilename string, pack core.Pack, index *core.Index, releaseType string) error {
	latestVersion, err := latestVersionOfProject(project, pack, index, releaseType)
	if err != nil {
		return err
	}
	return installVersion(project, latestVersion, versionFilename, pack, index, releaseType)
}

// latestVersionOfProject finds the version of a project that adding it without saying which adds: its latest one, for
// the pack. releaseType is the release type to accept if it was asked for, and else the project is looked up with the one
// it was added with, if the pack has it.
func latestVersionOfProject(project *modrinthApi.Project, pack core.Pack, index *core.Index, releaseType string) (*modrinthApi.Version, error) {
	lookupReleaseType := releaseType
	if lookupReleaseType == "" {
		// A project that is already added is looked up with the release type it was added with, as 'packwiz mr update'
		// does, so that both agree on what its latest version is
		existing, err := findInstalledMod(index, *project.ID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			data, _ := modrinthUpdateData(existing)
			lookupReleaseType = data.ReleaseType
		}
	}

	acceptFabric := runsFabricMods(pack, getInstalledProjectIDs(index))
	latestVersion, err := getLatestVersion(*project.ID, *project.Title, pack, lookupReleaseType, acceptFabric)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest version: %v", err)
	}
	if latestVersion.ID == nil {
		return nil, errors.New("mod not available for the configured Minecraft version(s) (use the 'packwiz settings acceptable-versions' command to accept more) or loader")
	}
	return latestVersion, nil
}

const maxCycles = 20

type depMetadataStore struct {
	projectInfo *modrinthApi.Project
	versionInfo *modrinthApi.Version
	fileInfo    *modrinthApi.File
}

// findDependencies finds what has to be added to the pack for the required dependencies given, by project ID and by
// version ID, and for what those need in turn, at their latest versions. installedProjects are the projects the pack
// has already, and acceptFabric says whether it runs Fabric mods (see runsFabricMods). A dependency it can't find a
// version of is said so and left out.
func findDependencies(pack core.Pack, projectIDs, versionIDs, installedProjects []string, acceptFabric bool) ([]depMetadataStore, error) {
	var depMetadata []depMetadataStore
	depProjectIDPendingQueue := slices.Clone(projectIDs)
	depVersionIDPendingQueue := slices.Clone(versionIDs)

	cycles := 0
	for len(depProjectIDPendingQueue)+len(depVersionIDPendingQueue) > 0 && cycles < maxCycles {
		// Look up version IDs
		if len(depVersionIDPendingQueue) > 0 {
			depVersions, err := mrDefaultClient.Versions.GetMultiple(depVersionIDPendingQueue)
			if err == nil {
				for _, v := range depVersions {
					// Add project ID to queue
					depProjectIDPendingQueue = append(depProjectIDPendingQueue, *v.ProjectID)
				}
			} else {
				notice.Errorf("Error retrieving dependency data: %s", err.Error())
			}
			depVersionIDPendingQueue = depVersionIDPendingQueue[:0]
		}

		// Remove installed project IDs from dep queue
		i := 0
		for _, id := range depProjectIDPendingQueue {
			contains := slices.Contains(installedProjects, id)
			for _, dep := range depMetadata {
				if *dep.projectInfo.ID == id {
					contains = true
					break
				}
			}
			if !contains {
				depProjectIDPendingQueue[i] = id
				i++
			}
		}
		depProjectIDPendingQueue = depProjectIDPendingQueue[:i]

		// Clean up duplicates from dep queue
		slices.Sort(depProjectIDPendingQueue)
		depProjectIDPendingQueue = slices.Compact(depProjectIDPendingQueue)

		if len(depProjectIDPendingQueue) == 0 {
			break
		}
		depProjects, err := mrDefaultClient.Projects.GetMultiple(depProjectIDPendingQueue)
		if err != nil {
			notice.Errorf("Error retrieving dependency data: %s", err.Error())
		}
		depProjectIDPendingQueue = depProjectIDPendingQueue[:0]

		for _, project := range depProjects {
			if project.ID == nil {
				return nil, errors.New("failed to get dependency data: invalid response")
			}
			// Get latest version - could reuse version lookup data but it's not as easy (particularly since the version won't necessarily be the latest)
			// Dependencies use the pack's default release type rather than inheriting the flag passed for the mod being added
			latestVersion, err := getLatestVersion(*project.ID, *project.Title, pack, "", acceptFabric)
			if err != nil {
				notice.Errorf("Failed to get latest version of dependency %v: %v", *project.Title, err)
				continue
			}
			// Only got a Fabric version because the pack runs Fabric mods
			noticeFabricMod(*project.Title, latestVersion, pack)

			for _, dep := range latestVersion.Dependencies {
				// TODO: recommend optional dependencies?
				if dep.DependencyType != nil && *dep.DependencyType == "required" {
					if dep.ProjectID != nil {
						depProjectIDPendingQueue = append(depProjectIDPendingQueue, *dep.ProjectID)
					}
					if dep.VersionID != nil {
						depVersionIDPendingQueue = append(depVersionIDPendingQueue, *dep.VersionID)
					}
				}
			}

			var file = latestVersion.Files[0]
			// Prefer the primary file
			for _, v := range latestVersion.Files {
				if isPrimary(v) {
					file = v
				}
			}

			depMetadata = append(depMetadata, depMetadataStore{
				projectInfo: project,
				versionInfo: latestVersion,
				fileInfo:    file,
			})
		}

		cycles++
	}
	if cycles >= maxCycles {
		return nil, errors.New("dependencies recurse too deeply, try increasing maxCycles")
	}

	return depMetadata, nil
}

// installPlan is what adding a version of a project to the pack would do, found out in stages, so that what a person
// is asked comes at the right point: newInstallPlan finds out whether the pack has the project already, lookUpDependencies
// what has to be added along with it, and apply does it.
type installPlan struct {
	pack        core.Pack
	project     *modrinthApi.Project
	version     *modrinthApi.Version
	file        *modrinthApi.File
	releaseType string

	// existing is the mod the pack has from the project already, which is updated in place, or nil. If it has this version
	// already (upToDate) or is pinned (pinned) nothing is done to it.
	existing         *core.Mod
	upToDate, pinned bool

	// requiresDependencies is whether the version requires any project at all, and deps are the ones the pack doesn't have,
	// with what to add of each, once they have been looked up
	requiresDependencies bool
	deps                 []depMetadataStore
}

// newInstallPlan starts the plan for adding version of project. It asks nothing of Modrinth.
func newInstallPlan(project *modrinthApi.Project, version *modrinthApi.Version, versionFilename string, pack core.Pack, index *core.Index, releaseType string) (*installPlan, error) {
	if len(version.Files) == 0 {
		return nil, errors.New("version doesn't have any files attached")
	}

	var file = version.Files[0]
	// Prefer the primary file
	for _, v := range version.Files {
		if isPrimary(v) || (versionFilename != "" && v.Filename != nil && versionFilename == *v.Filename) {
			file = v
		}
	}
	// TODO: handle optional/required resource pack files

	plan := &installPlan{pack: pack, project: project, version: version, file: file, releaseType: releaseType}
	var err error
	if plan.existing, err = findInstalledMod(index, *project.ID); err != nil {
		return nil, err
	}
	if plan.existing != nil {
		if data, _ := modrinthUpdateData(plan.existing); data.InstalledVersion == *version.ID {
			plan.upToDate = true
		} else if plan.existing.Pin {
			plan.pinned = true
		}
	}
	return plan, nil
}

// lookUpDependencies finds what has to be added along with the project: the required projects that the pack doesn't have,
// and what those require in turn, at their latest versions. It needs the network.
func (p *installPlan) lookUpDependencies(index *core.Index) error {
	installedProjects := getInstalledProjectIDs(index)
	acceptFabric := runsFabricMods(p.pack, installedProjects)
	if acceptFabric {
		noticeFabricMod(*p.project.Title, p.version, p.pack)
	}

	if len(p.version.Dependencies) == 0 {
		return nil
	}
	// TODO: could get installed version IDs, and compare to install the newest - i.e. preferring pinned versions over getting absolute latest?
	if acceptFabric {
		// Forgified Fabric API takes the place of Fabric API, which can't be added next to it
		installedProjects = append(installedProjects, fabricAPIProjectID)
	}

	var depProjectIDPendingQueue []string
	var depVersionIDPendingQueue []string

	for _, dep := range p.version.Dependencies {
		// TODO: recommend optional dependencies?
		if dep.DependencyType != nil && *dep.DependencyType == "required" {
			if dep.VersionID != nil {
				depVersionIDPendingQueue = append(depVersionIDPendingQueue, *dep.VersionID)
			} else {
				if dep.ProjectID != nil {
					depProjectIDPendingQueue = append(depProjectIDPendingQueue, *dep.ProjectID)
				}
			}
		}
	}

	if len(depProjectIDPendingQueue)+len(depVersionIDPendingQueue) == 0 {
		return nil
	}
	notice.Mutedf("Finding dependencies...")
	p.requiresDependencies = true

	var err error
	p.deps, err = findDependencies(p.pack, depProjectIDPendingQueue, depVersionIDPendingQueue, installedProjects, acceptFabric)
	return err
}

// apply adds the project to the pack, or updates it if the pack has it, and with addDependencies the dependencies that
// were found too. Each dependency that was added is passed to added, if that isn't nil. It saves the index and the pack.
func (p *installPlan) apply(index *core.Index, addDependencies bool, added func(depMetadataStore)) error {
	if addDependencies {
		for _, v := range p.deps {
			if err := createFileMeta(v.projectInfo, v.versionInfo, v.fileInfo, p.pack, index, "", true); err != nil {
				return err
			}
			if added != nil {
				added(v)
			}
		}
	}

	// Create the metadata file, or update the one the project already has
	var err error
	if p.existing != nil {
		err = updateFileMeta(p.existing, p.version, p.file, p.releaseType, index)
	} else {
		err = createFileMeta(p.project, p.version, p.file, p.pack, index, p.releaseType, false)
	}
	if err != nil {
		return err
	}

	// After both the mod and its dependencies are in the pack, so that it doesn't matter which was added first
	if err := promoteSides(index); err != nil {
		return err
	}

	if err := index.Write(); err != nil {
		return err
	}
	if err := p.pack.UpdateIndexHash(); err != nil {
		return err
	}
	return p.pack.Write()
}

func installVersion(project *modrinthApi.Project, version *modrinthApi.Version, versionFilename string, pack core.Pack, index *core.Index, releaseType string) error {
	plan, err := newInstallPlan(project, version, versionFilename, pack, index, releaseType)
	if err != nil {
		return err
	}

	if plan.existing != nil {
		update, err := confirmUpdate(plan)
		if err != nil {
			return err
		}
		if !update {
			return nil
		}
	}

	if err := plan.lookUpDependencies(index); err != nil {
		return err
	}

	addDependencies := false
	if plan.requiresDependencies {
		if len(plan.deps) > 0 {
			ui.Bold.Println("Dependencies found:")
			for _, v := range plan.deps {
				fmt.Println(*v.projectInfo.Title)
			}
			addDependencies = cmdshared.PromptYesNo("Would you like to add them? [Y/n]: ")
		} else {
			ui.Success.Println("All dependencies are already added!")
		}
	}

	err = plan.apply(index, addDependencies, func(v depMetadataStore) {
		ui.Success.Printf("Dependency \"%s\" successfully added! %s\n", ui.Bold.Sprint(*v.projectInfo.Title), ui.Muted.Sprintf("(%s)", *v.fileInfo.Filename))
	})
	if err != nil {
		return err
	}

	verb := "added"
	if plan.existing != nil {
		verb = "updated"
	}
	ui.Success.Printf("Project \"%s\" successfully %s! %s\n", ui.Bold.Sprint(*project.Title), verb, ui.Muted.Sprintf("(%s)", *plan.file.Filename))
	return nil
}

// confirmUpdate decides what to do when a version of a project is added but the pack already has the project. It
// reports whether the project should be updated to that version: not if it already has that version, if it is pinned, or
// if the user says no. It says why, except when it is an error.
func confirmUpdate(plan *installPlan) (bool, error) {
	existing := plan.existing
	if plan.upToDate {
		ui.Success.Printf("\"%s\" is already added and up to date! %s\n", ui.Bold.Sprint(existing.Name), ui.Muted.Sprintf("(%s)", existing.FileName))
		return false, nil
	}
	// Checked before asking
	if plan.pinned {
		return false, fmt.Errorf("\"%s\" is pinned; run the unpin command to allow updating", existing.Name)
	}

	from, to := existing.Version, versionNumberOf(plan.version)
	if from == "" || to == "" {
		// Mods added before their version was recorded only have a file name to go by
		from, to = existing.FileName, *plan.file.Filename
	}
	ui.Info.Printf("\"%s\" is already added. Update available: %s\n", ui.Bold.Sprint(existing.Name), ui.Transition(from, to))
	if !cmdshared.PromptYesNo("Would you like to update it? [Y/n]: ") {
		ui.Warning.Println("Cancelled!")
		return false, nil
	}
	return true, nil
}

// updateFileMeta updates the metadata file of a mod that is already in the pack to file, one of the files of
// version. Unlike createFileMeta this keeps what the user has set on the mod (its pin, option, and so on), and it
// stays where it is, whatever it is called.
func updateFileMeta(mod *core.Mod, version *modrinthApi.Version, file *modrinthApi.File, releaseType string, index *core.Index) error {
	if err := applyVersion(mod, version, file); err != nil {
		return err
	}
	// Only changed when explicitly overridden; otherwise the mod keeps the release type it has
	if releaseType != "" {
		mod.Update["modrinth"]["release-type"] = releaseType
	}

	format, hash, err := mod.Write()
	if err != nil {
		return err
	}
	return index.RefreshFileWithHash(mod.GetFilePath(), format, hash, true)
}

// createFileMeta adds a project to the pack, at version, by writing its metadata file and putting it in the index
func createFileMeta(project *modrinthApi.Project, version *modrinthApi.Version, file *modrinthApi.File, pack core.Pack, index *core.Index, releaseType string, isDependency bool) error {
	modMeta, err := newFileMeta(project, version, file, pack, releaseType, isDependency)
	if err != nil {
		return err
	}

	// If a file already exists here, this will overwrite it!!! A project that is already in the pack is updated in
	// place by installVersion instead of coming here, so that would be an unrelated file.
	// TODO: Should this be improved?
	// Current strategy is to go ahead and do stuff without asking, with the assumption that you are using
	// VCS anyway.

	format, hash, err := modMeta.Write()
	if err != nil {
		return err
	}
	return index.RefreshFileWithHash(modMeta.GetFilePath(), format, hash, true)
}

// newFileMeta is the metadata a project would be added to the pack with, at version, and where it would be saved. It
// isn't saved, so it can be shown, or changed, first.
func newFileMeta(project *modrinthApi.Project, version *modrinthApi.Version, file *modrinthApi.File, pack core.Pack, releaseType string, isDependency bool) (core.Mod, error) {
	updateMap := make(map[string]map[string]interface{})

	var err error
	updateMap["modrinth"], err = mrUpdateData{
		ProjectID:        *project.ID,
		InstalledVersion: *version.ID,
		// Only persisted when explicitly overridden; empty means "use the pack default"
		ReleaseType: releaseType,
	}.ToMap()
	if err != nil {
		return core.Mod{}, err
	}

	side := getSide(project)
	if side == "" {
		notice.Warnf("%s", "Warning: Project doesn't have a side that's supported; assuming universal. Server: "+*project.ServerSide+" Client: "+*project.ClientSide)
		side = core.UniversalSide
	}

	algorithm, hash := getBestHash(file)
	if algorithm == "" {
		return core.Mod{}, errors.New("file doesn't have a hash")
	}

	modMeta := core.Mod{
		Name:     *project.Title,
		FileName: *file.Filename,
		Version:  versionNumberOf(version),
		Side:     side,
		Download: core.ModDownload{
			URL:        *file.URL,
			HashFormat: algorithm,
			Hash:       hash,
		},
		Update:            updateMap,
		Dependencies:      buildDependencyList(version),
		AddedAsDependency: isDependency,
		ConfigFiles:       &[]string{},
	}
	folder, err := getProjectTypeFolder(*project.ProjectType, version.Loaders, pack.GetCompatibleLoaders())
	if err != nil {
		return core.Mod{}, err
	}
	if project.Slug != nil {
		modMeta.SetMetaPath(filepath.Join(folder, *project.Slug+core.MetaExtension))
	} else {
		modMeta.SetMetaPath(filepath.Join(folder, core.SlugifyName(*project.Title)+core.MetaExtension))
	}
	return modMeta, nil
}

var projectIDFlag string
var versionIDFlag string
var versionFilenameFlag string
var releaseTypeFlag string

func init() {
	modrinthCmd.AddCommand(installCmd)

	installCmd.Flags().StringVar(&projectIDFlag, "project-id", "", "The Modrinth project ID to use")
	installCmd.Flags().StringVar(&versionIDFlag, "version-id", "", "The Modrinth version ID to use")
	installCmd.Flags().StringVar(&versionFilenameFlag, "version-filename", "", "The Modrinth version filename to use")
	installCmd.Flags().StringVar(&releaseTypeFlag, "release-type", "", "The minimum release type to accept when looking up the latest version (release, beta or alpha); overrides the pack default for this mod")
}
