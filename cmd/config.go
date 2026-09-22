package cmd

import (
	"fmt"
	"os"

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
	Short: "List the pack's config files",
	Long: `List the pack's tracked files that aren't a mod's own metadata or destination file: normally what is in its
config/ folder, and anything else installed by hand rather than by a mod.

A mod can record which of these it owns in its metadata file's config-files (a path, or a path ending in "/" to claim
everything under it). With --invalid, only files that no mod's config-files claims are listed - for example, a config
file left behind by a mod that has since been removed, or never linked to the mod that installed it.`,
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

		files, err := index.ConfigFiles(mods)
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		onlyInvalid := viper.GetBool("config.list.invalid")
		for _, f := range files {
			if onlyInvalid && f.Claimed {
				continue
			}
			fmt.Println(f.Path)
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd)

	configListCmd.Flags().Bool("invalid", false, "Only list config files that no mod's config-files claims")
	_ = viper.BindPFlag("config.list.invalid", configListCmd.Flags().Lookup("invalid"))
}
