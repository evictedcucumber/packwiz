package cmd

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/fatih/camelcase"
	"github.com/igorsobreira/titlecase"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise a packwiz modpack",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_, err := os.Stat(viper.GetString("pack-file"))
		if err == nil && !viper.GetBool("init.reinit") {
			fmt.Println("Modpack metadata file already exists, use -r to override!")
			os.Exit(1)
		} else if err != nil && !os.IsNotExist(err) {
			fmt.Printf("Error checking pack file: %s\n", err)
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
				name = titlecase.Title(strings.ReplaceAll(strings.ReplaceAll(strings.Join(camelcase.Split(directoryName), " "), " - ", " "), " _ ", " "))
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
			fmt.Printf("Failed to get latest minecraft versions: %s\n", err)
			os.Exit(1)
		}

		mcVersion := viper.GetString("init.mc-version")
		if len(mcVersion) > 0 && !mcVersions.IsValid(mcVersion) {
			fmt.Println("\"" + mcVersion + "\" is not a valid Minecraft version!")
			if viper.GetBool("non-interactive") {
				os.Exit(1)
			}
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
			fmt.Println("\"" + modLoaderName + "\" is not a supported mod loader! Use \"none\" to specify no modloader, or to configure one manually.")
			fmt.Println("The following mod loaders are supported: " + strings.Join(validLoaderChoices, ", "))
			if viper.GetBool("non-interactive") {
				os.Exit(1)
			}
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
				fmt.Printf("Error loading versions: %s\n", err)
				os.Exit(1)
			}
			// NeoForge reused Forge's version-prefixing format (prefixed with the supported
			// minecraft version), but only during the 1.20.1 days; they've since switched formats.
			resolveVersion := func(v string) string {
				if loader.Name == "neoforge" && mcVersion == "1.20.1" {
					return cmdshared.GetRawForgeVersion(v)
				}
				return v
			}
			isValidComponentVersion := func(v string) bool { return slices.Contains(versionData.Versions, resolveVersion(v)) }

			componentVersion := viper.GetString("init." + loader.Name + "-version")
			if len(componentVersion) > 0 && !isValidComponentVersion(componentVersion) {
				fmt.Println("\"" + componentVersion + "\" is not a valid " + loader.FriendlyName + " version!")
				if viper.GetBool("non-interactive") {
					os.Exit(1)
				}
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

		indexFilePath := viper.GetString("init.index-file")
		_, err = os.Stat(indexFilePath)
		if os.IsNotExist(err) {
			// Create file
			err = os.WriteFile(indexFilePath, []byte{}, 0644)
			if err != nil {
				fmt.Printf("Error creating index file: %s\n", err)
				os.Exit(1)
			}
			fmt.Println(indexFilePath + " created!")
		} else if err != nil {
			fmt.Printf("Error checking index file: %s\n", err)
			os.Exit(1)
		}

		// Create the pack
		pack := core.Pack{
			Name:       name,
			Author:     author,
			Version:    version,
			PackFormat: core.CurrentPackFormat,
			Index: struct {
				File       string `toml:"file"`
				HashFormat string `toml:"hash-format"`
				Hash       string `toml:"hash,omitempty"`
			}{
				File: indexFilePath,
			},
			Versions: map[string]string{
				"minecraft": mcVersion,
			},
		}
		if modLoaderName != "none" {
			for k, v := range modLoaderVersions {
				pack.Versions[k] = v
			}
		}

		// Refresh the index and pack
		index, err := pack.LoadIndex()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		err = index.Refresh()
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
		fmt.Println(viper.GetString("pack-file") + " created!")
	},
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
	fmt.Print(prompt)
	if viper.GetBool("non-interactive") {
		fmt.Printf("%s\n", def)
		return def
	}
	value, err := stdinReader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %s\n", err)
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
// whole init process. In non-interactive mode, initReadValue immediately returns def, so def
// must always be valid.
func initReadValidValue(prompt string, def string, isValid func(string) bool, invalidMsg func(string) string) string {
	for {
		value := initReadValue(prompt, def)
		if isValid(value) {
			return value
		}
		fmt.Println(invalidMsg(value))
	}
}
