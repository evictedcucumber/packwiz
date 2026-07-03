package modrinth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/dixonwille/wmenu.v4"
)

var selectVersionCmd = &cobra.Command{
	Use:   "select-version <mod.pw.toml>",
	Short: "Interactively select a Modrinth version for a tracked mod file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
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

		if viper.GetBool("non-interactive") {
			fmt.Println("select-version cannot be used in non-interactive mode")
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

		rawData, ok := modData.GetParsedUpdateData("modrinth")
		if !ok {
			fmt.Println("This mod does not use the Modrinth updater")
			os.Exit(1)
		}
		data := rawData.(mrUpdateData)
		if data.ProjectID == "" {
			fmt.Println("This mod does not have a Modrinth project ID")
			os.Exit(1)
		}

		allowedChannel := pack.GetAllowedChannel(&modData)
		gameVersions, err := pack.GetSupportedMCVersions()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		var loaders []string
		if viper.GetString("datapack-folder") != "" {
			loaders = append(pack.GetCompatibleLoaders(), withDatapackPathMRLoaders...)
		} else {
			loaders = append(pack.GetCompatibleLoaders(), defaultMRLoaders...)
		}

		versions, err := getAllowedVersions(data.ProjectID, gameVersions, loaders, allowedChannel)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		currentVersionIndex := 0
		for i, version := range versions {
			if version.ID != nil && *version.ID == data.InstalledVersion {
				currentVersionIndex = i
				break
			}
		}

		menu := wmenu.NewMenu("Choose a version:")
		menu.Option("Cancel", nil, false, nil)
		for i, version := range versions {
			menu.Option(formatVersionSelectionLabel(version, data.InstalledVersion), version, i == currentVersionIndex, nil)
		}

		var selectedVersion *Version
		var cancelled bool
		menu.Action(func(menuRes []wmenu.Opt) error {
			if len(menuRes) != 1 || menuRes[0].Value == nil {
				cancelled = true
				return nil
			}

			version, ok := menuRes[0].Value.(*Version)
			if !ok {
				return errors.New("error converting interface from wmenu")
			}
			selectedVersion = version
			return nil
		})

		if err := menu.Run(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if cancelled {
			fmt.Println("Cancelled!")
			return
		}
		if selectedVersion == nil {
			fmt.Println("No version selected")
			os.Exit(1)
		}
		if len(selectedVersion.Files) == 0 {
			fmt.Println("Selected version doesn't have any files attached")
			os.Exit(1)
		}

		updater := mrUpdater{}
		err = updater.DoUpdate([]*core.Mod{&modData}, []interface{}{cachedStateStore{ProjectID: data.ProjectID, Version: selectedVersion}})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

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

		reloadedMod, err := core.LoadMod(modPath)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		err = CheckAndInstallDependencies([]*core.Mod{&reloadedMod}, pack, &index)
		if err != nil {
			fmt.Printf("Failed to check dependencies for %s: %v\n", modData.Name, err)
		}

		err = writePackAndIndex(&pack, &index, core.SyncDepsOpts{NormalizeAll: true, RefreshAll: true})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Printf("\"%s\" updated to %s!\n", modData.Name, getModrinthVersionLabel(selectedVersion))
	},
}

func formatVersionSelectionLabel(version *Version, currentVersionID string) string {
	if version == nil {
		return ""
	}

	label := getModrinthVersionLabel(version)
	parts := []string{label}
	if version.ID != nil && *version.ID == currentVersionID {
		parts[0] += " (current)"
	}
	if version.VersionType != nil && *version.VersionType != "" {
		parts = append(parts, strings.ToLower(*version.VersionType))
	}
	if len(version.GameVersions) > 0 {
		parts = append(parts, "mc "+strings.Join(version.GameVersions, ", "))
	}
	if len(version.Loaders) > 0 {
		parts = append(parts, strings.Join(version.Loaders, ", "))
	}
	if version.DatePublished != nil {
		parts = append(parts, version.DatePublished.Format("2006-01-02"))
	}
	return strings.Join(parts, " | ")
}

func resolveTrackedModMetaPath(index core.Index, input string) (string, error) {
	if stat, err := os.Stat(input); err == nil && !stat.IsDir() {
		absPath, err := filepath.Abs(input)
		if err != nil {
			return "", err
		}
		relPath, err := index.RelIndexPath(absPath)
		if err != nil {
			return "", err
		}
		if file, ok := index.Files[relPath]; ok && file.IsMetaFile() {
			return absPath, nil
		}
		return "", fmt.Errorf("%q is not a tracked metadata file in index.toml", input)
	}

	relInput := filepath.ToSlash(filepath.Clean(input))
	if file, ok := index.Files[relInput]; ok && file.IsMetaFile() {
		return index.ResolveIndexPath(relInput), nil
	}

	return "", fmt.Errorf("pin only accepts a tracked .pw.toml file, not %q", input)
}

func writePackAndIndex(pack *core.Pack, index *core.Index, opts core.SyncDepsOpts) error {
	if err := index.SyncDependencyMetadata(*pack, opts); err != nil {
		return err
	}
	if err := index.Write(); err != nil {
		return err
	}
	if err := pack.UpdateIndexHash(); err != nil {
		return err
	}
	return pack.Write()
}

func init() {
	modrinthCmd.AddCommand(selectVersionCmd)
}