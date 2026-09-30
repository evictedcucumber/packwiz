package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteManPages(t *testing.T) {
	dir := t.TempDir()
	if err := writeManPages(rootCmd, dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"packwiz.1", "packwiz-list.1"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), ".TH \"PACKWIZ") {
			t.Errorf("%s has no man page header", name)
		}
	}
}
