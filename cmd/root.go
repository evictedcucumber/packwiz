package cmd

import (
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/pflag"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var packFile string
var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "packwiz",
	Short: "A command line tool for creating Minecraft modpacks",
}

// Execute starts the root command for packwiz
func Execute() {
	// Cobra only runs initConfig for a command that runs, and not to print help or to say a command isn't known, which
	// have to know what the environment and the config file say about colour too
	loadConfig()
	applyColorIfSet()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// Add adds a new command as a subcommand to packwiz
func Add(newCommand *cobra.Command) {
	rootCmd.AddCommand(newCommand)
}

func init() {
	cobra.OnInitialize(initConfig)

	// Help and usage are coloured for the stream they are written to, and so is what cobra says of an error (see help.go)
	rootCmd.SetUsageFunc(usageFunc)
	rootCmd.SetHelpFunc(helpFunc)
	rootCmd.SetErrPrefix(errPrefix())
	// A flag that can't be parsed doesn't get as far as initConfig either, but --color may have been before it
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		applyColorAfterFlags()
		return err
	})

	rootCmd.PersistentFlags().StringVar(&packFile, "pack-file", "pack.toml", "The modpack metadata file to use")
	_ = viper.BindPFlag("pack-file", rootCmd.PersistentFlags().Lookup("pack-file"))

	// Make mods-folder an alias for meta-folder, and colour one for color
	viper.RegisterAlias("mods-folder", "meta-folder")
	rootCmd.SetGlobalNormalizationFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		switch name {
		case "mods-folder":
			return "meta-folder"
		case "colour":
			return "color"
		}
		return pflag.NormalizedName(name)
	})

	var metaFolder string
	rootCmd.PersistentFlags().StringVar(&metaFolder, "meta-folder", "", "The folder in which new metadata files will be added, defaulting to a folder based on the category (mods, resourcepacks, etc; if the category is unknown the current directory is used)")
	_ = viper.BindPFlag("meta-folder", rootCmd.PersistentFlags().Lookup("meta-folder"))

	var metaFolderBase string
	rootCmd.PersistentFlags().StringVar(&metaFolderBase, "meta-folder-base", ".", "The base folder from which meta-folder will be resolved, defaulting to the current directory (so you can put all mods/etc in a subfolder while still using the default behaviour)")
	_ = viper.BindPFlag("meta-folder-base", rootCmd.PersistentFlags().Lookup("meta-folder-base"))

	defaultCacheDir, err := core.GetPackwizCache()
	if err != nil {
		ui.Error.Println(err)
		os.Exit(1)
	}
	rootCmd.PersistentFlags().String("cache", defaultCacheDir, "The directory where packwiz will cache downloaded mods")
	_ = viper.BindPFlag("cache.directory", rootCmd.PersistentFlags().Lookup("cache"))

	file, err := core.GetPackwizLocalStore()
	if err != nil {
		ui.Error.Println(err)
		os.Exit(1)
	}
	file = filepath.Join(file, ".packwiz.toml")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "The config file to use (default \""+file+"\")")

	var nonInteractive bool
	rootCmd.PersistentFlags().BoolVarP(&nonInteractive, "yes", "y", false, "Accept all prompts with the default or \"yes\" option (non-interactive mode) - may pick unwanted options in search results")
	_ = viper.BindPFlag("non-interactive", rootCmd.PersistentFlags().Lookup("yes"))

	rootCmd.PersistentFlags().String("color", "auto", "When to colour output: auto (when writing to a terminal, unless NO_COLOR is set), always or never")
	_ = viper.BindPFlag("color", rootCmd.PersistentFlags().Lookup("color"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	configFile := loadConfig()

	// The config file can set the color option too, so it is applied once that is read
	if err := applyColor(); err != nil {
		ui.Error.Println(err)
		os.Exit(1)
	}
	if configFile != "" {
		ui.Muted.Println("Using config file:", configFile)
	}
}

// loadConfig reads in config file and ENV variables if set, and returns the config file that was found, if there is one.
func loadConfig() string {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		dir, err := core.GetPackwizLocalStore()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		viper.AddConfigPath(dir)
		viper.SetConfigName(".packwiz")
	}

	// Read in environment variables that match
	viper.SetEnvPrefix("packwiz")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		return ""
	}
	return viper.ConfigFileUsed()
}

// applyColorIfSet sets when output is coloured if the flag, the environment or the config file says, without saying
// anything if what they say is no mode: initConfig does that for a command that runs. It is for what is printed without
// one (help, and errors in what a command was given).
func applyColorIfSet() {
	if viper.IsSet("color") {
		_ = applyColor()
	}
}

// applyColorAfterFlags is applyColorIfSet for once the flags have been parsed, when --config may have named a config
// file other than the one Execute has read.
func applyColorAfterFlags() {
	if cfgFile != "" {
		loadConfig()
	}
	applyColorIfSet()
}

// applyColor sets when output is coloured, as the color option says.
func applyColor() error {
	mode, err := ui.ParseMode(viper.GetString("color"))
	if err != nil {
		return err
	}
	ui.SetMode(mode)
	// What cobra prefixes its errors with is written once, so it has to be written again for the mode
	rootCmd.SetErrPrefix(errPrefix())
	return nil
}
