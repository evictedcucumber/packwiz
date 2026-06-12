package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixPackConfigAddsMissingKeys(t *testing.T) {
	tempDir := t.TempDir()
	packFile := filepath.Join(tempDir, "pack.toml")
	if err := os.WriteFile(packFile, []byte(`
name = "Example"

[versions]
minecraft = "1.21.1"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	fixes, err := fixPackConfig(packFile, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) == 0 {
		t.Fatal("expected pack fixes to be applied")
	}

	contents, err := os.ReadFile(packFile)
	if err != nil {
		t.Fatal(err)
	}
	body := string(contents)
	for _, key := range []string{"pack-format", "loader", "modlist", "index"} {
		if !strings.Contains(body, key) {
			t.Fatalf("expected %q to be written to pack.toml, got:\n%s", key, body)
		}
	}
}
