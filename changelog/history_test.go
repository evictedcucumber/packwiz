package changelog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func sampleHistory() History {
	return History{
		Releases: []Release{
			{
				Version: "1.0.0", Date: "2026-01-01", Bump: BumpNone,
				Changes: []Change{{Kind: ModAdded, Path: "mods/sodium.pw.toml", Name: "Sodium", Side: core.ClientSide, To: "0.5.7"}},
			},
			{
				Version: "2.0.0", Date: "2026-02-01", Bump: BumpMajor,
				Changes: []Change{
					{Kind: ModAdded, Path: "mods/lithium.pw.toml", Name: "Lithium", Side: core.UniversalSide, To: "0.12.0"},
					{Kind: ModUpdated, Path: "mods/sodium.pw.toml", Name: "Sodium", Side: core.ClientSide, From: "0.5.7", To: "0.5.8"},
					{Kind: FileChanged, Path: "config/sodium.json"},
				},
			},
		},
		Snapshot: Snapshot{
			Mods: map[string]SnapshotMod{
				"mods/sodium.pw.toml":  {Name: "Sodium", Side: core.ClientSide, Version: "0.5.8"},
				"mods/lithium.pw.toml": {Name: "Lithium", Side: core.UniversalSide, Version: "0.12.0"},
			},
			Files: map[string]string{"config/sodium.json": "abc123"},
		},
	}
}

func TestHistoryWriteLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFile)
	want := sampleHistory()

	if err := want.Write(path); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	got, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadHistory() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestHistoryWriteIsDeterministic(t *testing.T) {
	// The history is committed to version control, so writing the same thing must not produce a diff
	dir := t.TempDir()
	var first []byte
	for i := 0; i < 20; i++ {
		path := filepath.Join(dir, HistoryFile)
		if err := sampleHistory().Write(path); err != nil {
			t.Fatalf("Write() returned error: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read history: %v", err)
		}
		if first == nil {
			first = data
		} else if string(data) != string(first) {
			t.Fatalf("Write() output changed between runs:\n%s\n---\n%s", first, data)
		}
	}
}

func TestHistoryWriteStoresBumpByName(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFile)
	if err := sampleHistory().Write(path); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read history: %v", err)
	}
	if want := `bump = "major"`; !strings.Contains(string(data), want) {
		t.Errorf("history file should be readable by hand and contain %s, got:\n%s", want, data)
	}
}

func TestLoadHistoryMissingFileIsEmpty(t *testing.T) {
	got, err := LoadHistory(filepath.Join(t.TempDir(), HistoryFile))
	if err != nil {
		t.Fatalf("LoadHistory() on a missing file returned error: %v", err)
	}
	if _, ok := got.Latest(); ok {
		t.Errorf("LoadHistory() on a missing file has releases: %+v", got.Releases)
	}
}

func TestLoadHistoryRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFile)
	if err := os.WriteFile(path, []byte("this is [not toml"), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	if _, err := LoadHistory(path); err == nil {
		t.Error("LoadHistory() on a corrupt file returned no error; silently treating it as empty would discard the history")
	}
}

func TestLoadHistoryRejectsUnknownBump(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFile)
	if err := os.WriteFile(path, []byte("[[release]]\nversion = \"1.0.0\"\ndate = \"2026-01-01\"\nbump = \"enormous\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	if _, err := LoadHistory(path); err == nil {
		t.Error("LoadHistory() accepted an unknown bump")
	}
}

func TestHistoryLatest(t *testing.T) {
	latest, ok := sampleHistory().Latest()
	if !ok || latest.Version != "2.0.0" {
		t.Errorf("Latest() = %+v, %v, want version 2.0.0", latest, ok)
	}
}

func TestWriteFileAtomicReplacesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("old contents that are longer"), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatalf("writeFileAtomic() returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Errorf("file = %q, %v, want %q", data, err, "new")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want only the written file (no leaked temp files)", len(entries))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() returned error: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644 rather than CreateTemp's private 0600", info.Mode().Perm())
	}
}

func TestWriteFileAtomicFailureLeavesNoTempFilesAndKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	// Renaming a file over a non-empty directory fails, after the temp file has been written
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o755); err != nil {
		t.Fatalf("failed to create fixture: %v", err)
	}

	if err := writeFileAtomic(target, []byte("data")); err == nil {
		t.Fatal("writeFileAtomic() over a directory returned no error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "target" {
		t.Errorf("dir = %v, want only the original target (temp file must be cleaned up)", entries)
	}
}
