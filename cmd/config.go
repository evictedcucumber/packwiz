package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Work with the pack's config files",
}

// configListCmd represents the config list command
var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show which mod, mod loader or pack each config file belongs to",
	Long: `Show the pack's tracked files that aren't a mod's own metadata or destination file - normally what is in its
config/ folder - as a tree of who each belongs to, with the files nothing claims listed under "Invalid" at the end.

A config file belongs to a mod, to the pack's mod loader (NeoForge's own config/neoforge-common.toml, say) or to the pack
as a whole (options.txt). A mod records which it owns in its metadata file's config-files, and the pack records the
others in pack.toml's [config-files], under "pack" or the name of the loader; either is a path, or a path ending in "/"
to claim everything under it. The pack and the loader come first, then the mods. An entry that no tracked file matches -
for example, a config file that has since been deleted - is listed under its owner, marked "(missing)". --state valid
shows only what is claimed, --state invalid only what isn't (for example, a config file left behind by a mod that has
since been removed, or never linked to the mod that installed it), and --state missing only the entries that match
nothing.

What is tracked is what the index says, so run "packwiz refresh" after adding or deleting a config file.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
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
		mods, err := index.LoadAllMods()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		tree, err := index.ConfigFileTree(mods, pack)
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		showValid, showInvalid, showMissing := true, true, true
		if viper.IsSet("config.list.state") {
			showValid, showInvalid, showMissing = false, false, false
			switch state := viper.GetString("config.list.state"); state {
			case "valid":
				showValid = true
			case "invalid":
				showInvalid = true
			case "missing":
				showMissing = true
			default:
				ui.Error.Printf("Invalid --state %q, must be one of valid, invalid, missing\n", state)
				os.Exit(1)
			}
		}

		// The pack and its loader come first, then the mods: each is a heading with what it claims under it
		type group struct {
			name           string
			files, missing []string
		}
		var groups []group
		for _, o := range tree.Owners {
			groups = append(groups, group{o.Name, o.Files, o.Missing})
		}
		for _, m := range tree.Mods {
			groups = append(groups, group{m.Mod.Name, m.Files, m.Missing})
		}

		printed := false
		for _, g := range groups {
			var entries []treeEntry
			if showValid {
				for _, f := range g.files {
					entries = append(entries, treeEntry{f, ui.Success})
				}
			}
			if showMissing {
				for _, f := range g.missing {
					entries = append(entries, treeEntry{f + " (missing)", ui.Warning})
				}
			}
			if len(entries) == 0 {
				continue
			}
			if printed {
				fmt.Println()
			}
			printGroup(ui.Bold.Sprint(g.name), entries)
			printed = true
		}
		if showInvalid && len(tree.Unclaimed) > 0 {
			if printed {
				fmt.Println()
			}
			entries := make([]treeEntry, len(tree.Unclaimed))
			for i, f := range tree.Unclaimed {
				entries[i] = treeEntry{f, ui.Warning}
			}
			printGroup(ui.Bold.Sprint(ui.Warning.Sprint("Invalid")), entries)
		}
	},
}

// configRelateCmd represents the config relate command
var configRelateCmd = &cobra.Command{
	Use:   "relate [--mod <mod>...] [--loader] [--pack] <config file/dir>...",
	Short: "Record that mods, the mod loader or the pack own one or more config files or folders",
	Long: `Record who owns config files or folders (see "packwiz config list"): every <config file/dir> is added to each of
the mods given with --mod, to the pack's mod loader with --loader, and to the pack as a whole with --pack, in any
combination. A folder is given a trailing "/", to claim everything under it; running this again with the same arguments
does nothing more.

--mod is its slug (unless it was renamed), the name of its .pw.toml file, or a path to that file, the same as
"packwiz mr pin" takes, including a part of a name if that is no mod's (see "packwiz mr pin --help"); give it once per mod,
and it is added to that file's config-files. A config
file shared between several mods can be related to all of them in one command. --loader is for the loader's own files,
such as NeoForge's config/neoforge-common.toml, and --pack for what belongs to no mod or loader, such as options.txt:
both are added to pack.toml's [config-files], under "pack" or the name of the loader.

<config file/dir> is a path to a file or folder that exists in the pack, from the current directory or the pack's root;
if the pack keeps its files in a mod's own folder (see Configured Defaults), it is written as if that folder didn't
exist, the same way every other config-files entry is.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		modRefs := viper.GetStringSlice("config.relate.mod")
		relatePack := viper.GetBool("config.relate.pack")
		relateLoader := viper.GetBool("config.relate.loader")
		if len(modRefs) == 0 && !relatePack && !relateLoader {
			ui.Error.Println("--mod, --loader or --pack is required; specify who to relate the config file(s) to")
			os.Exit(1)
		}

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

		// What the pack and its loader are, in the keys of pack.toml's [config-files]
		var owners []string
		if relatePack {
			owners = append(owners, core.ConfigOwnerPack)
		}
		if relateLoader {
			loaders := pack.GetLoaders()
			if len(loaders) == 0 {
				ui.Error.Println("This pack has no mod loader to relate the config file(s) to")
				os.Exit(1)
			}
			owners = append(owners, loaders...)
		}

		var relatedMods []*core.Mod
		seenModPaths := make(map[string]bool, len(modRefs))
		for _, ref := range modRefs {
			modPath := findMod(index, ref)
			if seenModPaths[modPath] {
				continue
			}
			seenModPaths[modPath] = true
			modData, err := core.LoadMod(modPath)
			if err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}
			relatedMods = append(relatedMods, &modData)
		}

		mods, err := index.LoadAllMods()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		var rels []string
		seenRels := make(map[string]bool, len(args))
		for _, arg := range args {
			info, err := os.Stat(arg)
			if err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}
			rel, err := relateConfigPath(index, mods, arg, info.IsDir())
			if err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}
			if seenRels[rel] {
				continue
			}
			seenRels[rel] = true
			rels = append(rels, rel)
		}

		// The pack and its loader first, as "packwiz config list" has them, then the mods
		ownersChanged := false
		for _, owner := range owners {
			name := pack.ConfigOwnerName(owner)
			var newlyClaimed, alreadyClaimed []string
			for _, rel := range rels {
				if pack.ClaimConfigFile(owner, rel) {
					newlyClaimed = append(newlyClaimed, rel)
				} else {
					alreadyClaimed = append(alreadyClaimed, rel)
				}
			}
			if len(alreadyClaimed) > 0 {
				ui.Info.Printf("%s already claims %s\n", ui.Bold.Sprint(name), ui.Bold.Sprint(strings.Join(alreadyClaimed, ", ")))
			}
			if len(newlyClaimed) > 0 {
				ownersChanged = true
				ui.Success.Printf("%s now claims %s\n", ui.Bold.Sprint(name), ui.Bold.Sprint(strings.Join(newlyClaimed, ", ")))
			}
		}

		modsChanged := false
		for _, mod := range relatedMods {
			var newlyClaimed, alreadyClaimed []string
			for _, rel := range rels {
				if mod.ClaimConfigFile(rel) {
					newlyClaimed = append(newlyClaimed, rel)
				} else {
					alreadyClaimed = append(alreadyClaimed, rel)
				}
			}
			if len(alreadyClaimed) > 0 {
				ui.Info.Printf("%s already claims %s\n", ui.Bold.Sprint(mod.Name), ui.Bold.Sprint(strings.Join(alreadyClaimed, ", ")))
			}
			if len(newlyClaimed) == 0 {
				continue
			}
			modsChanged = true

			if err := index.SaveMod(mod); err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}

			ui.Success.Printf("%s now claims %s\n", ui.Bold.Sprint(mod.Name), ui.Bold.Sprint(strings.Join(newlyClaimed, ", ")))
		}

		// A changed mod is in the index, which is then saved with the pack; what the pack and its loader claim is only in
		// pack.toml, which isn't in the index
		switch {
		case modsChanged:
			err = pack.SaveIndex(index)
		case ownersChanged:
			err = pack.Write()
		}
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

// relateConfigPath turns a path given on the command line into a config-files entry: relative to the pack, with a
// trailing "/" added if isDir says it is a folder, and with the pack's config folder (see core.ConfigDirResolver)
// taken back out, so it reads the same as any other config-files entry does.
func relateConfigPath(index core.Index, mods []*core.Mod, path string, isDir bool) (string, error) {
	rel, err := index.RelIndexPath(path)
	if err != nil {
		return "", err
	}
	return core.ConfigEntry(rel, isDir, core.ConfigDirOfMods(mods)), nil
}

// treeEntry is a line of a tree, and the style it is shown in.
type treeEntry struct {
	text  string
	style ui.Style
}

// printGroup prints a heading and its entries as a tree.
func printGroup(heading string, entries []treeEntry) {
	fmt.Println(heading)
	for i, e := range entries {
		branch := "├── "
		if i == len(entries)-1 {
			branch = "└── "
		}
		e.style.Println(branch + e.text)
	}
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configRelateCmd)

	configListCmd.Flags().String("state", "", "Only show config files in this state: valid, invalid or missing")
	_ = viper.BindPFlag("config.list.state", configListCmd.Flags().Lookup("state"))

	configRelateCmd.Flags().StringArrayP("mod", "m", nil, "A mod (slug, .pw.toml file name, or path) to relate the config file(s) to; repeat for multiple mods")
	_ = viper.BindPFlag("config.relate.mod", configRelateCmd.Flags().Lookup("mod"))
	configRelateCmd.Flags().Bool("loader", false, "Relate the config file(s) to the pack's mod loader, such as NeoForge")
	_ = viper.BindPFlag("config.relate.loader", configRelateCmd.Flags().Lookup("loader"))

	configRelateCmd.Flags().Bool("pack", false, "Relate the config file(s) to the pack as a whole, such as options.txt")
	_ = viper.BindPFlag("config.relate.pack", configRelateCmd.Flags().Lookup("pack"))
}
