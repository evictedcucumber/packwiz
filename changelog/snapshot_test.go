package changelog

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

// memOpen serves files from a map, like a snapshot of a pack held somewhere other than disk
func memOpen(files map[string]string) OpenFunc {
	return func(path string) (io.ReadCloser, error) {
		content, ok := files[path]
		if !ok {
			return nil, fmt.Errorf("%s: %w", path, fs.ErrNotExist)
		}
		return io.NopCloser(strings.NewReader(content)), nil
	}
}

func mustParseIndex(t *testing.T, content string) core.Index {
	t.Helper()
	index, err := core.ParseIndex([]byte(content))
	if err != nil {
		t.Fatalf("ParseIndex() returned error: %v", err)
	}
	return index
}

const snapshotIndex = `hash-format = "sha256"

[[files]]
file = "mods/a.pw.toml"
metafile = true

[[files]]
file = "mods/b.pw.toml"
metafile = true

[[files]]
file = "config/c.json"
`

func TestTakeSnapshotFrom(t *testing.T) {
	index := mustParseIndex(t, snapshotIndex)
	open := memOpen(map[string]string{
		"mods/a.pw.toml": "name = \"A\"\nfilename = \"a-1.jar\"\nversion = \"1\"\nside = \"client\"\n",
		"mods/b.pw.toml": "name = \"B\"\nfilename = \"b-2.jar\"\n", // no side and no version
		"config/c.json":  "hello",
	})

	snap, err := TakeSnapshotFrom(index, open)
	if err != nil {
		t.Fatalf("TakeSnapshotFrom() returned error: %v", err)
	}

	wantMods := map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "1"},
		"mods/b.pw.toml": {Name: "B", Side: core.UniversalSide, Version: "b-2.jar"},
	}
	if len(snap.Mods) != len(wantMods) {
		t.Fatalf("Mods = %+v, want %+v", snap.Mods, wantMods)
	}
	for path, want := range wantMods {
		if got := snap.Mods[path]; got != want {
			t.Errorf("Mods[%q] = %+v, want %+v", path, got, want)
		}
	}
	// Pins the algorithm as well as the input: this is SHA-256 of "hello"
	const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got := snap.Files["config/c.json"]; got != helloSHA256 || len(snap.Files) != 1 {
		t.Errorf("Files = %v, want only config/c.json = %s", snap.Files, helloSHA256)
	}
}

func TestTakeSnapshotFromTreatsMissingFilesAsAbsent(t *testing.T) {
	// An index can list files that were since deleted; they just aren't part of the pack
	index := mustParseIndex(t, snapshotIndex)
	open := memOpen(map[string]string{"mods/a.pw.toml": "name = \"A\"\nfilename = \"a-1.jar\"\n"})

	snap, err := TakeSnapshotFrom(index, open)
	if err != nil {
		t.Fatalf("TakeSnapshotFrom() returned error: %v", err)
	}
	if len(snap.Mods) != 1 || len(snap.Files) != 0 {
		t.Errorf("snapshot = %+v, want only mods/a.pw.toml", snap)
	}
}

func TestTakeSnapshotFromReportsProblemsNamingTheFile(t *testing.T) {
	index := mustParseIndex(t, snapshotIndex)

	t.Run("unreadable file", func(t *testing.T) {
		open := func(path string) (io.ReadCloser, error) {
			if path == "config/c.json" {
				return nil, errors.New("permission denied")
			}
			return memOpen(map[string]string{"mods/a.pw.toml": "name = \"A\"\n", "mods/b.pw.toml": "name = \"B\"\n"})(path)
		}
		_, err := TakeSnapshotFrom(index, open)
		if err == nil || !strings.Contains(err.Error(), "config/c.json") || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("error = %v, want one naming config/c.json and the cause", err)
		}
	})

	t.Run("invalid metadata", func(t *testing.T) {
		open := memOpen(map[string]string{"mods/a.pw.toml": "this is [not toml", "mods/b.pw.toml": "name = \"B\"\n", "config/c.json": "x"})
		_, err := TakeSnapshotFrom(index, open)
		if err == nil || !strings.Contains(err.Error(), "mods/a.pw.toml") {
			t.Errorf("error = %v, want one naming mods/a.pw.toml", err)
		}
	})
}

func TestTakeSnapshotFromClosesEveryFile(t *testing.T) {
	index := mustParseIndex(t, snapshotIndex)
	files := map[string]string{
		"mods/a.pw.toml": "name = \"A\"\n", "mods/b.pw.toml": "name = \"B\"\n", "config/c.json": "x",
	}
	var opened, closed int
	open := func(path string) (io.ReadCloser, error) {
		opened++
		return closeCounter{strings.NewReader(files[path]), &closed}, nil
	}

	if _, err := TakeSnapshotFrom(index, open); err != nil {
		t.Fatalf("TakeSnapshotFrom() returned error: %v", err)
	}
	if opened != 3 || closed != opened {
		t.Errorf("opened %d files and closed %d, want all 3 closed", opened, closed)
	}
}

type closeCounter struct {
	io.Reader
	closed *int
}

func (c closeCounter) Close() error {
	*c.closed++
	return nil
}

func TestTakeSnapshotReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	for path, content := range map[string]string{
		"index.toml":     snapshotIndex,
		"mods/a.pw.toml": "name = \"A\"\nfilename = \"a-1.jar\"\nversion = \"1\"\nside = \"server\"\n",
		"mods/b.pw.toml": "name = \"B\"\nfilename = \"b-2.jar\"\n",
		"config/c.json":  "hello",
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}
	index, err := core.LoadIndex(filepath.Join(dir, "index.toml"))
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}

	snap, err := TakeSnapshot(index)
	if err != nil {
		t.Fatalf("TakeSnapshot() returned error: %v", err)
	}
	if got := snap.Mods["mods/a.pw.toml"]; got != (SnapshotMod{Name: "A", Side: core.ServerSide, Version: "1"}) {
		t.Errorf("Mods[mods/a.pw.toml] = %+v", got)
	}
	if len(snap.Mods) != 2 || len(snap.Files) != 1 {
		t.Errorf("snapshot = %+v, want 2 mods and 1 file", snap)
	}
}
