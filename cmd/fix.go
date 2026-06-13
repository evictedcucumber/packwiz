package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var fixDepsFlag bool
var fixDeleteFlag bool

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Fix pack and mod metadata to comply with packwiz validation rules",
	Long: `Fix applies automatic repairs to pack.toml and mod metadata files.

By default, fix adds missing keys and fills metadata where possible. Use
--delete to remove invalid dependency references, obsolete keys, and unknown
keys that validate reports.

If modlist generation is enabled, modlist.md is also rewritten.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		packFile := viper.GetString("pack-file")
		if _, err := os.Stat(packFile); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No pack.toml file found, run 'packwiz init' to create one!")
				os.Exit(1)
			}
			fmt.Printf("Error reading pack file: %s\n", err)
			os.Exit(1)
		}

		packFixes, err := fixPackConfig(packFile, fixDeleteFlag)
		if err != nil {
			fmt.Printf("Error fixing pack file: %s\n", err)
			os.Exit(1)
		}
		if len(packFixes) > 0 {
			fmt.Println("Fixed pack.toml:")
			for _, fix := range packFixes {
				fmt.Printf("- %s\n", fix)
			}
		}

		pack, err := core.LoadPack()
		if err != nil {
			fmt.Printf("Error loading pack: %s\n", err)
			os.Exit(1)
		}

		index, err := pack.LoadIndex()
		if err != nil {
			fmt.Printf("Error loading index: %s\n", err)
			os.Exit(1)
		}

		fmt.Println("Fixing mod metadata...")
		modFixCount, err := index.FixModMetadata(pack, core.FixModMetadataOpts{Delete: fixDeleteFlag})
		if err != nil {
			fmt.Printf("Error fixing mod metadata: %s\n", err)
			os.Exit(1)
		}
		if modFixCount > 0 {
			fmt.Printf("Updated %d mod metadata file(s).\n", modFixCount)
		} else {
			fmt.Println("All mod metadata files already compliant.")
		}

		syncOpts := core.SyncDepsOpts{NormalizeAll: true}
		if fixDepsFlag {
			syncOpts.RefreshAll = true
			syncOpts.ForceDependencyTree = true
		}

		if err := index.SyncDependencyMetadata(pack, syncOpts); err != nil {
			fmt.Printf("Error syncing dependency metadata: %s\n", err)
			os.Exit(1)
		}

		if pack.ModList {
			if err := index.WriteModList(); err != nil {
				fmt.Printf("Error fixing modlist.md: %s\n", err)
				os.Exit(1)
			}
			fmt.Println("Rewrote modlist.md.")
		}

		if err := writePackAndIndex(&pack, &index); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		remaining, err := collectValidationIssues(packFile, index)
		if err != nil {
			fmt.Printf("Error validating after fix: %s\n", err)
			os.Exit(1)
		}
		if len(remaining) > 0 {
			printValidationIssues("Some issues could not be fixed automatically:", remaining)
			printValidationHints(remaining)
			os.Exit(1)
		}

		fmt.Println("Fix completed successfully!")
	},
}

func fixPackConfig(packFile string, removeInvalid bool) ([]string, error) {
	var raw map[string]interface{}
	if _, err := toml.DecodeFile(packFile, &raw); err != nil {
		return nil, err
	}

	fixes := make([]string, 0)

	if packFormat, ok := raw["pack-format"].(string); !ok || packFormat == "" {
		raw["pack-format"] = core.CurrentPackFormat
		fixes = append(fixes, "added pack-format")
	}

	if loader, ok := raw["loader"].(string); !ok || loader == "" {
		raw["loader"] = core.LoaderModrinth
		fixes = append(fixes, "added loader")
	}

	if _, ok := raw["modlist"]; !ok {
		raw["modlist"] = false
		fixes = append(fixes, "added modlist")
	}

	if releaseChannel, ok := raw["release-channel"].(string); !ok || releaseChannel == "" {
		raw["release-channel"] = "release"
		fixes = append(fixes, "added release-channel")
	}

	indexVal, hasIndex := raw["index"]
	indexMap, ok := indexVal.(map[string]interface{})
	if !hasIndex || !ok {
		indexMap = map[string]interface{}{}
		raw["index"] = indexMap
		fixes = append(fixes, "added index")
	}
	if indexFile, ok := indexMap["file"].(string); !ok || indexFile == "" {
		indexMap["file"] = "index.toml"
		fixes = append(fixes, "added index.file")
	} else if filepath.IsAbs(indexFile) {
		rel, err := filepath.Rel(filepath.Dir(packFile), indexFile)
		if err != nil {
			return fixes, err
		}
		indexMap["file"] = filepath.ToSlash(rel)
		fixes = append(fixes, "normalized index.file")
	}

	if removeInvalid {
		for key := range raw {
			if _, ok := allowedPackKeys[key]; !ok {
				delete(raw, key)
				fixes = append(fixes, fmt.Sprintf("removed unknown key %q", key))
			}
		}
	}

	if len(fixes) == 0 {
		return fixes, nil
	}

	f, err := os.Create(packFile)
	if err != nil {
		return fixes, err
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	enc.Indent = ""
	if err := enc.Encode(raw); err != nil {
		return fixes, err
	}
	return fixes, f.Close()
}

func init() {
	rootCmd.AddCommand(fixCmd)
	fixCmd.Flags().BoolVar(&fixDepsFlag, "deps", false, "Also resolve dependencies from provider APIs and regenerate dependencies.toml")
	fixCmd.Flags().BoolVar(&fixDeleteFlag, "delete", false, "Remove invalid dependency references, obsolete keys, and unknown keys")
}
