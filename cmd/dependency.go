package cmd

import (
	"fmt"
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
)

func markModAsDependency(args []string, isDependency bool) {
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
	modPath, ok := index.FindMod(args[0])
	if !ok {
		fmt.Println("Can't find this file; please ensure you have run packwiz refresh and use the name of the .pw.toml file (defaults to the project slug)")
		os.Exit(1)
	}
	modData, err := core.LoadMod(modPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	modData.AddedAsDependency = isDependency
	format, hash, err := modData.Write()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = index.RefreshFileWithHash(modPath, format, hash, true)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = index.Write()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = pack.UpdateIndexHash()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = pack.Write()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	message := "marked as a dependency"
	if !isDependency {
		message = "marked as a main mod"
	}
	fmt.Printf("%s %s successfully!\n", args[0], message)
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
