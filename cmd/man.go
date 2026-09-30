package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// manCmd writes the manual pages, which packaging (see nix/default.nix) installs. It is hidden as it is for building
// packwiz, not for using it.
var manCmd = &cobra.Command{
	Use:    "man <directory>",
	Short:  "Generate manual pages for packwiz",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := writeManPages(cmd.Root(), args[0]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	},
}

// writeManPages writes a page for the root command and each of its subcommands (packwiz.1, packwiz-mr-add.1, ...) into
// dir, creating it if needed. The date is fixed to SOURCE_DATE_EPOCH when set, so builds are reproducible.
func writeManPages(root *cobra.Command, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	date := time.Now()
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		var secs int64
		if _, err := fmt.Sscan(epoch, &secs); err == nil {
			date = time.Unix(secs, 0)
		}
	}
	header := &doc.GenManHeader{Title: "PACKWIZ", Section: "1", Date: &date, Source: "packwiz", Manual: "packwiz Manual"}
	return doc.GenManTree(root, header, dir)
}

func init() {
	rootCmd.AddCommand(manCmd)
}
