package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/viper"
)

type validationIssueKind int

const (
	issueKindFix validationIssueKind = iota
	issueKindDelete
)

type validationIssue struct {
	Key      string
	Message  string
	Expected string
	Example  string
	Kind     validationIssueKind
}

var allowedPackKeys = map[string]struct{}{
	"name":        {},
	"author":      {},
	"version":     {},
	"description": {},
	"pack-format": {},
	"loader":      {},
	"modlist":     {},
	"index":       {},
	"versions":    {},
	"export":      {},
	"options":     {},
}

var allowedModKeys = map[string]struct{}{
	"name":         {},
	"filename":     {},
	"version":      {},
	"page-url":     {},
	"category":     {},
	"side":         {},
	"pin":          {},
	"download":     {},
	"update":       {},
	"dependencies": {},
	"option":       {},
}

var obsoleteModOptionKeys = []string{"optional", "default"}

func collectValidationIssues(packFile string, index core.Index) ([]validationIssue, error) {
	issues := make([]validationIssue, 0)

	packIssues, err := validatePackConfig(packFile)
	if err != nil {
		return nil, err
	}
	issues = append(issues, packIssues...)

	unknownPackIssues, err := validateUnknownPackKeys(packFile)
	if err != nil {
		return nil, err
	}
	issues = append(issues, unknownPackIssues...)

	modIssues, err := validateModMetadata(index)
	if err != nil {
		return nil, err
	}
	issues = append(issues, modIssues...)

	depIssues, err := validateDependencyLinks(index)
	if err != nil {
		return nil, err
	}
	issues = append(issues, depIssues...)

	return issues, nil
}

func validateUnknownPackKeys(packFile string) ([]validationIssue, error) {
	var raw map[string]interface{}
	if _, err := toml.DecodeFile(packFile, &raw); err != nil {
		return nil, err
	}

	issues := make([]validationIssue, 0)
	for key := range raw {
		if _, ok := allowedPackKeys[key]; !ok {
			issues = append(issues, validationIssue{
				Key:      "pack.toml:" + key,
				Message:  fmt.Sprintf("unknown pack.toml key %q", key),
				Expected: "only documented pack.toml keys",
				Example:  "run 'packwiz fix --delete' to remove unknown keys",
				Kind:     issueKindDelete,
			})
		}
	}
	return issues, nil
}

func validateModMetadata(index core.Index) ([]validationIssue, error) {
	mods, err := index.LoadAllMods()
	if err != nil {
		return nil, err
	}

	issues := make([]validationIssue, 0)
	for _, mod := range mods {
		pageIssues, err := validateModPageURLsForMod(mod)
		if err != nil {
			return nil, err
		}
		issues = append(issues, pageIssues...)

		if mod.Option == nil {
			issues = append(issues, validationIssue{
				Key:      mod.GetFilePath(),
				Message:  "missing option.dependency in mod metadata",
				Expected: "an [option] table with dependency set",
				Example:  "option = { dependency = false }",
				Kind:     issueKindFix,
			})
		}

		unknownIssues, err := validateUnknownModKeys(mod.GetFilePath())
		if err != nil {
			return nil, err
		}
		issues = append(issues, unknownIssues...)

		obsoleteIssues, err := validateObsoleteModKeys(mod.GetFilePath())
		if err != nil {
			return nil, err
		}
		issues = append(issues, obsoleteIssues...)
	}

	return issues, nil
}

func validateModPageURLsForMod(mod *core.Mod) ([]validationIssue, error) {
	issues := make([]validationIssue, 0)
	if mod.PageURL == "" {
		issues = append(issues, validationIssue{
			Key:      mod.GetFilePath(),
			Message:  "missing page-url in mod metadata",
			Expected: "a project page URL for the mod source",
			Example:  "page-url = \"https://modrinth.com/mod/example\"",
			Kind:     issueKindFix,
		})
	}
	if mod.Version == "" {
		issues = append(issues, validationIssue{
			Key:      mod.GetFilePath(),
			Message:  "missing version in mod metadata",
			Expected: "a human-readable version string from the source project",
			Example:  "version = \"1.2.3\"",
			Kind:     issueKindFix,
		})
	}
	return issues, nil
}

func validateUnknownModKeys(modPath string) ([]validationIssue, error) {
	var raw map[string]interface{}
	if _, err := toml.DecodeFile(modPath, &raw); err != nil {
		return nil, err
	}

	issues := make([]validationIssue, 0)
	for key := range raw {
		if _, ok := allowedModKeys[key]; !ok {
			issues = append(issues, validationIssue{
				Key:      modPath,
				Message:  fmt.Sprintf("unknown mod metadata key %q", key),
				Expected: "only documented .pw.toml keys",
				Example:  "run 'packwiz fix --delete' to remove unknown keys",
				Kind:     issueKindDelete,
			})
		}
	}
	return issues, nil
}

func validateObsoleteModKeys(modPath string) ([]validationIssue, error) {
	var raw map[string]interface{}
	if _, err := toml.DecodeFile(modPath, &raw); err != nil {
		return nil, err
	}

	optionVal, ok := raw["option"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	issues := make([]validationIssue, 0)
	for _, key := range obsoleteModOptionKeys {
		if _, ok := optionVal[key]; ok {
			issues = append(issues, validationIssue{
				Key:      modPath,
				Message:  fmt.Sprintf("obsolete option.%s key", key),
				Expected: "option table with only dependency (and optional description)",
				Example:  "run 'packwiz fix --delete' to remove obsolete keys",
				Kind:     issueKindDelete,
			})
		}
	}
	return issues, nil
}

func validateModPageURLs(index core.Index) ([]validationIssue, error) {
	mods, err := index.LoadAllMods()
	if err != nil {
		return nil, err
	}

	issues := make([]validationIssue, 0)
	for _, mod := range mods {
		modIssues, err := validateModPageURLsForMod(mod)
		if err != nil {
			return nil, err
		}
		issues = append(issues, modIssues...)
	}
	return issues, nil
}

func validateDependencyLinks(index core.Index) ([]validationIssue, error) {
	mods, err := index.LoadAllMods()
	if err != nil {
		return nil, err
	}

	packRoot := filepath.Dir(viper.GetString("pack-file"))
	trackedMods := make(map[string]struct{})
	for _, mod := range mods {
		trackedMods[normalizeDependencyPath(packRoot, mod.GetFilePath())] = struct{}{}
	}

	issues := make([]validationIssue, 0)
	for _, mod := range mods {
		for _, dependencyPath := range mod.Dependencies {
			if dependencyPath == "" {
				issues = append(issues, validationIssue{
					Key:      mod.GetFilePath(),
					Message:  "contains an empty dependency reference",
					Expected: "a path to a tracked .pw.toml file",
					Example:  "run 'packwiz fix --delete' to remove invalid dependency references",
					Kind:     issueKindDelete,
				})
				continue
			}

			resolvedDependencyPath := normalizeDependencyPath(packRoot, dependencyPath)
			if _, err := os.Stat(resolvedDependencyPath); err != nil {
				issues = append(issues, validationIssue{
					Key:      mod.GetFilePath(),
					Message:  fmt.Sprintf("dependency reference %q does not exist", dependencyPath),
					Expected: "an existing tracked .pw.toml file",
					Example:  "run 'packwiz fix --delete' to remove invalid dependency references",
					Kind:     issueKindDelete,
				})
				continue
			}

			if _, ok := trackedMods[resolvedDependencyPath]; !ok {
				issues = append(issues, validationIssue{
					Key:      mod.GetFilePath(),
					Message:  fmt.Sprintf("dependency reference %q is not tracked in the index", dependencyPath),
					Expected: "a .pw.toml file that is present in the index",
					Example:  "run 'packwiz fix --delete' to remove invalid dependency references",
					Kind:     issueKindDelete,
				})
			}
		}
	}

	return issues, nil
}

func printValidationIssues(title string, issues []validationIssue) {
	fmt.Println(title)
	for _, issue := range issues {
		fmt.Printf("- %s: %s\n", issue.Key, issue.Message)
		if issue.Expected != "" {
			fmt.Printf("  Expected: %s\n", issue.Expected)
		}
		if issue.Example != "" {
			fmt.Printf("  Example: %s\n", issue.Example)
		}
	}
}

func normalizeDependencyPath(packRoot, dependencyPath string) string {
	if filepath.IsAbs(dependencyPath) {
		return filepath.Clean(dependencyPath)
	}
	return filepath.Clean(filepath.Join(packRoot, dependencyPath))
}

func printValidationHints(issues []validationIssue) {
	hasFixable := false
	hasDeletable := false
	for _, issue := range issues {
		switch issue.Kind {
		case issueKindFix:
			hasFixable = true
		case issueKindDelete:
			hasDeletable = true
		}
	}
	if hasFixable {
		fmt.Println("Run 'packwiz fix' to add or fill missing metadata where possible.")
	}
	if hasDeletable {
		fmt.Println("Run 'packwiz fix --delete' to remove invalid, obsolete, or unknown keys.")
	}
}
