package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestUpdatesAsksForNothingUntilToldTo(t *testing.T) {
	u := setUpUpdatable(t)
	s := newUpdates(t)
	if cmd := s.activate(); cmd != nil {
		t.Error("switching to the screen started something, but looking for updates asks the network, so it waits to be asked")
	}

	if out := s.view(); !strings.Contains(out, "Nothing has been looked for yet.") {
		t.Errorf("the screen doesn't say nothing has been looked for:\n%s", out)
	}
	if len(u.updated) != 0 {
		t.Errorf("mods were updated before anything was asked: %v", u.updated)
	}
}

func TestUpdatesListsWhatItFindsAndPicksAllOfIt(t *testing.T) {
	setUpUpdatable(t)
	s := newUpdates(t)
	press(t, s, "c")

	rows := body(t, s)
	if len(rows) != 2 {
		t.Fatalf("the rows are %q, want the two mods that have an update", rows)
	}
	for _, want := range []string{"Delta Mod", "Gamma Mod"} {
		found := false
		for _, r := range rows {
			if strings.Contains(r, want) && strings.Contains(r, "[x]") {
				found = true
			}
		}
		if !found {
			t.Errorf("the rows %q don't have %s picked", rows, want)
		}
	}
	for _, want := range []string{"gamma.jar -> gamma-2.jar", "delta.jar -> delta-2.jar"} {
		if !strings.Contains(strings.Join(rows, "\n"), want) {
			t.Errorf("the rows don't say %q:\n%s", want, strings.Join(rows, "\n"))
		}
	}
	summary := lines(s.view())[0]
	for _, want := range []string{"2 available", "2 picked", "3 up to date"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the summary %q doesn't say %q", summary, want)
		}
	}
	if got := statusOf(s); got != "2 updates found" {
		t.Errorf("the status line is %q, want it to say how many updates were found", got)
	}
}

func TestUpdatesPicksAndUnpicks(t *testing.T) {
	setUpUpdatable(t)
	s := newUpdates(t)
	press(t, s, "c", "space")

	if rows := body(t, s); strings.Contains(rows[0], "[x]") || !strings.Contains(rows[1], "[x]") {
		t.Errorf("the rows are %q, want the first unpicked by space and the other still picked", rows)
	}
	press(t, s, "a")
	if rows := body(t, s); !strings.Contains(rows[0], "[x]") || !strings.Contains(rows[1], "[x]") {
		t.Errorf("the rows are %q, want a to pick what wasn't picked", rows)
	}
	press(t, s, "a")
	if rows := body(t, s); strings.Contains(rows[0], "[x]") || strings.Contains(rows[1], "[x]") {
		t.Errorf("the rows are %q, want a to pick none when all are picked", rows)
	}
}

func TestUpdatesUpdatesWhatIsPickedOnceYouSayYes(t *testing.T) {
	u := setUpUpdatable(t)
	s := newUpdates(t)
	press(t, s, "c", "space", "enter")

	out := s.view()
	for _, want := range []string{"Update 1 mod?", "Gamma Mod"} {
		if !strings.Contains(out, want) {
			t.Errorf("the question doesn't have %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Delta Mod: ") {
		t.Errorf("the question lists a mod that wasn't picked:\n%s", out)
	}
	if !s.modal() {
		t.Error("the screen isn't modal while it asks")
	}

	press(t, s, "y")
	if len(u.updated) != 1 || u.updated[0] != "Gamma Mod" {
		t.Errorf("the mods updated are %v, want only the one that was picked", u.updated)
	}
	if mod, _ := core.LoadMod("mods/gamma.pw.toml"); mod.FileName != "gamma-2.jar" {
		t.Errorf("Gamma Mod is on %q, want it updated", mod.FileName)
	}
	if mod, _ := core.LoadMod("mods/delta.pw.toml"); mod.FileName != "delta.jar" {
		t.Errorf("Delta Mod is on %q, want it left alone", mod.FileName)
	}
	if got := statusOf(s); got != "Updated Gamma Mod" {
		t.Errorf("the status line is %q, want it to say what was updated", got)
	}
	if rows := body(t, s); len(rows) != 1 || !strings.Contains(rows[0], "Delta Mod") {
		t.Errorf("the rows are %q, want only what is still to be updated", rows)
	}
	assertIndexIsConsistent(t)
}

func TestUpdatesUpdatesEverythingThatIsFound(t *testing.T) {
	u := setUpUpdatable(t)
	s := newUpdates(t)
	press(t, s, "c", "enter", "y")

	if len(u.updated) != 2 {
		t.Errorf("the mods updated are %v, want both", u.updated)
	}
	if got := statusOf(s); got != "Updated 2 mods" {
		t.Errorf("the status line is %q, want it to say how many mods were updated", got)
	}
	if rows := body(t, s); len(rows) != 0 && !strings.Contains(strings.Join(rows, ""), "up to date") {
		t.Errorf("the rows are %q, want nothing left to update", rows)
	}
	assertIndexIsConsistent(t)
}

func TestUpdatesDoesNothingWhenNothingIsPicked(t *testing.T) {
	u := setUpUpdatable(t)
	s := newUpdates(t)
	press(t, s, "c", "a", "enter")

	if s.modal() {
		t.Error("a question is open though nothing is picked")
	}
	if got := statusOf(s); !strings.HasPrefix(got, "Nothing is picked") {
		t.Errorf("the status line is %q, want it to say nothing is picked", got)
	}
	if len(u.updated) != 0 {
		t.Errorf("mods were updated: %v", u.updated)
	}
}

func TestUpdatesLeavesThePackAloneWhenYouSayNo(t *testing.T) {
	u := setUpUpdatable(t)
	s := newUpdates(t)
	press(t, s, "c", "enter", "n")

	if len(u.updated) != 0 {
		t.Errorf("mods were updated though the answer was no: %v", u.updated)
	}
	if rows := body(t, s); len(rows) != 2 {
		t.Errorf("the rows are %q, want both updates still offered", rows)
	}
}

func TestUpdatesSaysWhatCouldNotBeChecked(t *testing.T) {
	u := setUpUpdatable(t)
	u.checks = map[string]core.UpdateCheck{"Gamma Mod": {Error: errors.New("boom")}}
	s := newUpdates(t)
	press(t, s, "c")

	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{"Couldn't be checked (1)", "Gamma Mod: boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, out)
		}
	}
	if got := statusOf(s); got != "No updates found, but 1 mod could not be checked" {
		t.Errorf("the status line is %q, want it not to say everything is up to date", got)
	}
	if strings.Contains(out, "Everything is up to date") {
		t.Errorf("the screen says everything is up to date though a check failed:\n%s", out)
	}
}

func TestUpdatesCountsEveryModOfASourceThatFailedAsAWhole(t *testing.T) {
	u := setUpUpdatable(t)
	u.err = errors.New("no network")
	s := newUpdates(t)
	press(t, s, "c")

	if got := statusOf(s); got != "No updates found, but 5 mods could not be checked" {
		t.Errorf("the status line is %q, want every mod of the source counted", got)
	}
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "stub: no network") {
		t.Errorf("the screen doesn't say which source failed:\n%s", out)
	}
}

func TestUpdatesSaysWhenEverythingIsUpToDate(t *testing.T) {
	u := setUpUpdatable(t)
	u.checks = nil
	s := newUpdates(t)
	press(t, s, "c")

	if got := statusOf(s); got != "Everything is up to date" {
		t.Errorf("the status line is %q, want it to say everything is up to date", got)
	}
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "Everything is up to date.") {
		t.Errorf("the screen doesn't say everything is up to date:\n%s", out)
	}
}

func TestUpdatesSaysWhichModsArePinned(t *testing.T) {
	setUpUpdatable(t)
	if err := (packBackend{}).setPinned("mods/gamma.pw.toml", true); err != nil {
		t.Fatalf("setPinned() returned error: %v", err)
	}
	s := newUpdates(t)
	press(t, s, "c")

	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{"Pinned, with an update that isn't offered (1)", "Gamma Mod is pinned", "Delta Mod"} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, out)
		}
	}
	if got := statusOf(s); got != "1 update found" {
		t.Errorf("the status line is %q, want the pinned mod not counted", got)
	}
}

func TestUpdatesWritesNothingToTheTerminal(t *testing.T) {
	setUpUpdatable(t)
	out := cmdtest.CaptureStdout(t, func() {
		s := newUpdates(t)
		press(t, s, "c", "enter", "y")
	})
	if out != "" {
		t.Errorf("the updates screen wrote %q to the terminal, which would be drawn over the screen", out)
	}
}

func TestUpdatesDoesNotLookForUpdatesWhileOneIsBeingMade(t *testing.T) {
	setUpUpdatable(t)
	s := newUpdates(t)
	cmd := s.start("Updating…", func(func(string)) tea.Msg { return nil })
	if !s.working() {
		t.Fatal("the screen isn't working while a job runs, so q would quit in the middle of it")
	}
	if _, next := s.updateKey(keyMsg(t, "c")); next != nil {
		t.Error("c started another job while one was running")
	}
	feed(t, s, cmd())
	if s.working() {
		t.Error("the screen is still working after the job was done")
	}
}
