package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// fakeRelease is a releaseBackend that says what a test tells it to, and records what it was asked.
type fakeRelease struct {
	data        releaseData
	loadErr     error
	committed   []string
	commitErr   error
	outcome     releaseOutcome
	releaseErr  error
	savePath    string
	saveNotices []string
	saveErr     error
	loads       []string
	commits     int
	releases    []string
	saves       int
}

func (f *fakeRelease) loadRelease(version string) (releaseData, error) {
	f.loads = append(f.loads, version)
	return f.data, f.loadErr
}

func (f *fakeRelease) commit() ([]string, []string, error) {
	f.commits++
	return f.committed, nil, f.commitErr
}

func (f *fakeRelease) release(version string, tag bool) (releaseOutcome, error) {
	f.releases = append(f.releases, version+"|"+map[bool]string{true: "tag", false: "no tag"}[tag])
	return f.outcome, f.releaseErr
}

func (f *fakeRelease) saveChangelog() (string, []string, error) {
	f.saves++
	return f.savePath, f.saveNotices, f.saveErr
}

// newFakeRelease is a pack in a repository with a release to make and two commits to make first.
func newFakeRelease() *fakeRelease {
	return &fakeRelease{
		data: releaseData{
			preview: changelog.Preview{
				InRepository: true, Last: "1.0.0", HasChanges: true,
				Release: changelog.Release{Version: "1.1.0", Bump: changelog.BumpMinor, Date: "2026-09-18"},
			},
			pending: []string{"feat(mods): add Lithium 0.12.0 (client)", "fix(config): update 1 config file\n\nconfig/sodium.json"},
		},
		committed: []string{"feat(mods): add Lithium 0.12.0 (client)", "fix(config): update 1 config file"},
		outcome:   releaseOutcome{made: true, version: "1.1.0", tag: "v1.1.0"},
		savePath:  "CHANGELOG.md",
	}
}

func releaseOn(t *testing.T, backend releaseBackend) *releaseScreen {
	t.Helper()
	setUpPack(t)
	s := newReleaseScreen(backend)
	s.setSize(100, 24)
	feed(t, s, s.activate()())
	return s
}

func TestReleaseShowsTheNextReleaseAndWhatIsNotCommittedTheFirstTimeItIsShown(t *testing.T) {
	f := newFakeRelease()
	s := releaseOn(t, f)

	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{
		"Last release", "1.0.0", "Repository", "git", "Not committed yet (2)",
		"feat(mods): add Lithium 0.12.0 (client)", "fix(config): update 1 config file", "First release; version is 1.1.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, out)
		}
	}
	if summary := lines(s.view())[0]; !strings.Contains(summary, "1.1.0 (minor bump)") || !strings.Contains(summary, "commits and tags") {
		t.Errorf("the summary is %q, want the version, what it is, and that releasing commits and tags", summary)
	}
	if len(f.loads) != 1 || f.loads[0] != "" {
		t.Errorf("the release was worked out %v, want once, for the version the changes make", f.loads)
	}
	if cmd := s.activate(); cmd != nil {
		t.Error("the release was worked out again when the screen was shown a second time")
	}
}

func TestReleaseSaysWhenThePackIsNotInARepository(t *testing.T) {
	f := newFakeRelease()
	f.data.preview.InRepository = false
	f.data.preview.Reason = "/pack isn't inside a git repository; run \"git init\" first"
	f.data.pending = nil
	s := releaseOn(t, f)

	out := strings.Join(body(t, s), "\n")
	if !strings.Contains(out, "none: /pack isn't inside a git repository") || !strings.Contains(out, "what differs from the pack as it was at the last one") {
		t.Errorf("the screen doesn't say the pack isn't in a repository:\n%s", out)
	}
	if summary := lines(s.view())[0]; strings.Contains(summary, "commits and tags") {
		t.Errorf("the summary is %q, want no commits and tags for a pack that isn't in a repository", summary)
	}
	for _, k := range []string{"C", "t"} {
		press(t, s, k)
		if got := statusOf(s); !strings.Contains(got, "isn't in a git repository") {
			t.Errorf("after %s the status line is %q, want it to say the pack isn't in a repository", k, got)
		}
	}
	press(t, s, "r", "y")
	if len(f.releases) != 1 || f.releases[0] != "|no tag" {
		t.Errorf("the release was made with %v, want it made without tagging, as there is no repository", f.releases)
	}
}

func TestReleaseAsksBeforeReleasingAndThenCommitsAndTags(t *testing.T) {
	f := newFakeRelease()
	s := releaseOn(t, f)
	press(t, s, "r")

	out := s.view()
	for _, want := range []string{
		"Release 1.1.0?", "1.1.0 (minor bump)", "first, 2 commits for what hasn't been committed",
		"the release in changelog.toml and CHANGELOG.md", "commits it as chore(release): 1.1.0 and tags it v1.1.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the question doesn't say %q:\n%s", want, out)
		}
	}
	if len(f.releases) != 0 {
		t.Fatalf("a release was made before it was answered: %v", f.releases)
	}
	press(t, s, "y")
	if len(f.releases) != 1 || f.releases[0] != "|tag" {
		t.Errorf("the release was made with %v, want once, with tagging", f.releases)
	}
	if got := statusOf(s); got != "Released 1.1.0, committed and tagged v1.1.0" {
		t.Errorf("the status line is %q, want it to say what was done", got)
	}
	if len(f.loads) != 2 {
		t.Errorf("the release was worked out %d times, want again afterwards, for what has changed since", len(f.loads))
	}
}

func TestReleaseDoesNothingWhenYouSayNo(t *testing.T) {
	f := newFakeRelease()
	s := releaseOn(t, f)
	press(t, s, "r", "n")
	if len(f.releases) != 0 {
		t.Errorf("a release was made though the answer was no: %v", f.releases)
	}
}

func TestReleaseCanLeaveOutCommittingAndTagging(t *testing.T) {
	f := newFakeRelease()
	f.outcome.tag = ""
	s := releaseOn(t, f)
	press(t, s, "t")
	if got := statusOf(s); !strings.Contains(got, "only record the release") {
		t.Errorf("the status line is %q, want it to say what releasing will do now", got)
	}
	if summary := lines(s.view())[0]; strings.Contains(summary, "commits and tags") {
		t.Errorf("the summary is %q, want no commits and tags", summary)
	}
	press(t, s, "r")
	if out := s.view(); strings.Contains(out, "tags it") {
		t.Errorf("the question says the release will be tagged:\n%s", out)
	}
	press(t, s, "y")
	if len(f.releases) != 1 || f.releases[0] != "|no tag" {
		t.Errorf("the release was made with %v, want it without tagging", f.releases)
	}
	if got := statusOf(s); got != "Released 1.1.0" {
		t.Errorf("the status line is %q, want it to say the release was made", got)
	}
	press(t, s, "t")
	if got := statusOf(s); !strings.Contains(got, "commit the release and tag it") {
		t.Errorf("the status line is %q, want t to turn it on again", got)
	}
}

func TestReleaseSaysWhenThereIsNothingToRelease(t *testing.T) {
	f := newFakeRelease()
	f.data.preview.HasChanges = false
	f.data.preview.NoChanges = "No changes since the last release (1.0.0)."
	s := releaseOn(t, f)

	press(t, s, "r")
	if got := statusOf(s); got != "No changes since the last release (1.0.0)." {
		t.Errorf("the status line is %q, want it to say there is nothing to release", got)
	}
	if s.modal() {
		t.Error("a question is open, though there is nothing to release")
	}
	if summary := lines(s.view())[0]; !strings.Contains(summary, "nothing to release") {
		t.Errorf("the summary is %q, want it to say there is nothing to release", summary)
	}
}

func TestReleaseChoosesTheVersionOfTheRelease(t *testing.T) {
	f := newFakeRelease()
	s := releaseOn(t, f)
	press(t, s, "v")
	typeText(t, s, "abc")
	if out := s.view(); !strings.Contains(out, "That isn't a version like 1.2.3") {
		t.Errorf("the box doesn't say the version is no good:\n%s", out)
	}
	press(t, s, "enter")
	if !s.modal() {
		t.Error("the box accepted a version that is no good")
	}
	press(t, s, "ctrl+u")
	typeText(t, s, "2.0.0")
	press(t, s, "enter")

	if len(f.loads) != 2 || f.loads[1] != "2.0.0" {
		t.Errorf("the release was worked out %v, want again for the version that was chosen", f.loads)
	}
	press(t, s, "r", "y")
	if len(f.releases) != 1 || !strings.HasPrefix(f.releases[0], "2.0.0|") {
		t.Errorf("the release was made with %v, want the version that was chosen", f.releases)
	}
	// It was for that release, and not for the next
	if last := f.loads[len(f.loads)-1]; last != "" {
		t.Errorf("after releasing the release was worked out for %q, want the version the changes make", last)
	}
}

func TestReleaseGoesBackToTheVersionTheChangesMakeWhenTheOneYouChoseCannotBeUsed(t *testing.T) {
	f := newFakeRelease()
	s := releaseOn(t, f)
	f.loadErr = errors.New("version 0.0.1 must be greater than the last release (1.0.0)")
	press(t, s, "v")
	typeText(t, s, "0.0.1")
	f.loadErr = errors.New("version 0.0.1 must be greater than the last release (1.0.0)")
	press(t, s, "enter")
	if got := statusOf(s); !strings.Contains(got, "must be greater than the last release") || !strings.Contains(got, "going back") {
		t.Errorf("the status line is %q, want the reason and that it went back", got)
	}
	f.loadErr = nil
	press(t, s, "c")
	if last := f.loads[len(f.loads)-1]; last != "" {
		t.Errorf("the release was worked out for %q, want the version the changes make", last)
	}
}

func TestReleaseAsksBeforeCommitting(t *testing.T) {
	f := newFakeRelease()
	s := releaseOn(t, f)
	press(t, s, "C")
	out := s.view()
	for _, want := range []string{"Make 2 commits?", "feat(mods): add Lithium 0.12.0 (client)", "fix(config): update 1 config file", "config/sodium.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("the question doesn't say %q:\n%s", want, out)
		}
	}
	if f.commits != 0 {
		t.Fatal("the pack was committed before it was answered")
	}
	press(t, s, "n")
	if f.commits != 0 {
		t.Error("the pack was committed though the answer was no")
	}
	press(t, s, "C", "y")
	if f.commits != 1 {
		t.Errorf("the pack was committed %d times, want once", f.commits)
	}
	if got := statusOf(s); got != "Made 2 commits" {
		t.Errorf("the status line is %q, want it to say how many commits were made", got)
	}
	if len(f.loads) != 2 {
		t.Errorf("the release was worked out %d times, want again afterwards", len(f.loads))
	}
}

func TestReleaseSaysWhenThereIsNothingToCommit(t *testing.T) {
	f := newFakeRelease()
	f.data.pending = nil
	s := releaseOn(t, f)
	press(t, s, "C")
	if got := statusOf(s); got != "Nothing to commit" || s.modal() {
		t.Errorf("the status line is %q with a question open %v, want it to say there is nothing to commit", got, s.modal())
	}
}

func TestReleaseWritesTheChangelog(t *testing.T) {
	f := newFakeRelease()
	s := releaseOn(t, f)
	press(t, s, "s")
	if f.saves != 1 || statusOf(s) != "Wrote CHANGELOG.md" {
		t.Errorf("saved %d times with the status line %q, want it written once, and said", f.saves, statusOf(s))
	}
}

func TestReleaseSaysWhatWritingTheChangelogHadToSay(t *testing.T) {
	f := newFakeRelease()
	f.saveNotices = []string{"Not listing unreleased changes: can't tell what has changed."}
	s := releaseOn(t, f)
	press(t, s, "s")
	if got := statusOf(s); got != "Wrote CHANGELOG.md. Not listing unreleased changes: can't tell what has changed." {
		t.Errorf("the status line is %q, want where it was written and what it said", got)
	}
}

func TestReleaseSaysWhenSomethingFails(t *testing.T) {
	for name, tc := range map[string]struct {
		setUp func(*fakeRelease)
		keys  []string
		want  string
	}{
		"working out the release": {setUp: func(f *fakeRelease) { f.loadErr = errors.New("can't read the log") }, want: "can't read the log"},
		"committing":              {setUp: func(f *fakeRelease) { f.commitErr = errors.New("hook failed") }, keys: []string{"C", "y"}, want: "hook failed"},
		"releasing":               {setUp: func(f *fakeRelease) { f.releaseErr = errors.New("tag exists") }, keys: []string{"r", "y"}, want: "tag exists"},
		"writing the changelog":   {setUp: func(f *fakeRelease) { f.saveErr = errors.New("read only") }, keys: []string{"s"}, want: "read only"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeRelease()
			if name != "working out the release" {
				tc.setUp(f)
			}
			setUpPack(t)
			s := newReleaseScreen(f)
			s.setSize(100, 24)
			if name == "working out the release" {
				tc.setUp(f)
			}
			feed(t, s, s.activate()())
			press(t, s, tc.keys...)
			if got := statusOf(s); !strings.Contains(got, tc.want) {
				t.Errorf("the status line is %q, want the reason: %s", got, tc.want)
			}
		})
	}
}

func TestReleaseSaysWhenNothingWasReleased(t *testing.T) {
	f := newFakeRelease()
	f.outcome = releaseOutcome{noChanges: "No changes since the last release (1.0.0)."}
	s := releaseOn(t, f)
	press(t, s, "r", "y")
	if got := statusOf(s); got != "No changes since the last release (1.0.0)." {
		t.Errorf("the status line is %q, want why nothing was released", got)
	}
}

// setUpGit makes the pack that setUpPack makes a git repository, isolated from the user's own git configuration.
func setUpGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	wd, _ := os.Getwd()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(wd))
	runGit(t, "init", "-q")
	for k, v := range map[string]string{"user.name": "Test", "user.email": "test@example.com", "commit.gpgsign": "false", "tag.gpgsign": "false"} {
		runGit(t, "config", k, v)
	}
}

// runGit runs a git command in the pack, failing the test if it fails, and returns what it printed.
func runGit(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

func TestReleaseCommitsReleasesAndTagsARealPackWithoutWritingToTheTerminal(t *testing.T) {
	setUpPack(t)
	setUpGit(t)
	s := newReleaseScreen(packBackend{})
	s.setSize(100, 30)

	out := cmdtest.CaptureStdout(t, func() {
		feed(t, s, s.activate()())
		if screenText := strings.Join(body(t, s), "\n"); !strings.Contains(screenText, "Not committed yet") || !strings.Contains(screenText, "initial commit") {
			t.Errorf("the screen doesn't say what isn't committed:\n%s", screenText)
		}
		// The first commit of a repository takes the whole pack
		press(t, s, "C", "y")
		if got := statusOf(s); !strings.HasPrefix(got, "Committed: chore(pack): initial commit") {
			t.Errorf("the status line is %q, want the commit that was made", got)
		}
		press(t, s, "r", "y")
	})
	if out != "" {
		t.Errorf("the release screen wrote %q to the terminal, which would be drawn over the screen", out)
	}

	if got := statusOf(s); got != "Released 1.2.0, committed and tagged v1.2.0" {
		t.Errorf("the status line is %q, want the release made, committed and tagged", got)
	}
	if tags := runGit(t, "tag", "--list"); tags != "v1.2.0" {
		t.Errorf("the tags are %q, want v1.2.0", tags)
	}
	if status := runGit(t, "status", "--porcelain"); status != "" {
		t.Errorf("the pack has uncommitted changes after releasing:\n%s", status)
	}
	if head := runGit(t, "log", "-1", "--format=%s"); head != "chore(release): 1.2.0" {
		t.Errorf("the last commit is %q, want the release", head)
	}
}

func TestReleaseWorksOutTheReleaseOfAPackThatIsNotInARepository(t *testing.T) {
	setUpPack(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	wd, _ := os.Getwd()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(wd))
	s := newReleaseScreen(packBackend{})
	s.setSize(100, 30)
	feed(t, s, s.activate()())

	out := strings.Join(body(t, s), "\n")
	if !strings.Contains(out, "none: ") || !strings.Contains(out, "First release; version is 1.2.0") {
		t.Errorf("the screen doesn't describe a release without a repository:\n%s", out)
	}
	press(t, s, "t")
	if got := statusOf(s); !strings.Contains(got, "isn't in a git repository") {
		t.Errorf("the status line is %q, want it to say there is no repository to commit to", got)
	}
	press(t, s, "r", "y")
	if got := statusOf(s); got != "Released 1.2.0" {
		t.Errorf("the status line is %q, want the release made, without a tag", got)
	}
	if _, err := os.Stat("changelog.toml"); err != nil {
		t.Errorf("the release wasn't recorded: %v", err)
	}
}
