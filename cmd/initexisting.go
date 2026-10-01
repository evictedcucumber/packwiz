package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/viper"
)

// InitCheck is what "packwiz init" finds when it is run where a pack already is: it asks nothing and changes nothing
// the person chose, only checking the files that make a pack (pack.toml and its index) and making any that are missing.
type InitCheck struct {
	// Created lists the files that were missing and have been made
	Created []string
	// Errors are what is wrong with a file and can't be fixed without a decision, so the pack can't be used as it is
	Errors []string
	// Warnings are problems that don't stop the pack from being used
	Warnings []string
}

// CheckExistingPack checks the pack in the current directory and creates the index file it names if that is missing,
// as "packwiz init" does when there is a pack already. It never changes what pack.toml says, except to record the hash of
// an index it made, and it doesn't look at the mods. With quiet, nothing is printed, not even the progress of the refresh
// of an index that is made. The error is for a failure to read or write, where Errors are for what is wrong with the
// files.
func CheckExistingPack(quiet bool) (InitCheck, error) {
	var result InitCheck
	packFile := viper.GetString("pack-file")
	data, err := os.ReadFile(packFile)
	if err != nil {
		return result, err
	}
	pack, err := core.ParsePack(data)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s can't be read: %s (use -r to create it again)", packFile, err))
		return result, nil
	}

	if pack.PackFormat != core.CurrentPackFormat {
		result.Errors = append(result.Errors, fmt.Sprintf("%s has pack-format %q, which is not supported; a pack must be created with this fork of packwiz (expected %q)", packFile, pack.PackFormat, core.CurrentPackFormat))
	}
	if _, err := pack.GetMCVersion(); err != nil {
		result.Errors = append(result.Errors, packFile+" has no Minecraft version (versions.minecraft)")
	}
	if pack.Name == "" {
		result.Warnings = append(result.Warnings, packFile+" has no name (name)")
	}
	for owner := range pack.ConfigFiles {
		if !pack.IsConfigOwner(owner) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s has config files for %q, which is neither the pack nor a mod loader it has", packFile, owner))
		}
	}

	indexPath := pack.Index.File
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(filepath.Dir(packFile), filepath.FromSlash(indexPath))
	}
	_, err = os.Stat(indexPath)
	switch {
	case os.IsNotExist(err):
		if len(result.Errors) > 0 {
			// Not worth making an index for a pack that can't be used
			result.Errors = append(result.Errors, indexPath+" is missing")
			return result, nil
		}
		if err := createIndex(&pack, indexPath, quiet); err != nil {
			return result, err
		}
		result.Created = append(result.Created, indexPath)
	case err != nil:
		return result, fmt.Errorf("Error checking index file: %s", err)
	default:
		if _, err := pack.LoadIndex(); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s can't be read: %s", indexPath, err))
			break
		}
		if pack.Index.Hash != "" {
			current := pack
			if err := current.UpdateIndexHash(); err != nil {
				return result, err
			}
			if current.Index.Hash != pack.Index.Hash {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s doesn't match the hash %s records for it; run packwiz refresh", indexPath, packFile))
			}
		}
	}
	return result, nil
}

// createIndex makes the index file a pack names and fills it with the files the pack has, then records its hash in the pack.
func createIndex(pack *core.Pack, indexPath string, quiet bool) error {
	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		return fmt.Errorf("Error creating index file: %s", err)
	}
	if err := os.WriteFile(indexPath, []byte{}, 0644); err != nil {
		return fmt.Errorf("Error creating index file: %s", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return err
	}
	if quiet {
		_, err = index.RefreshQuietly()
	} else {
		err = index.Refresh()
	}
	if err != nil {
		return err
	}
	return pack.SaveIndex(index)
}

// reportExistingPack is "packwiz init" where a pack is already: it checks the pack's files with CheckExistingPack, says
// what it created and what is wrong, and reports whether the pack is fine (warnings are not a failure).
func reportExistingPack() bool {
	result, err := CheckExistingPack(false)
	for _, file := range result.Created {
		ui.Success.Println(file + " created!")
	}
	for _, warning := range result.Warnings {
		ui.Warning.Println(warning)
	}
	for _, problem := range result.Errors {
		ui.Error.Println(problem)
	}
	if err != nil {
		ui.Error.Println(err)
		return false
	}
	if len(result.Errors) > 0 {
		return false
	}
	if len(result.Created) == 0 && len(result.Warnings) == 0 {
		ui.Success.Println(viper.GetString("pack-file") + " is valid, nothing to create")
	}
	return true
}
