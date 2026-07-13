package cmd

import (
	"fmt"
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
)

// removeCmd represents the remove command
var removeCmd = &cobra.Command{
	Use:     "remove",
	Short:   "Remove an external file from the modpack; equivalent to manually removing the file and running packwiz refresh",
	Aliases: []string{"delete", "uninstall", "rm"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Loading modpack...")
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
		resolvedMod, err := resolveModTargetPath(index, args[0])
		if err != nil {
			fmt.Printf("Can't find this file: %s\n", err)
			fmt.Println("Use the project slug or a path to a tracked .pw.toml file (you may need to run packwiz refresh).")
			os.Exit(1)
		}
		err = os.Remove(index.ResolveIndexPath(resolvedMod))
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("Removing file from index...")
		err = index.RemoveFile(resolvedMod)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if pack.ModList {
			err = index.WriteModList()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		err = finalizePackWithDependencies(&pack, &index, core.SyncDepsOpts{RemovedModPath: resolvedMod})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Printf("%s removed successfully!\n", args[0])
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
