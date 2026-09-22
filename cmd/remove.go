package cmd

import (
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

// removeCmd represents the remove command
var removeCmd = &cobra.Command{
	Use:     "remove",
	Short:   "Remove an external file from the modpack; equivalent to manually removing the file and running packwiz refresh",
	Aliases: []string{"delete", "uninstall", "rm"},
	Args:    cobra.ExactArgs(1),
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
		resolvedMod, ok := index.FindMod(args[0])
		if !ok {
			ui.Error.Println("Can't find this file; please ensure you have run packwiz refresh and specify its slug, its .pw.toml file name, or a path to that file")
			os.Exit(1)
		}
		err = os.Remove(resolvedMod)
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		ui.Muted.Println("Removing file from index...")
		err = index.RemoveFile(resolvedMod)
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		err = index.Write()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		err = pack.UpdateIndexHash()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		err = pack.Write()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		ui.Success.Printf("%s removed successfully!\n", ui.Bold.Sprint(args[0]))
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
