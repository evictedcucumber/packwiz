package cmd

import (
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

func pinMod(args []string, pinned bool) {
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
		ui.Error.Println("Can't find this file; please ensure you have run packwiz refresh and specify its slug, its .pw.toml file name, or a path to that file")
		os.Exit(1)
	}
	modData, err := core.LoadMod(modPath)
	if err != nil {
		ui.Error.Println(err)
		os.Exit(1)
	}
	modData.Pin = pinned
	format, hash, err := modData.Write()
	if err != nil {
		ui.Error.Println(err)
		os.Exit(1)
	}
	err = index.RefreshFileWithHash(modPath, format, hash, true)
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

	message := "pinned"
	if !pinned {
		message = "unpinned"
	}
	ui.Success.Printf("%s %s successfully!\n", ui.Bold.Sprint(args[0]), message)
}

// pinCmd represents the pin command
var pinCmd = &cobra.Command{
	Use:     "pin",
	Short:   "Pin a file so it does not get updated automatically",
	Aliases: []string{"hold"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pinMod(args, true)
	},
}

// unpinCmd represents the unpin command
var unpinCmd = &cobra.Command{
	Use:     "unpin",
	Short:   "Unpin a file so it receives updates",
	Aliases: []string{"unhold"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pinMod(args, false)
	},
}

func init() {
	rootCmd.AddCommand(pinCmd)
	rootCmd.AddCommand(unpinCmd)
}
