package modrinth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	modrinthApi "codeberg.org/jmansfield/go-modrinth/modrinth"
	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/spf13/viper"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"gopkg.in/dixonwille/wmenu.v4"
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:     "add [URL]",
	Short:   "Add a project from a Modrinth URL",
	Aliases: []string{"install", "get"},
	Args:    cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pack, err := core.LoadPack()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		index, err := pack.LoadIndex()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// If project/version IDs/version file name is provided in command line, use those
		var projectID, versionID, versionFilename string
		if projectIDFlag != "" {
			projectID = projectIDFlag
			if len(args) != 0 {
				fmt.Println("--project-id cannot be used with a separately specified URL")
				os.Exit(1)
			}
		}
		if versionIDFlag != "" {
			versionID = versionIDFlag
			if len(args) != 0 {
				fmt.Println("--version-id cannot be used with a separately specified URL")
				os.Exit(1)
			}
		}
		if versionFilenameFlag != "" {
			versionFilename = versionFilenameFlag
		}

		if (len(args) == 0 || len(args[0]) == 0) && projectID == "" {
			fmt.Println("You must specify a project; with the ID flags, or by passing a Modrinth URL directly.")
			os.Exit(1)
		}

		var version string
		var parsedSlug bool
		if projectID == "" && versionID == "" && len(args) == 1 {
			if !strings.HasPrefix(args[0], "https://") && !strings.HasPrefix(args[0], "http://") {
				fmt.Println("Only Modrinth links are accepted for add; search terms and slugs are not supported.")
				os.Exit(1)
			}

			// Parse project/version/CDN URL
			parsedSlug, err = parseSlugOrUrl(args[0], &projectID, &version, &versionID, &versionFilename)
			if err != nil {
				fmt.Printf("Failed to parse URL: %v\n", err)
				os.Exit(1)
			}
			if parsedSlug {
				fmt.Println("Only Modrinth links are accepted for add; search terms and slugs are not supported.")
				os.Exit(1)
			}
			if projectID == "" && versionID == "" {
				fmt.Println("Only valid Modrinth links are accepted for add.")
				os.Exit(1)
			}
		}

		// Got version ID; install using this ID
		if versionID != "" {
			allowedChannel := updateChannelFlag
			if allowedChannel == "" {
				allowedChannel = pack.GetAllowedChannel(nil)
			}
			err = installVersionById(versionID, versionFilename, pack, &index, allowedChannel)
			if err != nil {
				fmt.Printf("Failed to add project: %s\n", err)
				os.Exit(1)
			}
			return
		}

		// Look up project ID
		if projectID != "" {
			// Modrinth transparently handles slugs/project IDs in their API; we don't have to detect which one it is.
			var project *modrinthApi.Project
			project, err = mrDefaultClient.Projects.Get(projectID)
			if err == nil {
				// We found a project with that id/slug
				allowedChannel := updateChannelFlag
				if allowedChannel == "" {
					allowedChannel = pack.GetAllowedChannel(nil)
				}
				if version != "" {
					// Try to look up version number
					versionData, err := resolveVersion(project, version)
					if err != nil {
						fmt.Printf("Failed to add project: %s\n", err)
						os.Exit(1)
					}
					err = installVersion(project, versionData, versionFilename, pack, &index, allowedChannel)
					if err != nil {
						fmt.Printf("Failed to add project: %s\n", err)
						os.Exit(1)
					}
					return
				}

				// No version specified; find latest
				err = installProject(project, versionFilename, pack, &index, allowedChannel)
				if err != nil {
					fmt.Printf("Failed to add project: %s\n", err)
					os.Exit(1)
				}
				return
			}
		}

		fmt.Printf("Failed to add project: %s\n", err)
		os.Exit(1)
	},
}

func installVersionById(versionId string, versionFilename string, pack core.Pack, index *core.Index, allowedChannel string) error {
	version, err := mrDefaultClient.Versions.Get(versionId)
	if err != nil {
		return fmt.Errorf("failed to fetch version %s: %v", versionId, err)
	}

	project, err := mrDefaultClient.Projects.Get(*version.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to fetch project %s: %v", *version.ProjectID, err)
	}

	return installVersion(project, version, versionFilename, pack, index, allowedChannel)
}

func installViaSearch(query string, versionFilename string, autoAcceptFirst bool, pack core.Pack, index *core.Index, allowedChannel string) error {
	mcVersions, err := pack.GetSupportedMCVersions()
	if err != nil {
		return err
	}

	fmt.Println("Searching Modrinth...")

	results, err := getProjectIdsViaSearch(query, mcVersions)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		return errors.New("no projects found")
	}

	if viper.GetBool("non-interactive") || (len(results) == 1 && autoAcceptFirst) {
		// Install the first project found
		project, err := mrDefaultClient.Projects.Get(*results[0].ProjectID)
		if err != nil {
			return err
		}

		return installProject(project, versionFilename, pack, index, allowedChannel)
	}

	// Create menu for the user to choose the correct project
	menu := wmenu.NewMenu("Choose a number:")
	menu.Option("Cancel", nil, false, nil)
	for i, v := range results {
		// Should be non-nil (Title is a required field)
		menu.Option(*v.Title, v, i == 0, nil)
	}

	menu.Action(func(menuRes []wmenu.Opt) error {
		if len(menuRes) != 1 || menuRes[0].Value == nil {
			return errors.New("project selection cancelled")
		}

		// Get the selected project
		selectedProject, ok := menuRes[0].Value.(*modrinthApi.SearchResult)
		if !ok {
			return errors.New("error converting interface from wmenu")
		}

		// Install the selected project
		project, err := mrDefaultClient.Projects.Get(*selectedProject.ProjectID)
		if err != nil {
			return err
		}

		return installProject(project, versionFilename, pack, index, allowedChannel)
	})

	return menu.Run()
}

func installProject(project *modrinthApi.Project, versionFilename string, pack core.Pack, index *core.Index, allowedChannel string) error {
	latestVersion, err := getLatestVersion(*project.ID, *project.Title, pack, allowedChannel)
	if err != nil {
		return fmt.Errorf("failed to get latest version: %v", err)
	}
	if latestVersion.ID == nil {
		return errors.New("mod not available for the configured Minecraft version(s) (use the 'packwiz settings acceptable-versions' command to accept more) or loader")
	}

	return installVersion(project, latestVersion, versionFilename, pack, index, allowedChannel)
}

const maxCycles = 20

type depMetadataStore struct {
	projectInfo   *modrinthApi.Project
	versionInfo   *modrinthApi.Version
	fileInfo      *modrinthApi.File
	deps          []string
	metaPath      string
	updateChannel string
}

type modrinthProjectQueueItem struct {
	projectID string
	parentID  string
}

type modrinthVersionQueueItem struct {
	versionID string
	parentID  string
}

func installVersion(project *modrinthApi.Project, version *modrinthApi.Version, versionFilename string, pack core.Pack, index *core.Index, allowedChannel string) error {
	if len(version.Files) == 0 {
		return errors.New("version doesn't have any files attached")
	}

	var file = version.Files[0]
	// Prefer the primary file
	for _, v := range version.Files {
		if (*v.Primary) || (versionFilename != "" && versionFilename == *v.Filename) {
			file = v
		}
	}
	// TODO: handle optional/required resource pack files

	// Create the metadata file
	err := createFileMeta(project, version, file, pack, index, false, nil, updateChannelFlag)
	if err != nil {
		return err
	}

	// Load the newly created mod file to pass it to CheckAndInstallDependencies
	newModPath, err := getModrinthMetaPath(project, version, pack)
	if err != nil {
		return err
	}
	newMod, err := core.LoadMod(newModPath)
	if err != nil {
		return err
	}

	err = CheckAndInstallDependencies([]*core.Mod{&newMod}, pack, index)
	if err != nil {
		return err
	}

	if pack.ModList {
		err = index.WriteModList()
		if err != nil {
			return err
		}
	}

	err = index.SyncDependencyMetadata(pack, core.SyncDepsOpts{NormalizeAll: true, RefreshAll: true})
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

	fmt.Printf("Project \"%s\" successfully added! (%s)\n", *project.Title, *file.Filename)
	return nil
}

func createFileMeta(project *modrinthApi.Project, version *modrinthApi.Version, file *modrinthApi.File, pack core.Pack, index *core.Index, dependency bool, dependencies []string, updateChannel string) error {
	updateMap := make(map[string]map[string]interface{})

	var err error
	updateMap["modrinth"], err = mrUpdateData{
		ProjectID:        *project.ID,
		InstalledVersion: *version.ID,
	}.ToMap()
	if err != nil {
		return err
	}

	side := getSide(project)
	if side == "" {
		fmt.Println("Warning: Project doesn't have a side that's supported; assuming universal. Server: " + *project.ServerSide + " Client: " + *project.ClientSide)
		side = core.UniversalSide
	}

	algorithm, hash := getBestHash(file)
	if algorithm == "" {
		return errors.New("file doesn't have a hash")
	}
	var folder string
	folder = viper.GetString("meta-folder")
	if folder == "" {
		folder, err = getProjectTypeFolder(*project.ProjectType, version.Loaders, pack.GetCompatibleLoaders())
		if err != nil {
			return err
		}
	}

	path, err := getModrinthMetaPath(project, version, pack)
	if err != nil {
		return err
	}

	modMeta := core.Mod{
		Name:          *project.Title,
		FileName:      *file.Filename,
		Version:       getModrinthVersionLabel(version),
		PageURL:       getProjectPageURL(project),
		Category:      folder,
		Side:          side,
		Dependencies:  dependencies,
		Option:        &core.ModOption{Dependency: dependency},
		UpdateChannel: updateChannel,
		Download: core.ModDownload{
			URL:        *file.URL,
			HashFormat: algorithm,
			Hash:       hash,
		},
		Update: updateMap,
	}
	path = modMeta.SetMetaPath(path)

	// If the file already exists, this will overwrite it!!!
	// TODO: Should this be improved?
	// Current strategy is to go ahead and do stuff without asking, with the assumption that you are using
	// VCS anyway.

	format, hash, err := modMeta.Write()
	if err != nil {
		return err
	}
	return index.RefreshFileWithHash(path, format, hash, true)
}

func getModrinthMetaPath(project *modrinthApi.Project, version *modrinthApi.Version, pack core.Pack) (string, error) {
	var folder string
	folder = viper.GetString("meta-folder")
	if folder == "" {
		var err error
		folder, err = getProjectTypeFolder(*project.ProjectType, version.Loaders, pack.GetCompatibleLoaders())
		if err != nil {
			return "", err
		}
	}

	if project.Slug != nil {
		return filepath.Join(viper.GetString("meta-folder-base"), folder, *project.Slug+core.MetaExtension), nil
	}
	return filepath.Join(viper.GetString("meta-folder-base"), folder, core.SlugifyName(*project.Title)+core.MetaExtension), nil
}

func getInstalledProjectPaths(index *core.Index) map[string]string {
	installedProjects := make(map[string]string)
	mods, err := index.LoadAllMods()
	if err != nil {
		fmt.Printf("Failed to determine existing projects: %v\n", err)
		return installedProjects
	}

	for _, mod := range mods {
		data, ok := mod.GetParsedUpdateData("modrinth")
		if !ok {
			continue
		}
		updateData, ok := data.(mrUpdateData)
		if !ok || len(updateData.ProjectID) == 0 {
			continue
		}
		installedProjects[updateData.ProjectID] = mod.GetFilePath()
	}

	return installedProjects
}

func getProjectPageURL(project *modrinthApi.Project) string {
	if project == nil || project.ProjectType == nil || project.Slug == nil {
		return ""
	}
	return fmt.Sprintf("https://modrinth.com/%s/%s", *project.ProjectType, *project.Slug)
}

func getModrinthVersionLabel(version *modrinthApi.Version) string {
	if version == nil {
		return ""
	}
	if version.VersionNumber != nil && *version.VersionNumber != "" {
		return *version.VersionNumber
	}
	if version.ID != nil {
		return *version.ID
	}
	return ""
}

func CheckAndInstallDependencies(mods []*core.Mod, pack core.Pack, index *core.Index) error {
	installedProjects := getInstalledProjectIDs(index)
	isQuilt := slices.Contains(pack.GetCompatibleLoaders(), "quilt")
	mcVersion, err := pack.GetMCVersion()
	if err != nil {
		return err
	}

	resolvedDependencies := make(map[string]*depMetadataStore)
	dependencyChildren := make(map[string]map[string]struct{})
	queuedProjects := map[string]struct{}{}
	queuedVersions := map[string]struct{}{}
	projectQueue := make([]modrinthProjectQueueItem, 0)
	versionQueue := make([]modrinthVersionQueueItem, 0)

	addEdge := func(parentID, childID string) {
		if parentID == "" || childID == "" {
			return
		}
		if _, ok := dependencyChildren[parentID]; !ok {
			dependencyChildren[parentID] = make(map[string]struct{})
		}
		dependencyChildren[parentID][childID] = struct{}{}
	}

	enqueueProject := func(projectID, parentID string) {
		if projectID == "" {
			return
		}
		if slices.Contains(installedProjects, projectID) {
			addEdge(parentID, projectID)
			return
		}
		if _, ok := resolvedDependencies[projectID]; ok {
			addEdge(parentID, projectID)
			return
		}
		if _, ok := queuedProjects[projectID]; ok {
			addEdge(parentID, projectID)
			return
		}
		queuedProjects[projectID] = struct{}{}
		projectQueue = append(projectQueue, modrinthProjectQueueItem{projectID: projectID, parentID: parentID})
		addEdge(parentID, projectID)
	}

	enqueueVersion := func(versionID, parentID string) {
		if versionID == "" {
			return
		}
		if _, ok := queuedVersions[versionID]; ok {
			return
		}
		queuedVersions[versionID] = struct{}{}
		versionQueue = append(versionQueue, modrinthVersionQueueItem{versionID: versionID, parentID: parentID})
	}

	resolveDirectDependencies := func(nodeID string, deps []*modrinthApi.Dependency) {
		for _, dep := range deps {
			if dep == nil {
				continue
			}
			if dep.DependencyType == nil || *dep.DependencyType != "required" {
				continue
			}
			if dep.VersionID != nil {
				enqueueVersion(*dep.VersionID, nodeID)
				continue
			}
			if dep.ProjectID != nil {
				enqueueProject(mapDepOverride(*dep.ProjectID, isQuilt, mcVersion), nodeID)
			}
		}
	}

	for _, m := range mods {
		rawData, ok := m.GetParsedUpdateData("modrinth")
		if !ok {
			continue
		}
		data := rawData.(mrUpdateData)
		if data.InstalledVersion == "" {
			continue
		}
		depVersion, err := mrDefaultClient.Versions.Get(data.InstalledVersion)
		if err != nil {
			fmt.Printf("Warning: failed to get version metadata for %s: %v\n", m.Name, err)
			continue
		}
		resolveDirectDependencies(data.ProjectID, depVersion.Dependencies)
	}

	cycles := 0
	for len(projectQueue)+len(versionQueue) > 0 && cycles < maxCycles {
		for len(versionQueue) > 0 {
			item := versionQueue[0]
			versionQueue = versionQueue[1:]

			depVersion, err := mrDefaultClient.Versions.Get(item.versionID)
			if err != nil {
				fmt.Printf("Error retrieving dependency data: %s\n", err.Error())
				continue
			}
			if depVersion.ProjectID == nil {
				continue
			}
			depProjectID := mapDepOverride(*depVersion.ProjectID, isQuilt, mcVersion)
			enqueueProject(depProjectID, item.parentID)
		}

		if len(projectQueue) == 0 {
			break
		}

		projectIDs := make([]string, 0, len(projectQueue))
		for _, item := range projectQueue {
			projectIDs = append(projectIDs, item.projectID)
		}
		projectQueue = projectQueue[:0]
		slices.Sort(projectIDs)
		projectIDs = slices.Compact(projectIDs)

		depProjects, err := mrDefaultClient.Projects.GetMultiple(projectIDs)
		if err != nil {
			fmt.Printf("Error retrieving dependency data: %s\n", err.Error())
		}

		for _, depProject := range depProjects {
			if depProject.ID == nil {
				return errors.New("failed to get dependency data: invalid response")
			}
			depProjectID := *depProject.ID
			
			// Check if this dependency already exists and has an update channel override
			var depMod *core.Mod
			if slices.Contains(installedProjects, depProjectID) {
				// Load the mod to check for update channel override
				installedPaths := getInstalledProjectPaths(index)
				if modPath, ok := installedPaths[depProjectID]; ok {
					loadedMod, err := core.LoadMod(modPath)
					if err == nil {
						depMod = &loadedMod
					}
				}
			}
			
			allowedChannel := pack.GetAllowedChannel(depMod)
			latestVersion, err := getLatestVersion(depProjectID, *depProject.Title, pack, allowedChannel)

			var file = latestVersion.Files[0]
			for _, v := range latestVersion.Files {
				if *v.Primary {
					file = v
				}
			}

			metaPath, err := getModrinthMetaPath(depProject, latestVersion, pack)
			if err != nil {
				return err
			}

			resolvedDependencies[depProjectID] = &depMetadataStore{
				projectInfo:   depProject,
				versionInfo:   latestVersion,
				fileInfo:      file,
				metaPath:      metaPath,
				updateChannel: allowedChannel,
			}

			resolveDirectDependencies(depProjectID, latestVersion.Dependencies)
		}

		cycles++
	}
	if cycles >= maxCycles {
		return errors.New("dependencies recurse too deeply, try increasing maxCycles")
	}

	if len(resolvedDependencies) > 0 {
		depsToInstall := make([]*depMetadataStore, 0, len(resolvedDependencies))
		for id, dep := range resolvedDependencies {
			if !slices.Contains(installedProjects, id) {
				depsToInstall = append(depsToInstall, dep)
			}
		}

		if len(depsToInstall) > 0 {
			fmt.Println("Dependencies found:")
			for _, dep := range depsToInstall {
				fmt.Println(*dep.projectInfo.Title)
			}

			if cmdshared.PromptYesNo("Would you like to add them? [Y/n]: ") {
				for _, dep := range depsToInstall {
					err := createFileMeta(dep.projectInfo, dep.versionInfo, dep.fileInfo, pack, index, true, nil, dep.updateChannel)
					if err != nil {
						return err
					}
					fmt.Printf("Dependency \"%s\" successfully added! (%s)\n", *dep.projectInfo.Title, *dep.fileInfo.Filename)
				}
			} else {
				fmt.Println("Dependency installation skipped.")
			}
		}
	}

	return nil
}

type mrDependencyInstaller struct{}

func (i mrDependencyInstaller) CheckAndInstallDependencies(mods []*core.Mod, pack core.Pack, index *core.Index) error {
	return CheckAndInstallDependencies(mods, pack, index)
}

var projectIDFlag string
var versionIDFlag string
var versionFilenameFlag string
var updateChannelFlag string

func init() {
	modrinthCmd.AddCommand(installCmd)

	installCmd.Flags().StringVar(&projectIDFlag, "project-id", "", "The Modrinth project ID to use")
	installCmd.Flags().StringVar(&versionIDFlag, "version-id", "", "The Modrinth version ID to use")
	installCmd.Flags().StringVar(&versionFilenameFlag, "version-filename", "", "The Modrinth version filename to use")
	installCmd.Flags().StringVar(&updateChannelFlag, "update-channel", "", "The update channel to use for this mod (release, beta, alpha)")

	core.DependencyInstallers["modrinth"] = mrDependencyInstaller{}
}
