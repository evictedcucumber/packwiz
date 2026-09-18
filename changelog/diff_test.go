package changelog

import (
	"reflect"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestChangeBump(t *testing.T) {
	tests := []struct {
		name   string
		change Change
		want   Bump
	}{
		{"client mod added", Change{Kind: ModAdded, Side: core.ClientSide}, BumpMinor},
		{"client mod removed", Change{Kind: ModRemoved, Side: core.ClientSide}, BumpMinor},
		{"client mod updated", Change{Kind: ModUpdated, Side: core.ClientSide}, BumpPatch},

		{"server mod added", Change{Kind: ModAdded, Side: core.ServerSide}, BumpMajor},
		{"server mod removed", Change{Kind: ModRemoved, Side: core.ServerSide}, BumpMajor},
		{"server mod updated", Change{Kind: ModUpdated, Side: core.ServerSide}, BumpMajor},

		{"both mod added", Change{Kind: ModAdded, Side: core.UniversalSide}, BumpMajor},
		{"both mod removed", Change{Kind: ModRemoved, Side: core.UniversalSide}, BumpMajor},
		{"both mod updated", Change{Kind: ModUpdated, Side: core.UniversalSide}, BumpMajor},

		// An empty side means "both" to packwiz
		{"sideless mod added", Change{Kind: ModAdded, Side: core.EmptySide}, BumpMajor},
		{"sideless mod updated", Change{Kind: ModUpdated, Side: core.EmptySide}, BumpMajor},

		{"config added", Change{Kind: FileAdded}, BumpPatch},
		{"config changed", Change{Kind: FileChanged}, BumpPatch},
		{"config removed", Change{Kind: FileRemoved}, BumpPatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.change.Bump(); got != tt.want {
				t.Errorf("Bump() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunsOnServer(t *testing.T) {
	for side, want := range map[string]bool{
		core.ClientSide:    false,
		core.ServerSide:    true,
		core.UniversalSide: true,
		core.EmptySide:     true,
		"something-else":   true, // don't under-bump on a typo'd side
	} {
		if got := RunsOnServer(side); got != want {
			t.Errorf("RunsOnServer(%q) = %v, want %v", side, got, want)
		}
	}
}

func TestHighestBump(t *testing.T) {
	client := Change{Kind: ModAdded, Side: core.ClientSide}
	server := Change{Kind: ModAdded, Side: core.ServerSide}
	config := Change{Kind: FileChanged}

	tests := []struct {
		name    string
		changes []Change
		want    Bump
	}{
		{"no changes", nil, BumpNone},
		{"config only", []Change{config}, BumpPatch},
		{"client and config", []Change{config, client}, BumpMinor},
		{"server wins over everything", []Change{config, client, server}, BumpMajor},
		{"order doesn't matter", []Change{server, config, client}, BumpMajor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HighestBump(tt.changes); got != tt.want {
				t.Errorf("HighestBump() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiffMods(t *testing.T) {
	oldSnap := Snapshot{Mods: map[string]SnapshotMod{
		"mods/kept.pw.toml":    {Name: "Kept", Side: core.ClientSide, Version: "1.0"},
		"mods/updated.pw.toml": {Name: "Updated", Side: core.ClientSide, Version: "1.0"},
		"mods/removed.pw.toml": {Name: "Removed", Side: core.ServerSide, Version: "3.0"},
	}}
	newSnap := Snapshot{Mods: map[string]SnapshotMod{
		"mods/kept.pw.toml":    {Name: "Kept", Side: core.ClientSide, Version: "1.0"},
		"mods/updated.pw.toml": {Name: "Updated", Side: core.ClientSide, Version: "2.0"},
		"mods/added.pw.toml":   {Name: "Added", Side: core.UniversalSide, Version: "5.0"},
	}}

	want := []Change{
		{Kind: ModAdded, Path: "mods/added.pw.toml", Name: "Added", Side: core.UniversalSide, To: "5.0"},
		{Kind: ModRemoved, Path: "mods/removed.pw.toml", Name: "Removed", Side: core.ServerSide, From: "3.0"},
		{Kind: ModUpdated, Path: "mods/updated.pw.toml", Name: "Updated", Side: core.ClientSide, From: "1.0", To: "2.0"},
	}
	if got := Diff(oldSnap, newSnap); !reflect.DeepEqual(got, want) {
		t.Errorf("Diff() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestDiffFiles(t *testing.T) {
	oldSnap := Snapshot{Files: map[string]string{
		"config/kept.json":    "aaa",
		"config/changed.json": "bbb",
		"config/removed.json": "ccc",
	}}
	newSnap := Snapshot{Files: map[string]string{
		"config/kept.json":    "aaa",
		"config/changed.json": "BBB",
		"config/added.json":   "ddd",
	}}

	want := []Change{
		{Kind: FileAdded, Path: "config/added.json"},
		{Kind: FileChanged, Path: "config/changed.json"},
		{Kind: FileRemoved, Path: "config/removed.json"},
	}
	if got := Diff(oldSnap, newSnap); !reflect.DeepEqual(got, want) {
		t.Errorf("Diff() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestDiffUsesNewSideForUpdatedMod(t *testing.T) {
	// If a mod's side was edited alongside a version bump, it is classified by the side it has now
	oldSnap := Snapshot{Mods: map[string]SnapshotMod{"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "1"}}}
	newSnap := Snapshot{Mods: map[string]SnapshotMod{"mods/a.pw.toml": {Name: "A", Side: core.UniversalSide, Version: "2"}}}

	got := Diff(oldSnap, newSnap)
	if len(got) != 1 || got[0].Side != core.UniversalSide {
		t.Errorf("Diff() = %+v, want one change with side %q", got, core.UniversalSide)
	}
}

func TestDiffNoChanges(t *testing.T) {
	snap := Snapshot{
		Mods:  map[string]SnapshotMod{"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "1"}},
		Files: map[string]string{"config/a.json": "aaa"},
	}
	if got := Diff(snap, snap); len(got) != 0 {
		t.Errorf("Diff() of identical snapshots = %+v, want no changes", got)
	}
	if got := Diff(Snapshot{}, Snapshot{}); len(got) != 0 {
		t.Errorf("Diff() of empty snapshots = %+v, want no changes", got)
	}
}

func TestDiffFromEmptySnapshotAddsEverything(t *testing.T) {
	newSnap := Snapshot{
		Mods:  map[string]SnapshotMod{"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "1"}},
		Files: map[string]string{"config/a.json": "aaa"},
	}
	got := Diff(Snapshot{}, newSnap)
	if len(got) != 2 {
		t.Fatalf("Diff() = %+v, want two changes", got)
	}
	for _, c := range got {
		if c.Kind != ModAdded && c.Kind != FileAdded {
			t.Errorf("Diff() produced %v, want only additions", c.Kind)
		}
	}
}

func TestDiffIsDeterministic(t *testing.T) {
	// Map iteration order is random, so run enough times that an unsorted result would be caught
	newSnap := Snapshot{
		Mods: map[string]SnapshotMod{},
		Files: map[string]string{
			"config/z.json": "1", "config/a.json": "1", "config/m.json": "1", "config/b.json": "1",
		},
	}
	for _, p := range []string{"mods/q.pw.toml", "mods/c.pw.toml", "mods/x.pw.toml"} {
		newSnap.Mods[p] = SnapshotMod{Name: p, Side: core.ClientSide, Version: "1"}
	}

	first := Diff(Snapshot{}, newSnap)
	for i := 0; i < 50; i++ {
		if got := Diff(Snapshot{}, newSnap); !reflect.DeepEqual(got, first) {
			t.Fatalf("Diff() order changed between runs:\n%+v\n%+v", first, got)
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i-1].Path > first[i].Path {
			t.Errorf("Diff() not sorted by path: %q before %q", first[i-1].Path, first[i].Path)
		}
	}
}

func TestNewSnapshotModRecordsEmptySideAsBoth(t *testing.T) {
	got := NewSnapshotMod(core.Mod{Name: "A", FileName: "a-1.0.jar", Version: "1.0"})
	want := SnapshotMod{Name: "A", Side: core.UniversalSide, Version: "1.0", File: "a-1.0.jar"}
	if got != want {
		t.Errorf("NewSnapshotMod() = %+v, want %+v", got, want)
	}
}

func TestNewSnapshotModFallsBackToFileName(t *testing.T) {
	got := NewSnapshotMod(core.Mod{Name: "A", FileName: "a-1.0.jar", Side: core.ClientSide})
	if got.Version != "a-1.0.jar" {
		t.Errorf("Version = %q, want the file name when the mod has no version", got.Version)
	}
}

func TestDiffDoesNotCallANewlyKnownVersionAnUpdate(t *testing.T) {
	// The old snapshot recorded the mod by its file name, as happens when its version wasn't known. The version has
	// since been looked up, but the mod is still on the same file, so nothing has been updated.
	oldSnap := Snapshot{Mods: map[string]SnapshotMod{
		"mods/lithium.pw.toml": {Name: "Lithium", Side: core.ServerSide, Version: "lithium-0.12.0.jar"},
	}}
	newSnap := Snapshot{Mods: map[string]SnapshotMod{
		"mods/lithium.pw.toml": {Name: "Lithium", Side: core.ServerSide, Version: "0.12.0", File: "lithium-0.12.0.jar"},
	}}

	got := Diff(oldSnap, newSnap)

	if len(got) != 0 {
		t.Errorf("Diff() = %+v, want no changes; recording a version isn't an update", got)
	}
	// This matters most for a server mod, where a false update would force a major release
	if bump := HighestBump(got); bump != BumpNone {
		t.Errorf("HighestBump() = %v, want none", bump)
	}
}

func TestModUpdated(t *testing.T) {
	tests := []struct {
		name string
		old  SnapshotMod
		new  SnapshotMod
		want bool
	}{
		{
			"same recorded version",
			SnapshotMod{Version: "1.0"}, SnapshotMod{Version: "1.0", File: "a-1.0.jar"}, false,
		},
		{
			"different recorded versions",
			SnapshotMod{Version: "1.0"}, SnapshotMod{Version: "1.1", File: "a-1.1.jar"}, true,
		},
		{
			// A publisher that names the file the same for every version is still caught by its version
			"different versions in a file that kept its name",
			SnapshotMod{Version: "1.0"}, SnapshotMod{Version: "1.1", File: "a.jar"}, true,
		},
		{
			"old version was the file name, and the mod is still on it",
			SnapshotMod{Version: "a-1.0.jar"}, SnapshotMod{Version: "1.0", File: "a-1.0.jar"}, false,
		},
		{
			"old version was a file name, and the mod is on another now",
			SnapshotMod{Version: "a-1.0.jar"}, SnapshotMod{Version: "1.1", File: "a-1.1.jar"}, true,
		},
		{
			"neither version is known, and the file changed",
			SnapshotMod{Version: "a-1.0.jar"}, SnapshotMod{Version: "a-1.1.jar", File: "a-1.1.jar"}, true,
		},
		{
			"neither version is known, and the file didn't",
			SnapshotMod{Version: "a-1.0.jar"}, SnapshotMod{Version: "a-1.0.jar", File: "a-1.0.jar"}, false,
		},
		{
			// The mod at HEAD in git is read from a commit, so it knows its file too
			"both know their files, and the version is newly known",
			SnapshotMod{Version: "a-1.0.jar", File: "a-1.0.jar"}, SnapshotMod{Version: "1.0", File: "a-1.0.jar"}, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modUpdated(tt.old, tt.new); got != tt.want {
				t.Errorf("modUpdated(%+v, %+v) = %v, want %v", tt.old, tt.new, got, tt.want)
			}
		})
	}
}

func TestDiffReportsUpdateFromAFileNameByTheFileNameItWas(t *testing.T) {
	oldSnap := Snapshot{Mods: map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "a-1.0.jar"},
	}}
	newSnap := Snapshot{Mods: map[string]SnapshotMod{
		"mods/a.pw.toml": {Name: "A", Side: core.ClientSide, Version: "1.1", File: "a-1.1.jar"},
	}}

	want := []Change{{Kind: ModUpdated, Path: "mods/a.pw.toml", Name: "A", Side: core.ClientSide, From: "a-1.0.jar", To: "1.1"}}
	if got := Diff(oldSnap, newSnap); !reflect.DeepEqual(got, want) {
		t.Errorf("Diff() = %+v, want %+v", got, want)
	}
}
