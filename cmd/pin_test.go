package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestResolveTrackedModMetaPathAcceptsTrackedMetaFile(t *testing.T) {
	tempDir := t.TempDir()
	indexFile := filepath.Join(tempDir, "index.toml")
	modsDir := filepath.Join(tempDir, "mods")

	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexFile, []byte("hash-format = \"sha256\"\n\n[[files]]\nfile = \"mods/example.pw.toml\"\nmetafile = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(modsDir, "example.pw.toml")
	if err := os.WriteFile(metaPath, []byte("name = \"Example\"\nfilename = \"example.jar\"\nversion = \"1.0.0\"\npage-url = \"https://example.com/example\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	index, err := core.LoadIndex(indexFile)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveTrackedModMetaPath(index, metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "mods/example.pw.toml" {
		t.Fatalf("expected %q, got %q", "mods/example.pw.toml", resolved)
	}
}

func TestResolveTrackedModMetaPathRejectsSlug(t *testing.T) {
	tempDir := t.TempDir()
	indexFile := filepath.Join(tempDir, "index.toml")

	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexFile, []byte("hash-format = \"sha256\"\n\n[[files]]\nfile = \"mods/example.pw.toml\"\nmetafile = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "example.pw.toml"), []byte("name = \"Example\"\nfilename = \"example.jar\"\nversion = \"1.0.0\"\npage-url = \"https://example.com/example\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	index, err := core.LoadIndex(indexFile)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolveTrackedModMetaPath(index, "example"); err == nil {
		t.Fatal("expected slug input to be rejected")
	}
}
