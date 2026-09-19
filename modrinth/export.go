package modrinth

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"

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
	Args:  cobra.NoArgs,
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

		fileName := viper.GetString("modrinth.export.output")
		if fileName == "" {
			fileName = pack.GetPackName() + ".mrpack"
		}
		expFile, err := os.Create(fileName)
		if err != nil {
			ui.Error.Printf("Failed to create zip: %s\n", err.Error())
			os.Exit(1)
		}
		exp := zip.NewWriter(expFile)

		// Add an overrides folder even if there are no files to go in it
		_, err = exp.Create("overrides/")
		if err != nil {
			ui.Error.Printf("Failed to add overrides folder: %s\n", err.Error())
			os.Exit(1)
		}

		// Found now, from the mods as they are in the pack, and said once the files are listed
		promotions := sidePromotions(mods)

		ui.Muted.Printf("Retrieving %v external files...\n", len(mods))

		restrictDomains := viper.GetBool("modrinth.export.restrictDomains")

		for _, mod := range mods {
			if !canBeIncludedDirectly(mod, restrictDomains) {
				cmdshared.PrintDisclaimer()
				break
			}
		}

		session, err := core.CreateDownloadSession(mods, []string{"sha1", "sha512", "length-bytes"})
		if err != nil {
			ui.Error.Printf("Error retrieving external files: %v\n", err)
			os.Exit(1)
		}

		cmdshared.ListManualDownloads(session)

		manifestFiles := make([]PackFile, 0)
		var exported []exportedFile
		for dl := range session.StartDownloads() {
			if canBeIncludedDirectly(dl.Mod, restrictDomains) {
				if dl.Error != nil {
					ui.Error.Printf("Download of %s (%s) failed: %v\n", dl.Mod.Name, dl.Mod.FileName, dl.Error)
					continue
				}
				for _, warning := range dl.Warnings {
					ui.Warning.Printf("Warning for %s (%s): %v\n", dl.Mod.Name, dl.Mod.FileName, warning)
				}

				path, err := index.RelIndexPath(dl.Mod.GetDestFilePath())
				if err != nil {
					ui.Error.Printf("Error resolving external file: %s\n", err.Error())
					// TODO: exit(1)?
					continue
				}

				hashes := make(map[string]string)
				hashes["sha1"] = dl.Hashes["sha1"]
				hashes["sha512"] = dl.Hashes["sha512"]
				fileSize, err := strconv.ParseUint(dl.Hashes["length-bytes"], 10, 64)
				if err != nil {
					panic(err)
				}

				// Create env options based on configured optional/side
				clientEnv, serverEnv := exportEnv(dl.Mod.Side, dl.Mod.Option != nil && dl.Mod.Option.Optional)

				// Modrinth URLs must be RFC3986
				u, err := core.ReencodeURL(dl.Mod.Download.URL)
				if err != nil {
					ui.Error.Printf("Error re-encoding download URL: %s\n", err.Error())
					u = dl.Mod.Download.URL
				}

				manifestFiles = append(manifestFiles, PackFile{
					Path:   path,
					Hashes: hashes,
					Env: &struct {
						Client string `json:"client"`
						Server string `json:"server"`
					}{Client: clientEnv, Server: serverEnv},
					Downloads: []string{u},
					FileSize:  fileSize,
				})

				exported = append(exported, exportedFile{name: dl.Mod.Name, path: path, client: clientEnv, server: serverEnv, size: fileSize})
			} else {
				folder := "overrides"
				if dl.Mod.Side == core.ClientSide {
					folder = "client-overrides"
				} else if dl.Mod.Side == core.ServerSide {
					folder = "server-overrides"
				}
				if cmdshared.AddToZip(dl, exp, folder, &index) {
					exported = append(exported, bundledFile(dl, folder, &index))
				}
			}
		}
		// sort by `path` property before serialising to ensure reproducibility
		sort.Slice(manifestFiles, func(i, j int) bool {
			return manifestFiles[i].Path < manifestFiles[j].Path
		})

		fmt.Println()
		fmt.Print(breakdown(exported))
		for _, p := range promotions {
			ui.Warning.Printf("Warning: %s is only exported for the server, but %s needs it on the client; 'packwiz modrinth validate' says more\n", ui.Bold.Sprint(p.mod.Name), ui.Bold.Sprint(p.neededBy.Name))
		}

		err = session.SaveIndex()
		if err != nil {
			ui.Error.Printf("Error saving cache index: %v\n", err)
			os.Exit(1)
		}

		dependencies := make(map[string]string)
		dependencies["minecraft"], err = pack.GetMCVersion()
		if err != nil {
			_ = exp.Close()
			_ = expFile.Close()
			ui.Error.Println("Error creating manifest: " + err.Error())
			os.Exit(1)
		}
		if neoforgeVersion, ok := pack.Versions["neoforge"]; ok {
			dependencies["neoforge"] = neoforgeVersion
		}

		manifest := Pack{
			FormatVersion: 1,
			Game:          "minecraft",
			VersionID:     pack.Version,
			Name:          pack.Name,
			Summary:       pack.Description,
			Files:         manifestFiles,
			Dependencies:  dependencies,
		}

		if len(pack.Version) == 0 {
			ui.Warning.Println("Warning: pack.toml version field must not be empty to create a valid Modrinth pack")
		}

		manifestFile, err := exp.Create("modrinth.index.json")
		if err != nil {
			_ = exp.Close()
			_ = expFile.Close()
			ui.Error.Println("Error creating manifest: " + err.Error())
			os.Exit(1)
		}

		w := json.NewEncoder(manifestFile)
		w.SetIndent("", "    ") // Documentation uses 4 spaces
		err = w.Encode(manifest)
		if err != nil {
			_ = exp.Close()
			_ = expFile.Close()
			ui.Error.Println("Error writing manifest: " + err.Error())
			os.Exit(1)
		}

		cmdshared.AddNonMetafileOverrides(&index, exp)

		err = exp.Close()
		if err != nil {
			ui.Error.Println("Error writing export file: " + err.Error())
			os.Exit(1)
		}
		err = expFile.Close()
		if err != nil {
			ui.Error.Println("Error writing export file: " + err.Error())
			os.Exit(1)
		}

		ui.Success.Println("Modpack exported to " + ui.Bold.Sprint(fileName))
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
