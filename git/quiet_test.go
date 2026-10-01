package git

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestPlanCommitListsTheCommitsCommitWouldMakeAndMakesNone(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.mod(t, "Sodium", core.ClientSide, "0.5.8")
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	before := commitCount(t)

	var messages []string
	var err error
	out := cmdtest.CaptureStdout(t, func() { messages, _, err = PlanCommit() })

	if err != nil {
		t.Fatalf("PlanCommit() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("PlanCommit() wrote %q to the terminal", out)
	}
	if len(messages) != 2 || !strings.HasPrefix(messages[0], "feat(mods)!: add Lithium 0.12.0 (server)") || !strings.HasPrefix(messages[1], "fix(mods): update Sodium 0.5.7 -> 0.5.8 (client)") {
		t.Errorf("the messages are %q, want a commit for each mod, in the order of their paths", messages)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commit count = %d, want %d: planning makes no commits", got, before)
	}
}

func TestPlanCommitHasNothingWhenThereIsNothingToCommit(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)

	messages, _, err := PlanCommit()
	if err != nil || len(messages) != 0 {
		t.Errorf("PlanCommit() = %q, %v, want no commits and no error", messages, err)
	}
}

func TestCommitAllMakesTheCommitsAndSaysSoWithoutSayingAnythingOnTheTerminal(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	p.write(t, "config/lithium.json", "{}")

	var committed []string
	var err error
	out := cmdtest.CaptureStdout(t, func() { committed, _, err = CommitAll() })

	if err != nil {
		t.Fatalf("CommitAll() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("CommitAll() wrote %q to the terminal", out)
	}
	if len(committed) != 2 || !strings.HasPrefix(committed[0], "feat(mods)!: add Lithium") || !strings.HasPrefix(committed[1], "fix(config)") {
		t.Errorf("the commits are %q, want the mod and then the config", committed)
	}
	requireClean(t)
}

func TestCommitAllSaysWhenThereWasNothingToCommit(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)

	committed, notices, err := CommitAll()
	if err != nil || len(committed) != 0 {
		t.Fatalf("CommitAll() = %q, %v, want nothing committed and no error", committed, err)
	}
	if !reflect.DeepEqual(notices, []string{"Nothing to commit."}) {
		t.Errorf("the notices are %q, want it to say there was nothing to commit", notices)
	}
}

func TestPlanCommitAndCommitAllFailOutsideARepository(t *testing.T) {
	setUpPackOutsideARepo(t)
	for name, run := range map[string]func() error{
		"PlanCommit": func() error { _, _, err := PlanCommit(); return err },
		"CommitAll":  func() error { _, _, err := CommitAll(); return err },
	} {
		if err := run(); !errors.Is(err, changelog.ErrNoRepository) {
			t.Errorf("%s() returned %v, want an error that the pack isn't in a repository", name, err)
		}
	}
}

func TestReleaseAndTagReleasesCommitsAndTagsWithoutSayingAnythingOnTheTerminal(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.write(t, "config/sodium.json", "{}")
	commit(t)

	var released Released
	var err error
	out := cmdtest.CaptureStdout(t, func() { released, err = ReleaseAndTag("", "") })

	if err != nil {
		t.Fatalf("ReleaseAndTag() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("ReleaseAndTag() wrote %q to the terminal", out)
	}
	if !released.Made || released.Release.Version != "1.0.0" || released.Tag != "v1.0.0" {
		t.Errorf("the release is %+v, want 1.0.0, made, and tagged v1.0.0", released)
	}
	if got := lastMessages(t, 4); !reflect.DeepEqual(got, []string{
		"chore(pack): initial commit", "feat(mods): add Sodium 0.5.7 (client)", "fix(config): add config/sodium.json", "chore(release): 1.0.0",
	}) {
		t.Errorf("the commit messages are %q, want what releasing with the command makes", got)
	}
	if kind := git(t, "cat-file", "-t", "v1.0.0"); kind != "tag" {
		t.Errorf("v1.0.0 is a %q object, want an annotated tag", kind)
	}
	requireClean(t)
}

func TestReleaseAndTagWithAVersionOfYourOwn(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	if _, err := ReleaseAndTag("", ""); err != nil {
		t.Fatalf("ReleaseAndTag() returned error: %v", err)
	}
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	commit(t)

	released, err := ReleaseAndTag("5.0.0", "")
	if err != nil || released.Release.Version != "5.0.0" || released.Tag != "v5.0.0" {
		t.Errorf("ReleaseAndTag() = %+v, %v, want the version that was asked for", released, err)
	}
}

func TestReleaseAndTagSaysWhenThereIsNothingToRelease(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	if _, err := ReleaseAndTag("", ""); err != nil {
		t.Fatalf("ReleaseAndTag() returned error: %v", err)
	}
	before := commitCount(t)

	released, err := ReleaseAndTag("", "")
	if err != nil {
		t.Fatalf("ReleaseAndTag() returned error: %v", err)
	}
	if released.Made || released.NoChanges != "No changes since the last release (1.0.0)." || released.Tag != "" {
		t.Errorf("the release is %+v, want nothing made, and why", released)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commit count = %d, want %d: there was nothing to release", got, before)
	}
}

func TestReleaseAndTagFailsOutsideARepositoryBeforeLoadingAnything(t *testing.T) {
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	src.Err = errors.New("network is down")
	p := setUpPackOutsideARepo(t)
	p.unversioned(t, src, "Sodium", core.ClientSide, "id-a")

	if _, err := ReleaseAndTag("", ""); !errors.Is(err, changelog.ErrNoRepository) {
		t.Errorf("ReleaseAndTag() returned %v, want an error that the pack isn't in a repository", err)
	}
	if src.Calls != 0 {
		t.Errorf("%d versions were looked up before it refused", src.Calls)
	}
}

func TestReleaseAndTagTellsYouHowToFinishWhenTheTagAlreadyExists(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	git(t, "tag", "v1.0.0")

	_, err := ReleaseAndTag("", "")
	if err == nil || !strings.Contains(err.Error(), "couldn't tag it") || !strings.Contains(err.Error(), "v1.0.0 yourself") {
		t.Errorf("ReleaseAndTag() returned %v, want one that says the tag couldn't be made and how to finish", err)
	}
}
