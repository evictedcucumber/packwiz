package cmd

import (
	"fmt"
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
)

var modlistCmd = &cobra.Command{
	Use:   "modlist",
	Short: "Generate, validate, or fix modlist.md",
}

func loadPackAndIndex() (core.Pack, core.Index) {
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
	return pack, index
}

func writeModList(pack *core.Pack, index *core.Index, successMessage string, side string) {
	if !ensureModPageURLs(*index) {
		os.Exit(1)
	}
	err := index.WriteModListWithSide(side)
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
	fmt.Println(successMessage)
}

var modlistGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate or overwrite modlist.md",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pack, index := loadPackAndIndex()
		if !ensureModPageURLs(index) {
			os.Exit(1)
		}
		side, err := cmd.Flags().GetString("side")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if side != core.UniversalSide && side != core.ServerSide && side != core.ClientSide {
			fmt.Printf("Invalid side %q, must be one of client, server, or both (default)\n", side)
			os.Exit(1)
		}
		writeModList(&pack, &index, "Generated modlist.md successfully!", side)
	},
}

var modlistValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check whether modlist.md matches the current pack",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_, index := loadPackAndIndex()
		if !ensureModPageURLs(index) {
			os.Exit(1)
		}
		side, err := cmd.Flags().GetString("side")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if side != core.UniversalSide && side != core.ServerSide && side != core.ClientSide {
			fmt.Printf("Invalid side %q, must be one of client, server, or both (default)\n", side)
			os.Exit(1)
		}
		err = index.ValidateModListWithSide(side)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("modlist.md is missing; run 'packwiz modlist generate' or 'packwiz modlist fix'.")
			} else {
				fmt.Printf("modlist.md is invalid: %s\n", err)
				fmt.Println("Run 'packwiz modlist fix' to rewrite it from the current index.")
			}
			os.Exit(1)
		}
		fmt.Println("modlist.md is valid!")
	},
}

var modlistFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Rewrite modlist.md to match the current pack",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pack, index := loadPackAndIndex()
		if !ensureModPageURLs(index) {
			os.Exit(1)
		}
		side, err := cmd.Flags().GetString("side")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if side != core.UniversalSide && side != core.ServerSide && side != core.ClientSide {
			fmt.Printf("Invalid side %q, must be one of client, server, or both (default)\n", side)
			os.Exit(1)
		}
		writeModList(&pack, &index, "Fixed modlist.md successfully!", side)
	},
}

func ensureModPageURLs(index core.Index) bool {
	issues, err := validateModPageURLs(index)
	if err != nil {
		fmt.Printf("Validation failed! Error checking mod page URLs: %s\n", err)
		return false
	}
	if len(issues) == 0 {
		return true
	}

	fmt.Println("modlist.md generation failed! Missing mod page URLs:")
	for _, issue := range issues {
		fmt.Printf("- %s: %s\n", issue.Key, issue.Message)
		if issue.Expected != "" {
			fmt.Printf("  Expected: %s\n", issue.Expected)
		}
		if issue.Example != "" {
			fmt.Printf("  Example: %s\n", issue.Example)
		}
	}
	fmt.Println("Run 'packwiz validate' to find and fix the missing mod page URLs in your mod manifests.")
	return false
}

func init() {
	rootCmd.AddCommand(modlistCmd)
	modlistCmd.AddCommand(modlistGenerateCmd)
	modlistCmd.AddCommand(modlistValidateCmd)
	modlistCmd.AddCommand(modlistFixCmd)
	modlistGenerateCmd.Flags().StringP("side", "s", "both", "The side to include in modlist (client, server, or both)")
	modlistValidateCmd.Flags().StringP("side", "s", "both", "The side to include in modlist (client, server, or both)")
	modlistFixCmd.Flags().StringP("side", "s", "both", "The side to include in modlist (client, server, or both)")
}
