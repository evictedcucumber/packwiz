package cmd

import (
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

func markModAsDependency(args []string, isDependency bool) {
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
	modData.AddedAsDependency = isDependency
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

	message := "marked as a dependency"
	if !isDependency {
		message = "marked as a main mod"
	}
	ui.Success.Printf("%s %s successfully!\n", ui.Bold.Sprint(args[0]), message)
}

// markDependencyCmd represents the mark-dependency command
var markDependencyCmd = &cobra.Command{
	Use:   "mark-dependency",
	Short: "Mark a mod as having been added as a dependency of another mod, rather than a main mod",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		markModAsDependency(args, true)
	},
}

// unmarkDependencyCmd represents the unmark-dependency command
var unmarkDependencyCmd = &cobra.Command{
	Use:   "unmark-dependency",
	Short: "Mark a mod as a main mod, rather than a dependency of another mod",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		markModAsDependency(args, false)
	},
}

func init() {
	rootCmd.AddCommand(markDependencyCmd)
	rootCmd.AddCommand(unmarkDependencyCmd)
}
