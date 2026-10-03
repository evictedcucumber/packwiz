package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all the mods in the modpack",
	Long: `List the mods in the modpack, and anything else with a metadata file, such as resource packs and shader packs.

With --save, or --output, the list is written to a markdown file instead of printed: the pack's name and description, then what is in it, in alphabetical order under a heading for each kind, each as a link to its Modrinth page followed by its version. It doesn't say what was added as a dependency. --side and --only choose what is listed, as they do for the plain list (--only main leaves the dependencies out).

With --side server the list is the server's, and is written to ` + core.ServerModListFile + ` unless --output says where: that is the MODS.md of the server pack ("packwiz modrinth export --server"), in place of the pack's own.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {

		// Load pack
		pack, err := core.LoadPack()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		// Load index
		index, err := pack.LoadIndex()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		// Load mods
		mods, err := index.LoadAllMods()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		// Filter mods by main/dependency status
		if viper.IsSet("list.only") {
			only := viper.GetString("list.only")
			if only != OnlyMain && only != OnlyDependencies {
				ui.Error.Printf("Invalid --only %q, must be one of main, dependencies\n", only)
				os.Exit(1)
			}
			mods = FilterByKind(mods, only)
		}

		// Filter mods by side
		if viper.IsSet("list.side") {
			side := viper.GetString("list.side")
			if side != core.UniversalSide && side != core.ServerSide && side != core.ClientSide {
				ui.Error.Printf("Invalid side %q, must be one of client, server, or both (default)\n", side)
				os.Exit(1)
			}
			mods = FilterBySide(mods, side)
		}

		SortMods(mods)

		if viper.GetBool("list.save") || viper.IsSet("list.output") {
			if err := writeMarkdownList(markdownListPath(), pack, index, mods); err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}
			return
		}

		// Print mods
		showKind := viper.GetBool("list.show-kind")
		for _, mod := range mods {
			line := mod.Name
			if viper.GetBool("list.version") {
				line = fmt.Sprintf("%s %s", line, ui.Muted.Sprintf("(%s)", mod.FileName))
			}
			if showKind {
				// A dependency is there for another mod, so it is the one that fades back
				kind := ui.Info.Sprint("[main]")
				if mod.AddedAsDependency {
					kind = ui.Muted.Sprint("[dependency]")
				}
				line = fmt.Sprintf("%s %s", line, kind)
			}
			fmt.Println(line)
		}
	},
}

// What --only takes: the mods that were added on their own, or those that were added because another needs them
const (
	OnlyMain         = "main"
	OnlyDependencies = "dependencies"
)

// FilterByKind keeps the mods that are main mods (only is OnlyMain) or are dependencies of others (OnlyDependencies), as
// "packwiz list --only" lists them. It works in place, so the mods that it is given are not to be used as they were.
func FilterByKind(mods []*core.Mod, only string) []*core.Mod {
	i := 0
	for _, mod := range mods {
		if (only == OnlyDependencies) == mod.AddedAsDependency {
			mods[i] = mod
			i++
		}
	}
	return mods[:i]
}

// FilterBySide keeps the mods that run on a side, as "packwiz list --side" lists them: those that are on it, and those
// that are on both, which is what a mod with no side is taken to be. It works in place, as FilterByKind does.
func FilterBySide(mods []*core.Mod, side string) []*core.Mod {
	i := 0
	for _, mod := range mods {
		if mod.Side == side || mod.Side == core.EmptySide || mod.Side == core.UniversalSide || side == core.UniversalSide {
			mods[i] = mod
			i++
		}
	}
	return mods[:i]
}

// SortMods puts mods in alphabetical order of their names, ignoring case, which is the order they are listed in.
func SortMods(mods []*core.Mod) {
	sort.Slice(mods, func(i, j int) bool {
		return strings.ToLower(mods[i].Name) < strings.ToLower(mods[j].Name)
	})
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
	listCmd.Flags().Bool("save", false, "Write the list to a markdown file ("+core.ModListFile+" in the pack folder, or "+core.ServerModListFile+" with --side server, unless --output says where) instead of printing it")
	_ = viper.BindPFlag("list.save", listCmd.Flags().Lookup("save"))
	listCmd.Flags().StringP("output", "o", "", "Write the list as markdown to this file, or print it if \"-\" (implies --save)")
	_ = viper.BindPFlag("list.output", listCmd.Flags().Lookup("output"))

	// The markdown list is of names only, so what these add to the plain list has nowhere to go
	for _, save := range []string{"save", "output"} {
		for _, plain := range []string{"version", "show-kind"} {
			listCmd.MarkFlagsMutuallyExclusive(save, plain)
		}
	}
}
