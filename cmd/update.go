package cmd

import (
	"fmt"
	"os"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// UpdateCmd represents the update command
var UpdateCmd = &cobra.Command{
	Use:     "update [name]",
	Short:   "Update an external file (or all external files) in the modpack",
	Aliases: []string{"upgrade"},
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: --check flag?
		// TODO: specify multiple files to update at once?

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

		requeryMode := viper.GetBool("update.requery")
		if requeryMode {
			fmt.Println("Requery mode: checking all mods against release channel settings...")
		}

		var singleUpdatedName string
		updatedMods := make([]*core.Mod, 0)
		if viper.GetBool("update.all") {
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
					if check.UpdateAvailable || requeryMode {
						if v[i].Pin {
							fmt.Printf("Update skipped for pinned mod %s\n", v[i].Name)
							continue
						}

						if !updatesFound {
							if requeryMode {
								fmt.Println("Mods to requery:")
							} else {
								fmt.Println("Updates found:")
							}
							updatesFound = true
						}
						fmt.Printf("%s: %s\n", v[i].Name, check.UpdateString)
						updatableFiles[k] = append(updatableFiles[k], v[i])
						updaterCachedStateMap[k] = append(updaterCachedStateMap[k], check.CachedState)
					}
				}
			}

			if !updatesFound {
				if requeryMode {
					fmt.Println("All mods already match the release channel settings!")
				} else {
					fmt.Println("All files are up to date!")
				}
				return
			}

			promptMsg := "Do you want to update? [Y/n]: "
			if requeryMode {
				promptMsg = "Do you want to requery these mods? [Y/n]: "
			}
			if !cmdshared.PromptYesNo(promptMsg) {
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
					err = index.RefreshFileWithHash(modData.GetMetaPath(), format, hash, true)
					if err != nil {
						fmt.Println(err.Error())
						continue
					}
					updatedMods = append(updatedMods, modData)
				}
			}
		} else {
			if len(args) < 1 || len(args[0]) == 0 {
				fmt.Println("Must specify a valid file, or use the --all flag!")
				os.Exit(1)
			}
			modPath, err := resolveModTargetPath(index, args[0])
			if err != nil {
				fmt.Printf("Can't find this file: %s\n", err)
				fmt.Println("Use the project slug or a path to a tracked .pw.toml file (you may need to run packwiz refresh).")
				os.Exit(1)
			}
			modData, err := core.LoadMod(index.ResolveIndexPath(modPath))
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if modData.Pin {
				fmt.Println("Version is pinned; run the unpin command to allow updating")
				os.Exit(1)
			}
			singleUpdatedName = modData.Name
			updaterFound := false
			for k := range modData.Update {
				updater, ok := core.Updaters[k]
				if !ok {
					continue
				}
				updaterFound = true

				check, err := updater.CheckUpdate([]*core.Mod{&modData}, pack)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if len(check) != 1 {
					fmt.Println("Invalid update check response")
					os.Exit(1)
				}

				if check[0].UpdateAvailable || requeryMode {
					if requeryMode {
						fmt.Printf("Requery: %s\n", check[0].UpdateString)
					} else {
						fmt.Printf("Update available: %s\n", check[0].UpdateString)
					}

					if !cmdshared.PromptYesNo("Do you want to update? [Y/n]: ") {
						fmt.Println("Cancelled!")
						return
					}

					err = updater.DoUpdate([]*core.Mod{&modData}, []interface{}{check[0].CachedState})
					if err != nil {
						fmt.Println(err)
						os.Exit(1)
					}

					format, hash, err := modData.Write()
					if err != nil {
						fmt.Println(err)
						os.Exit(1)
					}
					err = index.RefreshFileWithHash(modPath, format, hash, true)
					if err != nil {
						fmt.Println(err)
						os.Exit(1)
					}
					updatedMods = append(updatedMods, &modData)
				} else {
					fmt.Printf("\"%s\" is already up to date!\n", modData.Name)
					return
				}

				break
			}
			if !updaterFound {
				// TODO: use file name instead of Name when len(Name) == 0 in all places?
				fmt.Println("A supported update system for \"" + modData.Name + "\" cannot be found.")
				os.Exit(1)
			}
		}

		// Check and install missing dependencies for the updated mods
		modsByUpdater := make(map[string][]*core.Mod)
		for _, m := range updatedMods {
			for updaterName := range m.Update {
				modsByUpdater[updaterName] = append(modsByUpdater[updaterName], m)
			}
		}
		for updaterName, modsList := range modsByUpdater {
			if installer, ok := core.DependencyInstallers[updaterName]; ok {
				err = installer.CheckAndInstallDependencies(modsList, pack, &index)
				if err != nil {
					fmt.Printf("Failed to check dependencies for %s: %v\n", updaterName, err)
				}
			}
		}

		err = finalizePackWithDependencies(&pack, &index, core.SyncDepsOpts{NormalizeAll: true, RefreshAll: true})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if viper.GetBool("update.all") {
			fmt.Println("Files updated!")
		} else {
			fmt.Printf("\"%s\" updated!\n", singleUpdatedName)
		}
	},
}

func init() {
	rootCmd.AddCommand(UpdateCmd)

	UpdateCmd.Flags().BoolP("all", "a", false, "Update all external files")
	_ = viper.BindPFlag("update.all", UpdateCmd.Flags().Lookup("all"))

	UpdateCmd.Flags().Bool("requery", false, "Requery all mods against release channel settings (ignores current version)")
	_ = viper.BindPFlag("update.requery", UpdateCmd.Flags().Lookup("requery"))
}
