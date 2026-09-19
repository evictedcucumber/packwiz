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
				Version: "1.0.0", Date: "2026-01-01", Bump: BumpNone, Commit: "1111111111111111111111111111111111111111",
				Changes: []Change{{Kind: ModAdded, Path: "mods/sodium.pw.toml", Name: "Sodium", Side: core.ClientSide, To: "0.5.7"}},
			},
			{
				Version: "2.0.0", Date: "2026-02-01", Bump: BumpMajor, Commit: "2222222222222222222222222222222222222222",
				Changes: []Change{
					{Kind: ModAdded, Name: "Lithium", Side: core.UniversalSide, To: "0.12.0"},
					{Kind: ModUpdated, Name: "Sodium", Side: core.ClientSide, From: "0.5.7", To: "0.5.8"},
					{Kind: FileChanged, Path: "config/sodium.json"},
					{Kind: Note, Type: "feat", Scope: "world", Text: "reset the spawn point", Breaking: true},
				},
			},
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

func TestReleaseIsInitial(t *testing.T) {
	for bump, want := range map[Bump]bool{BumpNone: true, BumpPatch: false, BumpMinor: false, BumpMajor: false} {
		if got := (Release{Bump: bump}).IsInitial(); got != want {
			t.Errorf("Release{Bump: %v}.IsInitial() = %v, want %v", bump, got, want)
		}
	}
}

// legacyHistory is a history written when versions weren't known, so the mods were recorded by their file names
func legacyHistory() History {
	return History{
		Releases: []Release{
			{Version: "1.0.0", Date: "2026-01-01", Bump: BumpNone, Changes: []Change{
				{Kind: ModAdded, Path: "mods/a.pw.toml", Name: "A", Side: core.ClientSide, To: "a-1.0.jar"},
				{Kind: ModAdded, Path: "mods/b.pw.toml", Name: "B", Side: core.ServerSide, To: "b-1.0.jar"},
				{Kind: ModAdded, Path: "mods/c.pw.toml", Name: "C", Side: core.ClientSide, To: "c-1.0.jar"},
				{Kind: FileAdded, Path: "config/a.json"},
			}},
			{Version: "2.0.0", Date: "2026-02-01", Bump: BumpMajor, Changes: []Change{
				{Kind: ModUpdated, Path: "mods/b.pw.toml", Name: "B", Side: core.ServerSide, From: "b-1.0.jar", To: "b-2.0.jar"},
				{Kind: ModRemoved, Path: "mods/c.pw.toml", Name: "C", Side: core.ClientSide, From: "c-1.0.jar"},
			}},
		},
	}
}

func TestUpgradeVersions(t *testing.T) {
	h := legacyHistory()
	current := Snapshot{Mods: map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "1.0", File: "a-1.0.jar"},
		"mods/b.pw.toml": {Name: "B", Side: core.ServerSide, Version: "2.0", File: "b-2.0.jar"},
	}}

	upgraded := h.UpgradeVersions(current)

	// A's addition and B's latest update are on the files the mods are still on
	if upgraded != 2 {
		t.Errorf("UpgradeVersions() = %d, want 2 lines upgraded", upgraded)
	}
	first, second := h.Releases[0].Changes, h.Releases[1].Changes
	if first[0].To != "1.0" {
		t.Errorf("A was added as %q, want the version 1.0 now that it is known", first[0].To)
	}
	if second[0].To != "2.0" {
		t.Errorf("B was updated to %q, want 2.0", second[0].To)
	}
	// What can't be known stays as it was: B's first file is long gone, so nothing says which version it was
	if first[1].To != "b-1.0.jar" || second[0].From != "b-1.0.jar" {
		t.Errorf("B's older versions were changed to %q and %q; only a file the mod is still on is certain", first[1].To, second[0].From)
	}
	// ...and so does a mod that isn't there any more, and everything that isn't a mod
	if first[2].To != "c-1.0.jar" || second[1].From != "c-1.0.jar" {
		t.Errorf("C, which has been removed, was changed: %+v %+v", first[2], second[1])
	}
	if first[3] != (Change{Kind: FileAdded, Path: "config/a.json"}) {
		t.Errorf("a config change was altered: %+v", first[3])
	}
}

func TestUpgradeVersionsIsIdempotent(t *testing.T) {
	h := legacyHistory()
	current := Snapshot{Mods: map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "1.0", File: "a-1.0.jar"},
		"mods/b.pw.toml": {Name: "B", Side: core.ServerSide, Version: "2.0", File: "b-2.0.jar"},
	}}
	h.UpgradeVersions(current)
	after := legacyHistory()
	after.UpgradeVersions(current)

	if again := h.UpgradeVersions(current); again != 0 {
		t.Errorf("a second UpgradeVersions() = %d, want nothing left to upgrade", again)
	}
	if !reflect.DeepEqual(h, after) {
		t.Error("upgrading twice gave a different history from upgrading once")
	}
}

func TestUpgradeVersionsLeavesAModWhoseVersionIsStillUnknown(t *testing.T) {
	h := legacyHistory()
	// Looking the version up found nothing, so the mod is still described by its file name
	current := Snapshot{Mods: map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "a-1.0.jar", File: "a-1.0.jar"},
	}}

	if upgraded := h.UpgradeVersions(current); upgraded != 0 {
		t.Errorf("UpgradeVersions() = %d, want 0 when no real version is known", upgraded)
	}
	if !reflect.DeepEqual(h, legacyHistory()) {
		t.Error("the history changed although no version was known")
	}
}

func TestUpgradeVersionsNeedsTheModOnTheSameFile(t *testing.T) {
	h := legacyHistory()
	// A has been updated since; its current version says nothing about the file it was added as
	current := Snapshot{Mods: map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "2.0", File: "a-2.0.jar"},
	}}

	if upgraded := h.UpgradeVersions(current); upgraded != 0 {
		t.Errorf("UpgradeVersions() = %d, want 0; 2.0 isn't the version of a-1.0.jar", upgraded)
	}
	if got := h.Releases[0].Changes[0].To; got != "a-1.0.jar" {
		t.Errorf("A was added as %q, want it left alone", got)
	}
}

func TestUpgradeVersionsWithNoHistory(t *testing.T) {
	var h History
	current := Snapshot{Mods: map[string]SnapshotMod{"mods/a.pw.toml": {Version: "1.0", File: "a.jar"}}}

	if upgraded := h.UpgradeVersions(current); upgraded != 0 {
		t.Errorf("UpgradeVersions() = %d, want 0 for an empty history", upgraded)
	}
}

func TestHistoryRecordsTheCommitEachReleaseWasMadeAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFile)
	if err := sampleHistory().Write(path); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	got, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if got.Releases[0].Commit != "1111111111111111111111111111111111111111" || got.Releases[1].Commit != "2222222222222222222222222222222222222222" {
		t.Errorf("commits = %q, %q, want each release's own", got.Releases[0].Commit, got.Releases[1].Commit)
	}
}

// olderHistory is a history file as older versions wrote it, which held the pack as of the last release to compare the
// next one with
const olderHistory = `[[release]]
version = "1.0.0"
date = "2026-01-01"
bump = "none"

[[release.change]]
kind = "mod-added"
path = "mods/sodium.pw.toml"
name = "Sodium"
side = "client"
to = "0.5.7"

[snapshot]
[snapshot.mods]
[snapshot.mods."mods/sodium.pw.toml"]
name = "Sodium"
side = "client"
version = "0.5.7"
[snapshot.files]
"config/a.json" = "abc"
`

func TestLoadHistoryReadsTheSnapshotOlderVersionsKept(t *testing.T) {
	// A pack that isn't in a repository compares its next release with it, so it is as good as one made now
	path := filepath.Join(t.TempDir(), HistoryFile)
	if err := os.WriteFile(path, []byte(olderHistory), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	got, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}

	if len(got.Releases) != 1 || got.Releases[0].Changes[0].Name != "Sodium" {
		t.Errorf("history = %+v, want the release read as it was", got)
	}
	want := &Snapshot{
		Mods:  map[string]SnapshotMod{"mods/sodium.pw.toml": {Name: "Sodium", Side: core.ClientSide, Version: "0.5.7"}},
		Files: map[string]string{"config/a.json": "abc"},
	}
	if !reflect.DeepEqual(got.Snapshot, want) {
		t.Errorf("snapshot = %+v, want %+v", got.Snapshot, want)
	}
}

func TestHistoryKeepsTheSnapshotOfAReleaseMadeWithoutARepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFile)
	want := sampleHistory()
	want.Snapshot = &Snapshot{
		Mods:  map[string]SnapshotMod{"mods/sodium.pw.toml": {Name: "Sodium", Side: core.ClientSide, Version: "0.5.8"}},
		Files: map[string]string{"config/sodium.json": "abc"},
	}

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

func TestHistoryWithoutASnapshotWritesNone(t *testing.T) {
	// A release made from the log doesn't keep the pack, so there is nothing in the file to read as the last release's
	path := filepath.Join(t.TempDir(), HistoryFile)
	if err := sampleHistory().Write(path); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read history: %v", err)
	}
	if strings.Contains(string(data), "snapshot") {
		t.Errorf("the history has a snapshot:\n%s", data)
	}
	got, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if got.Snapshot != nil {
		t.Errorf("snapshot = %+v, want none", got.Snapshot)
	}
}

func TestHistoryKeepsTheSnapshotOfAPackWithNothingInIt(t *testing.T) {
	// Every mod and file removed is a pack to compare with like any other, and not one with no snapshot at all, which
	// would leave the next release with nothing to compare with
	path := filepath.Join(t.TempDir(), HistoryFile)
	h := sampleHistory()
	h.Snapshot = &Snapshot{Mods: map[string]SnapshotMod{}, Files: map[string]string{}}
	if err := h.Write(path); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	got, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}

	if got.Snapshot == nil {
		t.Fatal("the snapshot of an empty pack was lost")
	}
	if len(got.Snapshot.Mods) != 0 || len(got.Snapshot.Files) != 0 {
		t.Errorf("snapshot = %+v, want it empty", got.Snapshot)
	}
}

func TestUpgradeVersionsUpgradesTheSnapshotToo(t *testing.T) {
	h := legacyHistory()
	h.Snapshot = &Snapshot{Mods: map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "a-1.0.jar"},
		"mods/b.pw.toml": {Name: "B", Side: core.ServerSide, Version: "b-2.0.jar"},
	}}
	current := Snapshot{Mods: map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "1.0", File: "a-1.0.jar"},
		// B has moved on to another file, so nothing says which version b-2.0.jar was
		"mods/b.pw.toml": {Name: "B", Side: core.ServerSide, Version: "3.0", File: "b-3.0.jar"},
	}}

	upgraded := h.UpgradeVersions(current)

	// Only lines of the releases count, as those are what the changelog shows
	if upgraded != 1 {
		t.Errorf("UpgradeVersions() = %d, want 1: A's addition", upgraded)
	}
	if got := h.Snapshot.Mods["mods/a.pw.toml"].Version; got != "1.0" {
		t.Errorf("A is %q in the snapshot, want 1.0 now that it is known", got)
	}
	if got := h.Snapshot.Mods["mods/b.pw.toml"].Version; got != "b-2.0.jar" {
		t.Errorf("B is %q in the snapshot, want it left as its file", got)
	}
}

func TestNotesAreStoredInTheHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFile)
	want := History{Releases: []Release{{
		Version: "1.1.0", Date: "2026-01-02", Bump: BumpMajor,
		Changes: []Change{{Kind: Note, Type: "feat", Scope: "world", Text: "reset the spawn point", Breaking: true}},
	}}}
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
