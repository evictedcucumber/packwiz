package cmd

import (
	"fmt"
	"os"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
)

var dependenciesCmd = &cobra.Command{
	Use:     "dependencies",
	Aliases: []string{"deps", "tree", "deptree"},
	Short:   "Manage and view the dependency tree for the modpack",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_, index := loadPackAndIndex()

		packDeps, err := index.GenerateDependencyTree()
		if err != nil {
			fmt.Println("Error generating dependency tree:", err)
			os.Exit(1)
		}

		fmt.Println(packDeps.FormatDependencyTree())
	},
}

var dependenciesGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate or update dependencies.toml and track it in index.toml",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pack, index := loadPackAndIndex()

		err := finalizePackWithDependencies(&pack, &index, core.SyncDepsOpts{
			NormalizeAll:        true,
			ForceDependencyTree: true,
		})
		if err != nil {
			fmt.Println("Error writing dependency tree:", err)
			os.Exit(1)
		}

		fmt.Println("Generated dependencies.toml successfully!")
	},
}

var dependenciesValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate that dependencies.toml is up-to-date and all dependencies are tracked",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_, index := loadPackAndIndex()

		err := index.ValidateDependencyTree()
		if err != nil {
			fmt.Printf("Validation failed: %s\n", err)
			os.Exit(1)
		}

		fmt.Println("dependencies.toml and dependency tree are valid!")
	},
}

var dependenciesFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Resolve and write dependency metadata into all existing .pw.toml files",
	Long: `Fix queries each mod's provider API (Modrinth/CurseForge) to determine its
required dependencies, then writes the resolved dependency paths into every
.pw.toml file. After fixing, it regenerates dependencies.toml automatically.

Use this to back-fill dependency data on packs that were created before
dependency tracking was added.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pack, index := loadPackAndIndex()

		fmt.Println("Resolving dependencies for all mods...")
		err := finalizePackWithDependencies(&pack, &index, core.SyncDepsOpts{
			RefreshAll:          true,
			ForceDependencyTree: true,
		})
		if err != nil {
			fmt.Printf("Error fixing dependencies: %s\n", err)
			os.Exit(1)
		}

		fmt.Println("All mod dependencies fixed and dependencies.toml regenerated!")
	},
}

func init() {
	rootCmd.AddCommand(dependenciesCmd)
	dependenciesCmd.AddCommand(dependenciesGenerateCmd)
	dependenciesCmd.AddCommand(dependenciesValidateCmd)
	dependenciesCmd.AddCommand(dependenciesFixCmd)
}
