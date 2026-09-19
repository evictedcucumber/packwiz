package utils

import (
	"os"

	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/viper"
)

// markdownCmd represents the markdown command
var markdownCmd = &cobra.Command{
	Use:     "markdown",
	Short:   "Generate markdown documentation (that you might be reading right now!!)",
	Aliases: []string{"md"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		outDir := viper.GetString("utils.markdown.dir")
		err := os.MkdirAll(outDir, os.ModePerm)
		if err != nil {
			ui.Error.Printf("Error creating directory: %s\n", err)
			os.Exit(1)
		}
		disableTag(cmd.Root())
		err = doc.GenMarkdownTree(cmd.Root(), outDir)
		if err != nil {
			ui.Error.Printf("Error generating markdown: %s\n", err)
			os.Exit(1)
		}
		ui.Success.Println("Generated markdown successfully!")
	},
}

func disableTag(cmd *cobra.Command) {
	cmd.DisableAutoGenTag = true
	for _, v := range cmd.Commands() {
		disableTag(v)
	}
}

func init() {
	utilsCmd.AddCommand(markdownCmd)

	markdownCmd.Flags().String("dir", ".", "The destination directory to save docs in")
	_ = viper.BindPFlag("utils.markdown.dir", markdownCmd.Flags().Lookup("dir"))
}
