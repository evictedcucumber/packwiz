package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestDisableTagRecursesThroughSubcommands(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	grandchild := &cobra.Command{Use: "grandchild"}
	child.AddCommand(grandchild)
	root.AddCommand(child)

	disableTag(root)

	if !root.DisableAutoGenTag {
		t.Error("root.DisableAutoGenTag = false, want true")
	}
	if !child.DisableAutoGenTag {
		t.Error("child.DisableAutoGenTag = false, want true")
	}
	if !grandchild.DisableAutoGenTag {
		t.Error("grandchild.DisableAutoGenTag = false, want true")
	}
}

func TestMarkdownCommandGeneratesDocs(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "docs")

	old := markdownCmd.Flags().Lookup("dir").Value.String()
	if err := markdownCmd.Flags().Set("dir", outDir); err != nil {
		t.Fatalf("failed to set --dir flag: %v", err)
	}
	t.Cleanup(func() { _ = markdownCmd.Flags().Set("dir", old) })

	markdownCmd.Run(markdownCmd, nil)

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("expected --dir to be created and populated: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one generated markdown file, got none")
	}
}
