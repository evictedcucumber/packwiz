package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// UpdateCmd represents the update command
var UpdateCmd = &cobra.Command{
	Use:     "update [name...]",
	Short:   "Update one or more external files (or all external files) in the modpack",
	Aliases: []string{"upgrade"},
	Args:    cobra.MinimumNArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: --check flag?

		fmt.Println("Loading modpack...")
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

		var singleUpdatedName string
		anyUpdated := false
		if viper.GetBool("update.all") {
			if len(args) > 0 {
				fmt.Println("Do not specify file names when using --all")
				os.Exit(1)
			}
			filesWithUpdater := make(map[string][]*core.Mod)
			fmt.Println("Reading metadata files...")
			mods, err := index.LoadAllMods()
			if err != nil {
				fmt.Printf("Failed to update all files: %v\n", err)
				os.Exit(1)
			}
			for _, modData := range mods {
				updaterFound := false
				for k := range modData.Update {
					slice, ok := filesWithUpdater[k]
					if !ok {
						_, ok = core.Updaters[k]
						if !ok {
							continue
						}
						slice = []*core.Mod{}
					}
					updaterFound = true
					filesWithUpdater[k] = append(slice, modData)
				}
				if !updaterFound {
					fmt.Printf("A supported update system for \"%s\" cannot be found.\n", modData.Name)
				}
			}

			fmt.Println("Checking for updates...")
			updatesFound := false
			updatableFiles := make(map[string][]*core.Mod)
			updaterCachedStateMap := make(map[string][]interface{})
			for k, v := range filesWithUpdater {
				checks, err := core.Updaters[k].CheckUpdate(v, pack)
				if err != nil {
					// TODO: do we return err code 1?
					fmt.Printf("Failed to check updates for %s: %s\n", k, err.Error())
					continue
				}
				for i, check := range checks {
					if check.Error != nil {
						// TODO: do we return err code 1?
						fmt.Printf("Failed to check updates for %s: %s\n", v[i].Name, check.Error.Error())
						continue
					}
					if check.UpdateAvailable {
						if v[i].Pin {
							fmt.Printf("Update skipped for pinned mod %s\n", v[i].Name)
							continue
						}

						if !updatesFound {
							fmt.Println("Updates found:")
							updatesFound = true
						}
						fmt.Printf("%s: %s\n", v[i].Name, check.UpdateString)
						updatableFiles[k] = append(updatableFiles[k], v[i])
						updaterCachedStateMap[k] = append(updaterCachedStateMap[k], check.CachedState)
					}
				}
			}

			if !updatesFound {
				fmt.Println("All files are up to date!")
				return
			}

			if !cmdshared.PromptYesNo("Do you want to update? [Y/n]: ") {
				fmt.Println("Cancelled!")
				return
			}

			for k, v := range updatableFiles {
				err := core.Updaters[k].DoUpdate(v, updaterCachedStateMap[k])
				if err != nil {
					// TODO: do we return err code 1?
					fmt.Println(err.Error())
					continue
				}
				for _, modData := range v {
					format, hash, err := modData.Write()
					if err != nil {
						fmt.Println(err.Error())
						continue
					}
					err = index.RefreshFileWithHash(modData.GetFilePath(), format, hash, true)
					if err != nil {
						fmt.Println(err.Error())
						continue
					}
				}
			}
		} else {
			if len(args) == 0 {
				fmt.Println("Must specify at least one file, or use the --all flag!")
				os.Exit(1)
			}

			modPaths, err := resolveModArgs(args, &index)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			updatedNames := make([]string, 0)
			hadFailure := false
			for _, modPath := range modPaths {
				updated, name, err := updateModAtPath(pack, &index, modPath)
				if err != nil {
					fmt.Println(err)
					hadFailure = true
					continue
				}
				if updated {
					updatedNames = append(updatedNames, name)
					anyUpdated = true
					if singleUpdatedName == "" {
						singleUpdatedName = name
					}
				}
			}

			if hadFailure {
				fmt.Println("Some files failed to update.")
			}
			if len(modPaths) > 1 {
				if len(updatedNames) > 0 {
					fmt.Printf("Updated %d file(s).\n", len(updatedNames))
				} else {
					fmt.Println("All selected files are up to date!")
				}
			}
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
		if viper.GetBool("update.all") {
			fmt.Println("Files updated!")
		} else if anyUpdated && singleUpdatedName != "" && len(args) == 1 {
			fmt.Printf("\"%s\" updated!\n", singleUpdatedName)
		}
	},
}

func init() {
	rootCmd.AddCommand(UpdateCmd)

	UpdateCmd.Flags().BoolP("all", "a", false, "Update all external files")
	_ = viper.BindPFlag("update.all", UpdateCmd.Flags().Lookup("all"))
}

func resolveModArgs(args []string, index *core.Index) ([]string, error) {
	modPaths := make([]string, 0, len(args))
	seen := make(map[string]bool)
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			return nil, fmt.Errorf("empty file name provided")
		}

		paths, err := resolveSingleModArg(arg, index)
		if err != nil {
			return nil, err
		}
		for _, modPath := range paths {
			if seen[modPath] {
				continue
			}
			seen[modPath] = true
			modPaths = append(modPaths, modPath)
		}
	}
	return modPaths, nil
}

func resolveSingleModArg(arg string, index *core.Index) ([]string, error) {
	hasSeparator := strings.ContainsAny(arg, "/\\")
	hasExtension := strings.HasSuffix(arg, core.MetaExtension) || strings.HasSuffix(arg, core.MetaExtensionOld)

	if filepath.IsAbs(arg) || hasSeparator {
		modPath, ok, err := resolveModPathCandidate(arg, index)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("can't find file %s; ensure it exists and is listed in the index", arg)
		}
		return []string{modPath}, nil
	}

	if hasExtension {
		modPath, ok, err := resolveModPathCandidate(arg, index)
		if err != nil {
			return nil, err
		}
		if ok {
			return []string{modPath}, nil
		}
		trimmed := trimMetaExtension(arg)
		return findModsByName(trimmed, index)
	}

	return findModsByName(arg, index)
}

func resolveModPathCandidate(arg string, index *core.Index) (string, bool, error) {
	argPath := arg
	if !filepath.IsAbs(argPath) {
		argPath = filepath.Join(index.PackRoot(), filepath.FromSlash(argPath))
	}
	if modPath, ok, err := validateModPath(argPath, index); err != nil || ok {
		return modPath, ok, err
	}

	base := viper.GetString("meta-folder-base")
	if base == "" {
		return "", false, nil
	}
	basePath := base
	if !filepath.IsAbs(basePath) {
		basePath = filepath.Join(index.PackRoot(), basePath)
	}
	return validateModPath(filepath.Join(basePath, filepath.FromSlash(arg)), index)
}

func validateModPath(candidate string, index *core.Index) (string, bool, error) {
	cleanPath := filepath.Clean(candidate)
	relPath, err := index.RelIndexPath(cleanPath)
	if err != nil {
		return "", false, err
	}
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == ".." || strings.HasPrefix(relPath, "../") {
		return "", false, nil
	}
	entry, ok := index.Files[relPath]
	if !ok || !entry.IsMetaFile() {
		return "", false, nil
	}
	return index.ResolveIndexPath(relPath), true, nil
}

func findModsByName(name string, index *core.Index) ([]string, error) {
	modPaths := make([]string, 0)
	for p, v := range index.Files {
		if !v.IsMetaFile() {
			continue
		}
		baseName := path.Base(p)
		trimmed := trimMetaExtension(baseName)
		if trimmed == name {
			modPaths = append(modPaths, index.ResolveIndexPath(p))
		}
	}
	if len(modPaths) == 0 {
		return nil, fmt.Errorf("can't find a metadata file named %s; ensure you have run packwiz refresh", name)
	}
	if len(modPaths) > 1 {
		return nil, fmt.Errorf("multiple files named %s found; specify the full path (for example modrinth/%s%s)", name, name, core.MetaExtension)
	}
	return modPaths, nil
}

func trimMetaExtension(fileName string) string {
	trimmed := strings.TrimSuffix(fileName, core.MetaExtension)
	if trimmed != fileName {
		return trimmed
	}
	return strings.TrimSuffix(fileName, core.MetaExtensionOld)
}

func updateModAtPath(pack core.Pack, index *core.Index, modPath string) (bool, string, error) {
	modData, err := core.LoadMod(modPath)
	if err != nil {
		return false, "", err
	}
	if modData.Pin {
		fmt.Printf("Update skipped for pinned mod %s\n", modData.Name)
		return false, modData.Name, nil
	}

	updaterFound := false
	for k := range modData.Update {
		updater, ok := core.Updaters[k]
		if !ok {
			continue
		}
		updaterFound = true

		check, err := updater.CheckUpdate([]*core.Mod{&modData}, pack)
		if err != nil {
			return false, modData.Name, err
		}
		if len(check) != 1 {
			return false, modData.Name, fmt.Errorf("invalid update check response for %s", modData.Name)
		}

		if check[0].UpdateAvailable {
			fmt.Printf("Update available for %s: %s\n", modData.Name, check[0].UpdateString)

			err = updater.DoUpdate([]*core.Mod{&modData}, []interface{}{check[0].CachedState})
			if err != nil {
				return false, modData.Name, err
			}

			format, hash, err := modData.Write()
			if err != nil {
				return false, modData.Name, err
			}
			err = index.RefreshFileWithHash(modPath, format, hash, true)
			if err != nil {
				return false, modData.Name, err
			}
			return true, modData.Name, nil
		}

		fmt.Printf("\"%s\" is already up to date!\n", modData.Name)
		return false, modData.Name, nil
	}

	if !updaterFound {
		return false, modData.Name, fmt.Errorf("a supported update system for \"%s\" cannot be found", modData.Name)
	}
	return false, modData.Name, nil
}
