package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateModListMarkdownSkipsDependencyMods(t *testing.T) {
	tempDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "normal.pw.toml"), []byte("name = \"Normal Mod\"\nfilename = \"normal.jar\"\nversion = \"1.0.0\"\npage-url = \"https://example.com/normal\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "dependency.pw.toml"), []byte("name = \"Dependency Mod\"\nfilename = \"dependency.jar\"\nversion = \"2.0.0\"\npage-url = \"https://example.com/dependency\"\noption = { dependency = true }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	index := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/normal.pw.toml":     &indexFile{File: "mods/normal.pw.toml", MetaFile: true},
			"mods/dependency.pw.toml": &indexFile{File: "mods/dependency.pw.toml", MetaFile: true},
		},
	}

	markdown, err := index.GenerateModListMarkdown()
	if err != nil {
		t.Fatal(err)
	}

	expected := "# Mods List\n\n## Mods\n\n- [Normal Mod - (1.0.0)](https://example.com/normal)\n\n## Dependencies\n\n- [Dependency Mod - (2.0.0)](https://example.com/dependency)\n"
	if markdown != expected {
		t.Fatalf("unexpected modlist markdown:\nexpected:\n%s\nactual:\n%s", expected, markdown)
	}
}
