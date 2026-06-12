package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/viper"
)

func TestValidateDetectsObsoleteOptionKeys(t *testing.T) {
	tempDir := t.TempDir()
	metaPath := filepath.Join(tempDir, "mods", "example.pw.toml")
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, []byte(`
name = "Example"
filename = "example.jar"
version = "1.0.0"
page-url = "https://example.com/example"

[option]
dependency = false
optional = true
`), 0o644); err != nil {
		t.Fatal(err)
	}

	issues, err := validateObsoleteModKeys(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Kind != issueKindDelete {
		t.Fatalf("expected one delete issue, got %#v", issues)
	}
}

func TestCollectValidationIssuesReportsDeleteIssuesForBrokenDependencies(t *testing.T) {
	tempDir := t.TempDir()
	packFile := filepath.Join(tempDir, "pack.toml")
	indexFile := filepath.Join(tempDir, "index.toml")

	packData := "pack-format = \"packwiz:1.1.0\"\nname = \"Test Pack\"\nloader = \"modrinth\"\nmodlist = false\n\n[index]\nfile = \"index.toml\"\n\n[versions]\nminecraft = \"1.21.0\"\n"
	if err := os.WriteFile(packFile, []byte(packData), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexFile, []byte("hash-format = \"sha256\"\n\n[[files]]\nfile = \"mods/root.pw.toml\"\nmetafile = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "root.pw.toml"), []byte(`
name = "Root"
filename = "root.jar"
version = "1.0.0"
page-url = "https://example.com/root"
dependencies = ["mods/missing.pw.toml"]

[option]
dependency = false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	oldPackFile := viper.GetString("pack-file")
	viper.Set("pack-file", packFile)
	t.Cleanup(func() {
		viper.Set("pack-file", oldPackFile)
	})

	index, err := core.LoadIndex(indexFile)
	if err != nil {
		t.Fatal(err)
	}

	issues, err := collectValidationIssues(packFile, index)
	if err != nil {
		t.Fatal(err)
	}

	foundDeleteIssue := false
	for _, issue := range issues {
		if issue.Kind == issueKindDelete {
			foundDeleteIssue = true
			break
		}
	}
	if !foundDeleteIssue {
		t.Fatalf("expected delete validation issue, got %#v", issues)
	}
}
