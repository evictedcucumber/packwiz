package cmdshared

import (
	"archive/zip"
	"bytes"
	"io"
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

func TestZipArchiveAddModeKeepsTheFilesPermissions(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	a := ZipArchive{Writer: w}
	if err := a.AddMode("run.sh", strings.NewReader("echo hi"), 0o755); err != nil {
		t.Fatalf("AddMode() returned error: %v", err)
	}
	if err := a.Add("plain.txt", strings.NewReader("hi")); err != nil {
		t.Fatalf("Add() returned error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("NewReader() returned error: %v", err)
	}
	for _, f := range r.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		switch f.Name {
		case "run.sh":
			if f.Mode().Perm() != 0o755 || string(data) != "echo hi" {
				t.Errorf("run.sh has mode %v and %q, want 755 and its content", f.Mode(), data)
			}
		case "plain.txt":
			if f.Mode()&0o111 != 0 {
				t.Errorf("plain.txt is executable: %v", f.Mode())
			}
		}
	}
}
