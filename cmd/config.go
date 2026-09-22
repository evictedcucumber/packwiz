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
	Short: "Show which mod each config file belongs to",
	Long: `Show the pack's tracked files that aren't a mod's own metadata or destination file - normally what is in its
config/ folder - as a tree of the mod each belongs to, with the files nothing claims listed under "Invalid" at the end.

A mod can record which of these it owns in its metadata file's config-files (a path, or a path ending in "/" to claim
everything under it). --state valid shows only what is claimed, and --state invalid shows only what isn't - for
example, a config file left behind by a mod that has since been removed, or never linked to the mod that installed it.`,
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

		tree, err := index.ConfigFileTree(mods)
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		showValid, showInvalid := true, true
		if viper.IsSet("config.list.state") {
			switch state := viper.GetString("config.list.state"); state {
			case "valid":
				showInvalid = false
			case "invalid":
				showValid = false
			default:
				ui.Error.Printf("Invalid --state %q, must be one of valid, invalid\n", state)
				os.Exit(1)
			}
		}

		printed := false
		if showValid {
			for _, m := range tree.Mods {
				if printed {
					fmt.Println()
				}
				printGroup(ui.Bold.Sprint(m.Mod.Name), m.Files, ui.Success)
				printed = true
			}
		}
		if showInvalid && len(tree.Unclaimed) > 0 {
			if printed {
				fmt.Println()
			}
			printGroup(ui.Bold.Sprint(ui.Warning.Sprint("Invalid")), tree.Unclaimed, ui.Warning)
		}
	},
}

// relatedMod is a mod loaded for "packwiz config relate", alongside its metadata file's path (for RefreshFileWithHash).
type relatedMod struct {
	path string
	data *core.Mod
}

// configRelateCmd represents the config relate command
var configRelateCmd = &cobra.Command{
	Use:   "relate --mod <mod> [--mod <mod>...] <config file/dir>...",
	Short: "Record that one or more mods own one or more config files or folders",
	Long: `Add paths to each given mod's metadata file's config-files, recording that it owns those config files or
folders (see "packwiz config list"). A folder is given a trailing "/", to claim everything under it; running this
again with the same arguments does nothing more.

--mod is its slug (unless it was renamed), the name of its .pw.toml file, or a path to that file, the same as
"packwiz pin" and "packwiz remove" take; give it once per mod. Every <config file/dir> is then added to every mod
given, so a config file shared between several mods can be related to all of them in one command. <config file/dir>
is a path to a file or folder that exists in the pack, from the current directory or the pack's root; if the pack
keeps its files in a mod's own folder (see Configured Defaults), it is written as if that folder didn't exist, the
same way every other config-files entry is.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		modRefs := viper.GetStringSlice("config.relate.mod")
		if len(modRefs) == 0 {
			ui.Error.Println("--mod is required; specify at least one mod to relate the config file(s) to")
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

		var relatedMods []relatedMod
		seenModPaths := make(map[string]bool, len(modRefs))
		for _, ref := range modRefs {
			modPath, ok := index.FindMod(ref)
			if !ok {
				ui.Error.Printf("Can't find %s; please ensure you have run packwiz refresh and specify its slug, its .pw.toml file name, or a path to that file\n", ui.Bold.Sprint(ref))
				os.Exit(1)
			}
			if seenModPaths[modPath] {
				continue
			}
			seenModPaths[modPath] = true
			modData, err := core.LoadMod(modPath)
			if err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}
			relatedMods = append(relatedMods, relatedMod{path: modPath, data: &modData})
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

		changed := false
		for _, rm := range relatedMods {
			var newlyClaimed, alreadyClaimed []string
			for _, rel := range rels {
				if rm.data.ClaimConfigFile(rel) {
					newlyClaimed = append(newlyClaimed, rel)
				} else {
					alreadyClaimed = append(alreadyClaimed, rel)
				}
			}
			if len(alreadyClaimed) > 0 {
				ui.Info.Printf("%s already claims %s\n", ui.Bold.Sprint(rm.data.Name), ui.Bold.Sprint(strings.Join(alreadyClaimed, ", ")))
			}
			if len(newlyClaimed) == 0 {
				continue
			}
			changed = true

			format, hash, err := rm.data.Write()
			if err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}
			if err := index.RefreshFileWithHash(rm.path, format, hash, true); err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}

			ui.Success.Printf("%s now claims %s\n", ui.Bold.Sprint(rm.data.Name), ui.Bold.Sprint(strings.Join(newlyClaimed, ", ")))
		}

		if !changed {
			return
		}

		if err := index.Write(); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		if err := pack.UpdateIndexHash(); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		if err := pack.Write(); err != nil {
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
	if isDir && !strings.HasSuffix(rel, "/") {
		rel += "/"
	}
	if dir := core.ConfigDirOfMods(mods); dir != "" {
		rel = strings.TrimPrefix(rel, dir+"/")
	}
	return rel, nil
}

// printGroup prints a heading and its files as a tree, each file in style.
func printGroup(heading string, files []string, style ui.Style) {
	fmt.Println(heading)
	for i, f := range files {
		branch := "├── "
		if i == len(files)-1 {
			branch = "└── "
		}
		style.Println(branch + f)
	}
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configRelateCmd)

	configListCmd.Flags().String("state", "", "Only show config files in this state: valid or invalid")
	_ = viper.BindPFlag("config.list.state", configListCmd.Flags().Lookup("state"))

	configRelateCmd.Flags().StringArrayP("mod", "m", nil, "A mod (slug, .pw.toml file name, or path) to relate the config file(s) to; repeat for multiple mods")
	_ = viper.BindPFlag("config.relate.mod", configRelateCmd.Flags().Lookup("mod"))
}
