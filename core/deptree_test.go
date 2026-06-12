package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDependencyTree(t *testing.T) {
	tempDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Mod A (root)
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "mod-a.pw.toml"), []byte(`
name = "Mod A"
filename = "mod-a.jar"
version = "1.0.0"
page-url = "https://example.com/mod-a"
dependencies = ["mods/mod-b.pw.toml"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Mod B (dependency)
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "mod-b.pw.toml"), []byte(`
name = "Mod B"
filename = "mod-b.jar"
version = "1.1.0"
page-url = "https://example.com/mod-b"
option = { dependency = true }
dependencies = ["mods/mod-c.pw.toml"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Mod C (transitive dependency, creates a cycle back to Mod A for testing)
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "mod-c.pw.toml"), []byte(`
name = "Mod C"
filename = "mod-c.jar"
version = "1.2.0"
page-url = "https://example.com/mod-c"
option = { dependency = true }
dependencies = ["mods/mod-a.pw.toml"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/mod-a.pw.toml": &indexFile{File: "mods/mod-a.pw.toml", MetaFile: true},
			"mods/mod-b.pw.toml": &indexFile{File: "mods/mod-b.pw.toml", MetaFile: true},
			"mods/mod-c.pw.toml": &indexFile{File: "mods/mod-c.pw.toml", MetaFile: true},
		},
	}

	packDeps, err := index.GenerateDependencyTree()
	if err != nil {
		t.Fatal(err)
	}

	if len(packDeps.Mods) != 1 {
		t.Fatalf("expected 1 root mod, got %d", len(packDeps.Mods))
	}

	root := packDeps.Mods[0]
	if root.Name != "Mod A" {
		t.Fatalf("expected root Mod A, got %s", root.Name)
	}

	if len(root.Dependencies) != 1 || root.Dependencies[0].Name != "Mod B" {
		t.Fatalf("expected Mod B as child of Mod A")
	}

	b := root.Dependencies[0]
	if len(b.Dependencies) != 1 || b.Dependencies[0].Name != "Mod C" {
		t.Fatalf("expected Mod C as child of Mod B")
	}

	c := b.Dependencies[0]
	if len(c.Dependencies) != 1 || !strings.Contains(c.Dependencies[0].Name, "Cycle") {
		t.Fatalf("expected Mod A with Cycle tag as child of Mod C, got dependencies count: %d", len(c.Dependencies))
	}

	// Test Format
	formatted := packDeps.FormatDependencyTree()
	expectedFormat := `Mods:
└── Mod A (mods/mod-a.pw.toml)
    └── Mod B (mods/mod-b.pw.toml)
        └── Mod C (mods/mod-c.pw.toml)
            └── Mod A [Cycle] (mods/mod-a.pw.toml)`

	// Normalize CRLF to LF
	formatted = strings.ReplaceAll(formatted, "\r\n", "\n")
	expectedFormat = strings.ReplaceAll(expectedFormat, "\r\n", "\n")

	if formatted != expectedFormat {
		t.Fatalf("unexpected formatted tree:\nexpected:\n%s\nactual:\n%s", expectedFormat, formatted)
	}
}

func TestValidateDependencyTree(t *testing.T) {
	tempDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Mod A
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "mod-a.pw.toml"), []byte(`
name = "Mod A"
filename = "mod-a.jar"
version = "1.0.0"
page-url = "https://example.com/mod-a"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/mod-a.pw.toml": &indexFile{File: "mods/mod-a.pw.toml", MetaFile: true},
		},
	}

	// Case 1: missing dependencies.toml
	err := index.ValidateDependencyTree()
	if err == nil || !strings.Contains(err.Error(), "dependencies.toml is missing") {
		t.Fatalf("expected missing dependencies.toml error, got: %v", err)
	}

	// Write dependency tree
	err = index.WriteDependencyTree()
	if err != nil {
		t.Fatal(err)
	}

	// Case 2: valid
	err = index.ValidateDependencyTree()
	if err != nil {
		t.Fatalf("expected validation to pass, got: %v", err)
	}

	// Case 3: out of date dependencies.toml (let's add a mod without updating dependencies.toml)
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "mod-b.pw.toml"), []byte(`
name = "Mod B"
filename = "mod-b.jar"
version = "1.0.0"
page-url = "https://example.com/mod-b"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	index.Files["mods/mod-b.pw.toml"] = &indexFile{File: "mods/mod-b.pw.toml", MetaFile: true}

	err = index.ValidateDependencyTree()
	if err == nil || !strings.Contains(err.Error(), "dependencies.toml is out of date") {
		t.Fatalf("expected out of date error, got: %v", err)
	}

	// Update dependencies.toml so we are valid again
	err = index.WriteDependencyTree()
	if err != nil {
		t.Fatal(err)
	}

	// Case 4: untracked/missing dependency referenced in Mod A
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "mod-a.pw.toml"), []byte(`
name = "Mod A"
filename = "mod-a.jar"
version = "1.0.0"
page-url = "https://example.com/mod-a"
dependencies = ["mods/missing-mod.pw.toml"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	err = index.ValidateDependencyTree()
	if err == nil || !strings.Contains(err.Error(), "missing or untracked dependency files") {
		t.Fatalf("expected missing dependency error, got: %v", err)
	}
}

func TestToIndexRelativePath(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(tempDir, "mods", "example.pw.toml")
	if err := os.WriteFile(metaPath, []byte("name = \"Example\"\nfilename = \"example.jar\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/example.pw.toml": &indexFile{File: "mods/example.pw.toml", MetaFile: true},
		},
	}

	rel, err := index.ToIndexRelativePath(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "mods/example.pw.toml" {
		t.Fatalf("expected mods/example.pw.toml, got %q", rel)
	}

	rel, err = index.ToIndexRelativePath("mods/example.pw.toml")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "mods/example.pw.toml" {
		t.Fatalf("expected mods/example.pw.toml, got %q", rel)
	}
}

func TestPruneDependencyReference(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	rootMeta := filepath.Join(tempDir, "mods", "root.pw.toml")
	depMeta := filepath.Join(tempDir, "mods", "dependency.pw.toml")
	if err := os.WriteFile(rootMeta, []byte(`
name = "Root"
filename = "root.jar"
version = "1.0.0"
page-url = "https://example.com/root"
dependencies = ["mods/dependency.pw.toml"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(depMeta, []byte(`
name = "Dependency"
filename = "dependency.jar"
version = "1.0.0"
page-url = "https://example.com/dependency"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/root.pw.toml":       &indexFile{File: "mods/root.pw.toml", MetaFile: true},
			"mods/dependency.pw.toml": &indexFile{File: "mods/dependency.pw.toml", MetaFile: true},
		},
	}

	if err := index.PruneDependencyReference(depMeta); err != nil {
		t.Fatal(err)
	}

	updated, err := LoadMod(rootMeta)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Dependencies) != 0 {
		t.Fatalf("expected dependency reference to be removed, got %#v", updated.Dependencies)
	}
}

func TestNormalizeAllModDependencyPaths(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	rootMeta := filepath.Join(tempDir, "mods", "root.pw.toml")
	depMeta := filepath.Join(tempDir, "mods", "dependency.pw.toml")
	if err := os.WriteFile(rootMeta, []byte(`
name = "Root"
filename = "root.jar"
version = "1.0.0"
page-url = "https://example.com/root"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(depMeta, []byte(`
name = "Dependency"
filename = "dependency.jar"
version = "1.0.0"
page-url = "https://example.com/dependency"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/root.pw.toml":       &indexFile{File: "mods/root.pw.toml", MetaFile: true},
			"mods/dependency.pw.toml": &indexFile{File: "mods/dependency.pw.toml", MetaFile: true},
		},
	}

	rootMod, err := LoadMod(rootMeta)
	if err != nil {
		t.Fatal(err)
	}
	rootMod.Dependencies = []string{depMeta}
	if _, _, err := rootMod.Write(); err != nil {
		t.Fatal(err)
	}

	if err := index.NormalizeAllModDependencyPaths(); err != nil {
		t.Fatal(err)
	}

	updated, err := LoadMod(rootMeta)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Dependencies) != 1 || updated.Dependencies[0] != "mods/dependency.pw.toml" {
		t.Fatalf("expected normalized dependency path, got %#v", updated.Dependencies)
	}
}
