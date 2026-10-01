package modrinth

import (
	"fmt"
	"net/url"
	"os"
	"slices"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/spf13/viper"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

// exportCmd represents the export command
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the current modpack into a .mrpack for Modrinth",
	Long: `Export the pack as a .mrpack: the mods are listed in its manifest for the launcher to download, and the files the
index tracks that aren't mods (config files and the like) go in its overrides. So does the pack's README.md, LICENSE and
CHANGELOG.md, whichever of them are in the pack's directory: packwiz doesn't track them in the index, so they are not
installed by anything else, and need not be listed anywhere for the export to include them.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
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
		// Do a refresh to ensure files are up to date
		err = index.Refresh()
		if err != nil {
			ui.Error.Println(err)
			return
		}
		err = index.Write()
		if err != nil {
			ui.Error.Println(err)
			return
		}
		err = pack.UpdateIndexHash()
		if err != nil {
			ui.Error.Println(err)
			return
		}
		err = pack.Write()
		if err != nil {
			ui.Error.Println(err)
			return
		}

		ui.Muted.Println("Reading external files...")
		mods, err := index.LoadAllMods()
		if err != nil {
			ui.Error.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}

		ui.Muted.Printf("Retrieving %v external files...\n", len(mods))
		result, err := exportPack(pack, &index, mods, ExportOptions{
			Output:          viper.GetString("modrinth.export.output"),
			RestrictDomains: viper.GetBool("modrinth.export.restrictDomains"),
		}, exportHooks{disclaimer: cmdshared.PrintDisclaimer, manual: cmdshared.ListManualDownloads})
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Print(result.Breakdown())
		for _, p := range result.Promotions {
			ui.Warning.Printf("Warning: %s is only exported for the server, but %s needs it on the client; 'packwiz validate' says more\n", ui.Bold.Sprint(p.Mod), ui.Bold.Sprint(p.NeededBy))
		}

		ui.Success.Println("Modpack exported to " + ui.Bold.Sprint(result.Path))
	},
}

var whitelistedHosts = []string{
	"cdn.modrinth.com",
	"github.com",
	"raw.githubusercontent.com",
	"gitlab.com",
}

func canBeIncludedDirectly(mod *core.Mod, restrictDomains bool) bool {
	if mod.Download.Mode == core.ModeURL || mod.Download.Mode == "" {
		if !restrictDomains {
			return true
		}

		modUrl, err := url.Parse(mod.Download.URL)
		if err == nil {
			if slices.Contains(whitelistedHosts, modUrl.Host) {
				return true
			}
		}
	}
	return false
}

func init() {
	modrinthCmd.AddCommand(exportCmd)
	exportCmd.Flags().Bool("restrictDomains", true, "Restricts domains to those allowed by modrinth.com")
	exportCmd.Flags().StringP("output", "o", "", "The file to export the modpack to")
	_ = viper.BindPFlag("modrinth.export.restrictDomains", exportCmd.Flags().Lookup("restrictDomains"))
	_ = viper.BindPFlag("modrinth.export.output", exportCmd.Flags().Lookup("output"))
}
