package cmdshared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirArchivePutsFilesInTheFolderReplacingWhatIsThere(t *testing.T) {
	root := t.TempDir()
	a := NewDirArchive(root)
	if err := a.Add("config/a.toml", strings.NewReader("one")); err != nil {
		t.Fatalf("Add() returned error: %v", err)
	}
	// A program that has the old one open keeps what it opened
	open, err := os.Open(filepath.Join(root, "config", "a.toml"))
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer func() { _ = open.Close() }()
	if err := a.Add("config/a.toml", strings.NewReader("two")); err != nil {
		t.Fatalf("Add() returned error: %v", err)
	}

	if data, _ := os.ReadFile(filepath.Join(root, "config", "a.toml")); string(data) != "two" {
		t.Errorf("config/a.toml = %q, want it replaced", data)
	}
	buf := make([]byte, 3)
	if n, _ := open.Read(buf); string(buf[:n]) != "one" {
		t.Errorf("the file that was open reads %q, want what was there when it was opened", buf[:n])
	}
	if !a.Added["config/a.toml"] || len(a.Added) != 1 {
		t.Errorf("Added = %v, want the file", a.Added)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "config"))
	if len(entries) != 1 {
		t.Errorf("the folder has %d files, want only the one put there", len(entries))
	}
}

func TestDirArchiveRefusesWhatIsOutsideItsFolder(t *testing.T) {
	a := NewDirArchive(filepath.Join(t.TempDir(), "root"))
	for _, name := range []string{"../escape.txt", "/etc/x", "a/../../b"} {
		if err := a.Add(name, strings.NewReader("x")); err == nil {
			t.Errorf("Add(%q) returned no error", name)
		}
	}
}
