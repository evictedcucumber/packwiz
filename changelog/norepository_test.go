package changelog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// withoutRepository puts the pack somewhere that isn't in a repository for the rest of the test, as git's OpenRepository
// finds when there is none.
func withoutRepository(t *testing.T) {
	t.Helper()
	old := OpenRepository
	OpenRepository = func() (Repository, error) {
		return nil, NoRepository("/pack isn't inside a git repository; run \"git init\" first")
	}
	t.Cleanup(func() { OpenRepository = old })
}

// releasedOnceWithoutARepository is a pack that has had its first release, made from its contents, with no repository.
func releasedOnceWithoutARepository(t *testing.T) {
	t.Helper()
	setUpPack(t, "1.0.0")
	withoutRepository(t)
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	writeMod(t, "Lithium", core.ServerSide, "0.12.0")
	writeConfig(t, "config/sodium.json", "{}")
	release(t, "")
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestNoRepositoryIsRecognisedWhateverItSays(t *testing.T) {
	err := NoRepository("somewhere isn't inside a git repository")

	if !errors.Is(err, ErrNoRepository) {
		t.Errorf("NoRepository() = %v, want an error that is ErrNoRepository", err)
	}
	if !errors.Is(fmt.Errorf("opening it: %w", err), ErrNoRepository) {
		t.Error("NoRepository() isn't recognised once wrapped")
	}
	if err.Error() != "somewhere isn't inside a git repository" {
		t.Errorf("message = %q, want the reason as it was given", err)
	}
	if errors.Is(errors.New("git failed"), ErrNoRepository) {
		t.Error("an error that says nothing about there being no repository is ErrNoRepository")
	}
}

func TestWithNoVersionControlSetUpThereIsNoRepository(t *testing.T) {
	// The changelog package doesn't set one up itself; git does that
	if _, err := OpenRepository(); !errors.Is(err, ErrNoRepository) {
		t.Errorf("OpenRepository() error = %v, want ErrNoRepository so a changelog is made without one", err)
	}
}

func TestFirstReleaseWithoutARepositoryDescribesThePackAndKeepsIt(t *testing.T) {
	setUpPack(t, "1.0.0")
	withoutRepository(t)
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	writeMod(t, "Lithium", core.ServerSide, "0.12.0")
	writeConfig(t, "config/sodium.json", "{}")

	out := release(t, "")

	for _, want := range []string{
		`Not reading the git log (/pack isn't inside a git repository; run "git init" first).`,
		"First release; version is 1.0.0", "Released 1.0.0!",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	history := loadHistory(t)
	if len(history.Releases) != 1 {
		t.Fatalf("history has %d releases, want 1", len(history.Releases))
	}
	first := history.Releases[0]
	if first.Version != "1.0.0" || first.Bump != BumpNone || first.Commit != "" {
		t.Errorf("release = {%s %v commit %q}, want {1.0.0 none, made at no commit}", first.Version, first.Bump, first.Commit)
	}
	if len(first.Changes) != 3 {
		t.Errorf("first release has %d changes, want 3 (two mods and a config file): %+v", len(first.Changes), first.Changes)
	}
	// It keeps the pack as it was, for the next release to be compared with
	snap := history.Snapshot
	if snap == nil {
		t.Fatal("the release kept no snapshot, so the next one has nothing to compare the pack with")
	}
	if got := snap.Mods["mods/sodium.pw.toml"]; got.Name != "Sodium" || got.Side != core.ClientSide || got.Version != "0.5.7" {
		t.Errorf("snapshot has Sodium as %+v", got)
	}
	if got := snap.Mods["mods/lithium.pw.toml"]; got.Version != "0.12.0" {
		t.Errorf("snapshot has Lithium as %+v", got)
	}
	if got := snap.Files["config/sodium.json"]; got != sha256Hex("{}") {
		t.Errorf("snapshot has config/sodium.json as %q, want its hash", got)
	}
	if got := packVersion(t); got != "1.0.0" {
		t.Errorf("pack.toml version = %q, want 1.0.0", got)
	}
	if changelog := readFile(t, MarkdownFile); !strings.Contains(changelog, "## 1.0.0 - 2026-09-18") || !strings.Contains(changelog, "**Sodium** 0.5.7 (client)") {
		t.Errorf("CHANGELOG.md doesn't describe the release:\n%s", changelog)
	}
}

func TestReleaseWithoutARepositoryWritesTheIndexThePackDescribes(t *testing.T) {
	// The index was refreshed to find what to release, and pack.toml, which is what consumers of the pack see the
	// version in, has to describe the one on disk
	setUpPack(t, "1.0.0")
	withoutRepository(t)
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")

	release(t, "")

	index := readFile(t, "index.toml")
	if !strings.Contains(index, "mods/sodium.pw.toml") || !strings.Contains(index, sha256Hex(readFile(t, "mods/sodium.pw.toml"))) {
		t.Errorf("index.toml doesn't list the mod as it is:\n%s", index)
	}
	if pack := readFile(t, "pack.toml"); !strings.Contains(pack, sha256Hex(index)) {
		t.Errorf("pack.toml doesn't have the hash of the index as written:\n%s", pack)
	}
}

func TestWithoutARepositoryAReleaseIsWhatChangedSinceTheLastOne(t *testing.T) {
	releasedOnceWithoutARepository(t)

	steps := []struct {
		name     string
		change   func()
		wantVer  string
		wantBump Bump
		want     Change
	}{
		{
			"client mod added is minor", func() { writeMod(t, "Iris", core.ClientSide, "1.0") },
			"1.1.0", BumpMinor, Change{Kind: ModAdded, Path: "mods/iris.pw.toml", Name: "Iris", Side: core.ClientSide, To: "1.0"},
		},
		{
			"client mod updated is patch", func() { writeMod(t, "Iris", core.ClientSide, "1.1") },
			"1.1.1", BumpPatch, Change{Kind: ModUpdated, Path: "mods/iris.pw.toml", Name: "Iris", Side: core.ClientSide, From: "1.0", To: "1.1"},
		},
		{
			"config changed is patch", func() { writeConfig(t, "config/sodium.json", `{"a": 1}`) },
			"1.1.2", BumpPatch, Change{Kind: FileChanged, Path: "config/sodium.json"},
		},
		{
			"config added is patch", func() { writeConfig(t, "config/iris.json", "{}") },
			"1.1.3", BumpPatch, Change{Kind: FileAdded, Path: "config/iris.json"},
		},
		{
			"config removed is patch", func() { remove(t, "config/iris.json") },
			"1.1.4", BumpPatch, Change{Kind: FileRemoved, Path: "config/iris.json"},
		},
		{
			"client mod removed is minor", func() { remove(t, "mods/iris.pw.toml") },
			"1.2.0", BumpMinor, Change{Kind: ModRemoved, Path: "mods/iris.pw.toml", Name: "Iris", Side: core.ClientSide, From: "1.1"},
		},
		{
			"server mod updated is major", func() { writeMod(t, "Lithium", core.ServerSide, "0.12.1") },
			"2.0.0", BumpMajor, Change{Kind: ModUpdated, Path: "mods/lithium.pw.toml", Name: "Lithium", Side: core.ServerSide, From: "0.12.0", To: "0.12.1"},
		},
		{
			"both mod added is major", func() { writeMod(t, "Ferrite", core.UniversalSide, "6.0") },
			"3.0.0", BumpMajor, Change{Kind: ModAdded, Path: "mods/ferrite.pw.toml", Name: "Ferrite", Side: core.UniversalSide, To: "6.0"},
		},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			step.change()

			release(t, "")

			if got := packVersion(t); got != step.wantVer {
				t.Errorf("pack.toml version = %q, want %q", got, step.wantVer)
			}
			history := loadHistory(t)
			latest, _ := history.Latest()
			if latest.Version != step.wantVer || latest.Bump != step.wantBump || latest.Commit != "" {
				t.Errorf("latest release = {%s %v commit %q}, want {%s %v} made at no commit", latest.Version, latest.Bump, latest.Commit, step.wantVer, step.wantBump)
			}
			// Only what changed since the release before, not what an earlier one already said
			if len(latest.Changes) != 1 || latest.Changes[0] != step.want {
				t.Errorf("changes = %+v, want just %+v", latest.Changes, step.want)
			}
			if history.Snapshot == nil {
				t.Error("the release kept no snapshot for the next one")
			}
		})
	}

	// The rendered changelog has every release, newest first
	changelog := readFile(t, MarkdownFile)
	prev := -1
	for _, v := range []string{"3.0.0", "2.0.0", "1.2.0", "1.1.4", "1.1.3", "1.1.2", "1.1.1", "1.1.0", "1.0.0"} {
		i := strings.Index(changelog, "## "+v+" ")
		if i < 0 {
			t.Fatalf("CHANGELOG.md is missing release %s:\n%s", v, changelog)
		}
		if i < prev {
			t.Errorf("CHANGELOG.md lists %s above a newer release; it should be newest first", v)
		}
		prev = i
	}
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("failed to remove %s: %v", path, err)
	}
}

func TestWithoutARepositoryTheMostSignificantChangeSetsTheVersion(t *testing.T) {
	releasedOnceWithoutARepository(t)
	writeConfig(t, "config/a.json", "{}")
	writeMod(t, "Sodium", core.ClientSide, "0.5.8")
	writeMod(t, "Zoom", core.ClientSide, "1.0")
	writeMod(t, "Lithium", core.ServerSide, "0.13.0")

	release(t, "")

	latest, _ := loadHistory(t).Latest()
	if latest.Version != "2.0.0" || latest.Bump != BumpMajor || len(latest.Changes) != 4 {
		t.Errorf("latest release = {%s %v, %d changes}, want {2.0.0 major, 4 changes}: %+v", latest.Version, latest.Bump, len(latest.Changes), latest.Changes)
	}
}

func TestWithoutARepositoryNothingChangedIsNothingToRelease(t *testing.T) {
	releasedOnceWithoutARepository(t)
	before := releaseFiles(t)
	indexBefore := readFile(t, "index.toml")

	out := release(t, "")

	if !strings.Contains(out, "No changes since the last release (1.0.0).") {
		t.Errorf("output missing the no-changes message:\n%s", out)
	}
	requireUnchanged(t, before)
	if readFile(t, "index.toml") != indexBefore {
		t.Error("index.toml was rewritten although there was nothing to release")
	}
}

func TestWithoutARepositoryPreviewShowsWhatChangedAndWritesNothing(t *testing.T) {
	releasedOnceWithoutARepository(t)
	writeMod(t, "Lithium", core.ServerSide, "0.12.1")
	before, indexBefore := releaseFiles(t), readFile(t, "index.toml")

	out := preview(t)

	for _, want := range []string{"Not reading the git log", "Changes since 1.0.0; next version is 2.0.0", "**Lithium** 0.12.0 → 0.12.1 (server)"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview output missing %q:\n%s", want, out)
		}
	}
	requireUnchanged(t, before)
	if readFile(t, "index.toml") != indexBefore {
		t.Error("preview rewrote index.toml")
	}
}

func TestWithoutARepositoryDeclinedReleaseWritesNothing(t *testing.T) {
	releasedOnceWithoutARepository(t)
	writeMod(t, "Lithium", core.ServerSide, "0.12.1")
	before, indexBefore := releaseFiles(t), readFile(t, "index.toml")
	cmdtest.SetViperBool(t, "non-interactive", false)
	cmdtest.SetStdin(t, "n\n")

	var released bool
	var err error
	out := cmdtest.CaptureStdout(t, func() { _, released, err = RunRelease("", "") })

	if err != nil || released || !strings.Contains(out, "Cancelled!") {
		t.Errorf("RunRelease() = released %v, error %v, output %q, want it declined", released, err, out)
	}
	requireUnchanged(t, before)
	if readFile(t, "index.toml") != indexBefore {
		t.Error("declining rewrote index.toml")
	}
}

func TestWithoutARepositoryAReleaseSavesTheVersionsItLooksUp(t *testing.T) {
	// With a repository, committing saves them; nothing else does when there isn't one
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	setUpPack(t, "1.0.0")
	withoutRepository(t)
	writeUnversionedMod(t, src, "Sodium", core.ClientSide, "id-a")

	release(t, "")

	mod, err := core.LoadMod("mods/sodium.pw.toml")
	if err != nil || mod.Version != "0.5.7" {
		t.Errorf("Sodium's version = %q, %v, want 0.5.7 saved in its file", mod.Version, err)
	}
	if index := readFile(t, "index.toml"); !strings.Contains(index, sha256Hex(readFile(t, "mods/sodium.pw.toml"))) {
		t.Errorf("index.toml has the mod as it was before its version was saved:\n%s", index)
	}
	if snap := loadHistory(t).Snapshot; snap == nil || snap.Mods["mods/sodium.pw.toml"].Version != "0.5.7" {
		t.Errorf("snapshot = %+v, want Sodium at its version rather than its file name", snap)
	}

	// So it isn't looked up again, and isn't mistaken for a change
	calls := src.Calls
	out := release(t, "")
	if src.Calls != calls || !strings.Contains(out, "No changes since the last release (1.0.0).") {
		t.Errorf("the second release looked up %d more versions, output:\n%s", src.Calls-calls, out)
	}
}

func TestWithoutARepositoryAnOlderHistoryIsFollowedFromItsSnapshot(t *testing.T) {
	// Histories written before the changelog was read from the log kept the pack as of the last release
	setUpPack(t, "1.0.0")
	withoutRepository(t)
	if err := os.WriteFile(HistoryFile, []byte(olderHistory), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	writeMod(t, "Sodium", core.ClientSide, "0.5.8")

	release(t, "")

	latest, _ := loadHistory(t).Latest()
	want := []Change{
		{Kind: FileRemoved, Path: "config/a.json"},
		{Kind: ModUpdated, Path: "mods/sodium.pw.toml", Name: "Sodium", Side: core.ClientSide, From: "0.5.7", To: "0.5.8"},
	}
	if latest.Version != "1.0.1" || len(latest.Changes) != 2 || latest.Changes[0] != want[0] || latest.Changes[1] != want[1] {
		t.Errorf("latest release = {%s %+v}, want 1.0.1 with %+v", latest.Version, latest.Changes, want)
	}
}

func TestWithoutARepositoryAReleaseMadeFromTheLogCantBeFollowed(t *testing.T) {
	// Nothing says what the pack was like then, and the log that would is out of reach. That is found out before the
	// pack is loaded, which can need the network
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	src.Err = errors.New("network is down")
	releasedOnce(t)
	writeUnversionedMod(t, src, "Zoom", core.ClientSide, "id-a")
	withoutRepository(t)
	before := releaseFiles(t)

	var releaseErr, previewErr error
	cmdtest.CaptureStdout(t, func() {
		_, _, releaseErr = RunRelease("", "")
		previewErr = runPreview("")
	})

	for name, err := range map[string]error{"release": releaseErr, "preview": previewErr} {
		if err == nil || !strings.Contains(err.Error(), "can't tell what has changed since release 1.0.0") || !strings.Contains(err.Error(), "isn't in a git repository") {
			t.Errorf("%s error = %v, want one saying what the last release was made from and why it can't be followed", name, err)
		}
	}
	if src.Calls != 0 {
		t.Errorf("%d versions were looked up before that was found out", src.Calls)
	}
	requireUnchanged(t, before)
}

func TestWithoutARepositorySinceIsAnError(t *testing.T) {
	tests := []struct {
		name  string
		setUp func(t *testing.T)
	}{
		{"before the first release", func(t *testing.T) { setUpPack(t, "1.0.0"); writeMod(t, "Sodium", core.ClientSide, "0.5.7") }},
		{"after a release", releasedOnceWithoutARepository},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
			tt.setUp(t)
			withoutRepository(t)
			writeUnversionedMod(t, src, "Zoom", core.ClientSide, "id-a")
			before := releaseFiles(t)

			var releaseErr, previewErr error
			cmdtest.CaptureStdout(t, func() {
				_, _, releaseErr = RunRelease("", "v1.0.0")
				previewErr = runPreview("v1.0.0")
			})

			for name, err := range map[string]error{"release": releaseErr, "preview": previewErr} {
				if err == nil || !strings.Contains(err.Error(), "--since") || !strings.Contains(err.Error(), "isn't in a git repository") {
					t.Errorf("%s error = %v, want one saying --since needs a git repository", name, err)
				}
			}
			if src.Calls != 0 {
				t.Errorf("%d versions were looked up before that was found out", src.Calls)
			}
			requireUnchanged(t, before)
		})
	}
}

func TestARepositoryTakesOverFromAReleaseMadeWithoutOne(t *testing.T) {
	releasedOnceWithoutARepository(t)
	// The pack is put in a repository, whose log begins with the commit of the whole pack, changelog.toml included
	repo := newFakeRepo(t)
	base := repo.commit("chore(pack): initial commit")
	repo.touched[HistoryFile] = base
	repo.commit("fix(config): change config/sodium.json")

	out := release(t, "")

	if strings.Contains(out, "Not reading the git log") {
		t.Errorf("output says the log isn't read although there is one:\n%s", out)
	}
	history := loadHistory(t)
	latest, _ := history.Latest()
	if latest.Version != "1.0.1" || latest.Commit != repo.commits[len(repo.commits)-1].hash {
		t.Errorf("latest release = {%s commit %q}, want 1.0.1 made at the last commit", latest.Version, latest.Commit)
	}
	// The log knows what the pack was like at each release from here on, so the old snapshot mustn't be left to be
	// compared with once it is out of date
	if history.Snapshot != nil {
		t.Errorf("snapshot = %+v, want it dropped now that releases are made from the log", history.Snapshot)
	}
	if data := readFile(t, HistoryFile); strings.Contains(data, "snapshot") {
		t.Errorf("changelog.toml still has a snapshot:\n%s", data)
	}
}

func TestReleaseWritesNothingWhenTheRepositoryCantBeOpened(t *testing.T) {
	// Only there being no repository is made up for; any other reason it can't be opened is an error
	setUpPack(t, "1.0.0")
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")
	old := OpenRepository
	OpenRepository = func() (Repository, error) { return nil, errors.New("git: detected dubious ownership in repository") }
	t.Cleanup(func() { OpenRepository = old })
	before := releaseFiles(t)

	var releaseErr, previewErr error
	out := cmdtest.CaptureStdout(t, func() {
		_, _, releaseErr = RunRelease("", "")
		previewErr = runPreview("")
	})

	for name, err := range map[string]error{"release": releaseErr, "preview": previewErr} {
		if err == nil || !strings.Contains(err.Error(), "dubious ownership") {
			t.Errorf("%s error = %v, want the reason the repository couldn't be opened", name, err)
		}
	}
	if strings.Contains(out, "Not reading the git log") {
		t.Errorf("output treats a repository that couldn't be opened as there being none:\n%s", out)
	}
	requireUnchanged(t, before)
}
