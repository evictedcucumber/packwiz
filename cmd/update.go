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
	Long:    "Update an external file, or all of them with --all.\n\n" + modRefHelp,
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
			ui.Muted.Println("Reading metadata files...")
			mods, err := index.LoadAllMods()
			if err != nil {
				ui.Error.Printf("Failed to update all files: %v\n", err)
				os.Exit(1)
			}

			ui.Muted.Println("Checking for updates...")
			search := FindUpdates(pack, mods, nil)
			for _, mod := range search.Unsupported {
				ui.Warning.Printf("A supported update system for \"%s\" cannot be found.\n", mod.Name)
			}
			for _, failure := range search.Failures {
				// TODO: do we return err code 1?
				ui.Error.Printf("Failed to check updates for %s: %s\n", failure.Name, failure.Err.Error())
			}
			for _, mod := range search.Pinned {
				ui.Muted.Printf("Update skipped for pinned mod %s\n", mod.Name)
			}

			if len(search.Offers) == 0 {
				if failedChecks := search.FailedChecks(); failedChecks > 0 {
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

			ui.Bold.Println("Updates found:")
			for _, offer := range search.Offers {
				fmt.Printf("%s: %s\n", ui.Bold.Sprint(offer.Mod.Name), StyleUpdate(offer.Change))
			}

			if !cmdshared.PromptYesNo("Do you want to update? [Y/n]: ") {
				ui.Warning.Println("Cancelled!")
				return
			}

			_, errs := ApplyUpdates(&index, search.Offers)
			for _, err := range errs {
				// TODO: do we return err code 1?
				ui.Error.Println(err.Error())
			}
		} else {
			if len(args) < 1 || len(args[0]) == 0 {
				ui.Error.Println("Must specify a valid file, or use the --all flag!")
				os.Exit(1)
			}
			modPath := findMod(index, args[0])
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
					ui.Info.Printf("Update available: %s\n", StyleUpdate(check[0].UpdateString))

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

// StyleUpdate shows what an updater says an update is, which is usually "old -> new", with the old and the new set apart
func StyleUpdate(update string) string {
	from, to, ok := strings.Cut(update, " -> ")
	if !ok {
		return update
	}
	return ui.Transition(from, to)
}

func init() {
	UpdateCmd.Flags().BoolP("all", "a", false, "Update all external files")
	_ = viper.BindPFlag("update.all", UpdateCmd.Flags().Lookup("all"))
}
