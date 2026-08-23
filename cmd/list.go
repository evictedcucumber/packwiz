package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all the mods in the modpack",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {

		// Load pack
		pack, err := core.LoadPack()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Load index
		index, err := pack.LoadIndex()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Load mods
		mods, err := index.LoadAllMods()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Filter mods by main/dependency status
		if viper.IsSet("list.only") {
			only := viper.GetString("list.only")
			if only != "main" && only != "dependencies" {
				fmt.Printf("Invalid --only %q, must be one of main, dependencies\n", only)
				os.Exit(1)
			}

			i := 0
			for _, mod := range mods {
				if (only == "dependencies") == mod.AddedAsDependency {
					mods[i] = mod
					i++
				}
			}
			mods = mods[:i]
		}

		// Filter mods by side
		if viper.IsSet("list.side") {
			side := viper.GetString("list.side")
			if side != core.UniversalSide && side != core.ServerSide && side != core.ClientSide {
				fmt.Printf("Invalid side %q, must be one of client, server, or both (default)\n", side)
				os.Exit(1)
			}

			i := 0
			for _, mod := range mods {
				if mod.Side == side || mod.Side == core.EmptySide || mod.Side == core.UniversalSide || side == core.UniversalSide {
					mods[i] = mod
					i++
				}
			}
			mods = mods[:i]
		}

		sort.Slice(mods, func(i, j int) bool {
			return strings.ToLower(mods[i].Name) < strings.ToLower(mods[j].Name)
		})

		// Print mods
		showKind := viper.GetBool("list.show-kind")
		for _, mod := range mods {
			line := mod.Name
			if viper.GetBool("list.version") {
				line = fmt.Sprintf("%s (%s)", line, mod.FileName)
			}
			if showKind {
				kind := "main"
				if mod.AddedAsDependency {
					kind = "dependency"
				}
				line = fmt.Sprintf("%s [%s]", line, kind)
			}
			fmt.Println(line)
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().BoolP("version", "v", false, "Print name and version")
	_ = viper.BindPFlag("list.version", listCmd.Flags().Lookup("version"))
	listCmd.Flags().StringP("side", "s", "", "Filter mods by side (e.g., client or server)")
	_ = viper.BindPFlag("list.side", listCmd.Flags().Lookup("side"))
	listCmd.Flags().String("only", "", "Filter mods by kind: \"main\" or \"dependencies\"")
	_ = viper.BindPFlag("list.only", listCmd.Flags().Lookup("only"))
	listCmd.Flags().Bool("show-kind", false, "Show whether each mod is a main mod or a dependency")
	_ = viper.BindPFlag("list.show-kind", listCmd.Flags().Lookup("show-kind"))

}
