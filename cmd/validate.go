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

type validationIssue struct {
	Key      string
	Message  string
	Expected string
	Example  string
}

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

		issues, err := validatePackConfig(packFile)
		if err != nil {
			fmt.Printf("Error validating pack file: %s\n", err)
			os.Exit(1)
		}
		if len(issues) > 0 {
			fmt.Println("Validation failed! Missing or invalid config keys:")
			for _, issue := range issues {
				fmt.Printf("- %s: %s\n", issue.Key, issue.Message)
				if issue.Expected != "" {
					fmt.Printf("  Expected: %s\n", issue.Expected)
				}
				if issue.Example != "" {
					fmt.Printf("  Example: %s\n", issue.Example)
				}
			}
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
		})
	} else if packFormat, ok := packFormatVal.(string); !ok || packFormat == "" {
		issues = append(issues, validationIssue{
			Key:      "pack-format",
			Message:  "must be a non-empty string",
			Expected: "non-empty string (packwiz format version)",
			Example:  "pack-format = \"packwiz:1.1.0\"",
		})
	}

	loaderVal, hasLoader := raw["loader"]
	if !hasLoader {
		issues = append(issues, validationIssue{
			Key:      "loader",
			Message:  "required key is missing",
			Expected: "one of: \"modrinth\", \"curseforge\"",
			Example:  "loader = \"modrinth\"",
		})
	} else if loader, ok := loaderVal.(string); !ok || (loader != core.LoaderModrinth && loader != core.LoaderCurseforge) {
		issues = append(issues, validationIssue{
			Key:      "loader",
			Message:  "must be either \"modrinth\" or \"curseforge\"",
			Expected: "one of: \"modrinth\", \"curseforge\"",
			Example:  "loader = \"modrinth\"",
		})
	}

	indexVal, hasIndex := raw["index"]
	if !hasIndex {
		issues = append(issues, validationIssue{
			Key:      "index",
			Message:  "required table is missing",
			Expected: "TOML table with required key \"file\"",
			Example:  "[index]\\nfile = \"index.toml\"",
		})
	} else {
		indexMap, ok := indexVal.(map[string]interface{})
		if !ok {
			issues = append(issues, validationIssue{
				Key:      "index",
				Message:  "must be a TOML table",
				Expected: "TOML table with required key \"file\"",
				Example:  "[index]\\nfile = \"index.toml\"",
			})
		} else {
			indexFileVal, hasIndexFile := indexMap["file"]
			if !hasIndexFile {
				issues = append(issues, validationIssue{
					Key:      "index.file",
					Message:  "required key is missing",
					Expected: "non-empty relative string path",
					Example:  "[index]\\nfile = \"index.toml\"",
				})
			} else if indexFile, ok := indexFileVal.(string); !ok || indexFile == "" {
				issues = append(issues, validationIssue{
					Key:      "index.file",
					Message:  "must be a non-empty string",
					Expected: "non-empty relative string path",
					Example:  "[index]\\nfile = \"index.toml\"",
				})
			} else if filepath.IsAbs(indexFile) {
				issues = append(issues, validationIssue{
					Key:      "index.file",
					Message:  "should be a relative path",
					Expected: "relative string path",
					Example:  "[index]\\nfile = \"index.toml\"",
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
		})
	} else {
		versionsMap, ok := versionsVal.(map[string]interface{})
		if !ok {
			issues = append(issues, validationIssue{
				Key:      "versions",
				Message:  "must be a TOML table",
				Expected: "TOML table with required key \"minecraft\"",
				Example:  "[versions]\\nminecraft = \"1.21.0\"",
			})
		} else {
			mcVersionVal, hasMcVersion := versionsMap["minecraft"]
			if !hasMcVersion {
				issues = append(issues, validationIssue{
					Key:      "versions.minecraft",
					Message:  "required key is missing",
					Expected: "non-empty string (Minecraft version)",
					Example:  "[versions]\\nminecraft = \"1.21.0\"",
				})
			} else if mcVersion, ok := mcVersionVal.(string); !ok || mcVersion == "" {
				issues = append(issues, validationIssue{
					Key:      "versions.minecraft",
					Message:  "must be a non-empty string",
					Expected: "non-empty string (Minecraft version)",
					Example:  "[versions]\\nminecraft = \"1.21.0\"",
				})
			}
		}
	}

	return issues, nil
}

func init() {
	rootCmd.AddCommand(validateCmd)
}
