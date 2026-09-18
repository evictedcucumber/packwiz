package cmdshared

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

// fakeDownloadSession is a minimal core.DownloadSession for testing
// ListManualDownloads without a real cache/network dependency.
type fakeDownloadSession struct {
	manual []core.ManualDownload
}

func (f fakeDownloadSession) GetManualDownloads() []core.ManualDownload   { return f.manual }
func (f fakeDownloadSession) StartDownloads() chan core.CompletedDownload { return nil }
func (f fakeDownloadSession) SaveIndex() error                            { return nil }

func TestListManualDownloadsNoneDoesNotExit(t *testing.T) {
	// If this reaches os.Exit(1) (the non-empty-list branch), the test
	// process itself would be killed, so an empty list is the only branch
	// that can be exercised safely here.
	ListManualDownloads(fakeDownloadSession{})
}

func newTestIndex(t *testing.T) (core.Index, string) {
	t.Helper()
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.toml")
	if err := os.WriteFile(indexPath, []byte("hash-format = \"sha256\"\n"), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}
	idx, err := core.LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	return idx, dir
}

func TestAddToZipSuccess(t *testing.T) {
	idx, dir := newTestIndex(t)

	modDir := filepath.Join(dir, "mods")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	metaPath := filepath.Join(modDir, "test.pw.toml")

	content := []byte("jar file contents")
	tmpFile, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		t.Fatalf("failed to create temp download file: %v", err)
	}
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("failed to write temp download file: %v", err)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek temp download file: %v", err)
	}

	mod := &core.Mod{Name: "Test Mod", FileName: "test.jar"}
	mod.SetMetaPath(metaPath)

	var buf bytes.Buffer
	exp := zip.NewWriter(&buf)

	dl := core.CompletedDownload{Mod: mod, File: tmpFile}
	if ok := AddToZip(dl, exp, "overrides", &idx); !ok {
		t.Fatal("AddToZip() = false, want true")
	}
	if err := exp.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to read back zip: %v", err)
	}
	if len(reader.File) != 1 {
		t.Fatalf("got %d files in zip, want 1", len(reader.File))
	}
	wantName := "overrides/mods/test.jar"
	if reader.File[0].Name != wantName {
		t.Errorf("zip entry name = %q, want %q", reader.File[0].Name, wantName)
	}

	rc, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("failed to open zip entry: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("failed to read zip entry: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("zip entry contents = %q, want %q", got, content)
	}
}

func TestAddToZipDownloadError(t *testing.T) {
	idx, _ := newTestIndex(t)

	var buf bytes.Buffer
	exp := zip.NewWriter(&buf)

	dl := core.CompletedDownload{
		Mod:   &core.Mod{Name: "Broken Mod"},
		Error: errors.New("download failed"),
	}
	if ok := AddToZip(dl, exp, "overrides", &idx); ok {
		t.Error("AddToZip() = true, want false when dl.Error is set")
	}
}

func TestAddNonMetafileOverrides(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.toml")

	if err := os.WriteFile(filepath.Join(dir, "config.txt"), []byte("config contents"), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "mods"), 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mods", "foo.pw.toml"), []byte("name = \"Foo\"\n"), 0644); err != nil {
		t.Fatalf("failed to write metafile fixture: %v", err)
	}

	if err := os.WriteFile(indexPath, []byte(`hash-format = "sha256"

[[files]]
file = "config.txt"
hash = "irrelevant"

[[files]]
file = "mods/foo.pw.toml"
hash = "irrelevant"
metafile = true
`), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}

	idx, err := core.LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}

	var buf bytes.Buffer
	exp := zip.NewWriter(&buf)
	AddNonMetafileOverrides(&idx, exp)
	if err := exp.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to read back zip: %v", err)
	}
	if len(reader.File) != 1 {
		t.Fatalf("got %d files in zip, want 1 (only the non-metafile entry)", len(reader.File))
	}
	wantName := "overrides/config.txt"
	if reader.File[0].Name != wantName {
		t.Errorf("zip entry name = %q, want %q", reader.File[0].Name, wantName)
	}
}

func TestPrintDisclaimerDoesNotPanic(t *testing.T) {
	PrintDisclaimer()
}
