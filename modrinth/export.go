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
	Short: "Export the current modpack into a .mrpack for Modrinth, or a server pack",
	Long: `Export the pack as a .mrpack: the mods are listed in its manifest for the launcher to download, and the files the
index tracks that aren't mods (config files and the like) go in its overrides. So does the pack's README.md, LICENSE and
CHANGELOG.md, whichever of them are in the pack's directory: packwiz doesn't track them in the index, so they are not
installed by anything else, and need not be listed anywhere for the export to include them.

With --server, export the server pack instead: a zip (named after the pack, ending "` + core.ServerPackSuffix + `", unless
--output says otherwise) of what a server needs, laid out as it is on the server, to be unzipped there. The mods that run
on the server (those on the server or on both sides, but not optional ones that aren't on by default) are downloaded
into it, and it has the files the index tracks that aren't mods, and the pack's README.md, LICENSE and CHANGELOG.md.
Last come the files in the pack's ` + core.ServerConfigDir + `/ folder, such as server.properties, at the top of the zip: each
replaces whatever else would be at its path, so a config file can differ on the server. The folder isn't in the index,
so it is never installed on a client. The server pack's MODS.md is ` + core.ServerModListFile + `, the list of the server's
mods that "packwiz list --save --side server" writes, and its CHANGELOG.md is serverconfig/CHANGELOG.md, the
changelog of the server that "packwiz changelog --save" and every release write; exporting says if either is missing or
out of date.

With --bisect, export a server pack that Bisect Hosting can run as it is (named after the pack, ending
"` + core.BisectPackSuffix + `", unless --output says otherwise): the server pack above, with the NeoForge server installed in it
(its libraries and run scripts) and NeoForge's Server Starter Jar as server.jar, all at the top of the zip. On Bisect,
install "Custom JAR" on the server, upload the zip and Unarchive it in the server's root folder, and set the Java version
on the Home tab to the one the pack's Minecraft needs (17 for 1.18 to 1.20.4, 21 for 1.20.5 to 1.21.x, 25 from 26.1);
exporting says which. The first time, the server is installed by running NeoForge's installer with java (--java says which
java; it isn't needed again), and it and the jar are kept in packwiz's cache. A file in serverconfig/ replaces an installed
file of the same path.`,
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

		ui.Muted.Println("Retrieving external files...")
		result, err := exportWith(pack, &index, mods, ExportOptions{
			Output:          viper.GetString("modrinth.export.output"),
			RestrictDomains: viper.GetBool("modrinth.export.restrictDomains"),
			Server:          viper.GetBool("modrinth.export.server"),
			Dev:             viper.GetBool("modrinth.export.dev"),
			Bisect:          viper.GetBool("modrinth.export.bisect"),
			Java:            viper.GetString("modrinth.export.java"),
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

		if result.Bisect {
			ui.Success.Println("Bisect Hosting server pack exported to " + ui.Bold.Sprint(result.Path))
			ui.Info.Println(result.Instructions)
			return
		}
		if result.Server {
			ui.Success.Println("Server pack exported to " + ui.Bold.Sprint(result.Path))
			return
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
	exportCmd.Flags().Bool("server", false, "Export the server pack, a zip with the server's mods and the files in "+core.ServerConfigDir+"/, instead of a .mrpack")
	_ = viper.BindPFlag("modrinth.export.restrictDomains", exportCmd.Flags().Lookup("restrictDomains"))
	_ = viper.BindPFlag("modrinth.export.output", exportCmd.Flags().Lookup("output"))
	exportCmd.Flags().Bool("dev", false, "Export a .mrpack with every mod on both the client and the server, whatever its side, for development")
	exportCmd.Flags().Bool("bisect", false, "Export a server pack for Bisect Hosting: the server pack with the NeoForge server installed in it and a jar to start it")
	exportCmd.Flags().String("java", "java", "The java to install the NeoForge server with, for --bisect, the first time")
	exportCmd.MarkFlagsMutuallyExclusive("server", "dev", "bisect")
	_ = viper.BindPFlag("modrinth.export.bisect", exportCmd.Flags().Lookup("bisect"))
	_ = viper.BindPFlag("modrinth.export.java", exportCmd.Flags().Lookup("java"))
	_ = viper.BindPFlag("modrinth.export.dev", exportCmd.Flags().Lookup("dev"))
	_ = viper.BindPFlag("modrinth.export.server", exportCmd.Flags().Lookup("server"))
}
