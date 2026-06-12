package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixModMetadataAddsOptionDependencyFalse(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	metaPath := filepath.Join(tempDir, "mods", "example.pw.toml")
	if err := os.WriteFile(metaPath, []byte(`
name = "Example"
filename = "example.jar"
version = "1.0.0"
page-url = "https://example.com/example"

[download]
url = "https://example.com/example.jar"
hash-format = "sha256"
hash = "abc"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/example.pw.toml": &indexFile{File: "mods/example.pw.toml", MetaFile: true},
		},
	}

	fixed, err := index.FixModMetadata(Pack{}, FixModMetadataOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if fixed != 1 {
		t.Fatalf("expected 1 fixed mod, got %d", fixed)
	}

	contents, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(contents)
	if !strings.Contains(body, "[option]") || !strings.Contains(body, "dependency = false") {
		t.Fatalf("expected explicit option.dependency = false, got:\n%s", body)
	}
}

func TestFixModMetadataDeleteRemovesInvalidDependency(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	metaPath := filepath.Join(tempDir, "mods", "root.pw.toml")
	if err := os.WriteFile(metaPath, []byte(`
name = "Root"
filename = "root.jar"
version = "1.0.0"
page-url = "https://example.com/root"
dependencies = ["mods/missing.pw.toml", ""]

[option]
dependency = false

[download]
url = "https://example.com/root.jar"
hash-format = "sha256"
hash = "abc"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/root.pw.toml": &indexFile{File: "mods/root.pw.toml", MetaFile: true},
		},
	}

	fixed, err := index.FixModMetadata(Pack{}, FixModMetadataOpts{Delete: true})
	if err != nil {
		t.Fatal(err)
	}
	if fixed != 1 {
		t.Fatalf("expected 1 fixed mod, got %d", fixed)
	}

	updated, err := LoadMod(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Dependencies) != 0 {
		t.Fatalf("expected invalid dependencies to be removed, got %#v", updated.Dependencies)
	}
}

func TestFixModMetadataDeleteRemovesObsoleteOptionKeys(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	metaPath := filepath.Join(tempDir, "mods", "example.pw.toml")
	if err := os.WriteFile(metaPath, []byte(`
name = "Example"
filename = "example.jar"
version = "1.0.0"
page-url = "https://example.com/example"

[option]
dependency = false
optional = true
default = false

[download]
url = "https://example.com/example.jar"
hash-format = "sha256"
hash = "abc"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/example.pw.toml": &indexFile{File: "mods/example.pw.toml", MetaFile: true},
		},
	}

	fixed, err := index.FixModMetadata(Pack{}, FixModMetadataOpts{Delete: true})
	if err != nil {
		t.Fatal(err)
	}
	if fixed != 1 {
		t.Fatalf("expected 1 fixed mod, got %d", fixed)
	}

	contents, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(contents)
	if strings.Contains(body, "optional") || strings.Contains(body, "default") {
		t.Fatalf("expected obsolete option keys to be removed, got:\n%s", body)
	}
}

func TestEnsureOptionDefaultsPreservesDependencyTrue(t *testing.T) {
	mod := Mod{
		Option: &ModOption{Dependency: true},
	}
	mod.EnsureOptionDefaults()
	if mod.Option == nil || !mod.Option.Dependency {
		t.Fatal("expected dependency=true to be preserved")
	}
}
