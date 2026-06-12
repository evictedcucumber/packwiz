package curseforge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/sahilm/fuzzy"
	"github.com/spf13/viper"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"gopkg.in/dixonwille/wmenu.v4"
)

const maxCycles = 20

type installableDep struct {
	modInfo
	fileInfo modFileInfo
	metaPath string
}

type cfProjectQueueItem struct {
	modID    uint32
	parentID uint32
}

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:     "add [URL]",
	Short:   "Add a project from a CurseForge URL or ID",
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
		mcVersions, err := pack.GetSupportedMCVersions()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		primaryMCVersion, err := pack.GetMCVersion()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		game := gameFlag
		category := categoryFlag
		var modID, fileID uint32
		var slug string

		// If mod/file IDs are provided in command line, use those
		if fileIDFlag != 0 {
			fileID = fileIDFlag
		}
		if addonIDFlag != 0 {
			modID = addonIDFlag
		}

		if (len(args) == 0 || len(args[0]) == 0) && modID == 0 {
			fmt.Println("You must specify a project; with the ID flags, or by passing a CurseForge URL directly.")
			os.Exit(1)
		}
		if modID == 0 && len(args) == 1 {
			if !strings.HasPrefix(args[0], "https://") && !strings.HasPrefix(args[0], "http://") {
				fmt.Println("Only CurseForge links are accepted for add; search terms and slugs are not supported.")
				os.Exit(1)
			}

			parsedGame, parsedCategory, parsedSlug, parsedFileID, err := parseSlugOrUrl(args[0])
			if err != nil {
				fmt.Printf("Failed to parse URL: %v\n", err)
				os.Exit(1)
			}

			if parsedGame != "" {
				game = parsedGame
			}
			if parsedCategory != "" {
				category = parsedCategory
			}
			if parsedSlug != "" {
				slug = parsedSlug
			} else {
				fmt.Println("Only valid CurseForge links are accepted for add.")
				os.Exit(1)
			}
			if parsedFileID != 0 {
				fileID = parsedFileID
			}
		} else if modID == 0 {
			fmt.Println("Only a single CurseForge link is accepted for add.")
			os.Exit(1)
		}

		modInfoObtained := false
		var modInfoData modInfo

		if modID == 0 {
			var cancelled bool
			cancelled, modInfoData = searchCurseforgeInternal(slug, true, game, category, mcVersions, getSearchLoaderType(pack))
			if cancelled {
				return
			}
			modID = modInfoData.ID
			modInfoObtained = true
		}

		if modID == 0 {
			fmt.Println("No projects found!")
			os.Exit(1)
		}

		if !modInfoObtained {
			modInfoData, err = cfDefaultClient.getModInfo(modID)
			if err != nil {
				fmt.Printf("Failed to get project info: %v\n", err)
				os.Exit(1)
			}
		}

		var fileInfoData modFileInfo
		fileInfoData, err = getLatestFile(modInfoData, mcVersions, fileID, pack.GetCompatibleLoaders())
		if err != nil {
			fmt.Printf("Failed to get file for project: %v\n", err)
			os.Exit(1)
		}

		installedProjectPaths := getInstalledCurseForgeProjectPaths(&index)
		resolvedDependencies := make(map[uint32]*installableDep)
		dependencyChildren := make(map[uint32]map[uint32]struct{})

		if len(fileInfoData.Dependencies) > 0 {
			isQuilt := slices.Contains(pack.GetCompatibleLoaders(), "quilt")
			queuedProjects := make(map[uint32]struct{})
			depIDPendingQueue := make([]cfProjectQueueItem, 0)

			addEdge := func(parentID, childID uint32) {
				if parentID == 0 || childID == 0 {
					return
				}
				if _, ok := dependencyChildren[parentID]; !ok {
					dependencyChildren[parentID] = make(map[uint32]struct{})
				}
				dependencyChildren[parentID][childID] = struct{}{}
			}

			enqueueProject := func(modID uint32, parentID uint32) {
				if modID == 0 {
					return
				}
				if _, ok := resolvedDependencies[modID]; ok {
					addEdge(parentID, modID)
					return
				}
				if _, ok := queuedProjects[modID]; ok {
					addEdge(parentID, modID)
					return
				}
				queuedProjects[modID] = struct{}{}
				depIDPendingQueue = append(depIDPendingQueue, cfProjectQueueItem{modID: modID, parentID: parentID})
				addEdge(parentID, modID)
			}

			for _, dep := range fileInfoData.Dependencies {
				if dep.Type == dependencyTypeRequired {
					enqueueProject(mapDepOverride(dep.ModID, isQuilt, primaryMCVersion), modInfoData.ID)
				}
			}

			if len(depIDPendingQueue) > 0 {
				fmt.Println("Finding dependencies...")

				cycles := 0
				for len(depIDPendingQueue) > 0 && cycles < maxCycles {
					projectIDs := make([]uint32, 0, len(depIDPendingQueue))
					for _, item := range depIDPendingQueue {
						projectIDs = append(projectIDs, item.modID)
					}
					depIDPendingQueue = depIDPendingQueue[:0]
					slices.Sort(projectIDs)
					projectIDs = slices.Compact(projectIDs)

					filteredIDs := make([]uint32, 0, len(projectIDs))
					for _, id := range projectIDs {
						if _, ok := installedProjectPaths[id]; ok {
							continue
						}
						if _, ok := resolvedDependencies[id]; ok {
							continue
						}
						filteredIDs = append(filteredIDs, id)
					}

					if len(filteredIDs) == 0 {
						break
					}

					depInfoData, err := cfDefaultClient.getModInfoMultiple(filteredIDs)
					if err != nil {
						fmt.Printf("Error retrieving dependency data: %s\n", err.Error())
					}

					for _, currData := range depInfoData {
						depFileInfo, err := getLatestFile(currData, mcVersions, 0, pack.GetCompatibleLoaders())
						if err != nil {
							fmt.Printf("Error retrieving dependency data: %s\n", err.Error())
							continue
						}

						metaPath := getCurseForgeMetaPath(currData)
						resolvedDependencies[currData.ID] = &installableDep{
							modInfo:  currData,
							fileInfo: depFileInfo,
							metaPath: metaPath,
						}

						for _, dep := range depFileInfo.Dependencies {
							if dep.Type == dependencyTypeRequired {
								enqueueProject(mapDepOverride(dep.ModID, isQuilt, primaryMCVersion), currData.ID)
							}
						}
					}

					cycles++
				}
				if cycles >= maxCycles {
					fmt.Println("Dependencies recurse too deeply! Try increasing maxCycles.")
					os.Exit(1)
				}

				if len(resolvedDependencies) > 0 {
					fmt.Println("Dependencies found:")
					for _, v := range resolvedDependencies {
						fmt.Println(v.Name)
					}

					if cmdshared.PromptYesNo("Would you like to add them? [Y/n]: ") {
						for _, v := range resolvedDependencies {
							depRefs := getCurseForgeDependencyRefs(v.ID, dependencyChildren, resolvedDependencies, installedProjectPaths, &index)
							err = createModFile(v.modInfo, v.fileInfo, &index, true, depRefs)
							if err != nil {
								fmt.Println(err)
								os.Exit(1)
							}
							fmt.Printf("Dependency \"%s\" successfully added! (%s)\n", v.modInfo.Name, v.fileInfo.FileName)
						}
					}
				} else {
					fmt.Println("All dependencies are already added!")
				}
			}
		}

		rootDeps := getCurseForgeDependencyRefs(modInfoData.ID, dependencyChildren, resolvedDependencies, installedProjectPaths, &index)
		err = createModFile(modInfoData, fileInfoData, &index, false, rootDeps)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if pack.ModList {
			err = index.WriteModList()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}

		err = index.SyncDependencyMetadata(pack, core.SyncDepsOpts{NormalizeAll: true})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		err = index.Write()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		err = pack.UpdateIndexHash()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		err = pack.Write()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Printf("Project \"%s\" successfully added! (%s)\n", modInfoData.Name, fileInfoData.FileName)
	},
}

func getInstalledCurseForgeProjectPaths(index *core.Index) map[uint32]string {
	installedProjects := make(map[uint32]string)
	mods, err := index.LoadAllMods()
	if err != nil {
		fmt.Printf("Failed to determine existing projects: %v\n", err)
		return installedProjects
	}

	for _, mod := range mods {
		data, ok := mod.GetParsedUpdateData("curseforge")
		if !ok {
			continue
		}
		updateData, ok := data.(cfUpdateData)
		if !ok || updateData.ProjectID == 0 {
			continue
		}
		installedProjects[updateData.ProjectID] = mod.GetFilePath()
	}

	return installedProjects
}

func getCurseForgeDependencyRefs(projectID uint32, dependencyChildren map[uint32]map[uint32]struct{}, resolvedDependencies map[uint32]*installableDep, installedProjectPaths map[uint32]string, index *core.Index) []string {
	childIDs := dependencyChildren[projectID]
	if len(childIDs) == 0 {
		return nil
	}

	dependencies := make([]string, 0, len(childIDs))
	for childID := range childIDs {
		var path string
		if dep, ok := resolvedDependencies[childID]; ok {
			path = dep.metaPath
		} else if installedPath, ok := installedProjectPaths[childID]; ok {
			path = installedPath
		} else {
			continue
		}
		relPath, err := index.ToIndexRelativePath(path)
		if err != nil {
			relPath = filepath.ToSlash(filepath.Clean(path))
		}
		dependencies = append(dependencies, relPath)
	}
	slices.Sort(dependencies)
	dependencies = slices.Compact(dependencies)
	return dependencies
}

// Used to implement interface for fuzzy matching
type modResultsList []modInfo

func (r modResultsList) String(i int) string {
	return r[i].Name
}

func (r modResultsList) Len() int {
	return len(r)
}

func searchCurseforgeInternal(searchTerm string, isSlug bool, game string, category string, mcVersions []string, searchLoaderType modloaderType) (bool, modInfo) {
	if isSlug {
		fmt.Println("Looking up CurseForge slug...")
	} else {
		fmt.Println("Searching CurseForge...")
	}

	var gameID, categoryID, classID uint32
	if game == "minecraft" {
		gameID = 432
	}
	if category == "mc-mods" {
		classID = 6
	}
	if gameID == 0 {
		games, err := cfDefaultClient.getGames()
		if err != nil {
			fmt.Printf("Failed to lookup game %s: %v\n", game, err)
			os.Exit(1)
		}
		for _, v := range games {
			if v.Slug == game {
				if v.Status != gameStatusLive {
					fmt.Printf("Failed to lookup game %s: selected game is not live!\n", game)
					os.Exit(1)
				}
				if v.APIStatus != gameApiStatusPublic {
					fmt.Printf("Failed to lookup game %s: selected game does not have a public API!\n", game)
					os.Exit(1)
				}
				gameID = v.ID
				break
			}
		}
		if gameID == 0 {
			fmt.Printf("Failed to lookup: game %s could not be found!\n", game)
			os.Exit(1)
		}
	}
	if categoryID == 0 && classID == 0 && category != "" {
		categories, err := cfDefaultClient.getCategories(gameID)
		if err != nil {
			fmt.Printf("Failed to lookup categories: %v\n", err)
			os.Exit(1)
		}
		for _, v := range categories {
			if v.Slug == category {
				if v.IsClass {
					classID = v.ID
				} else {
					classID = v.ClassID
					categoryID = v.ID
				}
				break
			}
		}
		if categoryID == 0 && classID == 0 {
			fmt.Printf("Failed to lookup: category %s could not be found!\n", category)
			os.Exit(1)
		}
	}

	// If there are more than one acceptable version, we shouldn't filter by game version at all (as we can't filter by multiple)
	filterGameVersion := ""
	if len(mcVersions) == 1 {
		filterGameVersion = getCurseforgeVersion(mcVersions[0])
	}
	var search, slug string
	if isSlug {
		slug = searchTerm
	} else {
		search = searchTerm
	}
	results, err := cfDefaultClient.getSearch(search, slug, gameID, classID, categoryID, filterGameVersion, searchLoaderType)
	if err != nil {
		fmt.Printf("Failed to search for project: %v\n", err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Println("No projects found!")
		os.Exit(1)
		return false, modInfo{}
	} else if len(results) == 1 {
		return false, results[0]
	} else {
		// Fuzzy search on results list
		fuzzySearchResults := fuzzy.FindFrom(searchTerm, modResultsList(results))

		if viper.GetBool("non-interactive") {
			if len(fuzzySearchResults) > 0 {
				return false, results[fuzzySearchResults[0].Index]
			}
			return false, results[0]
		}

		menu := wmenu.NewMenu("Choose a number:")

		menu.Option("Cancel", nil, false, nil)
		if len(fuzzySearchResults) == 0 {
			for i, v := range results {
				menu.Option(v.Name+" ("+v.Summary+")", v, i == 0, nil)
			}
		} else {
			for i, v := range fuzzySearchResults {
				menu.Option(results[v.Index].Name+" ("+results[v.Index].Summary+")", results[v.Index], i == 0, nil)
			}
		}

		var modInfoData modInfo
		var cancelled bool
		menu.Action(func(menuRes []wmenu.Opt) error {
			if len(menuRes) != 1 || menuRes[0].Value == nil {
				fmt.Println("Cancelled!")
				cancelled = true
				return nil
			}

			// Why is variable shadowing a thing!!!!
			var ok bool
			modInfoData, ok = menuRes[0].Value.(modInfo)
			if !ok {
				return errors.New("error converting interface from wmenu")
			}
			return nil
		})
		err = menu.Run()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if cancelled {
			return true, modInfo{}
		}
		return false, modInfoData
	}
}

func getLatestFile(modInfoData modInfo, mcVersions []string, fileID uint32, packLoaders []string) (modFileInfo, error) {
	if fileID == 0 {
		if len(modInfoData.LatestFiles) == 0 && len(modInfoData.GameVersionLatestFiles) == 0 {
			return modFileInfo{}, fmt.Errorf("addon %d has no files", modInfoData.ID)
		}

		var fileInfoData *modFileInfo
		fileID, fileInfoData, _ = findLatestFile(modInfoData, mcVersions, packLoaders)
		if fileInfoData != nil {
			return *fileInfoData, nil
		}

		// Possible to reach this point without obtaining file info; particularly from GameVersionLatestFiles
		if fileID == 0 {
			return modFileInfo{}, errors.New("mod not available for the configured Minecraft version(s) (use the 'packwiz settings acceptable-versions' command to accept more) or loader")
		}
	}

	fileInfoData, err := cfDefaultClient.getFileInfo(modInfoData.ID, fileID)
	if err != nil {
		return modFileInfo{}, err
	}
	return fileInfoData, nil
}

var addonIDFlag uint32
var fileIDFlag uint32

var gameFlag string
var categoryFlag string

func init() {
	curseforgeCmd.AddCommand(installCmd)

	installCmd.Flags().Uint32Var(&addonIDFlag, "addon-id", 0, "The CurseForge project ID to use")
	installCmd.Flags().Uint32Var(&fileIDFlag, "file-id", 0, "The CurseForge file ID to use")
	installCmd.Flags().StringVar(&gameFlag, "game", "minecraft", "The game to add files from (slug, as stored in URLs); the game in the URL takes precedence")
	installCmd.Flags().StringVar(&categoryFlag, "category", "", "The category to add files from (slug, as stored in URLs); the category in the URL takes precedence")
}
