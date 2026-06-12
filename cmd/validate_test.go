package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/viper"
)

func TestValidateDependencyLinksPassesForTrackedDependencies(t *testing.T) {
	tempDir := t.TempDir()
	packFile := filepath.Join(tempDir, "pack.toml")
	indexFile := filepath.Join(tempDir, "index.toml")

	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	packData := "pack-format = \"packwiz:1.1.0\"\nname = \"Test Pack\"\nloader = \"modrinth\"\nmodlist = false\n\n[index]\nfile = \"index.toml\"\n\n[versions]\nminecraft = \"1.21.0\"\n"
	if err := os.WriteFile(packFile, []byte(packData), 0o644); err != nil {
		t.Fatal(err)
	}

	indexData := "hash-format = \"sha256\"\n\n[[files]]\nfile = \"mods/root.pw.toml\"\nmetafile = true\n\n[[files]]\nfile = \"mods/dep.pw.toml\"\nmetafile = true\n"
	if err := os.WriteFile(indexFile, []byte(indexData), 0o644); err != nil {
		t.Fatal(err)
	}

	rootMod := "name = \"Root\"\nfilename = \"root.jar\"\nversion = \"1.0.0\"\npage-url = \"https://example.com/root\"\ndependencies = [\"mods/dep.pw.toml\"]\n"
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "root.pw.toml"), []byte(rootMod), 0o644); err != nil {
		t.Fatal(err)
	}

	depMod := "name = \"Dependency\"\nfilename = \"dep.jar\"\nversion = \"1.0.0\"\npage-url = \"https://example.com/dep\"\n"
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "dep.pw.toml"), []byte(depMod), 0o644); err != nil {
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

	issues, err := validateDependencyLinks(index)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) > 0 {
		t.Fatalf("expected no dependency issues, got %#v", issues)
	}
}

func TestValidateDependencyLinksFailsForMissingDependency(t *testing.T) {
	tempDir := t.TempDir()
	packFile := filepath.Join(tempDir, "pack.toml")
	indexFile := filepath.Join(tempDir, "index.toml")

	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	packData := "pack-format = \"packwiz:1.1.0\"\nname = \"Test Pack\"\nloader = \"modrinth\"\nmodlist = false\n\n[index]\nfile = \"index.toml\"\n\n[versions]\nminecraft = \"1.21.0\"\n"
	if err := os.WriteFile(packFile, []byte(packData), 0o644); err != nil {
		t.Fatal(err)
	}

	indexData := "hash-format = \"sha256\"\n\n[[files]]\nfile = \"mods/root.pw.toml\"\nmetafile = true\n"
	if err := os.WriteFile(indexFile, []byte(indexData), 0o644); err != nil {
		t.Fatal(err)
	}

	rootMod := "name = \"Root\"\nfilename = \"root.jar\"\nversion = \"1.0.0\"\npage-url = \"https://example.com/root\"\ndependencies = [\"mods/missing.pw.toml\"]\n"
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "root.pw.toml"), []byte(rootMod), 0o644); err != nil {
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

	issues, err := validateDependencyLinks(index)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected dependency validation to report a missing link")
	}
}
