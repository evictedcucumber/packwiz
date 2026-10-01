package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
)

// newCheck makes a check screen on the pack that setUpPack made, and shows it, which checks the pack. Nothing the
// fixture has is on Modrinth, so nothing is asked of it, and anything that is asked for anyway fails the test.
func newCheck(t *testing.T) *checkScreen {
	t.Helper()
	setUpPack(t)
	httpmock.Activate(t)
	s := newCheckScreen(packBackend{})
	s.setSize(100, 24)
	feed(t, s, s.activate()())
	return s
}

func TestCheckChecksThePackTheFirstTimeItIsShown(t *testing.T) {
	s := newCheck(t)
	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{
		"The pack",
		"error: has no NeoForge version",
		"Alpha Mod (mods/alpha.pw.toml)",
		"warning: has no side, so it is on both",
		"warning: isn't from Modrinth",
		"config-files has 1 entry that matches no file in the pack: config/gone.json",
		"error: 3 files aren't claimed by any mod, the mod loader or the pack",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report doesn't say %q:\n%s", want, out)
		}
	}
	summary := lines(s.view())[0]
	for _, want := range []string{"2 mods", "2 errors", "warnings"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the summary %q doesn't say %q", summary, want)
		}
	}
	if got := statusOf(s); !strings.HasPrefix(got, "Found 2 errors and ") {
		t.Errorf("the status line is %q, want it to say what was found", got)
	}
}

func TestCheckDoesNotCheckAgainWhenSwitchedBackTo(t *testing.T) {
	s := newCheck(t)
	if cmd := s.activate(); cmd != nil {
		t.Error("the pack was checked again when the screen was shown a second time, which asks the network")
	}
}

func TestCheckChecksAgainWhenAsked(t *testing.T) {
	s := newCheck(t)
	writeModAt(t, "gamma", "Gamma Mod", "")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	press(t, s, "c")
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "Gamma Mod") {
		t.Errorf("the report doesn't have the mod that was added:\n%s", out)
	}
	if summary := lines(s.view())[0]; !strings.Contains(summary, "3 mods") {
		t.Errorf("the summary %q doesn't count the mod that was added", summary)
	}
}

func TestCheckShowsWhatCouldBeFixedAndAsks(t *testing.T) {
	s := newCheck(t)
	press(t, s, "f")

	out := s.view()
	for _, want := range []string{
		"Make these changes?",
		"Alpha Mod (mods/alpha.pw.toml)",
		"side: (none) -> both",
		"config-files: remove config/gone.json",
		"Beta Mod (mods/beta.pw.toml)",
		"config-files: added, empty",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the question doesn't say %q:\n%s", want, out)
		}
	}
	if !s.modal() {
		t.Error("the screen isn't modal while it asks")
	}
}

func TestCheckLeavesThePackAloneWhenYouSayNo(t *testing.T) {
	s := newCheck(t)
	before, _ := readFileQuiet("mods/alpha.pw.toml")
	press(t, s, "f", "n")

	if after, _ := readFileQuiet("mods/alpha.pw.toml"); after != before {
		t.Error("a mod changed though the answer was no")
	}
	if s.modal() {
		t.Error("the question is still open after n")
	}
}

func TestCheckFixesThePackOnceYouSayYesAndChecksItAgain(t *testing.T) {
	s := newCheck(t)
	press(t, s, "f", "y")

	alpha, _ := core.LoadMod("mods/alpha.pw.toml")
	if alpha.Side != core.UniversalSide {
		t.Errorf("Alpha Mod's side is %q, want it fixed", alpha.Side)
	}
	if entries := alpha.ConfigEntries(); len(entries) != 2 || entries[0] != "config/alpha.json" || entries[1] != "config/alpha/" {
		t.Errorf("Alpha Mod's config-files are %v, want the entry that matched no file taken out", entries)
	}
	beta, _ := core.LoadMod("mods/beta.pw.toml")
	if beta.ConfigFiles == nil {
		t.Error("Beta Mod still has no config-files, want an empty one")
	}
	if got := statusOf(s); !strings.HasPrefix(got, "Changed 2 files. ") {
		t.Errorf("the status line is %q, want it to say what was changed, and what checking again found", got)
	}
	if out := strings.Join(body(t, s), "\n"); strings.Contains(out, "has no side") {
		t.Errorf("the report still has what was fixed:\n%s", out)
	}
	assertIndexIsConsistent(t)
}

func TestCheckSaysWhenWhatWasFoundCannotBeFixed(t *testing.T) {
	s := newCheck(t)
	press(t, s, "f", "y", "f")

	if got := statusOf(s); got != "Nothing that was found can be fixed automatically" {
		t.Errorf("the status line is %q, want it to say what was found can't be fixed", got)
	}
	if s.modal() {
		t.Error("a question is open, though there is nothing to fix")
	}
}

func TestCheckSaysWhenThereIsNothingToFix(t *testing.T) {
	setUpPack(t)
	withLoader(t)
	httpmock.Activate(t)
	// A pack with no mods at all has nothing wrong with it
	for _, slug := range []string{"alpha", "beta"} {
		mustRemove(t, "mods/"+slug+".pw.toml")
		mustRemove(t, "mods/"+slug+".jar")
	}
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := newCheckScreen(packBackend{})
	s.setSize(100, 24)
	feed(t, s, s.activate()())

	if got := statusOf(s); got == "" {
		t.Error("the status line is empty, want it to say what was found")
	}
	press(t, s, "f")
	if got := statusOf(s); got != "Nothing to fix" && got != "Nothing that was found can be fixed automatically" {
		t.Errorf("the status line is %q, want it to say there is nothing to fix", got)
	}
}

func TestCheckScrollsThroughALongReport(t *testing.T) {
	setUpPack(t)
	httpmock.Activate(t)
	for i := range 30 {
		writeModAt(t, fmt.Sprintf("extra-%02d", i), fmt.Sprintf("Extra %02d", i), "")
	}
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := newCheckScreen(packBackend{})
	s.setSize(100, 14)
	feed(t, s, s.activate()())

	first := body(t, s)[0]
	if summary := lines(s.view())[0]; !strings.Contains(summary, "1-12/") {
		t.Errorf("the summary %q doesn't say where in the report the screen is", summary)
	}
	press(t, s, "pgdown")
	if body(t, s)[0] == first {
		t.Error("page down didn't scroll the report")
	}
	press(t, s, "g")
	if body(t, s)[0] != first {
		t.Error("g didn't go back to the start of the report")
	}
	press(t, s, "G")
	if last := lines(s.view()); last[len(last)-2] == "" {
		t.Errorf("at the end of the report the last line is blank, want it to end with what is found:\n%s", s.view())
	}
}

func TestCheckWrapsWhatIsTooLongForTheScreen(t *testing.T) {
	setUpPack(t)
	httpmock.Activate(t)
	s := newCheckScreen(packBackend{})
	s.setSize(44, 20)
	feed(t, s, s.activate()())

	rows := body(t, s)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "an exported pack has no mod loader") {
		t.Errorf("the report has lost the end of a long message:\n%s", joined)
	}
	for _, row := range rows {
		if w := ansi.StringWidth(row); w > 44 {
			t.Errorf("the line %q is %d columns wide, want no more than 44", row, w)
		}
		if strings.HasSuffix(row, "…") {
			t.Errorf("the line %q was cut off, want it wrapped so none of it is lost", row)
		}
	}
}

func TestCheckWritesNothingToTheTerminal(t *testing.T) {
	setUpPack(t)
	httpmock.Activate(t)
	out := cmdtest.CaptureStdout(t, func() {
		s := newCheckScreen(packBackend{})
		s.setSize(100, 24)
		feed(t, s, s.activate()())
		press(t, s, "f", "y", "c")
	})
	if out != "" {
		t.Errorf("the check screen wrote %q to the terminal, which would be drawn over the screen", out)
	}
}
