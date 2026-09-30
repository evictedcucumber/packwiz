package changelog

import (
	"errors"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestLoadPreviewDescribesTheNextReleaseAndSaysNothingOnTheTerminal(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods): add Lithium 0.12.0 (client)")
	writeMod(t, "Lithium", core.ClientSide, "0.12.0")

	var p Preview
	var err error
	out := cmdtest.CaptureStdout(t, func() { p, err = LoadPreview("", "") })

	if err != nil {
		t.Fatalf("LoadPreview() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("LoadPreview() wrote %q to the terminal", out)
	}
	if !p.InRepository || p.Last != "1.0.0" || !p.HasChanges || p.Release.Version != "1.1.0" || p.Release.Bump != BumpMinor {
		t.Errorf("the preview is %+v, want a minor release, 1.1.0, after 1.0.0, read from a repository", p)
	}
	text := ui.Strip(p.Text())
	for _, want := range []string{"Changes since 1.0.0; next version is 1.1.0 (minor bump)", "### Added"} {
		if !strings.Contains(text, want) {
			t.Errorf("the text doesn't have %q:\n%s", want, text)
		}
	}
}

func TestPreviewTextIsWhatTheCommandPrints(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods): add Lithium 0.12.0 (client)")
	writeMod(t, "Lithium", core.ClientSide, "0.12.0")

	p, err := LoadPreview("", "")
	if err != nil {
		t.Fatalf("LoadPreview() returned error: %v", err)
	}
	if printed := cmdtest.WithoutProgress(preview(t)); printed != p.Text() {
		t.Errorf("the command prints\n%q\nwant the same as the preview's text\n%q", printed, p.Text())
	}
}

func TestLoadPreviewWithAVersionOfYourOwn(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods): add Lithium 0.12.0 (client)")
	writeMod(t, "Lithium", core.ClientSide, "0.12.0")

	p, err := LoadPreview("", "3.0.0")
	if err != nil || p.Release.Version != "3.0.0" {
		t.Errorf("LoadPreview() = %+v, %v, want the version that was asked for", p.Release, err)
	}
	if _, err := LoadPreview("", "0.0.1"); err == nil || !strings.Contains(err.Error(), "must be greater") {
		t.Errorf("LoadPreview() with a version below the last returned %v, want an error that says it must be greater", err)
	}
}

func TestLoadPreviewSaysWhenThereIsNothingToRelease(t *testing.T) {
	releasedOnce(t)
	p, err := LoadPreview("", "")
	if err != nil {
		t.Fatalf("LoadPreview() returned error: %v", err)
	}
	if p.HasChanges || p.NoChanges != "No changes since the last release (1.0.0)." {
		t.Errorf("the preview is %+v, want it to say there are no changes", p)
	}
	if got := p.Text(); got != p.NoChanges+"\n" {
		t.Errorf("the text is %q, want the message", got)
	}
}

func TestLoadPreviewOfAPackThatIsNotInARepository(t *testing.T) {
	setUpPack(t, "1.0.0")
	old := OpenRepository
	OpenRepository = func() (Repository, error) { return nil, NoRepository("git isn't installed") }
	t.Cleanup(func() { OpenRepository = old })
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")

	var p Preview
	var err error
	out := cmdtest.CaptureStdout(t, func() { p, err = LoadPreview("", "") })
	if err != nil {
		t.Fatalf("LoadPreview() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("LoadPreview() wrote %q to the terminal", out)
	}
	if p.InRepository || p.Reason != "git isn't installed" || !p.HasChanges {
		t.Errorf("the preview is %+v, want a release without a repository, and why", p)
	}
	if len(p.Notices) != 0 {
		t.Errorf("the notices are %v, want what Reason says not to be said twice", p.Notices)
	}
	if !strings.Contains(ui.Strip(p.Text()), "First release; version is 1.0.0") {
		t.Errorf("the text is %q, want the first release", ui.Strip(p.Text()))
	}
}

func TestLoadPreviewFailsWhenTheRepositoryCannotBeOpened(t *testing.T) {
	setUpPack(t, "1.0.0")
	old := OpenRepository
	OpenRepository = func() (Repository, error) { return nil, errors.New("git refused") }
	t.Cleanup(func() { OpenRepository = old })
	if _, err := LoadPreview("", ""); err == nil || !strings.Contains(err.Error(), "git refused") {
		t.Errorf("LoadPreview() returned %v, want the error the repository gave", err)
	}
}

func TestLoadPreviewNotesThatPackTomlHasAnotherVersion(t *testing.T) {
	repo := releasedOnce(t)
	pack, _ := core.LoadPack()
	pack.Version = "7.0.0"
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	repo.commit("feat(mods): add Lithium 0.12.0 (client)")
	writeMod(t, "Lithium", core.ClientSide, "0.12.0")

	p, err := LoadPreview("", "")
	if err != nil {
		t.Fatalf("LoadPreview() returned error: %v", err)
	}
	if len(p.Notes) != 1 || !strings.Contains(p.Notes[0], "pack.toml has version 7.0.0, but the last release was 1.0.0") {
		t.Errorf("the notes are %v, want one that says pack.toml has another version", p.Notes)
	}
}

func TestMakeReleaseReleasesWithoutAskingOrSayingAnythingOnTheTerminal(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods): add Lithium 0.12.0 (client)")
	writeMod(t, "Lithium", core.ClientSide, "0.12.0")

	var made Released
	var err error
	out := cmdtest.CaptureStdout(t, func() { made, err = MakeRelease("", "") })

	if err != nil {
		t.Fatalf("MakeRelease() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("MakeRelease() wrote %q to the terminal", out)
	}
	if !made.Made || made.Release.Version != "1.1.0" {
		t.Errorf("the release is %+v, want 1.1.0 made", made)
	}
	if got := packVersion(t); got != "1.1.0" {
		t.Errorf("pack.toml's version is %q, want the release's", got)
	}
	if h := loadHistory(t); len(h.Releases) != 2 {
		t.Errorf("the history has %d releases, want the new one recorded", len(h.Releases))
	}
	if !repo.asked("CommitPending") {
		t.Error("the changes weren't committed first")
	}
}

func TestMakeReleaseSaysWhenThereIsNothingToRelease(t *testing.T) {
	releasedOnce(t)
	made, err := MakeRelease("", "")
	if err != nil {
		t.Fatalf("MakeRelease() returned error: %v", err)
	}
	if made.Made || made.NoChanges != "No changes since the last release (1.0.0)." {
		t.Errorf("the release is %+v, want nothing made, and why", made)
	}
}

func TestMakeReleaseFailsWithAVersionNotAboveTheLast(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods): add Lithium 0.12.0 (client)")
	writeMod(t, "Lithium", core.ClientSide, "0.12.0")
	before := releaseFiles(t)
	if _, err := MakeRelease("0.0.1", ""); err == nil {
		t.Fatal("MakeRelease() returned no error for a version below the last release")
	}
	requireUnchanged(t, before)
}

// A repository that can say which commits it makes has them told, and is not made to print them
type reportingRepo struct {
	*fakeRepo
	reported []string
}

func (r *reportingRepo) CommitPendingReporting(report func(string)) error {
	r.events = append(r.events, "CommitPendingReporting")
	report("chore(pack): from the repository")
	// What it says it committed is what the repository has
	return r.fakeRepo.CommitPending()
}

func TestMakeReleaseHasTheRepositoryTellWhichCommitsItMade(t *testing.T) {
	setUpPack(t, "1.0.0")
	repo := &reportingRepo{fakeRepo: &fakeRepo{touched: map[string]string{}}}
	old := OpenRepository
	OpenRepository = func() (Repository, error) { return repo, nil }
	t.Cleanup(func() { OpenRepository = old })
	writeMod(t, "Sodium", core.ClientSide, "0.5.7")

	made, err := MakeRelease("", "")
	if err != nil {
		t.Fatalf("MakeRelease() returned error: %v", err)
	}
	if len(made.Committed) != 1 || made.Committed[0] != "chore(pack): from the repository" {
		t.Errorf("the commits are %q, want what the repository reported", made.Committed)
	}
	if len(repo.events) == 0 || repo.events[0] != "CommitPendingReporting" {
		t.Errorf("the repository was asked %v, want it asked to report first, as CommitPending would print what it commits", repo.events)
	}
}

func TestSaveMarkdownWritesTheChangelogAndSaysNothing(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods): add Lithium 0.12.0 (client)")
	writeMod(t, "Lithium", core.ClientSide, "0.12.0")

	var path string
	var err error
	out := cmdtest.CaptureStdout(t, func() { path, _, err = SaveMarkdown() })
	if err != nil {
		t.Fatalf("SaveMarkdown() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("SaveMarkdown() wrote %q to the terminal", out)
	}
	text := readFile(t, MarkdownFile)
	if path != MarkdownFile || !strings.Contains(text, "## 1.0.0") {
		t.Errorf("SaveMarkdown() = %q, want %s written from the history", path, MarkdownFile)
	}
	// What hasn't been released is listed above what has
	if !strings.Contains(text, "Unreleased") || !strings.Contains(text, "Lithium") {
		t.Errorf("the changelog doesn't list the change that isn't released yet:\n%s", text)
	}
}

func TestSaveMarkdownSaysWhenItCannotListWhatIsNotReleased(t *testing.T) {
	releasedOnceWithoutARepository(t)
	// Released from the log, so there is nothing to compare the pack with without one
	h := loadHistory(t)
	h.Snapshot = nil
	if err := h.Write(historyPath()); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	var notices []string
	var err error
	out := cmdtest.CaptureStdout(t, func() { _, notices, err = SaveMarkdown() })
	if err != nil {
		t.Fatalf("SaveMarkdown() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("SaveMarkdown() wrote %q to the terminal", out)
	}
	joined := strings.Join(notices, "\n")
	if !strings.Contains(joined, "Not listing unreleased changes") {
		t.Errorf("the notices are %q, want one that says what couldn't be listed", notices)
	}
}
