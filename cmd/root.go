package cmd

import (
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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

	// The pack is always the pack.toml of the current directory; the key is what the rest of packwiz reads it through
	viper.SetDefault("pack-file", "pack.toml")
	viper.SetDefault("color", "auto")
	rootCmd.CompletionOptions.DisableDefaultCmd = true
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
	dir, err := core.GetPackwizLocalStore()
	if err != nil {
		ui.Error.Println(err)
		os.Exit(1)
	}
	viper.AddConfigPath(dir)
	viper.SetConfigName(".packwiz")

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
