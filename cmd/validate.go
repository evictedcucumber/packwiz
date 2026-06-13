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

var validateFullFlag bool

var validateCmd = &cobra.Command{
	Use:     "validate",
	Short:   "Validate the current modpack and config",
	Aliases: []string{"check"},
	Args:    cobra.NoArgs,
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

		pack, err := core.LoadPack()
		if err != nil {
			fmt.Printf("Validation failed! Error loading pack: %s\n", err)
			os.Exit(1)
		}

		index, err := pack.LoadIndex()
		if err != nil {
			fmt.Printf("Validation failed! Error loading index: %s\n", err)
			os.Exit(1)
		}

		_, err = index.LoadAllMods()
		if err != nil {
			fmt.Printf("Validation failed! Error loading metadata files: %s\n", err)
			os.Exit(1)
		}

		issues, err := collectValidationIssues(packFile, index)
		if err != nil {
			fmt.Printf("Validation failed! %s\n", err)
			os.Exit(1)
		}

		if validateFullFlag {
			fmt.Println("Performing full validation (checking versions and links)...")
			fullIssues, err := collectFullValidationIssues(pack, index)
			if err != nil {
				fmt.Printf("Validation failed! %s\n", err)
				os.Exit(1)
			}
			issues = append(issues, fullIssues...)
		}

		if len(issues) > 0 {
			printValidationIssues("Validation failed!", issues)
			printValidationHints(issues)
			os.Exit(1)
		}

		fmt.Println("Validation successful! Pack and config look correct.")
	},
}

func validatePackConfig(packFile string) ([]validationIssue, error) {
	var raw map[string]interface{}
	if _, err := toml.DecodeFile(packFile, &raw); err != nil {
		return nil, err
	}

	issues := make([]validationIssue, 0)

	packFormatVal, hasPackFormat := raw["pack-format"]
	if !hasPackFormat {
		issues = append(issues, validationIssue{
			Key:      "pack-format",
			Message:  "required key is missing",
			Expected: "non-empty string (packwiz format version)",
			Example:  "pack-format = \"packwiz:1.1.0\"",
			Kind:     issueKindFix,
		})
	} else if packFormat, ok := packFormatVal.(string); !ok || packFormat == "" {
		issues = append(issues, validationIssue{
			Key:      "pack-format",
			Message:  "must be a non-empty string",
			Expected: "non-empty string (packwiz format version)",
			Example:  "pack-format = \"packwiz:1.1.0\"",
			Kind:     issueKindFix,
		})
	}

	loaderVal, hasLoader := raw["loader"]
	if !hasLoader {
		issues = append(issues, validationIssue{
			Key:      "loader",
			Message:  "required key is missing",
			Expected: "one of: \"modrinth\", \"curseforge\"",
			Example:  "loader = \"modrinth\"",
			Kind:     issueKindFix,
		})
	} else if loader, ok := loaderVal.(string); !ok || (loader != core.LoaderModrinth && loader != core.LoaderCurseforge) {
		issues = append(issues, validationIssue{
			Key:      "loader",
			Message:  "must be either \"modrinth\" or \"curseforge\"",
			Expected: "one of: \"modrinth\", \"curseforge\"",
			Example:  "loader = \"modrinth\"",
			Kind:     issueKindFix,
		})
	}

	modListVal, hasModList := raw["modlist"]
	if !hasModList {
		issues = append(issues, validationIssue{
			Key:      "modlist",
			Message:  "required key is missing",
			Expected: "boolean",
			Example:  "modlist = false",
			Kind:     issueKindFix,
		})
	} else if _, ok := modListVal.(bool); !ok {
		issues = append(issues, validationIssue{
			Key:      "modlist",
			Message:  "must be a boolean",
			Expected: "boolean",
			Example:  "modlist = false",
			Kind:     issueKindFix,
		})
	}

	indexVal, hasIndex := raw["index"]
	if !hasIndex {
		issues = append(issues, validationIssue{
			Key:      "index",
			Message:  "required table is missing",
			Expected: "TOML table with required key \"file\"",
			Example:  "[index]\\nfile = \"index.toml\"",
			Kind:     issueKindFix,
		})
	} else {
		indexMap, ok := indexVal.(map[string]interface{})
		if !ok {
			issues = append(issues, validationIssue{
				Key:      "index",
				Message:  "must be a TOML table",
				Expected: "TOML table with required key \"file\"",
				Example:  "[index]\\nfile = \"index.toml\"",
				Kind:     issueKindFix,
			})
		} else {
			indexFileVal, hasIndexFile := indexMap["file"]
			if !hasIndexFile {
				issues = append(issues, validationIssue{
					Key:      "index.file",
					Message:  "required key is missing",
					Expected: "non-empty relative string path",
					Example:  "[index]\\nfile = \"index.toml\"",
					Kind:     issueKindFix,
				})
			} else if indexFile, ok := indexFileVal.(string); !ok || indexFile == "" {
				issues = append(issues, validationIssue{
					Key:      "index.file",
					Message:  "must be a non-empty string",
					Expected: "non-empty relative string path",
					Example:  "[index]\\nfile = \"index.toml\"",
					Kind:     issueKindFix,
				})
			} else if filepath.IsAbs(indexFile) {
				issues = append(issues, validationIssue{
					Key:      "index.file",
					Message:  "should be a relative path",
					Expected: "relative string path",
					Example:  "[index]\\nfile = \"index.toml\"",
					Kind:     issueKindFix,
				})
			}
		}
	}

	versionsVal, hasVersions := raw["versions"]
	if !hasVersions {
		issues = append(issues, validationIssue{
			Key:      "versions",
			Message:  "required table is missing",
			Expected: "TOML table with required key \"minecraft\"",
			Example:  "[versions]\\nminecraft = \"1.21.0\"",
			Kind:     issueKindFix,
		})
	} else {
		versionsMap, ok := versionsVal.(map[string]interface{})
		if !ok {
			issues = append(issues, validationIssue{
				Key:      "versions",
				Message:  "must be a TOML table",
				Expected: "TOML table with required key \"minecraft\"",
				Example:  "[versions]\\nminecraft = \"1.21.0\"",
				Kind:     issueKindFix,
			})
		} else {
			mcVersionVal, hasMcVersion := versionsMap["minecraft"]
			if !hasMcVersion {
				issues = append(issues, validationIssue{
					Key:      "versions.minecraft",
					Message:  "required key is missing",
					Expected: "non-empty string (Minecraft version)",
					Example:  "[versions]\\nminecraft = \"1.21.0\"",
					Kind:     issueKindFix,
				})
			} else if mcVersion, ok := mcVersionVal.(string); !ok || mcVersion == "" {
				issues = append(issues, validationIssue{
					Key:      "versions.minecraft",
					Message:  "must be a non-empty string",
					Expected: "non-empty string (Minecraft version)",
					Example:  "[versions]\\nminecraft = \"1.21.0\"",
					Kind:     issueKindFix,
				})
			}
		}
	}

	return issues, nil
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().BoolVar(&validateFullFlag, "full", false, "Perform complete checks including version mismatches and link validation")
}
