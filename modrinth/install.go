package modrinth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/evictedcucumber/packwiz/cmdshared"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/spf13/viper"

	"github.com/evictedcucumber/packwiz/core"
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

		// Got version ID; install using this ID
		if versionID != "" {
			err = installVersionById(versionID, versionFilename, pack, &index, releaseTypeFlag)
			if err != nil {
				ui.Error.Printf("Failed to add project: %s\n", err)
				os.Exit(1)
			}
			return
		}

		// Look up project ID
		// Modrinth transparently handles slugs/project IDs in their API; we don't have to detect which one it is.
		project, err := mrDefaultClient.Projects.Get(projectID)
		if err != nil {
			ui.Error.Printf("Failed to add project: %s\n", err)
			os.Exit(1)
		}

		if version != "" {
			// Try to look up version number
			versionData, err := resolveVersion(project, version)
			if err != nil {
				ui.Error.Printf("Failed to add project: %s\n", err)
				os.Exit(1)
			}
			err = installVersion(project, versionData, versionFilename, pack, &index, releaseTypeFlag)
			if err != nil {
				ui.Error.Printf("Failed to add project: %s\n", err)
				os.Exit(1)
			}
			return
		}

		// No version specified; find latest
		err = installProject(project, versionFilename, pack, &index, releaseTypeFlag)
		if err != nil {
			ui.Error.Printf("Failed to add project: %s\n", err)
			os.Exit(1)
		}
	},
}

func installVersionById(versionId string, versionFilename string, pack core.Pack, index *core.Index, releaseType string) error {
	version, err := mrDefaultClient.Versions.Get(versionId)
	if err != nil {
		return fmt.Errorf("failed to fetch version %s: %v", versionId, err)
	}

	project, err := mrDefaultClient.Projects.Get(*version.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to fetch project %s: %v", *version.ProjectID, err)
	}

	return installVersion(project, version, versionFilename, pack, index, releaseType)
}

func installProject(project *modrinthApi.Project, versionFilename string, pack core.Pack, index *core.Index, releaseType string) error {
	lookupReleaseType := releaseType
	if lookupReleaseType == "" {
		// A project that is already added is looked up with the release type it was added with, as 'packwiz update'
		// does, so that both agree on what its latest version is
		existing, err := findInstalledMod(index, *project.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			data, _ := modrinthUpdateData(existing)
			lookupReleaseType = data.ReleaseType
		}
	}

	acceptFabric := runsFabricMods(pack, getInstalledProjectIDs(index))
	latestVersion, err := getLatestVersion(*project.ID, *project.Title, pack, lookupReleaseType, acceptFabric)
	if err != nil {
		return fmt.Errorf("failed to get latest version: %v", err)
	}
	if latestVersion.ID == nil {
		return errors.New("mod not available for the configured Minecraft version(s) (use the 'packwiz settings acceptable-versions' command to accept more) or loader")
	}

	return installVersion(project, latestVersion, versionFilename, pack, index, releaseType)
}

const maxCycles = 20

type depMetadataStore struct {
	projectInfo *modrinthApi.Project
	versionInfo *modrinthApi.Version
	fileInfo    *modrinthApi.File
}

func installVersion(project *modrinthApi.Project, version *modrinthApi.Version, versionFilename string, pack core.Pack, index *core.Index, releaseType string) error {
	if len(version.Files) == 0 {
		return errors.New("version doesn't have any files attached")
	}

	var file = version.Files[0]
	// Prefer the primary file
	for _, v := range version.Files {
		if isPrimary(v) || (versionFilename != "" && v.Filename != nil && versionFilename == *v.Filename) {
			file = v
		}
	}
	// TODO: handle optional/required resource pack files

	existing, err := findInstalledMod(index, *project.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		update, err := confirmUpdate(existing, version, file)
		if err != nil {
			return err
		}
		if !update {
			return nil
		}
	}

	installedProjects := getInstalledProjectIDs(index)
	acceptFabric := runsFabricMods(pack, installedProjects)
	if acceptFabric {
		noticeFabricMod(*project.Title, version, pack)
	}

	if len(version.Dependencies) > 0 {
		// TODO: could get installed version IDs, and compare to install the newest - i.e. preferring pinned versions over getting absolute latest?
		if acceptFabric {
			// Forgified Fabric API takes the place of Fabric API, which can't be added next to it
			installedProjects = append(installedProjects, fabricAPIProjectID)
		}

		var depMetadata []depMetadataStore
		var depProjectIDPendingQueue []string
		var depVersionIDPendingQueue []string

		for _, dep := range version.Dependencies {
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

		if len(depProjectIDPendingQueue)+len(depVersionIDPendingQueue) > 0 {
			ui.Muted.Println("Finding dependencies...")

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
						ui.Error.Printf("Error retrieving dependency data: %s\n", err.Error())
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
					ui.Error.Printf("Error retrieving dependency data: %s\n", err.Error())
				}
				depProjectIDPendingQueue = depProjectIDPendingQueue[:0]

				for _, project := range depProjects {
					if project.ID == nil {
						return errors.New("failed to get dependency data: invalid response")
					}
					// Get latest version - could reuse version lookup data but it's not as easy (particularly since the version won't necessarily be the latest)
					// Dependencies use the pack's default release type rather than inheriting the flag passed for the mod being added
					latestVersion, err := getLatestVersion(*project.ID, *project.Title, pack, "", acceptFabric)
					if err != nil {
						ui.Error.Printf("Failed to get latest version of dependency %v: %v\n", *project.Title, err)
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
				return errors.New("dependencies recurse too deeply, try increasing maxCycles")
			}

			if len(depMetadata) > 0 {
				ui.Bold.Println("Dependencies found:")
				for _, v := range depMetadata {
					fmt.Println(*v.projectInfo.Title)
				}

				if cmdshared.PromptYesNo("Would you like to add them? [Y/n]: ") {
					for _, v := range depMetadata {
						err := createFileMeta(v.projectInfo, v.versionInfo, v.fileInfo, pack, index, "", true)
						if err != nil {
							return err
						}
						ui.Success.Printf("Dependency \"%s\" successfully added! %s\n", ui.Bold.Sprint(*v.projectInfo.Title), ui.Muted.Sprintf("(%s)", *v.fileInfo.Filename))
					}
				}
			} else {
				ui.Success.Println("All dependencies are already added!")
			}
		}
	}

	// Create the metadata file, or update the one the project already has
	if existing != nil {
		err = updateFileMeta(existing, version, file, releaseType, index)
	} else {
		err = createFileMeta(project, version, file, pack, index, releaseType, false)
	}
	if err != nil {
		return err
	}

	// After both the mod and its dependencies are in the pack, so that it doesn't matter which was added first
	err = promoteSides(index)
	if err != nil {
		return err
	}

	err = index.Write()
	if err != nil {
		return err
	}
	err = pack.UpdateIndexHash()
	if err != nil {
		return err
	}
	err = pack.Write()
	if err != nil {
		return err
	}

	verb := "added"
	if existing != nil {
		verb = "updated"
	}
	ui.Success.Printf("Project \"%s\" successfully %s! %s\n", ui.Bold.Sprint(*project.Title), verb, ui.Muted.Sprintf("(%s)", *file.Filename))
	return nil
}

// confirmUpdate decides what to do when version of a project is added but the pack already has the project, as
// existing. It reports whether existing should be updated to version (using file): not if it already has that version,
// if it is pinned, or if the user says no. It says why, except when it is an error.
func confirmUpdate(existing *core.Mod, version *modrinthApi.Version, file *modrinthApi.File) (bool, error) {
	if data, _ := modrinthUpdateData(existing); data.InstalledVersion == *version.ID {
		ui.Success.Printf("\"%s\" is already added and up to date! %s\n", ui.Bold.Sprint(existing.Name), ui.Muted.Sprintf("(%s)", existing.FileName))
		return false, nil
	}
	// Checked before asking, so that -y can't be used to get past it
	if existing.Pin {
		return false, fmt.Errorf("\"%s\" is pinned; run the unpin command to allow updating", existing.Name)
	}

	from, to := existing.Version, versionNumberOf(version)
	if from == "" || to == "" {
		// Mods added before their version was recorded only have a file name to go by
		from, to = existing.FileName, *file.Filename
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

func createFileMeta(project *modrinthApi.Project, version *modrinthApi.Version, file *modrinthApi.File, pack core.Pack, index *core.Index, releaseType string, isDependency bool) error {
	updateMap := make(map[string]map[string]interface{})

	var err error
	updateMap["modrinth"], err = mrUpdateData{
		ProjectID:        *project.ID,
		InstalledVersion: *version.ID,
		// Only persisted when explicitly overridden; empty means "use the pack default"
		ReleaseType: releaseType,
	}.ToMap()
	if err != nil {
		return err
	}

	side := getSide(project)
	if side == "" {
		ui.Warning.Println("Warning: Project doesn't have a side that's supported; assuming universal. Server: " + *project.ServerSide + " Client: " + *project.ClientSide)
		side = core.UniversalSide
	}

	algorithm, hash := getBestHash(file)
	if algorithm == "" {
		return errors.New("file doesn't have a hash")
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
	}
	var path string
	folder := viper.GetString("meta-folder")
	if folder == "" {
		folder, err = getProjectTypeFolder(*project.ProjectType, version.Loaders, pack.GetCompatibleLoaders())
		if err != nil {
			return err
		}
	}
	if project.Slug != nil {
		path = modMeta.SetMetaPath(filepath.Join(viper.GetString("meta-folder-base"), folder, *project.Slug+core.MetaExtension))
	} else {
		path = modMeta.SetMetaPath(filepath.Join(viper.GetString("meta-folder-base"), folder, core.SlugifyName(*project.Title)+core.MetaExtension))
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
	return index.RefreshFileWithHash(path, format, hash, true)
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
