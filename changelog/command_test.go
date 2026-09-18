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

const serverFooter = "BREAKING CHANGE: changes mods that run on the server; servers must be updated to match"

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

// release runs "packwiz changelog release", returning what it printed
func release(t *testing.T, versionOverride string) string {
	t.Helper()
	return releaseSince(t, versionOverride, "")
}

func releaseSince(t *testing.T, versionOverride, since string) string {
	t.Helper()
	var err error
	out := cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease(versionOverride, since) })
	if err != nil {
		t.Fatalf("RunRelease() returned error: %v\noutput: %s", err, out)
	}
	return out
}

func preview(t *testing.T) string {
	t.Helper()
	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runPreview("") })
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

// releasedOnce is a pack that has had its first release, made from its contents, with a repository holding the commit
// that was made for it.
func releasedOnce(t *testing.T) *fakeRepo {
	t.Helper()
	setUpPack(t, "1.0.0")
	repo := newFakeRepo(t)
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")
	return repo
}

// releaseFiles is everything a release writes, for checking that a release that failed or didn't happen wrote nothing
func releaseFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{"pack.toml": readFile(t, "pack.toml")}
	for _, name := range []string{HistoryFile, MarkdownFile} {
		if data, err := os.ReadFile(name); err == nil {
			files[name] = string(data)
		}
	}
	return files
}

func requireUnchanged(t *testing.T, before map[string]string) {
	t.Helper()
	after := releaseFiles(t)
	for name, content := range before {
		if after[name] != content {
			t.Errorf("%s changed:\n--- before\n%s\n--- after\n%s", name, content, after[name])
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			t.Errorf("%s was written", name)
		}
	}
}

func TestFirstReleaseDescribesThePackAsItIs(t *testing.T) {
	setUpPack(t, "1.0.0")
	repo := newFakeRepo(t)
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
	// It is made at the commit that the pending changes were committed in, so it covers all of them
	if len(repo.commits) != 1 || first.Commit != repo.commits[0].hash {
		t.Errorf("release commit = %q, want the commit made for it (%v)", first.Commit, repo.commits)
	}
	if len(first.Changes) != 3 {
		t.Errorf("first release has %d changes, want 3 (two mods and a config file): %+v", len(first.Changes), first.Changes)
	}
	if got := packVersion(t); got != "1.0.0" {
		t.Errorf("pack.toml version = %q, want 1.0.0", got)
	}
	changelog := readFile(t, MarkdownFile)
	for _, want := range []string{"## 1.0.0 - 2026-09-18", "Initial release with 2 mods and 1 config file.", "**Sodium** 0.5.7 (client)", "**Lithium** 0.12.0 (server)", "`config/sodium.json`"} {
		if !strings.Contains(changelog, want) {
			t.Errorf("CHANGELOG.md missing %q:\n%s", want, changelog)
		}
	}
}

func TestFirstReleaseWithoutPackVersionStartsAtOne(t *testing.T) {
	setUpPack(t, "")
	newFakeRepo(t)
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")

	release(t, "")

	if got := packVersion(t); got != "1.0.0" {
		t.Errorf("pack.toml version = %q, want 1.0.0", got)
	}
}

func TestFirstReleaseIsMadeFromWhatIsInThePackNotFromTheLog(t *testing.T) {
	// The mods that were in the pack from the start are in the first commit, which doesn't list them, and commits
	// about a mod that has since gone would list one that isn't there
	setUpPack(t, "1.0.0")
	repo := newFakeRepo(t)
	repo.commit("chore(pack): initial commit")
	repo.commit("feat(mods): add Ghost 1.0 (client)")
	repo.commit("feat(mods): remove Ghost 1.0 (client)")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")

	release(t, "")

	changelog := readFile(t, MarkdownFile)
	if !strings.Contains(changelog, "**Sodium** 0.5.7 (client)") || strings.Contains(changelog, "Ghost") {
		t.Errorf("CHANGELOG.md should list what is in the pack, and only that:\n%s", changelog)
	}
}

func TestPendingChangesAreCommittedBeforeAnythingIsRead(t *testing.T) {
	repo := releasedOnce(t)
	repo.events = nil
	repo.pending = []Commit{commit("feat(mods)!: add Lithium 0.12.0 (server)", "", serverFooter)}

	release(t, "")

	// Both the log and the current commit are only known once the changes are in them
	if len(repo.events) == 0 || repo.events[0] != "CommitPending" {
		t.Errorf("events = %v, want CommitPending first", repo.events)
	}
	if got := packVersion(t); got != "2.0.0" {
		t.Errorf("pack.toml version = %q, want 2.0.0: the commit that was pending is part of the release", got)
	}
	latest, _ := loadHistory(t).Latest()
	if head := repo.commits[len(repo.commits)-1].hash; latest.Commit != head {
		t.Errorf("release commit = %q, want the commit that was made for the pending changes, %q", latest.Commit, head)
	}
	if !strings.Contains(readFile(t, MarkdownFile), "**Lithium** 0.12.0 (server)") {
		t.Error("CHANGELOG.md doesn't have the change that was pending")
	}
}

func TestReleaseStopsIfPendingChangesCantBeCommitted(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods): add Iris 1.0 (client)")
	repo.commitErr = errors.New("a hook said no")
	before := releaseFiles(t)

	var err error
	cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease("", "") })

	if err == nil || !strings.Contains(err.Error(), "a hook said no") {
		t.Errorf("RunRelease() error = %v, want the reason the commit failed", err)
	}
	requireUnchanged(t, before)
}

// Walks a pack through releases that each exercise one level of the bump policy, from the commits made between them
func TestReleasesBumpByTheMostSignificantCommit(t *testing.T) {
	repo := releasedOnce(t)

	steps := []struct {
		name     string
		commits  func()
		wantVer  string
		wantBump Bump
	}{
		{"client mod added is minor", func() { repo.commit("feat(mods): add Iris 1.0 (client)") }, "1.1.0", BumpMinor},
		{"client mod updated is patch", func() { repo.commit("fix(mods): update Iris 1.0 -> 1.1 (client)") }, "1.1.1", BumpPatch},
		{"config added is patch", func() { repo.commit("fix(config): add config/iris.json") }, "1.1.2", BumpPatch},
		{"config changed is patch", func() { repo.commit("fix(config): change config/iris.json") }, "1.1.3", BumpPatch},
		{"config removed is patch", func() { repo.commit("fix(config): remove config/iris.json") }, "1.1.4", BumpPatch},
		{"client mod removed is minor", func() { repo.commit("feat(mods): remove Iris 1.1 (client)") }, "1.2.0", BumpMinor},
		{"server mod added is major", func() { repo.commit("feat(mods)!: add Lithium 0.12.0 (server)", "", serverFooter) }, "2.0.0", BumpMajor},
		{"both mod added is major", func() { repo.commit("feat(mods)!: add Fabric 0.100 (both)", "", serverFooter) }, "3.0.0", BumpMajor},
		{"server mod updated is major", func() { repo.commit("feat(mods)!: update Lithium 0.12.0 -> 0.12.1 (server)", "", serverFooter) }, "4.0.0", BumpMajor},
		{"server mod removed is major", func() { repo.commit("feat(mods)!: remove Lithium 0.12.1 (server)", "", serverFooter) }, "5.0.0", BumpMajor},
		{"a feature written by hand is minor", func() { repo.commit("feat: add a splash screen") }, "5.1.0", BumpMinor},
		{"a fix written by hand is patch", func() { repo.commit("fix: correct a typo") }, "5.1.1", BumpPatch},
		{"a breaking change written by hand is major", func() { repo.commit("feat!: update to Minecraft 1.21.4") }, "6.0.0", BumpMajor},
		{
			"the most significant commit wins",
			func() {
				repo.commit("fix(config): add config/a.json")
				repo.commit("fix(mods): update Sodium 0.5.7 -> 0.5.8 (client)")
				repo.commit("feat(mods)!: add Lithium 0.13.0 (server)", "", serverFooter)
				repo.commit("feat(mods): add Zoom 1.0 (client)")
				repo.commit("chore(pack): update pack files")
			},
			"7.0.0", BumpMajor,
		},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			step.commits()
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
	for _, v := range []string{"7.0.0", "6.0.0", "5.1.1", "5.1.0", "5.0.0", "4.0.0", "3.0.0", "2.0.0", "1.2.0", "1.1.4", "1.1.3", "1.1.2", "1.1.1", "1.1.0", "1.0.0"} {
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
}

func TestEachReleaseCoversOnlyTheCommitsSinceTheLast(t *testing.T) {
	repo := releasedOnce(t)
	first, _ := loadHistory(t).Latest()

	repo.commit("feat(mods): add Iris 1.0 (client)")
	release(t, "")
	second, _ := loadHistory(t).Latest()
	repo.commit("fix(config): change config/a.json")
	release(t, "")
	third, _ := loadHistory(t).Latest()

	if len(third.Changes) != 1 || third.Changes[0].Kind != FileChanged {
		t.Errorf("the third release has %+v, want only the config change made since the second", third.Changes)
	}
	// Each was read from where the one before was made, which is recorded in it
	if len(repo.logSince) != 2 || repo.logSince[0] != first.Commit || repo.logSince[1] != second.Commit {
		t.Errorf("the log was read from %v, want %v", repo.logSince, []string{first.Commit, second.Commit})
	}
	if first.Commit == "" || second.Commit == first.Commit || third.Commit == second.Commit {
		t.Errorf("releases were made at %q, %q and %q, want a commit each", first.Commit, second.Commit, third.Commit)
	}
}

func TestAModAddedAndRemovedInOneReleaseChangesNothing(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods)!: add Lithium 0.12.0 (server)", "", serverFooter)
	repo.commit("feat(mods)!: remove Lithium 0.12.0 (server)", "", serverFooter)
	repo.commit("fix(config): change config/a.json")

	release(t, "")

	// Servers have nothing to update, as the mod was never there for any release, so it isn't a major release
	if got := packVersion(t); got != "1.0.1" {
		t.Errorf("pack.toml version = %q, want 1.0.1", got)
	}
	if strings.Contains(readFile(t, MarkdownFile), "Lithium") {
		t.Errorf("CHANGELOG.md mentions a mod that came and went:\n%s", readFile(t, MarkdownFile))
	}
}

func TestSeveralUpdatesToAModAreOneLine(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("fix(mods): update Sodium 0.5.7 -> 0.5.8 (client)")
	repo.commit("fix(mods): update Sodium 0.5.8 -> 0.5.9 (client)")

	release(t, "")

	changelog := readFile(t, MarkdownFile)
	if !strings.Contains(changelog, "- **Sodium** 0.5.7 → 0.5.9 (client)") || strings.Contains(changelog, "0.5.8") {
		t.Errorf("CHANGELOG.md should show one update from where the mod started to where it ended:\n%s", changelog)
	}
}

func TestNotesWrittenByHandAreInTheChangelog(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("fix(config): lower the particle count")
	repo.commit("feat!: update to Minecraft 1.21.4")
	repo.commit("feat(shaders): add a *fancy* pack")
	repo.commit("docs: explain the settings")
	repo.commit("Fix a typo") // not a conventional commit

	release(t, "")

	changelog := readFile(t, MarkdownFile)
	for _, want := range []string{
		"### Config\n\n- lower the particle count",
		"### Changes\n\n- **Breaking:** update to Minecraft 1.21.4\n- **shaders:** add a \\*fancy\\* pack",
	} {
		if !strings.Contains(changelog, want) {
			t.Errorf("CHANGELOG.md missing %q:\n%s", want, changelog)
		}
	}
	for _, unwanted := range []string{"explain the settings", "typo"} {
		if strings.Contains(changelog, unwanted) {
			t.Errorf("CHANGELOG.md has %q, which isn't a change to the pack:\n%s", unwanted, changelog)
		}
	}
	// A change that breaks compatibility isn't necessarily one to do with the server
	if strings.Contains(changelog, "Server update required") {
		t.Errorf("CHANGELOG.md asks for a server update, but no mod on the server changed:\n%s", changelog)
	}
	if got := packVersion(t); got != "2.0.0" {
		t.Errorf("pack.toml version = %q, want 2.0.0 for a breaking change", got)
	}
}

func TestPreviewIncludesChangesThatAreNotCommittedYetAndCommitsNothing(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("fix(config): change config/a.json")
	repo.pending = []Commit{commit("feat(mods)!: add Lithium 0.12.0 (server)", "", serverFooter)}
	before := releaseFiles(t)
	repo.events = nil // what the release that set this up asked for isn't the preview's

	out := preview(t)

	for _, want := range []string{"Changes since 1.0.0; next version is 2.0.0 (major bump)", "**Lithium** 0.12.0 (server)", "`config/a.json`"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q:\n%s", want, out)
		}
	}
	if repo.asked("CommitPending") || len(repo.pending) != 1 {
		t.Errorf("a preview committed something: events %v, pending %v", repo.events, repo.pending)
	}
	requireUnchanged(t, before)
}

func TestNothingToReleaseWhenNoCommitMakesOne(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("chore(pack): update pack files")
	repo.commit("chore(release): 1.0.0")
	repo.commit("docs: explain the config")
	repo.commit("Merge branch 'main'")
	before := releaseFiles(t)

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
	requireUnchanged(t, before)
}

func TestReleaseVersionOverride(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("fix(config): add config/a.json") // would be 1.0.1

	release(t, "1.5.0")

	if got := packVersion(t); got != "1.5.0" {
		t.Errorf("pack.toml version = %q, want the overridden 1.5.0", got)
	}
	latest, _ := loadHistory(t).Latest()
	if latest.Bump != BumpPatch {
		t.Errorf("Bump = %v, want the commits' own bump (patch) to be recorded honestly", latest.Bump)
	}

	// The next release continues from the overridden version
	repo.commit("fix(config): add config/b.json")
	release(t, "")
	if got := packVersion(t); got != "1.5.1" {
		t.Errorf("pack.toml version = %q, want 1.5.1", got)
	}
}

func TestReleaseRejectsBadVersionOverrideWithoutWriting(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("fix(config): add config/a.json")
	before := releaseFiles(t)

	for _, override := range []string{"1.0.0", "0.9.0", "banana"} {
		var err error
		cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease(override, "") })
		if err == nil {
			t.Errorf("RunRelease(%q) returned no error", override)
		}
	}
	requireUnchanged(t, before)
}

func TestReleaseDeclinedWritesNothing(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("fix(config): add config/a.json")
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
	before := releaseFiles(t)

	out := release(t, "")

	if !strings.Contains(out, "Cancelled!") {
		t.Errorf("output = %q, want Cancelled!", out)
	}
	requireUnchanged(t, before)
}

func TestReleaseCanBeRerunAfterInterruption(t *testing.T) {
	// The history is written last, so a run that dies before then leaves the release still to be made
	repo := releasedOnce(t)
	repo.commit("feat(mods)!: add Lithium 0.12.0 (server)", "", serverFooter)

	p, err := loadPending(repo, true, "", false)
	if err != nil {
		t.Fatalf("loadPending() returned error: %v", err)
	}
	planned, err := p.plan("")
	if err != nil {
		t.Fatalf("plan() returned error: %v", err)
	}
	// Do what saving does except for the last step, as if it had been killed just before it
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

func TestSinceSaysWhereToStartReading(t *testing.T) {
	repo := releasedOnce(t)
	added := repo.commit("feat(mods): add Iris 1.0 (client)")
	repo.commit("fix(mods): update Iris 1.0 -> 1.1 (client)")

	releaseSince(t, "", added)

	// Only what came after the commit that was named, whatever the last release says
	latest, _ := loadHistory(t).Latest()
	if latest.Version != "1.0.1" || len(latest.Changes) != 1 || latest.Changes[0].Kind != ModUpdated {
		t.Errorf("release = %s %+v, want 1.0.1 with the one update", latest.Version, latest.Changes)
	}
	if got := repo.logSince[len(repo.logSince)-1]; got != added {
		t.Errorf("the log was read from %q, want %q", got, added)
	}
}

// writeLegacyHistory is the release history of a pack whose last release didn't record the commit it was made at, as
// before that was kept, with mods recorded by file name
func writeLegacyHistory(t *testing.T, src *cmdtest.VersionSource) {
	t.Helper()
	h := History{Releases: []Release{{
		Version: "1.0.0", Date: "2026-01-01", Bump: BumpNone,
		Changes: []Change{
			{Kind: ModAdded, Path: "mods/sodium.pw.toml", Name: "Sodium", Side: core.ClientSide, To: "sodium-id-a.jar"},
			{Kind: ModAdded, Path: "mods/lithium.pw.toml", Name: "Lithium", Side: core.ServerSide, To: "lithium-id-b.jar"},
		},
	}}}
	if err := h.Write(historyPath()); err != nil {
		t.Fatalf("failed to write history: %v", err)
	}
	if err := writeFileAtomic(markdownPath(), []byte(RenderMarkdown(h.Releases))); err != nil {
		t.Fatalf("failed to write changelog: %v", err)
	}
	// The mods have a source now, which knows their versions
	for _, mod := range []core.Mod{
		{Name: "Sodium", FileName: "sodium-id-a.jar", Side: core.ClientSide, Update: src.UpdateData("id-a")},
		{Name: "Lithium", FileName: "lithium-id-b.jar", Side: core.ServerSide, Update: src.UpdateData("id-b")},
	} {
		mod.Download = core.ModDownload{HashFormat: "sha256", Hash: "h-" + mod.Name}
		writeModFile(t, mod)
	}
}

func TestAReleaseThatDidNotRecordItsCommitIsFoundFromTheHistoryFile(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7", "id-b": "0.12.0"})
	setUpPack(t, "1.0.0")
	repo := newFakeRepo(t)
	repo.commit("feat(mods): add Old 1 (client)") // from before the release, so not part of the next one
	released := repo.commit("chore(release): 1.0.0")
	repo.touched[HistoryFile] = released
	writeLegacyHistory(t, src)
	repo.commit("feat(mods): add Iris 1.0 (client)")

	release(t, "")

	latest, _ := loadHistory(t).Latest()
	if latest.Version != "1.1.0" || len(latest.Changes) != 1 || latest.Changes[0].Name != "Iris" {
		t.Errorf("release = %s %+v, want 1.1.0 with only Iris, which came after the release", latest.Version, latest.Changes)
	}
	if got := repo.logSince[len(repo.logSince)-1]; got != released {
		t.Errorf("the log was read from %q, want the commit that last changed %s, %q", got, HistoryFile, released)
	}
}

func TestReleaseAsksForSinceWhenItCannotFindWhereTheLastOneWasMade(t *testing.T) {
	tests := []struct {
		name  string
		setUp func(t *testing.T, repo *fakeRepo, src *cmdtest.VersionSource)
		since string
		want  []string
	}{
		{
			"a release that didn't record its commit, and a history file that was never committed",
			func(t *testing.T, repo *fakeRepo, src *cmdtest.VersionSource) {
				repo.commit("chore(pack): initial commit")
				writeLegacyHistory(t, src)
			},
			"", []string{"can't tell where release 1.0.0 was made", "--since"},
		},
		{
			"a commit that isn't in the repository any more",
			func(t *testing.T, repo *fakeRepo, src *cmdtest.VersionSource) {
				repo.commit("chore(pack): initial commit")
				writeLegacyHistory(t, src)
				h := loadHistory(t)
				h.Releases[0].Commit = "deadbeefdeadbeef"
				if err := h.Write(historyPath()); err != nil {
					t.Fatalf("failed to write history: %v", err)
				}
			},
			"", []string{"deadbeefdeadbeef", "--since"},
		},
		{
			"a commit that was named and isn't in the repository",
			func(t *testing.T, repo *fakeRepo, src *cmdtest.VersionSource) {
				repo.commit("chore(pack): initial commit")
				writeLegacyHistory(t, src)
			},
			"nonsense", []string{"nonsense", "--since"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7", "id-b": "0.12.0"})
			setUpPack(t, "1.0.0")
			repo := newFakeRepo(t)
			tt.setUp(t, repo, src)
			before := releaseFiles(t)

			var err error
			cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease("", tt.since) })

			if err == nil {
				t.Fatal("RunRelease() succeeded although it can't tell what to read the log from")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err, want)
				}
			}
			requireUnchanged(t, before)
		})
	}
}

func TestReleaseWritesNothingWithoutARepository(t *testing.T) {
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	old := OpenRepository
	OpenRepository = func() (Repository, error) { return nil, errors.New("not inside a git repository") }
	t.Cleanup(func() { OpenRepository = old })
	before := releaseFiles(t)

	var releaseErr, previewErr error
	cmdtest.CaptureStdout(t, func() {
		_, _, releaseErr = RunRelease("", "")
		previewErr = runPreview("")
	})

	for name, err := range map[string]error{"release": releaseErr, "preview": previewErr} {
		if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
			t.Errorf("%s error = %v, want the reason there is no repository", name, err)
		}
	}
	requireUnchanged(t, before)
}

func TestReleaseFilesAreNotPartOfThePack(t *testing.T) {
	releasedOnce(t)

	w, err := LoadWorking(true)
	if err != nil {
		t.Fatalf("LoadWorking() returned error: %v", err)
	}
	snap, err := w.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() returned error: %v", err)
	}

	// They sit in the pack directory, but aren't distributed with it, so aren't in what a release describes
	for _, name := range []string{HistoryFile, MarkdownFile} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
		if _, ok := snap.Files[name]; ok {
			t.Errorf("%s is part of the pack; it would be described as one of its files", name)
		}
	}
}

// --- Looking up the versions of mods that don't record one ---

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

func TestFirstReleaseShowsLookedUpVersionsAndLeavesSavingThemToTheCommit(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7", "id-b": "0.12.0"})
	setUpPack(t, "1.0.0")
	newFakeRepo(t)
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	writeUnversionedMod(t, src, "Lithium", core.ServerSide, "id-b")
	modBefore := readFile(t, "mods/sodium.pw.toml")

	release(t, "")

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
	// Saving them into the mods is what committing does, and that has been done by the time a release is made; the
	// release itself only reports
	if readFile(t, "mods/sodium.pw.toml") != modBefore {
		t.Error("the release modified a mod's file")
	}
}

func TestPreviewShowsLookedUpVersionsButSavesNothing(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	setUpPack(t, "1.0.0")
	newFakeRepo(t)
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	modBefore, indexBefore := readFile(t, "mods/sodium.pw.toml"), readFile(t, "index.toml")
	before := releaseFiles(t)

	out := preview(t)

	if !strings.Contains(out, "**Sodium** 0.5.7 (client)") || strings.Contains(out, "sodium-id-a.jar") {
		t.Errorf("preview should show the looked-up version, not the file name:\n%s", out)
	}
	if readFile(t, "mods/sodium.pw.toml") != modBefore || readFile(t, "index.toml") != indexBefore {
		t.Error("preview modified the pack")
	}
	requireUnchanged(t, before)
}

func TestReleaseWritesNothingWhenVersionsCannotBeLookedUp(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	src.Err = errors.New("network is down")
	setUpPack(t, "1.0.0")
	newFakeRepo(t)
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	before := releaseFiles(t)

	var err error
	cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease("", "") })

	// Releasing with file names would put them in the history for good
	if err == nil || !strings.Contains(err.Error(), "couldn't look up") || !strings.Contains(err.Error(), "network is down") {
		t.Errorf("RunRelease() error = %v, want one saying the versions couldn't be looked up and why", err)
	}
	requireUnchanged(t, before)
}

func TestPreviewCarriesOnWhenVersionsCannotBeLookedUp(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	src.Err = errors.New("network is down")
	setUpPack(t, "1.0.0")
	newFakeRepo(t)
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
	newFakeRepo(t)
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")
	writeUnversionedMod(t, src, "Ghost", core.ClientSide, "id-gone") // the source has never heard of this one

	release(t, "")

	changelog := readFile(t, MarkdownFile)
	if !strings.Contains(changelog, "**Sodium** 0.5.7 (client)") || !strings.Contains(changelog, "**Ghost** ghost-id-gone.jar (client)") {
		t.Errorf("CHANGELOG.md should show a version where there is one and the file name where there isn't:\n%s", changelog)
	}
}

func TestOldChangelogLinesAreFixedWhenThereIsNothingNewToRelease(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7", "id-b": "0.12.0"})
	setUpPack(t, "1.0.0")
	repo := newFakeRepo(t)
	repo.touched[HistoryFile] = repo.commit("chore(release): 1.0.0")
	writeLegacyHistory(t, src)
	if !strings.Contains(readFile(t, MarkdownFile), "sodium-id-a.jar") {
		t.Fatalf("the fixture should start with file names in the changelog:\n%s", readFile(t, MarkdownFile))
	}

	out := release(t, "")

	for _, want := range []string{"No changes since the last release (1.0.0).", "Updated 2 lines in the changelog."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := len(loadHistory(t).Releases); got != 1 {
		t.Errorf("history has %d releases, want still 1; nothing was released", got)
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

	// It is done once: there's nothing left to fix, so the next run leaves everything as it is
	before := readFile(t, HistoryFile)
	if again := release(t, ""); strings.Contains(again, "Updated") {
		t.Errorf("a second release changed the changelog again:\n%s", again)
	}
	if readFile(t, HistoryFile) != before {
		t.Error("the history changed on a run that had nothing to fix")
	}
}

func TestOldChangelogLinesAreFixedAlongsideARealRelease(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7", "id-b": "0.12.0"})
	setUpPack(t, "1.0.0")
	repo := newFakeRepo(t)
	repo.touched[HistoryFile] = repo.commit("chore(release): 1.0.0")
	writeLegacyHistory(t, src)
	repo.commit("fix(config): add config/sodium.json")

	release(t, "")

	// Only a config file changed, so this is a patch, and the 1.0.0 entry shows versions
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
		{name: "a note that breaks is major", history: released("1.2.3"), changes: []Change{config, {Kind: Note, Type: "feat", Breaking: true}}, wantVersion: "2.0.0", wantBump: BumpMajor},
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
