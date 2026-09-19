package modrinth

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// depsCmd represents the deps command
var depsCmd = &cobra.Command{
	Use:   "deps",
	Short: "List the dependencies of Modrinth-managed mods, and whether they are already added to the pack",
	Args:  cobra.NoArgs,
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
		mods, err := index.LoadAllMods()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// installedNames maps installed project IDs to the mod's local name
		installedNames := make(map[string]string)
		var mrMods []*core.Mod
		for _, mod := range mods {
			data, ok := mod.GetParsedUpdateData("modrinth")
			if !ok {
				continue
			}
			updateData := data.(mrUpdateData)
			if updateData.ProjectID == "" {
				continue
			}
			installedNames[updateData.ProjectID] = mod.Name
			mrMods = append(mrMods, mod)
		}

		if len(mrMods) == 0 {
			fmt.Println("No Modrinth-managed mods found.")
			return
		}

		// Forgified Fabric API takes the place of Fabric API, so what needs Fabric API has it
		if runsFabricMods(pack, slices.Collect(maps.Keys(installedNames))) {
			installedNames[fabricAPIProjectID] = installedNames[forgifiedFabricAPIProjectID]
		}

		refresh := viper.GetBool("deps.refresh")

		// Fetch dependency data for any mod that doesn't have it stored yet (or all mods, with --refresh)
		var fetched []*core.Mod
		var fetchFailed []string
		for _, mod := range mrMods {
			if len(mod.Dependencies) > 0 && !refresh {
				continue
			}
			data, _ := mod.GetParsedUpdateData("modrinth")
			updateData := data.(mrUpdateData)
			version, err := mrDefaultClient.Versions.Get(updateData.InstalledVersion)
			if err != nil {
				fetchFailed = append(fetchFailed, mod.Name)
				continue
			}
			mod.Dependencies = buildDependencyList(version)
			fetched = append(fetched, mod)
		}

		for _, name := range fetchFailed {
			fmt.Printf("Warning: failed to fetch dependency data for %q\n", name)
		}

		if len(fetched) > 0 {
			fmt.Printf("Fetched dependency data for %d mod(s) from Modrinth.\n", len(fetched))
			if cmdshared.PromptYesNo("Save this dependency data to the pack for faster future reports? [Y/n]: ") {
				for _, mod := range fetched {
					format, hash, err := mod.Write()
					if err != nil {
						fmt.Printf("Failed to save dependency data for %q: %v\n", mod.Name, err)
						continue
					}
					err = index.RefreshFileWithHash(mod.GetFilePath(), format, hash, true)
					if err != nil {
						fmt.Printf("Failed to update index for %q: %v\n", mod.Name, err)
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
			}
			fmt.Println()
		}

		// Resolve display names for dependencies that aren't already in the pack
		var unresolvedIDs []string
		for _, mod := range mrMods {
			for _, dep := range mod.Dependencies {
				if _, ok := installedNames[dep.ID]; !ok {
					unresolvedIDs = append(unresolvedIDs, dep.ID)
				}
			}
		}
		slices.Sort(unresolvedIDs)
		unresolvedIDs = slices.Compact(unresolvedIDs)

		depNames := make(map[string]string)
		if len(unresolvedIDs) > 0 {
			projects, err := mrDefaultClient.Projects.GetMultiple(unresolvedIDs)
			if err != nil {
				fmt.Printf("Warning: failed to resolve dependency project names: %v\n", err)
			} else {
				for _, p := range projects {
					if p.ID != nil && p.Title != nil {
						depNames[*p.ID] = *p.Title
					}
				}
			}
		}

		sort.Slice(mrMods, func(i, j int) bool {
			return strings.ToLower(mrMods[i].Name) < strings.ToLower(mrMods[j].Name)
		})

		missingRequiredTotal := 0
		for _, mod := range mrMods {
			if len(mod.Dependencies) == 0 {
				continue
			}
			fmt.Println(mod.Name + ":")
			for _, dep := range mod.Dependencies {
				name, installed := installedNames[dep.ID]
				status := "already in pack"
				if !installed {
					status = "missing"
					if resolved, ok := depNames[dep.ID]; ok {
						name = resolved
					} else {
						name = dep.ID
					}
					if dep.Type == "required" {
						missingRequiredTotal++
					}
				}
				fmt.Printf("  [%s] %s (%s)\n", dep.Type, name, status)
			}
		}

		if missingRequiredTotal > 0 {
			fmt.Printf("\n%d required dependencies are missing from the pack. Use 'packwiz mr add' to install them.\n", missingRequiredTotal)
		} else {
			fmt.Println("\nAll required dependencies are already added to the pack!")
		}
	},
}

func init() {
	modrinthCmd.AddCommand(depsCmd)

	depsCmd.Flags().BoolP("refresh", "r", false, "Re-fetch dependency data from Modrinth even for mods that already have it cached")
	_ = viper.BindPFlag("deps.refresh", depsCmd.Flags().Lookup("refresh"))
}
