package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
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

		ui.Muted.Println("Loading modpack...")
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

		var singleUpdatedName string
		if viper.GetBool("update.all") {
			filesWithUpdater := make(map[string][]*core.Mod)
			ui.Muted.Println("Reading metadata files...")
			mods, err := index.LoadAllMods()
			if err != nil {
				ui.Error.Printf("Failed to update all files: %v\n", err)
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
					ui.Warning.Printf("A supported update system for \"%s\" cannot be found.\n", modData.Name)
				}
			}

			ui.Muted.Println("Checking for updates...")
			updatesFound := false
			// How many files there is no answer for, which isn't the same as being up to date
			failedChecks := 0
			updatableFiles := make(map[string][]*core.Mod)
			updaterCachedStateMap := make(map[string][]interface{})
			for k, v := range filesWithUpdater {
				checks, err := core.Updaters[k].CheckUpdate(v, pack)
				if err != nil {
					// TODO: do we return err code 1?
					ui.Error.Printf("Failed to check updates for %s: %s\n", k, err.Error())
					failedChecks += len(v)
					continue
				}
				for i, check := range checks {
					if check.Error != nil {
						// TODO: do we return err code 1?
						ui.Error.Printf("Failed to check updates for %s: %s\n", v[i].Name, check.Error.Error())
						failedChecks++
						continue
					}
					if check.UpdateAvailable {
						if v[i].Pin {
							ui.Muted.Printf("Update skipped for pinned mod %s\n", v[i].Name)
							continue
						}

						if !updatesFound {
							ui.Bold.Println("Updates found:")
							updatesFound = true
						}
						fmt.Printf("%s: %s\n", ui.Bold.Sprint(v[i].Name), styleUpdate(check.UpdateString))
						updatableFiles[k] = append(updatableFiles[k], v[i])
						updaterCachedStateMap[k] = append(updaterCachedStateMap[k], check.CachedState)
					}
				}
			}

			if !updatesFound {
				if failedChecks > 0 {
					// Not knowing whether a file has an update isn't being up to date
					noun := "files"
					if failedChecks == 1 {
						noun = "file"
					}
					ui.Warning.Printf("No updates found, but %d %s could not be checked.\n", failedChecks, noun)
					return
				}
				ui.Success.Println("All files are up to date!")
				return
			}

			if !cmdshared.PromptYesNo("Do you want to update? [Y/n]: ") {
				ui.Warning.Println("Cancelled!")
				return
			}

			for k, v := range updatableFiles {
				err := core.Updaters[k].DoUpdate(v, updaterCachedStateMap[k])
				if err != nil {
					// TODO: do we return err code 1?
					ui.Error.Println(err.Error())
					continue
				}
				for _, modData := range v {
					format, hash, err := modData.Write()
					if err != nil {
						ui.Error.Println(err.Error())
						continue
					}
					err = index.RefreshFileWithHash(modData.GetFilePath(), format, hash, true)
					if err != nil {
						ui.Error.Println(err.Error())
						continue
					}
				}
			}
		} else {
			if len(args) < 1 || len(args[0]) == 0 {
				ui.Error.Println("Must specify a valid file, or use the --all flag!")
				os.Exit(1)
			}
			modPath, ok := index.FindMod(args[0])
			if !ok {
				ui.Error.Println("Can't find this file; please ensure you have run packwiz refresh and use the name of the .pw.toml file (defaults to the project slug)")
				os.Exit(1)
			}
			modData, err := core.LoadMod(modPath)
			if err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}
			if modData.Pin {
				ui.Error.Println("Version is pinned; run the unpin command to allow updating")
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
					ui.Error.Println(err)
					os.Exit(1)
				}
				if len(check) != 1 {
					ui.Error.Println("Invalid update check response")
					os.Exit(1)
				}

				if check[0].Error != nil {
					ui.Error.Printf("Failed to check updates for %s: %s\n", modData.Name, check[0].Error.Error())
					os.Exit(1)
				}

				if check[0].UpdateAvailable {
					ui.Info.Printf("Update available: %s\n", styleUpdate(check[0].UpdateString))

					err = updater.DoUpdate([]*core.Mod{&modData}, []interface{}{check[0].CachedState})
					if err != nil {
						ui.Error.Println(err)
						os.Exit(1)
					}

					format, hash, err := modData.Write()
					if err != nil {
						ui.Error.Println(err)
						os.Exit(1)
					}
					err = index.RefreshFileWithHash(modPath, format, hash, true)
					if err != nil {
						ui.Error.Println(err)
						os.Exit(1)
					}
				} else {
					ui.Success.Printf("\"%s\" is already up to date!\n", ui.Bold.Sprint(modData.Name))
					return
				}

				break
			}
			if !updaterFound {
				// TODO: use file name instead of Name when len(Name) == 0 in all places?
				ui.Error.Println("A supported update system for \"" + modData.Name + "\" cannot be found.")
				os.Exit(1)
			}
		}

		err = index.Write()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		err = pack.UpdateIndexHash()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		err = pack.Write()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		if viper.GetBool("update.all") {
			ui.Success.Println("Files updated!")
		} else {
			ui.Success.Printf("\"%s\" updated!\n", ui.Bold.Sprint(singleUpdatedName))
		}
	},
}

// styleUpdate shows what an updater says an update is, which is usually "old -> new", with the old and the new set apart
func styleUpdate(update string) string {
	from, to, ok := strings.Cut(update, " -> ")
	if !ok {
		return update
	}
	return ui.Transition(from, to)
}

func init() {
	rootCmd.AddCommand(UpdateCmd)

	UpdateCmd.Flags().BoolP("all", "a", false, "Update all external files")
	_ = viper.BindPFlag("update.all", UpdateCmd.Flags().Lookup("all"))
}
