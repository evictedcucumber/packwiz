package settings

import (
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

var releaseTypeCommand = &cobra.Command{
	Use:     "release-type [release|beta|alpha]",
	Short:   "Manage your pack's default release type. This is the minimum stability of Modrinth versions that will be installed/updated to, unless overridden per-mod",
	Aliases: []string{"rt"},
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		modpack, err := core.LoadPack()
		if err != nil {
			if os.IsNotExist(err) {
				ui.Error.Println("No pack.toml file found, run 'packwiz init' to create one!")
				os.Exit(1)
			}
			ui.Error.Printf("Error loading pack: %s\n", err)
			os.Exit(1)
		}

		if len(args) == 0 {
			ui.Info.Println("Current release type: " + ui.Bold.Sprint(modpack.GetReleaseType()))
			return
		}

		releaseType := args[0]
		if !core.IsValidReleaseType(releaseType) {
			ui.Error.Printf("Invalid release type %q; must be one of: release, beta, alpha\n", releaseType)
			os.Exit(1)
		}

		if modpack.Options == nil {
			modpack.Options = make(map[string]interface{})
		}
		modpack.Options["release-type"] = releaseType
		err = modpack.Write()
		if err != nil {
			ui.Error.Printf("Error writing pack: %s\n", err)
			os.Exit(1)
		}
		ui.Success.Println("Set default release type to " + ui.Bold.Sprint(releaseType))
	},
}

func init() {
	settingsCmd.AddCommand(releaseTypeCommand)
}
