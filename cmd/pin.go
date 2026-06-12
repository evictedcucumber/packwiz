package cmd

import (
	"fmt"
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
)

func pinMod(args []string, pinned bool) {
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
	modPath, err := resolveTrackedModMetaPath(index, args[0])
	if err != nil {
		fmt.Printf("Can't find this file: %s\n", err)
		fmt.Println("Use a path to a tracked .pw.toml file (you may need to run packwiz refresh).")
		os.Exit(1)
	}
	modData, err := core.LoadMod(modPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	modData.Pin = pinned
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

	message := "pinned"
	if !pinned {
		message = "unpinned"
	}
	fmt.Printf("%s %s successfully!\n", args[0], message)
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
