package tui

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// The tree of the pack that setUpPack makes, as the config screen draws it.
var fixtureTree = []string{
	"▾ Alpha Mod (4)",
	"├── config/alpha.json",
	"├── config/alpha/deep/x.json",
	"├── config/alpha/sub.json",
	"└── config/gone.json (missing)",
	"▾ Invalid (3)",
	"├── config/orphan.json",
	"├── config/other/a.json",
	"└── config/other/b.json",
}

// Row numbers of the fixture tree, to move the cursor to.
const (
	rowAlphaJSON = 1
	rowDeepX     = 2
	rowGone      = 4
	rowInvalid   = 5
	rowOrphan    = 6
	rowOtherA    = 7
)

// goTo moves the cursor to a row of the tree, from the top.
func goTo(t *testing.T, s *configScreen, row int) {
	t.Helper()
	press(t, s, "g")
	for range row {
		press(t, s, "j")
	}
}

// goToRowWhere moves the cursor to the first row of the tree, from the top, that is drawn as something match accepts. It
// fails the test, showing the screen, if there is none: walking down until one turns up would never end if it didn't.
func goToRowWhere(t *testing.T, s *configScreen, what string, match func(drawn string) bool) {
	t.Helper()
	press(t, s, "g")
	for range len(s.rows) {
		if match(cursorRow(t, s)) {
			return
		}
		press(t, s, "j")
	}
	t.Fatalf("no row of the tree is %s:\n%s", what, s.view())
}

// goToFile moves the cursor to the row of a file, wherever it is in the tree now.
func goToFile(t *testing.T, s *configScreen, file string) {
	t.Helper()
	goToRowWhere(t, s, file, func(drawn string) bool {
		return strings.HasSuffix(drawn, file) && !strings.Contains(drawn, "(missing)")
	})
}

// goToMissing moves the cursor to the row of an entry that matches no file.
func goToMissing(t *testing.T, s *configScreen, entry string) {
	t.Helper()
	goToRowWhere(t, s, entry+" (missing)", func(drawn string) bool { return strings.HasSuffix(drawn, entry+" (missing)") })
}

func TestConfigScreenShowsWhatConfigListShows(t *testing.T) {
	s := newConfig(t)

	if got := tree(t, s); !slices.Equal(got, fixtureTree) {
		t.Errorf("the tree is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(fixtureTree, "\n"))
	}
	summary := lines(s.view())[0]
	for _, want := range []string{"Config files", "showing all", "3 valid", "3 invalid", "1 missing", "1/9"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the summary %q doesn't have %q", summary, want)
		}
	}
	if got := cursorRow(t, s); got != "▾ Alpha Mod (4)" {
		t.Errorf("the cursor is on %q, want the first group", got)
	}
}

func TestConfigScreenMovesAndScrolls(t *testing.T) {
	s := newConfig(t)
	s.setSize(100, 6) // four rows of the tree at a time

	if got := tree(t, s); len(got) != 4 || got[0] != "▾ Alpha Mod (4)" {
		t.Fatalf("the tree is %v, want its first four rows", got)
	}

	// The cursor going past the last row shown scrolls it into view
	press(t, s, "j", "j", "j", "j")
	if got := cursorRow(t, s); got != "└── config/gone.json (missing)" {
		t.Errorf("after four moves down the cursor is on %q", got)
	}
	if got := tree(t, s); got[0] != "├── config/alpha.json" || len(got) != 4 {
		t.Errorf("the tree is scrolled to %v, want it to start at the second row", got)
	}

	press(t, s, "G")
	if got := cursorRow(t, s); got != "└── config/other/b.json" {
		t.Errorf("G went to %q, want the last row", got)
	}
	if got := tree(t, s); got[len(got)-1] != "└── config/other/b.json" || got[0] != "▾ Invalid (3)" {
		t.Errorf("the tree is scrolled to %v, want the last four rows", got)
	}
	press(t, s, "j")
	if got := cursorRow(t, s); got != "└── config/other/b.json" {
		t.Errorf("moving down from the last row went to %q, want to stay", got)
	}

	press(t, s, "g")
	if got := cursorRow(t, s); got != "▾ Alpha Mod (4)" || tree(t, s)[0] != "▾ Alpha Mod (4)" {
		t.Errorf("g went to %q, want the first row, scrolled back to", got)
	}
	press(t, s, "k")
	if got := cursorRow(t, s); got != "▾ Alpha Mod (4)" {
		t.Errorf("moving up from the first row went to %q, want to stay", got)
	}

	press(t, s, "pgdown")
	if got := cursorRow(t, s); got != "└── config/gone.json (missing)" {
		t.Errorf("a page down went to %q, want four rows on", got)
	}
	press(t, s, "pgup")
	if got := cursorRow(t, s); got != "▾ Alpha Mod (4)" {
		t.Errorf("a page up went to %q, want back to the first row", got)
	}
	press(t, s, "down", "down", "up")
	if got := cursorRow(t, s); got != "├── config/alpha.json" {
		t.Errorf("the arrow keys went to %q, want the second row", got)
	}
}

func TestConfigScreenResizingKeepsTheCursorOnScreen(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, 8)
	s.setSize(100, 5) // three rows
	if got := tree(t, s); got[len(got)-1] != "└── config/other/b.json" || len(got) != 3 {
		t.Errorf("after shrinking the tree is %v, want it to end with the cursor's row", got)
	}
	s.setSize(100, 24)
	if got := tree(t, s); !slices.Equal(got, fixtureTree) {
		t.Errorf("after growing the tree is %v, want all of it", got)
	}
}

func TestConfigScreenFoldsAndUnfoldsGroups(t *testing.T) {
	s := newConfig(t)

	press(t, s, "left")
	want := append([]string{"▸ Alpha Mod (4)"}, fixtureTree[rowInvalid:]...)
	if got := tree(t, s); !slices.Equal(got, want) {
		t.Errorf("after folding the tree is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := cursorRow(t, s); got != "▸ Alpha Mod (4)" {
		t.Errorf("the cursor is on %q, want it to stay on the group it folded", got)
	}

	press(t, s, "right")
	if got := tree(t, s); !slices.Equal(got, fixtureTree) {
		t.Errorf("after unfolding the tree is %v, want all of it", got)
	}

	// Going left from a file goes to its group, without folding it
	goTo(t, s, rowDeepX)
	press(t, s, "h")
	if got := cursorRow(t, s); got != "▾ Alpha Mod (4)" {
		t.Errorf("going left from a file went to %q, want its group", got)
	}
	if got := tree(t, s); !slices.Equal(got, fixtureTree) {
		t.Errorf("going left from a file changed the tree to %v", got)
	}

	// Enter on a group folds it, and again unfolds it
	press(t, s, "enter")
	if got := tree(t, s); got[0] != "▸ Alpha Mod (4)" || len(got) != 1+len(fixtureTree)-rowInvalid {
		t.Errorf("after enter on a group the tree is %v, want it folded", got)
	}
	press(t, s, "enter")
	if got := tree(t, s); !slices.Equal(got, fixtureTree) {
		t.Errorf("after enter on a folded group the tree is %v, want all of it", got)
	}
}

func TestConfigScreenFilterShowsEachStateAndComesBackToAll(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowAlphaJSON)

	press(t, s, "f")
	if got, want := tree(t, s), []string{"▾ Alpha Mod (3)", "├── config/alpha.json", "├── config/alpha/deep/x.json", "└── config/alpha/sub.json"}; !slices.Equal(got, want) {
		t.Errorf("valid: the tree is %v, want %v", got, want)
	}
	if got := statusOf(s); got != "Showing valid" {
		t.Errorf("the status is %q, want it to say what is shown", got)
	}
	if got := cursorRow(t, s); got != "├── config/alpha.json" {
		t.Errorf("the cursor is on %q, want it to stay on the file it was on", got)
	}

	press(t, s, "f")
	if got, want := tree(t, s), []string{"▾ Invalid (3)", "├── config/orphan.json", "├── config/other/a.json", "└── config/other/b.json"}; !slices.Equal(got, want) {
		t.Errorf("invalid: the tree is %v, want %v", got, want)
	}

	press(t, s, "f")
	if got, want := tree(t, s), []string{"▾ Alpha Mod (1)", "└── config/gone.json (missing)"}; !slices.Equal(got, want) {
		t.Errorf("missing: the tree is %v, want %v", got, want)
	}

	press(t, s, "f")
	if got := tree(t, s); !slices.Equal(got, fixtureTree) {
		t.Errorf("back to all: the tree is %v, want all of it", got)
	}
}

func TestConfigScreenSaysWhenThereIsNothingToShow(t *testing.T) {
	t.Run("a pack with no config files", func(t *testing.T) {
		setUpPack(t)
		s := configOn(t, &fakeBackend{data: configData{pack: "Empty"}})
		view := s.view()
		for _, want := range []string{"No config files are tracked in this pack.", "press R to refresh the index"} {
			if !strings.Contains(view, want) {
				t.Errorf("the screen doesn't say %q:\n%s", want, view)
			}
		}
		// Keys that have nothing to work on say so rather than doing anything
		press(t, s, "r", "x", "space", "j", "enter")
		if s.modal() {
			t.Error("a box opened with no file to work on")
		}
	})

	t.Run("a state that none is in", func(t *testing.T) {
		setUpPack(t)
		s := configOn(t, &fakeBackend{data: configData{tree: core.ConfigFileTree{Unclaimed: []string{"config/a.json"}}}})
		press(t, s, "f") // valid
		if view := s.view(); !strings.Contains(view, "No config files are valid.") {
			t.Errorf("the screen doesn't say that none is valid:\n%s", view)
		}
	})
}

func TestRelateAFileToAMod(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)

	press(t, s, "r")
	if !s.modal() {
		t.Fatal("r didn't open the picker")
	}
	view := s.view()
	for _, want := range []string{"Relate config/orphan.json to", "Claims: config/orphan.json", "[ ] Pack  the pack as a whole", "[ ] Alpha Mod", "[ ] Beta Mod"} {
		if !strings.Contains(view, want) {
			t.Errorf("the picker doesn't have %q:\n%s", want, view)
		}
	}

	press(t, s, "j", "j", "space", "enter") // down past the pack and Alpha Mod, to Beta Mod
	if s.modal() {
		t.Error("the picker is still open after enter")
	}
	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/orphan.json"}) {
		t.Errorf("Beta Mod's config-files is %v, want the file", got)
	}
	if got := claims(t, "alpha"); len(got) != 3 {
		t.Errorf("Alpha Mod's config-files is %v, want it as it was", got)
	}

	// It is listed under the mod now, and the cursor went with it
	want := []string{
		"▾ Alpha Mod (4)", "├── config/alpha.json", "├── config/alpha/deep/x.json", "├── config/alpha/sub.json", "└── config/gone.json (missing)",
		"▾ Beta Mod (1)", "└── config/orphan.json",
		"▾ Invalid (2)", "├── config/other/a.json", "└── config/other/b.json",
	}
	if got := tree(t, s); !slices.Equal(got, want) {
		t.Errorf("the tree is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := cursorRow(t, s); got != "└── config/orphan.json" {
		t.Errorf("the cursor is on %q, want it on the file that was related", got)
	}
	if got := statusOf(s); got != "Beta Mod now claims config/orphan.json" {
		t.Errorf("the status is %q", got)
	}
	assertIndexIsConsistent(t)
}

// What is written is what "packwiz config list" reads: a mod that the command lists the file under is the one the
// screen related it to
func TestRelateIsWhatConfigListSees(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)
	press(t, s, "r", "j", "j", "enter") // nothing is ticked, so the mod under the cursor, Beta Mod, is the one

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		t.Fatalf("LoadAllMods() returned error: %v", err)
	}
	got, err := index.ConfigFileTree(mods, pack)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}
	var beta []string
	for _, m := range got.Mods {
		if m.Mod.Name == "Beta Mod" {
			beta = m.Files
		}
	}
	if !slices.Equal(beta, []string{"config/orphan.json"}) {
		t.Errorf("config list has Beta Mod's files as %v, want the file", beta)
	}
	if slices.Contains(got.Unclaimed, "config/orphan.json") {
		t.Error("config list still has the file as unclaimed")
	}
}

func TestRelateAFolderInsteadOfTheFile(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOtherA)

	press(t, s, "r")
	if view := s.view(); !strings.Contains(view, "Claims: config/other/a.json") {
		t.Fatalf("the picker starts out claiming the file:\n%s", view)
	}
	press(t, s, "tab")
	if view := s.view(); !strings.Contains(view, "Claims: config/other/\n") && !strings.Contains(view, "Claims: config/other/ ") {
		t.Errorf("tab didn't change it to the folder:\n%s", view)
	}
	press(t, s, "j", "j", "enter")

	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/other/"}) {
		t.Errorf("Beta Mod's config-files is %v, want the folder", got)
	}
	// Both files in the folder are Beta Mod's now
	if got := tree(t, s); !slices.Contains(got, "▾ Beta Mod (2)") || !slices.Contains(got, "├── config/other/a.json") || !slices.Contains(got, "└── config/other/b.json") {
		t.Errorf("the tree is %v, want both files under Beta Mod", got)
	}
	if got := statusOf(s); got != "Beta Mod now claims config/other/" {
		t.Errorf("the status is %q", got)
	}

	// Tab again goes back to the file
	goToFile(t, s, "config/orphan.json")
	press(t, s, "r", "tab", "tab")
	if view := s.view(); !strings.Contains(view, "Claims: config/orphan.json") {
		t.Errorf("tab twice didn't come back to the file:\n%s", view)
	}
}

func TestRelateSeveralMarkedFilesTogether(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)
	press(t, s, "space", "j", "j", "space")

	if got := lines(s.view())[0]; !strings.Contains(got, "2 marked") {
		t.Errorf("the summary %q doesn't say how many are marked", got)
	}
	if view := s.view(); !strings.Contains(view, "* ├── config/orphan.json") || !strings.Contains(view, "* └── config/other/b.json") {
		t.Errorf("the marked files aren't marked:\n%s", view)
	}

	// The cursor is on an unmarked file, but what is marked is what is related
	goTo(t, s, rowAlphaJSON)
	press(t, s, "r")
	if view := s.view(); !strings.Contains(view, "Relate 2 files to") || !strings.Contains(view, "Claims: config/orphan.json, config/other/b.json") {
		t.Errorf("the picker isn't for the two marked files:\n%s", view)
	}
	press(t, s, "j", "j", "enter")

	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/orphan.json", "config/other/b.json"}) {
		t.Errorf("Beta Mod's config-files is %v, want both files", got)
	}
	if got := statusOf(s); got != "Related 2 entries to 1 mod" {
		t.Errorf("the status is %q", got)
	}
	if got := lines(s.view())[0]; strings.Contains(got, "marked") {
		t.Errorf("the summary %q still has files marked after they were related", got)
	}
	assertIndexIsConsistent(t)
}

func TestMarkingAGroupMarksAllItsFilesAndAgainUnmarksThem(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowInvalid)

	press(t, s, "space")
	if got := lines(s.view())[0]; !strings.Contains(got, "3 marked") {
		t.Errorf("the summary %q doesn't have all three of the group's files marked", got)
	}
	press(t, s, "space")
	if got := lines(s.view())[0]; strings.Contains(got, "marked") {
		t.Errorf("the summary %q still has files marked after marking the group again", got)
	}

	press(t, s, "j", "space", "esc")
	if got := lines(s.view())[0]; strings.Contains(got, "marked") {
		t.Errorf("the summary %q still has files marked after esc", got)
	}
	if got := statusOf(s); got != "Unmarked 1 file" {
		t.Errorf("the status is %q", got)
	}
}

func TestMarkingAMissingEntryExplainsWhyNot(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowGone)
	press(t, s, "space")
	if got := statusOf(s); !strings.Contains(got, "matches no file") {
		t.Errorf("the status is %q, want it to say the entry matches no file", got)
	}
	if got := lines(s.view())[0]; strings.Contains(got, "marked") {
		t.Errorf("the summary %q has something marked", got)
	}
}

func TestRelateAGroupRelatesEveryFileInIt(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowInvalid)
	press(t, s, "r")
	if view := s.view(); !strings.Contains(view, "Relate 3 files to") {
		t.Errorf("the picker isn't for the group's three files:\n%s", view)
	}
	press(t, s, "j", "j", "enter")
	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/orphan.json", "config/other/a.json", "config/other/b.json"}) {
		t.Errorf("Beta Mod's config-files is %v, want all three", got)
	}
	if got := tree(t, s); slices.Contains(got, "▾ Invalid (3)") || slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "Invalid") }) {
		t.Errorf("the tree is %v, want no Invalid group once its files have a mod", got)
	}
}

func TestRelateToSeveralMods(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)
	press(t, s, "r", "j", "space", "j", "space", "enter") // Alpha Mod and Beta Mod, after the pack

	for _, slug := range []string{"alpha", "beta"} {
		if got := claims(t, slug); !slices.Contains(got, "config/orphan.json") {
			t.Errorf("%s's config-files is %v, want the file in it", slug, got)
		}
	}
	if got := claims(t, "alpha"); !slices.Equal(got, []string{"config/alpha.json", "config/alpha/", "config/gone.json", "config/orphan.json"}) {
		t.Errorf("Alpha Mod's config-files is %v, want the file added after what it had", got)
	}
	if got := statusOf(s); got != "Related 1 entry to 2 mods" {
		t.Errorf("the status is %q", got)
	}
	// A file both claim is under each
	if got := tree(t, s); !slices.Contains(got, "▾ Beta Mod (1)") || !slices.Contains(got, "▾ Alpha Mod (5)") {
		t.Errorf("the tree is %v, want the file under both mods", got)
	}
	assertIndexIsConsistent(t)
}

func TestRelateSomethingAModAlreadyClaimsChangesNothing(t *testing.T) {
	s := newConfig(t)
	before, err := os.ReadFile("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("failed to read the mod file: %v", err)
	}
	indexBefore, _ := os.ReadFile("index.toml")

	goTo(t, s, rowAlphaJSON)
	press(t, s, "r")
	if view := s.view(); !strings.Contains(view, "Alpha Mod  (already claims)") {
		t.Errorf("the picker doesn't say that Alpha Mod already claims the file:\n%s", view)
	}
	press(t, s, "j", "enter") // past the pack, to Alpha Mod

	if got := statusOf(s); got != "Alpha Mod already claims config/alpha.json" {
		t.Errorf("the status is %q", got)
	}
	after, _ := os.ReadFile("mods/alpha.pw.toml")
	if !slices.Equal(before, after) {
		t.Errorf("the mod's file changed:\n%s\nto\n%s", before, after)
	}
	if indexAfter, _ := os.ReadFile("index.toml"); !slices.Equal(indexBefore, indexAfter) {
		t.Error("the index changed")
	}
}

// The pack is always there to relate a file to, so there is something to pick if the pack has no mods
func TestRelateWithNoModsStillOffersThePack(t *testing.T) {
	setUpPack(t)
	s := configOn(t, &fakeBackend{data: configData{
		tree:   core.ConfigFileTree{Unclaimed: []string{"config/a.json"}},
		owners: []owner{{kind: ownerPack, id: "pack", name: "Pack", slug: "pack"}},
	}})
	press(t, s, "j", "r")
	if !s.modal() {
		t.Fatal("the picker didn't open")
	}
	if view := s.view(); !strings.Contains(view, "[ ] Pack  the pack as a whole") || strings.Contains(view, "No mod matches") {
		t.Errorf("the picker doesn't offer the pack:\n%s", view)
	}
}

func TestRelateRefusesOnAnEntryThatMatchesNoFile(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowGone)
	press(t, s, "r")
	if s.modal() {
		t.Error("the picker opened for an entry that matches no file")
	}
	if got := statusOf(s); !strings.Contains(got, "Nothing to relate here") {
		t.Errorf("the status is %q", got)
	}
}

func TestEnterOnAFileRelatesItAndOnAGroupFoldsIt(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)
	press(t, s, "enter")
	if !s.modal() {
		t.Error("enter on a file didn't open the picker")
	}
}

func TestPickerFiltersByNameAndSlugAndEnterPicksWhatIsUnderTheCursor(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)

	press(t, s, "r", "/")
	typeText(t, s, "bet")
	view := s.view()
	if !strings.Contains(view, "Filter: bet█") || strings.Contains(view, "Alpha Mod") || !strings.Contains(view, "[ ] Beta Mod") {
		t.Errorf("the picker isn't filtered to Beta Mod:\n%s", view)
	}
	// Letters that are commands elsewhere are text in a filter
	press(t, s, "backspace", "backspace", "backspace")
	typeText(t, s, "jq")
	if view := s.view(); !strings.Contains(view, "Filter: jq█") || !strings.Contains(view, "No mod matches") {
		t.Errorf("j and q weren't typed into the filter:\n%s", view)
	}
	if !s.modal() {
		t.Error("typing q closed the picker")
	}

	press(t, s, "ctrl+u")
	typeText(t, s, "BETA")
	press(t, s, "enter")
	if view := s.view(); !strings.Contains(view, "Filter: BETA  (esc clears)") {
		t.Errorf("enter didn't leave the filter applied:\n%s", view)
	}
	press(t, s, "enter")
	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/orphan.json"}) {
		t.Errorf("Beta Mod's config-files is %v, want the file", got)
	}
}

func TestPickerFilterAlsoMatchesTheSlug(t *testing.T) {
	setUpPack(t)
	addMod(t, "zzz-long-slug", "Plain")
	s := configOn(t, packBackend{})
	goTo(t, s, rowOrphan)
	press(t, s, "r", "/")
	typeText(t, s, "long-slug")
	if view := s.view(); !strings.Contains(view, "[ ] Plain") {
		t.Errorf("the picker doesn't have the mod that has that slug:\n%s", view)
	}
}

func TestPickerFilterCanHaveSpacesInIt(t *testing.T) {
	setUpPack(t)
	s := configOn(t, packBackend{})
	goTo(t, s, rowOrphan)
	press(t, s, "r", "/")
	typeText(t, s, "beta m")
	if view := s.view(); !strings.Contains(view, "[ ] Beta Mod") || strings.Contains(view, "Alpha") {
		t.Errorf("a filter with a space in it didn't match Beta Mod:\n%s", view)
	}
}

func TestPickerEscapeClearsTheFilterThenClosesTheBox(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)
	press(t, s, "r", "/")
	typeText(t, s, "bet")

	press(t, s, "esc") // out of typing, and the filter is cleared
	if view := s.view(); !s.modal() || !strings.Contains(view, "Alpha Mod") || !strings.Contains(view, "press / to search") {
		t.Errorf("esc while typing didn't clear the filter and leave the picker open:\n%s", view)
	}
	press(t, s, "/")
	typeText(t, s, "bet")
	press(t, s, "enter", "esc") // the filter is applied, and esc takes it off first
	if view := s.view(); !s.modal() || !strings.Contains(view, "Alpha Mod") {
		t.Errorf("esc with a filter applied didn't clear it and leave the picker open:\n%s", view)
	}
	press(t, s, "esc")
	if s.modal() {
		t.Error("esc didn't close the picker")
	}
	if got := claims(t, "beta"); len(got) != 0 {
		t.Errorf("Beta Mod's config-files is %v after cancelling, want nothing", got)
	}
}

func TestPickerEnterWithNothingToPickDoesNothing(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)
	press(t, s, "r", "/")
	typeText(t, s, "nothing matches this")
	press(t, s, "enter", "enter")
	if !s.modal() {
		t.Error("enter closed the picker with no mod to pick")
	}
}

func TestPickerScrollsToTheCursor(t *testing.T) {
	setUpPack(t)
	for _, name := range []string{"c", "d", "e", "f", "g", "h"} {
		addMod(t, "mod-"+name, "Mod "+strings.ToUpper(name))
	}
	s := configOn(t, packBackend{})
	s.setSize(100, 11) // little room for the list: a box of six mods doesn't fit
	goTo(t, s, rowOrphan)
	press(t, s, "r")
	for range 8 { // the pack, Alpha Mod, Beta Mod and six more: Mod H is the ninth
		press(t, s, "j")
	}
	view := s.view()
	if !strings.Contains(view, "> [ ] Mod H") {
		t.Errorf("the cursor isn't on Mod H, the last of them:\n%s", view)
	}
	if strings.Contains(view, "Alpha Mod") {
		t.Errorf("the first mod is still shown after scrolling to the last:\n%s", view)
	}
	press(t, s, "space", "enter")
	if got := claims(t, "mod-h"); !slices.Equal(got, []string{"config/orphan.json"}) {
		t.Errorf("Mod H's config-files is %v, want the file", got)
	}
}

func TestUnrelateAFileTakesOutTheEntryThatClaimsIt(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowDeepX)

	press(t, s, "x")
	view := s.view()
	for _, want := range []string{"Remove from Alpha Mod's config-files?", "config/alpha/  (covers 2 files)"} {
		if !strings.Contains(view, want) {
			t.Errorf("the prompt doesn't have %q:\n%s", want, view)
		}
	}
	press(t, s, "y")

	if got := claims(t, "alpha"); !slices.Equal(got, []string{"config/alpha.json", "config/gone.json"}) {
		t.Errorf("Alpha Mod's config-files is %v, want the folder taken out", got)
	}
	// Both files of the folder have no mod now
	want := []string{
		"▾ Alpha Mod (2)", "├── config/alpha.json", "└── config/gone.json (missing)",
		"▾ Invalid (5)", "├── config/alpha/deep/x.json", "├── config/alpha/sub.json", "├── config/orphan.json", "├── config/other/a.json", "└── config/other/b.json",
	}
	if got := tree(t, s); !slices.Equal(got, want) {
		t.Errorf("the tree is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := statusOf(s); got != "Alpha Mod no longer claims config/alpha/" {
		t.Errorf("the status is %q", got)
	}
	if got := cursorRow(t, s); got != "├── config/alpha/deep/x.json" {
		t.Errorf("the cursor is on %q, want it on the file that was unrelated", got)
	}
	assertIndexIsConsistent(t)
}

func TestUnrelateAFileThatAnEntryNamesLeavesTheFolderAlone(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowAlphaJSON)
	press(t, s, "x")
	if view := s.view(); !strings.Contains(view, "config/alpha.json") || strings.Contains(view, "covers") {
		t.Errorf("the prompt isn't just for the one file's entry:\n%s", view)
	}
	press(t, s, "enter")
	if got := claims(t, "alpha"); !slices.Equal(got, []string{"config/alpha/", "config/gone.json"}) {
		t.Errorf("Alpha Mod's config-files is %v, want only that file's entry gone", got)
	}
}

func TestUnrelateAnEntryThatMatchesNoFile(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowGone)
	press(t, s, "x")
	if view := s.view(); !strings.Contains(view, "config/gone.json  (matches no file in the pack)") {
		t.Errorf("the prompt doesn't say that the entry matches no file:\n%s", view)
	}
	press(t, s, "y")
	if got := claims(t, "alpha"); !slices.Equal(got, []string{"config/alpha.json", "config/alpha/"}) {
		t.Errorf("Alpha Mod's config-files is %v, want the entry taken out", got)
	}
	if got := tree(t, s); slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, "missing") }) {
		t.Errorf("the tree is %v, want nothing missing now", got)
	}
	assertIndexIsConsistent(t)
}

func TestUnrelateDeclinedChangesNothing(t *testing.T) {
	for _, decline := range []string{"n", "esc"} {
		t.Run(decline, func(t *testing.T) {
			s := newConfig(t)
			before, _ := os.ReadFile("mods/alpha.pw.toml")
			goTo(t, s, rowDeepX)
			press(t, s, "x")
			if !s.modal() {
				t.Fatal("x didn't open the prompt")
			}
			press(t, s, decline)
			if s.modal() {
				t.Errorf("%s didn't close the prompt", decline)
			}
			if after, _ := os.ReadFile("mods/alpha.pw.toml"); !slices.Equal(before, after) {
				t.Errorf("the mod's file changed:\n%s", after)
			}
			if got := tree(t, s); !slices.Equal(got, fixtureTree) {
				t.Errorf("the tree is %v, want it as it was", got)
			}
		})
	}
}

func TestUnrelatePromptIgnoresOtherKeys(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowDeepX)
	press(t, s, "x", "j", "r", "f", "q", "space")
	if !s.modal() {
		t.Error("another key closed the prompt")
	}
	if got := claims(t, "alpha"); len(got) != 3 {
		t.Errorf("Alpha Mod's config-files is %v, want it as it was", got)
	}
}

func TestUnrelateExplainsWhenThereIsNothingToUnrelate(t *testing.T) {
	for name, row := range map[string]int{"a file that no mod claims": rowOrphan, "a group": 0, "the Invalid group": rowInvalid} {
		t.Run(name, func(t *testing.T) {
			s := newConfig(t)
			goTo(t, s, row)
			press(t, s, "x")
			if s.modal() {
				t.Error("the prompt opened with nothing to remove")
			}
			if got := statusOf(s); !strings.Contains(got, "Nothing to unrelate here") {
				t.Errorf("the status is %q", got)
			}
		})
	}
}

func TestRefreshFindsNewFilesWithoutWritingToTheTerminal(t *testing.T) {
	s := newConfig(t)
	writeFile(t, "config/brand-new.json", "{}")
	if err := os.Remove("config/orphan.json"); err != nil {
		t.Fatalf("failed to delete the file: %v", err)
	}

	// The progress bar that refreshing draws would be written over the screen
	out := cmdtest.CaptureStdout(t, func() { press(t, s, "R") })
	if out != "" {
		t.Errorf("refreshing wrote %q to stdout", out)
	}

	got := tree(t, s)
	if !slices.Contains(got, "├── config/brand-new.json") {
		t.Errorf("the tree is %v, want the new file in it", got)
	}
	if slices.Contains(got, "├── config/orphan.json") {
		t.Errorf("the tree is %v, want the deleted file gone from it", got)
	}
	if got := statusOf(s); got != "Index refreshed" {
		t.Errorf("the status is %q", got)
	}
	assertIndexIsConsistent(t)
}

// A pack that keeps its files in a folder of a mod's own has its entries written as if it didn't, both when they are
// made and when they are taken out
func TestRelateAndUnrelateResolveEntriesAgainstTheConfigDir(t *testing.T) {
	setUpPack(t)
	cmdtest.RegisterConfigDirSource(t, "defaults", "configureddefaults")
	writeMod(t, "defaults", "Configured Defaults", "", "\n[update.defaults]\nversion = \"any\"\n")
	writeFile(t, "configureddefaults/config/sodium.json", "{}")
	writeFile(t, "configureddefaults/config/sodium/extra.json", "{}")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}

	s := configOn(t, packBackend{})
	if got := tree(t, s); !slices.Contains(got, "▾ Invalid (2)") || !slices.Contains(got, "├── configureddefaults/config/sodium.json") {
		t.Fatalf("the tree is %v, want the two files of the folder unclaimed", got)
	}
	if s.data.configDir != "configureddefaults" {
		t.Fatalf("the config dir is %q", s.data.configDir)
	}

	// Find the file, and relate its folder to Beta Mod
	goToFile(t, s, "configureddefaults/config/sodium/extra.json")
	press(t, s, "r", "tab")
	if view := s.view(); !strings.Contains(view, "Claims: config/sodium/") {
		t.Fatalf("the picker isn't claiming the folder as it is written without the config dir:\n%s", view)
	}
	press(t, s, "j", "space") // Alpha Mod, which is after the pack
	press(t, s, "enter")
	if got := claims(t, "alpha"); !slices.Contains(got, "config/sodium/") {
		t.Errorf("Alpha Mod's config-files is %v, want config/sodium/ in it, without the config dir", got)
	}
	if got := tree(t, s); !slices.Contains(got, "└── configureddefaults/config/sodium/extra.json") && !slices.Contains(got, "├── configureddefaults/config/sodium/extra.json") {
		t.Errorf("the file isn't claimed by the entry: %v", got)
	}
	got := tree(t, s)
	if slices.Contains(got, "▾ Invalid (2)") {
		t.Errorf("the tree is %v, want the folder's file claimed", got)
	}

	// Taking it out finds the entry as it is written
	press(t, s, "x")
	if view := s.view(); !strings.Contains(view, "config/sodium/") {
		t.Fatalf("the prompt doesn't have the entry:\n%s", view)
	}
	press(t, s, "y")
	if got := claims(t, "alpha"); slices.Contains(got, "config/sodium/") {
		t.Errorf("Alpha Mod's config-files is %v, want the entry gone", got)
	}
}

func TestErrorsAreShownAndTheScreenKeepsWorking(t *testing.T) {
	setUpPack(t)
	fake := &fakeBackend{data: configData{
		tree:   core.ConfigFileTree{Unclaimed: []string{"config/a.json", "config/b.json"}},
		owners: []owner{fakeOwner("Alpha", "alpha")},
	}, relateErr: errors.New("the disk is full")}
	s := configOn(t, fake)

	press(t, s, "j", "r", "enter")
	if got := statusOf(s); got != "the disk is full" {
		t.Errorf("the status is %q, want the error", got)
	}
	// It was still tried, and the pack is read again, as part of it may have been saved
	if len(fake.related) != 1 {
		t.Errorf("relate was called %d times, want once", len(fake.related))
	}
	press(t, s, "j")
	if got := cursorRow(t, s); got != "└── config/b.json" {
		t.Errorf("the cursor is on %q after the error, want the screen to carry on", got)
	}

	t.Run("reading the pack again fails", func(t *testing.T) {
		fake.loadErr = errors.New("index.toml is gone")
		press(t, s, "R")
		if got := statusOf(s); got != "index.toml is gone" {
			t.Errorf("the status is %q, want the error", got)
		}
		// What was there stays
		if got := tree(t, s); !slices.Contains(got, "└── config/b.json") {
			t.Errorf("the tree is %v, want what it had before", got)
		}
	})

	t.Run("refreshing fails", func(t *testing.T) {
		fake.loadErr = nil
		fake.refreshErr = errors.New("permission denied")
		press(t, s, "R")
		if got := statusOf(s); got != "permission denied" {
			t.Errorf("the status is %q, want the error", got)
		}
	})
}

func TestRefreshNoticesAreShown(t *testing.T) {
	setUpPack(t)
	fake := &fakeBackend{data: configData{}, notices: []string{"Notice: 2 files are no longer tracked"}}
	s := configOn(t, fake)
	press(t, s, "R")
	if got := statusOf(s); got != "Index refreshed. Notice: 2 files are no longer tracked" {
		t.Errorf("the status is %q", got)
	}
}

// Changes aren't started while one is being made, but moving about is fine
func TestKeysThatChangeThePackAreIgnoredWhileOneIsBeingMade(t *testing.T) {
	setUpPack(t)
	fake := &fakeBackend{data: configData{tree: core.ConfigFileTree{Unclaimed: []string{"config/a.json"}}, owners: []owner{fakeOwner("Alpha", "alpha")}}}
	s := configOn(t, fake)

	_, cmd := s.update(keyMsg(t, "R"))
	if cmd == nil || s.busy == "" {
		t.Fatal("R didn't start refreshing")
	}
	if got := statusOf(s); got != "Refreshing the index…" {
		t.Errorf("the status is %q, want what is being waited for", got)
	}

	if _, again := s.update(keyMsg(t, "R")); again != nil {
		t.Error("R started a second refresh")
	}
	press(t, s, "j", "r", "x")
	if s.modal() {
		t.Error("a box opened while the pack was being changed")
	}

	feed(t, s, cmd()) // it finishes
	if s.busy != "" {
		t.Errorf("still waiting for %q after it finished", s.busy)
	}
	if fake.refreshes != 1 {
		t.Errorf("the index was refreshed %d times, want once", fake.refreshes)
	}
	press(t, s, "r")
	if !s.modal() {
		t.Error("r did nothing once it was finished")
	}
}

func TestMarksOfFilesThatAreGoneAreDropped(t *testing.T) {
	setUpPack(t)
	fake := &fakeBackend{data: configData{tree: core.ConfigFileTree{Unclaimed: []string{"config/a.json", "config/b.json"}}}}
	s := configOn(t, fake)
	press(t, s, "j", "space", "j", "space")
	if got := lines(s.view())[0]; !strings.Contains(got, "2 marked") {
		t.Fatalf("the summary %q doesn't have two marked", got)
	}

	fake.data = configData{tree: core.ConfigFileTree{Unclaimed: []string{"config/b.json"}}}
	press(t, s, "R")
	if got := lines(s.view())[0]; !strings.Contains(got, "1 marked") {
		t.Errorf("the summary %q doesn't have one marked, now that one of the files is gone", got)
	}
}

func TestConfigScreenKeysAreListedForHelp(t *testing.T) {
	s := newConfig(t)
	var keys []string
	for _, b := range s.keys() {
		keys = append(keys, b.Help().Key)
	}
	for _, want := range []string{"r", "x", "space", "f", "R"} {
		if !slices.Contains(keys, want) {
			t.Errorf("the keys %v don't have %q", keys, want)
		}
	}
	for _, b := range s.keys() {
		if b.Help().Key == "" || b.Help().Desc == "" {
			t.Errorf("a key has no help: %+v", b.Help())
		}
	}

	goTo(t, s, rowOrphan)
	press(t, s, "r")
	for _, b := range s.keys() {
		if b.Help().Key == "" || b.Help().Desc == "" {
			t.Errorf("a key of the picker has no help: %+v", b.Help())
		}
	}
	if s.keys()[0].Help().Key != "enter" {
		t.Errorf("the keys of the picker start with %q, want the ones for picking", s.keys()[0].Help().Key)
	}
}

func TestPickerShowsWhereTheCursorIsOnlyWhenTheListScrolls(t *testing.T) {
	setUpPack(t)
	s := configOn(t, packBackend{})
	goTo(t, s, rowOrphan)
	press(t, s, "r")
	if view := s.view(); strings.Contains(view, "1/3") {
		t.Errorf("the picker shows a position though all three fit:\n%s", view)
	}

	s.overlay = nil
	for _, name := range []string{"c", "d", "e", "f", "g", "h"} {
		addMod(t, "mod-"+name, "Mod "+strings.ToUpper(name))
	}
	s = configOn(t, packBackend{})
	s.setSize(100, 11) // room for three mods
	goTo(t, s, rowOrphan)
	press(t, s, "r", "j", "j", "j")
	if view := s.view(); !strings.Contains(view, "4/9") {
		t.Errorf("the picker doesn't show that the cursor is on the fourth of nine owners:\n%s", view)
	}
}

// A file is claimed by an entry for each folder it is in as well as by its own, and all of them are removed: a prompt in
// a small terminal has to say so rather than lose its bottom edge
func TestUnrelatePromptWithManyEntriesFitsASmallTerminalAndCountsWhatIsLeftOut(t *testing.T) {
	setUpPack(t)
	entries := []string{"config/", "config/a/", "config/a/b/", "config/a/b/c/", "config/a/b/c/d/", "config/a/b/c/d/e.json"}
	mod := fakeMod("Deep", "deep", entries...)
	fake := &fakeBackend{data: configData{
		tree:   core.ConfigFileTree{Mods: []core.ModConfigFiles{{Mod: mod, Files: []string{"config/a/b/c/d/e.json"}}}},
		owners: []owner{modOwner(mod)},
	}}
	s := configOn(t, fake)
	// The smallest that the app gives a screen. Its summary and status take two of the lines, which leaves six for a
	// box: the two edges of the border, the title and a gap, and two for entries, one of which says how many are left out
	s.setSize(40, 8)

	press(t, s, "j", "x")
	if !s.modal() {
		t.Fatal("x didn't open the prompt")
	}
	out := lines(s.view())
	if len(out) != 8 {
		t.Fatalf("the screen is %d lines, want 8:\n%s", len(out), s.view())
	}
	view := s.view()
	if !strings.Contains(view, "╭") || !strings.Contains(view, "╰") {
		t.Errorf("the prompt lost an edge of its border:\n%s", view)
	}
	if !strings.Contains(view, "and 5 more") {
		t.Errorf("the prompt doesn't count the entries it had no room for:\n%s", view)
	}

	// All six are removed, whether or not they were shown
	press(t, s, "y")
	if len(fake.unrelated) != 1 || !slices.Equal(fake.unrelated[0][1:], entries) {
		t.Errorf("unrelate was called with %v, want all %d entries", fake.unrelated, len(entries))
	}
}

// What the filter found is picked out in the list, so it is clear why each mod is there and in that place
func TestPickerShowsWhatTheFilterMatched(t *testing.T) {
	setUpPack(t)
	addMod(t, "sodium", "Sodium")
	addMod(t, "sodium-extra", "Sodium Extra")
	addMod(t, "zzz-long-slug", "Plain")
	s := configOn(t, packBackend{})
	goTo(t, s, rowOrphan)
	press(t, s, "r", "/")
	typeText(t, s, "sod")

	// The cursor is on the first, and the second is what is shown as it is written
	cmdtest.SetColor(t, ui.Always)
	view := s.view()
	if want := ui.Info.Sprint("Sod") + "ium Extra"; !strings.Contains(view, want) {
		t.Errorf("the second mod doesn't have the letters that matched picked out as %q:\n%s", want, ui.Strip(view))
	}
	cmdtest.SetColor(t, ui.Never)

	// The letters are the same whether or not they are picked out, and with the filter cleared nothing is
	view = s.view()
	if !strings.Contains(view, "> [ ] Sodium\n") && !strings.Contains(view, "> [ ] Sodium ") {
		t.Errorf("the best match isn't first:\n%s", view)
	}
	press(t, s, "ctrl+u")
	typeText(t, s, "longslug")
	view = s.view()
	if !strings.Contains(view, "[ ] Plain  zzz-long-slug") {
		t.Errorf("a mod found by its slug doesn't show it:\n%s", view)
	}
	press(t, s, "ctrl+u")
	typeText(t, s, "plain")
	if view := s.view(); strings.Contains(view, "zzz-long-slug") {
		t.Errorf("the slug is shown though the name is what matched:\n%s", view)
	}
}
