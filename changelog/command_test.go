package changelog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// setUpPack creates an empty pack in a fresh temp directory, with prompts auto-accepted and a fixed release date.
func setUpPack(t *testing.T, version string) {
	t.Helper()
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", Version: version, PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.21"},
	})
	if err := os.WriteFile("index.toml", []byte("hash-format = \"sha256\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}
	cmdtest.SetViperBool(t, "non-interactive", true)

	oldNow := now
	now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = oldNow })
}

func writeMod(t *testing.T, name, side, version string) {
	t.Helper()
	writeModFile(t, core.Mod{
		Name: name, FileName: strings.ToLower(name) + "-" + version + ".jar", Version: version, Side: side,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-" + version},
	})
}

// writeModFile writes a mod's metadata file the way the CLI would, at mods/<lowercased name>.pw.toml
func writeModFile(t *testing.T, mod core.Mod) {
	t.Helper()
	mod.SetMetaPath(filepath.Join("mods", strings.ToLower(mod.Name)+core.MetaExtension))
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("failed to write mod fixture %s: %v", mod.Name, err)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("failed to remove %s: %v", path, err)
	}
}

// release runs "packwiz changelog release", returning what it printed
func release(t *testing.T, versionOverride string) string {
	t.Helper()
	var err error
	out := cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease(versionOverride) })
	if err != nil {
		t.Fatalf("RunRelease() returned error: %v\noutput: %s", err, out)
	}
	return out
}

func preview(t *testing.T) string {
	t.Helper()
	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runPreview() })
	if err != nil {
		t.Fatalf("runPreview() returned error: %v\noutput: %s", err, out)
	}
	return out
}

func loadHistory(t *testing.T) History {
	t.Helper()
	h, err := LoadHistory(historyPath())
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	return h
}

func packVersion(t *testing.T) string {
	t.Helper()
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	return pack.Version
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}

func TestFirstReleaseAdoptsPackVersionAndRecordsEverything(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	writeMod(t, "Lithium", core.ServerSide, "0.12.0")
	writeConfig(t, "config/sodium.json", "{}")

	out := release(t, "")
	for _, want := range []string{"First release; version is 1.0.0", "Released 1.0.0!"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	history := loadHistory(t)
	if len(history.Releases) != 1 {
		t.Fatalf("history has %d releases, want 1", len(history.Releases))
	}
	first := history.Releases[0]
	if first.Version != "1.0.0" || first.Date != "2026-09-18" || first.Bump != BumpNone {
		t.Errorf("release = {%s %s %v}, want {1.0.0 2026-09-18 none}", first.Version, first.Date, first.Bump)
	}
	if len(first.Changes) != 3 {
		t.Errorf("first release has %d changes, want 3 (two mods and a config): %+v", len(first.Changes), first.Changes)
	}
	if len(history.Snapshot.Mods) != 2 || len(history.Snapshot.Files) != 1 {
		t.Errorf("snapshot = %+v, want 2 mods and 1 file", history.Snapshot)
	}
	if _, ok := history.Snapshot.Files["config/sodium.json"]; !ok {
		t.Errorf("snapshot files = %v, want config/sodium.json (and not pack.toml or index.toml)", history.Snapshot.Files)
	}

	if got := packVersion(t); got != "1.0.0" {
		t.Errorf("pack.toml version = %q, want 1.0.0", got)
	}
	changelog := readFile(t, MarkdownFile)
	for _, want := range []string{"## 1.0.0 - 2026-09-18", "**Sodium** 0.5.7 (client)", "**Lithium** 0.12.0 (server)", "`config/sodium.json`"} {
		if !strings.Contains(changelog, want) {
			t.Errorf("CHANGELOG.md missing %q:\n%s", want, changelog)
		}
	}
}

func TestFirstReleaseWithoutPackVersionStartsAtOne(t *testing.T) {
	setUpPack(t, "")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")

	release(t, "")

	if got := packVersion(t); got != "1.0.0" {
		t.Errorf("pack.toml version = %q, want 1.0.0", got)
	}
}

// Walks a pack through releases that each exercise one level of the bump policy
func TestReleasesBumpByTheMostSignificantChange(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")

	steps := []struct {
		name     string
		change   func()
		wantVer  string
		wantBump Bump
	}{
		{"client mod added is minor", func() { writeMod(t, "Iris", core.ClientSide, "1.0") }, "1.1.0", BumpMinor},
		{"client mod updated is patch", func() { writeMod(t, "Iris", core.ClientSide, "1.1") }, "1.1.1", BumpPatch},
		{"config added is patch", func() { writeConfig(t, "config/iris.json", "{}") }, "1.1.2", BumpPatch},
		{"config changed is patch", func() { writeConfig(t, "config/iris.json", `{"a": 1}`) }, "1.1.3", BumpPatch},
		{"config removed is patch", func() { remove(t, "config/iris.json") }, "1.1.4", BumpPatch},
		{"client mod removed is minor", func() { remove(t, "mods/iris.pw.toml") }, "1.2.0", BumpMinor},
		{"server mod added is major", func() { writeMod(t, "Lithium", core.ServerSide, "0.12.0") }, "2.0.0", BumpMajor},
		{"both mod added is major", func() { writeMod(t, "Fabric API", core.UniversalSide, "0.100") }, "3.0.0", BumpMajor},
		{"server mod updated is major", func() { writeMod(t, "Lithium", core.ServerSide, "0.12.1") }, "4.0.0", BumpMajor},
		{"server mod removed is major", func() { remove(t, "mods/lithium.pw.toml") }, "5.0.0", BumpMajor},
		{
			"the most significant change wins",
			func() {
				writeConfig(t, "config/a.json", "{}")                   // patch
				writeMod(t, "Sodium", core.ClientSide, "0.5.8")         // patch
				writeMod(t, "Lithium", core.ServerSide, "0.13.0")       // major
				writeModFile(t, core.Mod{Name: "Extra", FileName: "e"}) // sideless (both): major
			},
			"6.0.0", BumpMajor,
		},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			step.change()
			release(t, "")

			if got := packVersion(t); got != step.wantVer {
				t.Errorf("pack.toml version = %q, want %q", got, step.wantVer)
			}
			latest, _ := loadHistory(t).Latest()
			if latest.Version != step.wantVer || latest.Bump != step.wantBump {
				t.Errorf("latest release = {%s %v}, want {%s %v}", latest.Version, latest.Bump, step.wantVer, step.wantBump)
			}
		})
	}

	// The rendered changelog has every release, newest first
	changelog := readFile(t, MarkdownFile)
	prev := -1
	for _, v := range []string{"6.0.0", "5.0.0", "4.0.0", "3.0.0", "2.0.0", "1.2.0", "1.1.4", "1.1.3", "1.1.2", "1.1.1", "1.1.0", "1.0.0"} {
		i := strings.Index(changelog, "## "+v+" ")
		if i < 0 {
			t.Fatalf("CHANGELOG.md is missing release %s:\n%s", v, changelog)
		}
		// Walking from newest to oldest, each release must appear further down the file than the one before
		if i < prev {
			t.Errorf("CHANGELOG.md lists %s above a newer release; it should be newest first", v)
		}
		prev = i
	}

	// The release files sit in the pack directory but are not distributed as part of the pack
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	for _, name := range []string{HistoryFile, MarkdownFile} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
		if _, ok := index.Files[name]; ok {
			t.Errorf("%s is tracked in the index; it would be shipped inside the pack", name)
		}
	}
}

func TestPreviewChangesNothing(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")

	beforeChangelog := readFile(t, MarkdownFile)
	beforeHistory := readFile(t, HistoryFile)
	beforeIndex := readFile(t, "index.toml")
	beforePack := readFile(t, "pack.toml")

	// Both a client mod update and a brand new (unindexed) config file are pending
	writeMod(t, "Sodium", core.ClientSide, "0.5.8")
	writeConfig(t, "config/new.json", "{}")

	out := preview(t)
	for _, want := range []string{"Changes since 1.0.0; next version is 1.0.1 (patch bump)", "**Sodium** 0.5.7 → 0.5.8 (client)", "`config/new.json`"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q:\n%s", want, out)
		}
	}

	for name, before := range map[string]string{
		MarkdownFile: beforeChangelog, HistoryFile: beforeHistory, "index.toml": beforeIndex, "pack.toml": beforePack,
	} {
		if after := readFile(t, name); after != before {
			t.Errorf("preview modified %s:\n--- before\n%s\n--- after\n%s", name, before, after)
		}
	}
}

func TestReleaseNoticesFilesAddedSinceLastRefresh(t *testing.T) {
	// The index on disk knows nothing about this config file, as if "packwiz refresh" hadn't been run
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")
	writeConfig(t, "config/late.json", "{}")

	release(t, "")

	if got := packVersion(t); got != "1.0.1" {
		t.Errorf("pack.toml version = %q, want 1.0.1", got)
	}
	// ...and the index now matches what was released, so consumers of the pack get the file
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	if _, ok := index.Files["config/late.json"]; !ok {
		t.Errorf("index files = %v, want config/late.json to have been added by the release", index.Files)
	}
	if pack.Index.Hash == "" {
		t.Error("pack.toml index hash is empty after release; it should describe the rewritten index")
	}
}

func TestNoChangesSinceLastRelease(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")
	before := readFile(t, HistoryFile)

	for name, run := range map[string]func(*testing.T) string{
		"release": func(t *testing.T) string { return release(t, "") },
		"preview": preview,
	} {
		t.Run(name, func(t *testing.T) {
			if out := run(t); !strings.Contains(out, "No changes since the last release (1.0.0).") {
				t.Errorf("output = %q, want a no-changes message", out)
			}
		})
	}
	if got := readFile(t, HistoryFile); got != before {
		t.Errorf("history changed although there was nothing to release:\n%s", got)
	}
}

func TestEmptyPackHasNothingToRelease(t *testing.T) {
	setUpPack(t, "1.0.0")

	out := release(t, "")

	if !strings.Contains(out, "no files to release") {
		t.Errorf("output = %q, want a message that there is nothing to release", out)
	}
	for _, name := range []string{HistoryFile, MarkdownFile} {
		if _, err := os.Stat(name); err == nil {
			t.Errorf("%s was written for an empty pack", name)
		}
	}
}

func TestChangesToPinAndDependencyFlagsAreNotReleased(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")

	writeModFile(t, core.Mod{
		Name: "Sodium", FileName: "sodium-0.5.7.jar", Version: "0.5.7", Side: core.ClientSide,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-0.5.7"},
		Pin:      true, AddedAsDependency: true,
	})

	if out := preview(t); !strings.Contains(out, "No changes") {
		t.Errorf("preview = %q; pinning a mod doesn't change the pack, so there should be nothing to release", out)
	}
}

func TestModsWithoutRecordedVersionAreComparedByFileName(t *testing.T) {
	// Packs written before the version field existed must still get sensible changelogs
	setUpPack(t, "1.0.0")
	legacy := core.Mod{Name: "Legacy", FileName: "legacy-1.0.jar", Side: core.ClientSide, Download: core.ModDownload{HashFormat: "sha256", Hash: "h"}}
	writeModFile(t, legacy)
	release(t, "")

	legacy.FileName = "legacy-2.0.jar"
	writeModFile(t, legacy)
	release(t, "")

	latest, _ := loadHistory(t).Latest()
	if latest.Version != "1.0.1" || len(latest.Changes) != 1 {
		t.Fatalf("latest release = %+v, want 1.0.1 with the one update", latest)
	}
	if c := latest.Changes[0]; c.Kind != ModUpdated || c.From != "legacy-1.0.jar" || c.To != "legacy-2.0.jar" {
		t.Errorf("change = %+v, want an update from legacy-1.0.jar to legacy-2.0.jar", c)
	}
}

func TestReleaseVersionOverride(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")

	writeConfig(t, "config/a.json", "{}") // would be 1.0.1
	release(t, "1.5.0")

	if got := packVersion(t); got != "1.5.0" {
		t.Errorf("pack.toml version = %q, want the overridden 1.5.0", got)
	}
	latest, _ := loadHistory(t).Latest()
	if latest.Bump != BumpPatch {
		t.Errorf("Bump = %v, want the changes' own bump (patch) to be recorded honestly", latest.Bump)
	}

	// The next release continues from the overridden version
	writeConfig(t, "config/b.json", "{}")
	release(t, "")
	if got := packVersion(t); got != "1.5.1" {
		t.Errorf("pack.toml version = %q, want 1.5.1", got)
	}
}

func TestReleaseDeclinedChangesNothing(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	cmdtest.SetViperBool(t, "non-interactive", false)

	stdin, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatalf("failed to write answer: %v", err)
	}
	_ = w.Close()
	oldStdin := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	beforePack := readFile(t, "pack.toml")

	out := release(t, "")

	if !strings.Contains(out, "Cancelled!") {
		t.Errorf("output = %q, want Cancelled!", out)
	}
	for _, name := range []string{HistoryFile, MarkdownFile} {
		if _, err := os.Stat(name); err == nil {
			t.Errorf("%s was written although the release was declined", name)
		}
	}
	if got := readFile(t, "pack.toml"); got != beforePack {
		t.Errorf("pack.toml changed although the release was declined:\n%s", got)
	}
}

func TestReleaseRejectsBadVersionOverrideWithoutWriting(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")
	writeConfig(t, "config/a.json", "{}")
	before := readFile(t, HistoryFile)

	for _, override := range []string{"1.0.0", "0.9.0", "banana"} {
		var err error
		cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease(override) })
		if err == nil {
			t.Errorf("RunRelease(%q) returned no error", override)
		}
	}
	if got := readFile(t, HistoryFile); got != before {
		t.Errorf("history changed although every override was rejected:\n%s", got)
	}
}

func TestReleaseCanBeRerunAfterInterruption(t *testing.T) {
	// apply() writes the history last, so a run that dies before then leaves the release still pending
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")
	writeMod(t, "Lithium", core.ServerSide, "0.12.0")

	p, err := loadPending(true)
	if err != nil {
		t.Fatalf("loadPending() returned error: %v", err)
	}
	planned, err := p.plan("")
	if err != nil {
		t.Fatalf("plan() returned error: %v", err)
	}
	// Do everything apply() does except record the release, as if it had been killed just before the last write
	if err := p.index.Write(); err != nil {
		t.Fatalf("index.Write() returned error: %v", err)
	}
	p.pack.Version = planned.Version
	if err := p.pack.UpdateIndexHash(); err != nil {
		t.Fatalf("UpdateIndexHash() returned error: %v", err)
	}
	if err := p.pack.Write(); err != nil {
		t.Fatalf("pack.Write() returned error: %v", err)
	}

	release(t, "")

	if got := packVersion(t); got != "2.0.0" {
		t.Errorf("pack.toml version = %q, want 2.0.0 (the rerun must not skip or double-bump)", got)
	}
	if got := len(loadHistory(t).Releases); got != 2 {
		t.Errorf("history has %d releases, want 2", got)
	}
}

func TestPlan(t *testing.T) {
	client := Change{Kind: ModAdded, Side: core.ClientSide}
	server := Change{Kind: ModAdded, Side: core.ServerSide}
	config := Change{Kind: FileChanged}
	released := func(version string) History {
		return History{Releases: []Release{{Version: version}}}
	}

	tests := []struct {
		name        string
		packVersion string
		history     History
		changes     []Change
		override    string
		wantVersion string
		wantBump    Bump
		wantErrLike string
	}{
		{name: "first release adopts pack version", packVersion: "2.3.4", changes: []Change{client}, wantVersion: "2.3.4", wantBump: BumpNone},
		{name: "first release defaults to 1.0.0", changes: []Change{client}, wantVersion: "1.0.0", wantBump: BumpNone},
		{name: "first release accepts a v prefix", packVersion: "v0.1.0", changes: []Change{client}, wantVersion: "0.1.0", wantBump: BumpNone},
		{name: "first release with override", packVersion: "2.3.4", changes: []Change{client}, override: "3.0.0", wantVersion: "3.0.0", wantBump: BumpNone},
		{name: "first release with unusable pack version", packVersion: "beta", changes: []Change{client}, wantErrLike: "--version"},
		{name: "first release with unusable pack version but override", packVersion: "beta", changes: []Change{client}, override: "1.0.0", wantVersion: "1.0.0", wantBump: BumpNone},

		{name: "patch", history: released("1.2.3"), changes: []Change{config}, wantVersion: "1.2.4", wantBump: BumpPatch},
		{name: "minor", history: released("1.2.3"), changes: []Change{config, client}, wantVersion: "1.3.0", wantBump: BumpMinor},
		{name: "major", history: released("1.2.3"), changes: []Change{config, client, server}, wantVersion: "2.0.0", wantBump: BumpMajor},
		{name: "base is the last release not pack.toml", packVersion: "9.9.9", history: released("1.2.3"), changes: []Change{config}, wantVersion: "1.2.4", wantBump: BumpPatch},

		{name: "override above the bump", history: released("1.2.3"), changes: []Change{config}, override: "1.5.0", wantVersion: "1.5.0", wantBump: BumpPatch},
		{name: "override equal to last", history: released("1.2.3"), changes: []Change{config}, override: "1.2.3", wantErrLike: "must be greater"},
		{name: "override below last", history: released("1.2.3"), changes: []Change{config}, override: "1.2.2", wantErrLike: "must be greater"},
		{name: "override not a version", history: released("1.2.3"), changes: []Change{config}, override: "next", wantErrLike: "not a valid version"},
		{name: "corrupt last version", history: released("oops"), changes: []Change{config}, wantErrLike: "invalid version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pending{pack: core.Pack{Version: tt.packVersion}, history: tt.history, changes: tt.changes}

			got, err := p.plan(tt.override)

			if tt.wantErrLike != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrLike) {
					t.Fatalf("plan() error = %v, want one containing %q", err, tt.wantErrLike)
				}
				return
			}
			if err != nil {
				t.Fatalf("plan() returned error: %v", err)
			}
			if got.Version != tt.wantVersion || got.Bump != tt.wantBump {
				t.Errorf("plan() = {%s %v}, want {%s %v}", got.Version, got.Bump, tt.wantVersion, tt.wantBump)
			}
			if len(got.Changes) != len(tt.changes) {
				t.Errorf("plan() has %d changes, want %d", len(got.Changes), len(tt.changes))
			}
		})
	}
}

// writeUnversionedMod writes a mod as ones added before versions were recorded are: with no version, but with a
// source that can look it up.
func writeUnversionedMod(t *testing.T, src *cmdtest.VersionSource, name, side, versionID string) {
	t.Helper()
	writeModFile(t, core.Mod{
		Name: name, FileName: strings.ToLower(name) + "-" + versionID + ".jar", Side: side,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-" + versionID},
		Update:   src.UpdateData(versionID),
	})
}

// requireIndexUpToDate fails unless index.toml already describes every file as it is, which is what refreshing it
// would otherwise change.
func requireIndexUpToDate(t *testing.T) {
	t.Helper()
	before := readFile(t, "index.toml")
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	cmdtest.CaptureStdout(t, func() { err = index.Refresh() })
	if err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}
	if err := index.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if after := readFile(t, "index.toml"); after != before {
		t.Errorf("index.toml was out of date with the files:\n--- as left\n%s\n--- refreshed\n%s", before, after)
	}
}

func modVersion(t *testing.T, path string) string {
	t.Helper()
	mod, err := core.LoadMod(path)
	if err != nil {
		t.Fatalf("LoadMod(%s) returned error: %v", path, err)
	}
	return mod.Version
}

func TestReleaseLooksUpAndSavesMissingVersions(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7", "id-b": "0.12.0"})
	setUpPack(t, "1.0.0")
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	writeUnversionedMod(t, src, "Lithium", core.ServerSide, "id-b")

	release(t, "")

	// The changelog says which version, not which file
	changelog := readFile(t, MarkdownFile)
	for _, want := range []string{"**Sodium** 0.5.7 (client)", "**Lithium** 0.12.0 (server)"} {
		if !strings.Contains(changelog, want) {
			t.Errorf("CHANGELOG.md missing %q:\n%s", want, changelog)
		}
	}
	for _, name := range []string{"sodium-id-a.jar", "lithium-id-b.jar"} {
		if strings.Contains(changelog, name) {
			t.Errorf("CHANGELOG.md shows the file name %s instead of a version:\n%s", name, changelog)
		}
	}

	// ...and the version has been added to the mod's file, leaving the rest of it as it was
	if got := modVersion(t, "mods/sodium.pw.toml"); got != "0.5.7" {
		t.Errorf("sodium.pw.toml version = %q, want 0.5.7 saved", got)
	}
	if got := modVersion(t, "mods/lithium.pw.toml"); got != "0.12.0" {
		t.Errorf("lithium.pw.toml version = %q, want 0.12.0 saved", got)
	}
	mod, err := core.LoadMod("mods/sodium.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.FileName != "sodium-id-a.jar" || mod.Side != core.ClientSide || mod.Download.Hash != "hash-id-a" {
		t.Errorf("mod = %+v, want everything but the version kept", mod)
	}
	if data, ok := mod.GetParsedUpdateData("testsource"); !ok || data != "id-a" {
		t.Errorf("update data = %v, %v, want the source's version ID kept", data, ok)
	}

	// The index describes the files as they are now, and the release remembers versions rather than file names
	requireIndexUpToDate(t)
	if got := loadHistory(t).Snapshot.Mods["mods/sodium.pw.toml"].Version; got != "0.5.7" {
		t.Errorf("snapshot has Sodium as %q, want 0.5.7", got)
	}
	if src.Calls != 1 {
		t.Errorf("source was called %d times, want the mods looked up together in one call", src.Calls)
	}
}

func TestVersionsAreOnlyLookedUpOnce(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	setUpPack(t, "1.0.0")
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	release(t, "")

	writeConfig(t, "config/a.json", "{}")
	release(t, "")
	preview(t)

	if src.Calls != 1 {
		t.Errorf("source was called %d times, want the version not asked for again once it is saved", src.Calls)
	}
}

func TestModsWithVersionsNeverAskTheSource(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "should not be used"})
	setUpPack(t, "1.0.0")
	writeModFile(t, core.Mod{
		Name: "Sodium", FileName: "sodium.jar", Version: "0.5.7", Side: core.ClientSide,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "h"}, Update: src.UpdateData("id-a"),
	})

	release(t, "")

	if src.Calls != 0 {
		t.Errorf("source was called %d times for a mod that records its version", src.Calls)
	}
	if !strings.Contains(readFile(t, MarkdownFile), "**Sodium** 0.5.7 (client)") {
		t.Error("the recorded version wasn't used")
	}
}

func TestPreviewShowsLookedUpVersionsButSavesNothing(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	setUpPack(t, "1.0.0")
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	modBefore, indexBefore, packBefore := readFile(t, "mods/sodium.pw.toml"), readFile(t, "index.toml"), readFile(t, "pack.toml")

	out := preview(t)

	if !strings.Contains(out, "**Sodium** 0.5.7 (client)") || strings.Contains(out, "sodium-id-a.jar") {
		t.Errorf("preview should show the looked-up version, not the file name:\n%s", out)
	}
	if !strings.Contains(out, `Note: the versions of 1 mod that don't record one were looked up and will be saved by "packwiz changelog release".`) {
		t.Errorf("preview should say the versions aren't saved yet:\n%s", out)
	}
	for name, before := range map[string]string{"mods/sodium.pw.toml": modBefore, "index.toml": indexBefore, "pack.toml": packBefore} {
		if after := readFile(t, name); after != before {
			t.Errorf("preview modified %s:\n%s", name, after)
		}
	}
	if _, err := os.Stat(HistoryFile); err == nil {
		t.Error("preview wrote a history file")
	}
}

func TestReleaseWritesNothingWhenVersionsCannotBeLookedUp(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	src.Err = errors.New("network is down")
	setUpPack(t, "1.0.0")
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	modBefore, packBefore := readFile(t, "mods/sodium.pw.toml"), readFile(t, "pack.toml")

	var err error
	cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease("") })

	// Releasing with file names would put them in the history for good
	if err == nil || !strings.Contains(err.Error(), "couldn't look up") || !strings.Contains(err.Error(), "network is down") {
		t.Errorf("RunRelease() error = %v, want one saying the versions couldn't be looked up and why", err)
	}
	for _, name := range []string{HistoryFile, MarkdownFile} {
		if _, statErr := os.Stat(name); statErr == nil {
			t.Errorf("%s was written although the release failed", name)
		}
	}
	if readFile(t, "mods/sodium.pw.toml") != modBefore || readFile(t, "pack.toml") != packBefore {
		t.Error("files were modified although the release failed")
	}
}

func TestPreviewCarriesOnWhenVersionsCannotBeLookedUp(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	src.Err = errors.New("network is down")
	setUpPack(t, "1.0.0")
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")

	out := preview(t)

	for _, want := range []string{"Warning: couldn't look up the versions of mods that don't record one", "network is down", "sodium-id-a.jar"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview output missing %q:\n%s", want, out)
		}
	}
}

func TestModsTheSourceDoesNotKnowKeepTheirFileName(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	setUpPack(t, "1.0.0")
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	writeUnversionedMod(t, src, "Ghost", core.ClientSide, "id-gone") // the source has never heard of this one

	release(t, "")

	changelog := readFile(t, MarkdownFile)
	if !strings.Contains(changelog, "**Sodium** 0.5.7 (client)") || !strings.Contains(changelog, "**Ghost** ghost-id-gone.jar (client)") {
		t.Errorf("CHANGELOG.md should show a version where there is one and the file name where there isn't:\n%s", changelog)
	}
	if got := modVersion(t, "mods/ghost.pw.toml"); got != "" {
		t.Errorf("ghost.pw.toml version = %q, want none saved for a mod nothing is known about", got)
	}
}

// The state of a pack that was released before versions were looked up: mods with no version, recorded in the
// history by file name. The mods now have a source that can say what version they are.
func legacyPack(t *testing.T) *cmdtest.VersionSource {
	t.Helper()
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	setUpPack(t, "1.0.0")
	// With no source to ask, the first release can only record file names
	for _, mod := range []core.Mod{
		{Name: "Sodium", FileName: "sodium-id-a.jar", Side: core.ClientSide},
		{Name: "Lithium", FileName: "lithium-id-b.jar", Side: core.ServerSide},
	} {
		mod.Download = core.ModDownload{HashFormat: "sha256", Hash: "h-" + mod.Name}
		writeModFile(t, mod)
	}
	release(t, "")
	if got := readFile(t, MarkdownFile); !strings.Contains(got, "sodium-id-a.jar") {
		t.Fatalf("the fixture should start with file names in the changelog:\n%s", got)
	}

	// Now they have a source to look up, which knows their versions
	src.Versions = map[string]string{"id-a": "0.5.7", "id-b": "0.12.0"}
	for _, mod := range []core.Mod{
		{Name: "Sodium", FileName: "sodium-id-a.jar", Side: core.ClientSide, Update: src.UpdateData("id-a")},
		{Name: "Lithium", FileName: "lithium-id-b.jar", Side: core.ServerSide, Update: src.UpdateData("id-b")},
	} {
		mod.Download = core.ModDownload{HashFormat: "sha256", Hash: "h-" + mod.Name}
		writeModFile(t, mod)
	}
	return src
}

func TestFillingInVersionsIsNotAnUpdate(t *testing.T) {
	legacyPack(t)

	// Nothing about the pack has changed. If looking up Lithium's version were taken for an update, this would want
	// a major release.
	out := preview(t)

	if !strings.Contains(out, "No changes since the last release (1.0.0).") {
		t.Errorf("preview should find nothing to release:\n%s", out)
	}
	if strings.Contains(out, "next version") {
		t.Errorf("preview wants a release for versions that were only looked up:\n%s", out)
	}
}

func TestReleaseWithNothingNewStillSavesVersionsAndFixesTheChangelog(t *testing.T) {
	src := legacyPack(t)

	out := release(t, "")

	// No release is made, but the versions are saved and the changelog shows them
	for _, want := range []string{
		"No changes since the last release (1.0.0).",
		"Recorded the versions of 2 mods and updated 2 lines in the changelog.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := len(loadHistory(t).Releases); got != 1 {
		t.Errorf("history has %d releases, want still 1; nothing was released", got)
	}
	if got := packVersion(t); got != "1.0.0" {
		t.Errorf("pack.toml version = %q, want it left at 1.0.0", got)
	}
	if got := modVersion(t, "mods/sodium.pw.toml"); got != "0.5.7" {
		t.Errorf("sodium.pw.toml version = %q, want 0.5.7 saved", got)
	}
	changelog := readFile(t, MarkdownFile)
	for _, want := range []string{"**Sodium** 0.5.7 (client)", "**Lithium** 0.12.0 (server)"} {
		if !strings.Contains(changelog, want) {
			t.Errorf("CHANGELOG.md missing %q:\n%s", want, changelog)
		}
	}
	if strings.Contains(changelog, "sodium-id-a.jar") || strings.Contains(changelog, "lithium-id-b.jar") {
		t.Errorf("CHANGELOG.md still shows file names:\n%s", changelog)
	}
	requireIndexUpToDate(t)

	// It is done once: there's nothing left to fix, so the next run leaves everything as it is
	before := readFile(t, HistoryFile)
	again := release(t, "")
	if strings.Contains(again, "Recorded") {
		t.Errorf("a second release saved versions again:\n%s", again)
	}
	if readFile(t, HistoryFile) != before {
		t.Error("the history changed on a run that had nothing to fix")
	}
	// The versions were asked for once, to record them, and not again now they are saved
	if src.Calls != 1 {
		t.Errorf("source was called %d times, want 1", src.Calls)
	}
}

func TestOldChangelogLinesAreFixedAlongsideARealRelease(t *testing.T) {
	legacyPack(t)
	writeConfig(t, "config/sodium.json", "{}")

	release(t, "")

	// Only the config file changed since the last release: a patch, not the major release that a server mod
	// "updating" from its file name to its version would have made
	if got := packVersion(t); got != "1.0.1" {
		t.Errorf("pack.toml version = %q, want 1.0.1", got)
	}
	changelog := readFile(t, MarkdownFile)
	for _, want := range []string{"## 1.0.1", "`config/sodium.json`", "**Sodium** 0.5.7 (client)", "**Lithium** 0.12.0 (server)"} {
		if !strings.Contains(changelog, want) {
			t.Errorf("CHANGELOG.md missing %q:\n%s", want, changelog)
		}
	}
	if strings.Contains(changelog, "sodium-id-a.jar") || strings.Contains(changelog, "lithium-id-b.jar") {
		t.Errorf("the 1.0.0 entry still shows file names:\n%s", changelog)
	}
	if strings.Contains(changelog, "Server update required") {
		t.Errorf("CHANGELOG.md asks for a server update that no release made:\n%s", changelog)
	}
}
