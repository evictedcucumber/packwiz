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
				printGroup(ui.Bold.Sprint(m.Mod.Name), m.Files, false)
				printed = true
			}
		}
		if showInvalid && len(tree.Unclaimed) > 0 {
			if printed {
				fmt.Println()
			}
			printGroup(ui.Bold.Sprint(ui.Warning.Sprint("Invalid")), tree.Unclaimed, true)
		}
	},
}

// configRelateCmd represents the config relate command
var configRelateCmd = &cobra.Command{
	Use:   "relate <mod> <config file/dir>",
	Short: "Record that a mod owns a config file or folder",
	Long: `Add a path to a mod's metadata file's config-files, recording that it owns that config file or folder (see
"packwiz config list"). A folder is given a trailing "/", to claim everything under it; running this again with the
same arguments does nothing more.

<mod> is the name of its .pw.toml file (its slug, unless it was renamed), the same as "packwiz pin" and "packwiz
remove" take. <config file/dir> is a path to a file or folder that exists in the pack, from the current directory or
the pack's root; if the pack keeps its files in a mod's own folder (see Configured Defaults), it is written as if that
folder didn't exist, the same way every other config-files entry is.`,
	Args: cobra.ExactArgs(2),
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
		modPath, ok := index.FindMod(args[0])
		if !ok {
			ui.Error.Println("Can't find this file; please ensure you have run packwiz refresh and use the name of the .pw.toml file (defaults to the project slug)")
			os.Exit(1)
		}
		modData, err := core.LoadMod(modPath)
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		info, err := os.Stat(args[1])
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		mods, err := index.LoadAllMods()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		rel, err := relateConfigPath(index, mods, args[1], info.IsDir())
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		if !modData.ClaimConfigFile(rel) {
			ui.Info.Printf("%s already claims %s\n", ui.Bold.Sprint(modData.Name), ui.Bold.Sprint(rel))
			return
		}

		format, hash, err := modData.Write()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		if err := index.RefreshFileWithHash(modPath, format, hash, true); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
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

		ui.Success.Printf("%s now claims %s\n", ui.Bold.Sprint(modData.Name), ui.Bold.Sprint(rel))
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

// printGroup prints a heading and its files as a tree, styling each file as a warning if invalid says to.
func printGroup(heading string, files []string, invalid bool) {
	fmt.Println(heading)
	for i, f := range files {
		branch := "├── "
		if i == len(files)-1 {
			branch = "└── "
		}
		line := branch + f
		if invalid {
			ui.Warning.Println(line)
		} else {
			fmt.Println(line)
		}
	}
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configRelateCmd)

	configListCmd.Flags().String("state", "", "Only show config files in this state: valid or invalid")
	_ = viper.BindPFlag("config.list.state", configListCmd.Flags().Lookup("state"))
}
