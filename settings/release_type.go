package settings

import (
	"fmt"
	"os"

	"github.com/evictedcucumber/packwiz/core"
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
				fmt.Println("No pack.toml file found, run 'packwiz init' to create one!")
				os.Exit(1)
			}
			fmt.Printf("Error loading pack: %s\n", err)
			os.Exit(1)
		}

		if len(args) == 0 {
			fmt.Println("Current release type: " + modpack.GetReleaseType())
			return
		}

		releaseType := args[0]
		if !core.IsValidReleaseType(releaseType) {
			fmt.Printf("Invalid release type %q; must be one of: release, beta, alpha\n", releaseType)
			os.Exit(1)
		}

		if modpack.Options == nil {
			modpack.Options = make(map[string]interface{})
		}
		modpack.Options["release-type"] = releaseType
		err = modpack.Write()
		if err != nil {
			fmt.Printf("Error writing pack: %s\n", err)
			os.Exit(1)
		}
		fmt.Println("Set default release type to " + releaseType)
	},
}

func init() {
	settingsCmd.AddCommand(releaseTypeCommand)
}
