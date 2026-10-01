package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/fatih/camelcase"
	"github.com/igorsobreira/titlecase"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise a packwiz modpack",
	Long: `Creates pack.toml and the index for a new pack in the current directory, asking for what it needs.

It also writes a ` + core.FileCategoriesFile + ` with the categories that "packwiz git commit" sorts files that packwiz
doesn't track into (flake.nix as a dev file, README.md as docs, and so on), unless there is one already. Change it to
suit the pack.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_, err := os.Stat(viper.GetString("pack-file"))
		if err == nil && !viper.GetBool("init.reinit") {
			ui.Error.Println("Modpack metadata file already exists, use -r to override!")
			os.Exit(1)
		} else if err != nil && !os.IsNotExist(err) {
			ui.Error.Printf("Error checking pack file: %s\n", err)
			os.Exit(1)
		}

		name, err := cmd.Flags().GetString("name")
		if err != nil || len(name) == 0 {
			// Get current file directory name
			wd, err := os.Getwd()
			directoryName := "."
			if err == nil {
				directoryName = filepath.Base(wd)
			}
			if directoryName != "." && len(directoryName) > 0 {
				// Turn directory name into a space-seperated proper name
				name = packNameFromDirectory(directoryName)
				name = initReadValue("Modpack name ["+name+"]: ", name)
			} else {
				name = initReadValue("Modpack name: ", "")
			}
		}

		author, err := cmd.Flags().GetString("author")
		if err != nil || len(author) == 0 {
			author = initReadValue("Author: ", "")
		}

		version, err := cmd.Flags().GetString("version")
		if err != nil || len(version) == 0 {
			version = initReadValue("Version [1.0.0]: ", "1.0.0")
		}

		mcVersions, err := cmdshared.GetValidMCVersions()
		if err != nil {
			ui.Error.Printf("Failed to get latest minecraft versions: %s\n", err)
			os.Exit(1)
		}

		mcVersion := viper.GetString("init.mc-version")
		if len(mcVersion) > 0 && !mcVersions.IsValid(mcVersion) {
			ui.Error.Println("\"" + mcVersion + "\" is not a valid Minecraft version!")
			mcVersion = ""
		}
		if len(mcVersion) == 0 {
			var latestVersion string
			if viper.GetBool("init.snapshot") {
				latestVersion = mcVersions.Latest.Snapshot
			} else {
				latestVersion = mcVersions.Latest.Release
			}
			if viper.GetBool("init.latest") {
				mcVersion = latestVersion
			} else {
				mcVersion = initReadValidValue(
					"Minecraft version ["+latestVersion+"]: ", latestVersion,
					mcVersions.IsValid,
					func(v string) string { return "\"" + v + "\" is not a valid Minecraft version, please try again." },
				)
			}
		}

		loaderNames := slices.Collect(maps.Keys(core.ModLoaders))
		slices.Sort(loaderNames)
		validLoaderChoices := append([]string{"none"}, loaderNames...)
		isValidLoaderChoice := func(name string) bool { return slices.Contains(validLoaderChoices, name) }

		modLoaderName := strings.ToLower(viper.GetString("init.modloader"))
		if len(modLoaderName) > 0 && !isValidLoaderChoice(modLoaderName) {
			ui.Error.Println("\"" + modLoaderName + "\" is not a supported mod loader! Use \"none\" to specify no modloader, or to configure one manually.")
			ui.Info.Println("The following mod loaders are supported: " + strings.Join(validLoaderChoices, ", "))
			modLoaderName = ""
		}
		if len(modLoaderName) == 0 {
			modLoaderName = strings.ToLower(initReadValidValue(
				"Mod loader [neoforge] ("+strings.Join(validLoaderChoices, ", ")+"): ", "neoforge",
				isValidLoaderChoice,
				func(v string) string {
					return "\"" + v + "\" is not a supported mod loader, please try again. Supported mod loaders: " + strings.Join(validLoaderChoices, ", ")
				},
			))
		}

		loader, ok := core.ModLoaders[modLoaderName]
		modLoaderVersions := make(map[string]string)
		if modLoaderName != "none" && ok {
			versionData, err := core.DoQuery(core.MakeQuery(loader, mcVersion))
			if err != nil {
				ui.Error.Printf("Error loading versions: %s\n", err)
				os.Exit(1)
			}
			resolveVersion := func(v string) string { return LoaderVersion(loader, mcVersion, v) }
			isValidComponentVersion := func(v string) bool { return slices.Contains(versionData.Versions, resolveVersion(v)) }

			componentVersion := viper.GetString("init." + loader.Name + "-version")
			if len(componentVersion) > 0 && !isValidComponentVersion(componentVersion) {
				ui.Error.Println("\"" + componentVersion + "\" is not a valid " + loader.FriendlyName + " version!")
				componentVersion = ""
			}
			if len(componentVersion) == 0 {
				if viper.GetBool("init." + loader.Name + "-latest") {
					componentVersion = versionData.Latest
				} else {
					componentVersion = initReadValidValue(
						loader.FriendlyName+" version ["+versionData.Latest+"]: ", versionData.Latest,
						isValidComponentVersion,
						func(v string) string {
							return "\"" + v + "\" is not a valid " + loader.FriendlyName + " version, please try again."
						},
					)
				}
			}
			modLoaderVersions[loader.Name] = resolveVersion(componentVersion)
		}

		_, err = createPack(NewPack{
			Name: name, Author: author, Version: version, MCVersion: mcVersion,
			Loader: modLoaderName, LoaderVersion: modLoaderVersions[modLoaderName],
			IndexFile: viper.GetString("init.index-file"),
		}, false, func(indexFile string) { ui.Success.Println(indexFile + " created!") })
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		ui.Success.Println(viper.GetString("pack-file") + " created!")
	},
}

// packNameFromDirectory turns the name of a folder into what a pack in it could be called: "my-cool_pack" is "My Cool Pack".
func packNameFromDirectory(directory string) string {
	return titlecase.Title(strings.ReplaceAll(strings.ReplaceAll(strings.Join(camelcase.Split(directory), " "), " - ", " "), " _ ", " "))
}

// DefaultPackName is what a pack in the current directory is called if it isn't told otherwise, going by the name of the
// folder, or "" if that isn't known.
func DefaultPackName() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	directory := filepath.Base(wd)
	if directory == "." || directory == "" || directory == string(filepath.Separator) {
		return ""
	}
	return packNameFromDirectory(directory)
}

// LoaderVersion is the version of a mod loader as a pack records it, given the one that was chosen for a Minecraft
// version. NeoForge reused Forge's version-prefixing format (prefixed with the supported Minecraft version), but only
// during the 1.20.1 days; they've since switched formats.
func LoaderVersion(loader core.ModLoaderComponent, mcVersion, chosen string) string {
	if loader.Name == "neoforge" && mcVersion == "1.20.1" {
		return cmdshared.GetRawForgeVersion(chosen)
	}
	return chosen
}

// LoaderVersions lists the versions of a mod loader that are for a Minecraft version, and which of them is the latest. It
// asks the network. loader is one of the names in core.ModLoaders.
func LoaderVersions(loader, mcVersion string) (*core.ModLoaderVersions, error) {
	component, ok := core.ModLoaders[loader]
	if !ok {
		return nil, fmt.Errorf("%q is not a supported mod loader", loader)
	}
	return core.DoQuery(core.MakeQuery(component, mcVersion))
}

// NewPack is what a pack is created with.
type NewPack struct {
	Name, Author, Version string
	// MCVersion is the Minecraft version, which isn't checked here
	MCVersion string
	// Loader is the name of the mod loader, which is one of the names in core.ModLoaders, or "none" or empty for no mod
	// loader, and LoaderVersion the version of it
	Loader, LoaderVersion string
	// IndexFile is where the index is, relative to pack.toml
	IndexFile string
}

// CreatePack creates a pack in the current directory: pack.toml, and the index of the files it has, as "packwiz init" does
// without asking anything. It says nothing on the terminal. It won't replace a pack that is there.
func CreatePack(pack NewPack) error {
	if _, err := os.Stat(viper.GetString("pack-file")); err == nil {
		return errors.New("Modpack metadata file already exists")
	}
	_, err := createPack(pack, true, nil)
	return err
}

// createPack writes the index file if it isn't there, then the pack, and refreshes the index to have the files the pack
// has. indexCreated, if it isn't nil, is told when the index file is made, which is before the rest. With quiet, nothing is
// printed, not even the progress of the refresh.
func createPack(np NewPack, quiet bool, indexCreated func(indexFile string)) (created bool, err error) {
	indexFilePath := np.IndexFile
	if indexFilePath == "" {
		indexFilePath = "index.toml"
	}
	_, err = os.Stat(indexFilePath)
	if os.IsNotExist(err) {
		// Create file
		err = os.WriteFile(indexFilePath, []byte{}, 0644)
		if err != nil {
			return false, fmt.Errorf("Error creating index file: %s", err)
		}
		created = true
		if indexCreated != nil {
			indexCreated(indexFilePath)
		}
	} else if err != nil {
		return false, fmt.Errorf("Error checking index file: %s", err)
	}

	// Sorting the files alongside the pack is left to whoever has one already
	categoriesPath := filepath.Join(filepath.Dir(viper.GetString("pack-file")), core.FileCategoriesFile)
	if _, err := os.Stat(categoriesPath); os.IsNotExist(err) {
		if err := os.WriteFile(categoriesPath, []byte(core.DefaultFileCategories), 0o644); err != nil {
			return false, fmt.Errorf("Error creating %s: %s", core.FileCategoriesFile, err)
		}
	}

	// Create the pack
	pack := core.Pack{
		Name:       np.Name,
		Author:     np.Author,
		Version:    np.Version,
		PackFormat: core.CurrentPackFormat,
		Index: struct {
			File       string `toml:"file"`
			HashFormat string `toml:"hash-format"`
			Hash       string `toml:"hash,omitempty"`
		}{
			File: indexFilePath,
		},
		Versions: map[string]string{
			"minecraft": np.MCVersion,
		},
	}
	if np.Loader != "" && np.Loader != "none" {
		pack.Versions[np.Loader] = np.LoaderVersion
	}

	// Refresh the index and pack
	index, err := pack.LoadIndex()
	if err != nil {
		return created, err
	}
	if quiet {
		_, err = index.RefreshQuietly()
	} else {
		err = index.Refresh()
	}
	if err != nil {
		return created, err
	}
	return created, pack.SaveIndex(index)
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().String("name", "", "The name of the modpack (omit to define interactively)")
	initCmd.Flags().String("author", "", "The author of the modpack (omit to define interactively)")
	initCmd.Flags().String("version", "", "The version of the modpack (omit to define interactively)")
	initCmd.Flags().String("index-file", "index.toml", "The index file to use")
	_ = viper.BindPFlag("init.index-file", initCmd.Flags().Lookup("index-file"))
	initCmd.Flags().String("mc-version", "", "The Minecraft version to use (omit to define interactively)")
	_ = viper.BindPFlag("init.mc-version", initCmd.Flags().Lookup("mc-version"))
	initCmd.Flags().BoolP("latest", "l", false, "Automatically select the latest version of Minecraft")
	_ = viper.BindPFlag("init.latest", initCmd.Flags().Lookup("latest"))
	initCmd.Flags().BoolP("snapshot", "s", false, "Use the latest snapshot version with --latest")
	_ = viper.BindPFlag("init.snapshot", initCmd.Flags().Lookup("snapshot"))
	initCmd.Flags().BoolP("reinit", "r", false, "Recreate the pack file if it already exists, rather than exiting")
	_ = viper.BindPFlag("init.reinit", initCmd.Flags().Lookup("reinit"))
	initCmd.Flags().String("modloader", "", "The mod loader to use (omit to define interactively)")
	_ = viper.BindPFlag("init.modloader", initCmd.Flags().Lookup("modloader"))

	// ok this is epic
	for _, loader := range core.ModLoaders {
		initCmd.Flags().String(loader.Name+"-version", "", "The "+loader.FriendlyName+" version to use (omit to define interactively)")
		_ = viper.BindPFlag("init."+loader.Name+"-version", initCmd.Flags().Lookup(loader.Name+"-version"))
		initCmd.Flags().Bool(loader.Name+"-latest", false, "Automatically select the latest version of "+loader.FriendlyName)
		_ = viper.BindPFlag("init."+loader.Name+"-latest", initCmd.Flags().Lookup(loader.Name+"-latest"))
	}
}

// stdinReader is shared across all initReadValue calls. A fresh bufio.Reader per call would
// read ahead into its own internal buffer and discard whatever it didn't consume as a line,
// silently dropping already-typed or piped-in answers to later prompts.
var stdinReader = bufio.NewReader(os.Stdin)

func initReadValue(prompt string, def string) string {
	fmt.Print(ui.Prompt(prompt))
	value, err := stdinReader.ReadString('\n')
	if err != nil {
		ui.Error.Printf("Error reading input: %s\n", err)
		os.Exit(1)
	}
	// Trims both CR and LF
	value = strings.TrimSpace(strings.TrimRight(value, "\r\n"))
	if len(value) > 0 {
		return value
	}
	return def
}

// initReadValidValue repeatedly prompts until isValid accepts the entered value, printing
// invalidMsg's result in between attempts. This keeps a single bad answer from aborting the
// whole init process.
func initReadValidValue(prompt string, def string, isValid func(string) bool, invalidMsg func(string) string) string {
	for {
		value := initReadValue(prompt, def)
		if isValid(value) {
			return value
		}
		ui.Error.Println(invalidMsg(value))
	}
}
