package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// search types a search into the screen and leaves it being typed.
func search(t *testing.T, s *configScreen, text string) {
	t.Helper()
	press(t, s, "/")
	typeText(t, s, text)
}

func TestSearchFiltersTheTreeAsItIsTyped(t *testing.T) {
	s := newConfig(t)
	press(t, s, "/")
	if !s.modal() {
		t.Error("/ didn't start a search: keys should go to it now, which is what being modal says")
	}
	if got := statusOf(s); got != "/ █" {
		t.Errorf("the status line is %q, want the search being typed", got)
	}

	typeText(t, s, "orph")
	if got, want := tree(t, s), []string{"▾ Invalid (1)", "└── config/orphan.json"}; !slices.Equal(got, want) {
		t.Errorf("the tree is %v, want only what matches, in its group", got)
	}
	if got := statusOf(s); got != "/ orph█" {
		t.Errorf("the status line is %q", got)
	}
	// The cursor is on the file, not the group, as that is what is being looked for
	if got := cursorRow(t, s); got != "└── config/orphan.json" {
		t.Errorf("the cursor is on %q, want the file that matches", got)
	}

	// It gets wider as words are added, and narrows
	press(t, s, "ctrl+u")
	typeText(t, s, "alpha sub")
	if got, want := tree(t, s), []string{"▾ Alpha Mod (1)", "└── config/alpha/sub.json"}; !slices.Equal(got, want) {
		t.Errorf("'alpha sub': the tree is %v, want %v", got, want)
	}
	press(t, s, "backspace", "backspace", "backspace")
	if got := len(tree(t, s)); got != 5 {
		t.Errorf("'alpha': the tree has %d rows, want the whole of Alpha Mod: %v", got, tree(t, s))
	}
}

func TestSearchIsFuzzy(t *testing.T) {
	s := newConfig(t)
	search(t, s, "cfgorph") // not anywhere in the path as it is written
	if got, want := tree(t, s), []string{"▾ Invalid (1)", "└── config/orphan.json"}; !slices.Equal(got, want) {
		t.Errorf("the tree is %v, want the file that has those characters in order", got)
	}
}

func TestSearchFindsFilesByTheNameOfTheirGroup(t *testing.T) {
	s := newConfig(t)
	search(t, s, "alpha")
	want := []string{"▾ Alpha Mod (4)", "├── config/alpha.json", "├── config/alpha/deep/x.json", "├── config/alpha/sub.json", "└── config/gone.json (missing)"}
	if got := tree(t, s); !slices.Equal(got, want) {
		t.Errorf("the tree is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSearchPutsTheCursorOnTheBestMatch(t *testing.T) {
	s := newConfig(t)
	search(t, s, "a.json")
	best := 0
	for _, r := range s.rows {
		if r.kind != groupRow && r.score > best {
			best = r.score
		}
	}
	r, ok := s.current()
	if !ok || r.kind == groupRow || r.score != best {
		t.Fatalf("the cursor is on %+v, want a file with the best score, %d", r, best)
	}
	// Of those that are as good, the first, so that the cursor isn't moving about among them
	for i, other := range s.rows {
		if other.kind != groupRow && other.score == best {
			if i != s.cursor {
				t.Errorf("the cursor is on row %d, but row %d matches as well", s.cursor, i)
			}
			break
		}
	}
}

func TestSearchEnterLeavesItAsTheSearchAndKeysAreCommandsAgain(t *testing.T) {
	s := newConfig(t)
	search(t, s, "json")
	want := tree(t, s)
	press(t, s, "enter")

	if s.modal() {
		t.Error("enter left the search being typed")
	}
	if got := statusOf(s); got != "/ json  (esc clears)" {
		t.Errorf("the status line is %q, want the search that is left on, and how to clear it", got)
	}
	if got := tree(t, s); !slices.Equal(got, want) {
		t.Errorf("the tree changed from %v to %v", want, got)
	}

	// Keys are commands: f cycles what is shown, without losing the search
	press(t, s, "f")
	if got := statusOf(s); got != "Showing valid" {
		t.Errorf("the status line is %q after f", got)
	}
	for _, line := range tree(t, s) {
		if strings.Contains(line, "orphan") || strings.Contains(line, "other/") {
			t.Errorf("the tree has %q, which isn't valid, so isn't shown: %v", line, tree(t, s))
		}
	}
	press(t, s, "j")
	press(t, s, "f", "f", "f") // back to all; the search is what shows the status line again
	if got := statusOf(s); got != "Showing all" {
		t.Errorf("the status line is %q", got)
	}
}

func TestSearchEscapeClearsItAndTheTreeComesBack(t *testing.T) {
	s := newConfig(t)
	search(t, s, "orph")
	press(t, s, "esc")
	if s.modal() {
		t.Error("esc left the search being typed")
	}
	if got := tree(t, s); !slices.Equal(got, fixtureTree) {
		t.Errorf("the tree is %v, want all of it once the search is cleared", got)
	}
	if got := statusOf(s); got != "" {
		t.Errorf("the status line is %q, want nothing", got)
	}
}

// A search that is left on is cleared by the first esc, and marks by the next
func TestSearchThatIsLeftOnIsClearedBeforeMarks(t *testing.T) {
	s := newConfig(t)
	goTo(t, s, rowOrphan)
	press(t, s, "space")
	search(t, s, "orph")
	press(t, s, "enter")

	press(t, s, "esc")
	if got := tree(t, s); !slices.Equal(got, fixtureTree) {
		t.Errorf("the tree is %v, want the search cleared by the first esc", got)
	}
	if got := lines(s.view())[0]; !strings.Contains(got, "1 marked") {
		t.Errorf("the summary %q doesn't still have the file marked", got)
	}
	press(t, s, "esc")
	if got := lines(s.view())[0]; strings.Contains(got, "marked") {
		t.Errorf("the summary %q still has a file marked after the second esc", got)
	}
}

func TestSearchArrowKeysMoveThroughWhatMatchesAndLettersAreText(t *testing.T) {
	s := newConfig(t)
	search(t, s, "json")
	before := s.cursor
	press(t, s, "down")
	if s.cursor != before+1 {
		t.Errorf("down moved the cursor from %d to %d, want one row on", before, s.cursor)
	}
	press(t, s, "up", "up")
	if s.cursor != max(before-1, 0) {
		t.Errorf("up twice left the cursor at %d", s.cursor)
	}

	// j and k move in the tree, but are letters of a name here
	press(t, s, "j", "k")
	if got := statusOf(s); got != "/ jsonjk█" {
		t.Errorf("the status line is %q, want j and k typed into the search", got)
	}
}

func TestSearchCanHaveSpacesAndPasteInIt(t *testing.T) {
	s := newConfig(t)
	search(t, s, "alpha sub")
	if got := statusOf(s); got != "/ alpha sub█" {
		t.Errorf("the status line is %q", got)
	}
	press(t, s, "ctrl+u")
	feed(t, s, tea.PasteMsg{Content: "orph"})
	if got, want := tree(t, s), []string{"▾ Invalid (1)", "└── config/orphan.json"}; !slices.Equal(got, want) {
		t.Errorf("after pasting the tree is %v, want %v", got, want)
	}
}

func TestSearchWithNothingFoundSaysSo(t *testing.T) {
	s := newConfig(t)
	search(t, s, "zzz")
	view := s.view()
	for _, want := range []string{`No config file matches "zzz".`, "esc clears the search."} {
		if !strings.Contains(view, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, view)
		}
	}
	// Nothing to act on, and keys that need a file say so rather than doing something
	press(t, s, "enter", "r")
	press(t, s, "r")
	if s.overlay != nil {
		t.Error("a box opened with nothing shown")
	}
}

// A group that is folded isn't while it is searched, as what matches would be hidden in it, and is when the search is gone
func TestSearchShowsWhatIsInAFoldedGroupAndItIsFoldedAgainAfter(t *testing.T) {
	s := newConfig(t)
	press(t, s, "left") // folds Alpha Mod
	if got := tree(t, s)[0]; got != "▸ Alpha Mod (4)" {
		t.Fatalf("the first row is %q, want Alpha Mod folded", got)
	}

	search(t, s, "sub")
	if got, want := tree(t, s), []string{"▾ Alpha Mod (1)", "└── config/alpha/sub.json"}; !slices.Equal(got, want) {
		t.Errorf("the tree is %v, want the file in the folded group", got)
	}
	press(t, s, "esc")
	if got := tree(t, s)[0]; got != "▸ Alpha Mod (4)" {
		t.Errorf("the first row is %q, want Alpha Mod folded again", got)
	}
}

// What a search found is what is marked and related from a group: its files that matched, not all of them
func TestSearchMakesMarkingAndRelatingAGroupActOnWhatMatches(t *testing.T) {
	s := newConfig(t)
	search(t, s, "other")
	press(t, s, "enter")
	goToRowWhere(t, s, "the Invalid group", func(drawn string) bool { return strings.Contains(drawn, "Invalid") })

	press(t, s, "space")
	if got := lines(s.view())[0]; !strings.Contains(got, "2 marked") {
		t.Errorf("the summary %q doesn't have the two files that matched marked, not the group's three", got)
	}
	press(t, s, "esc", "esc") // the search, then the marks
	goToRowWhere(t, s, "the Invalid group", func(drawn string) bool { return strings.Contains(drawn, "Invalid") })
	search(t, s, "other")
	press(t, s, "enter")
	goToRowWhere(t, s, "the Invalid group", func(drawn string) bool { return strings.Contains(drawn, "Invalid") })
	press(t, s, "r")
	if view := s.view(); !strings.Contains(view, "Relate 2 files to") || !strings.Contains(view, "Claims: config/other/a.json, config/other/b.json") {
		t.Errorf("the picker isn't for the two files that matched:\n%s", view)
	}
}

func TestSearchStaysAfterRefreshingAndChanging(t *testing.T) {
	s := newConfig(t)
	search(t, s, "orph")
	press(t, s, "enter")
	press(t, s, "R")
	if got, want := tree(t, s), []string{"▾ Invalid (1)", "└── config/orphan.json"}; !slices.Equal(got, want) {
		t.Errorf("after a refresh the tree is %v, want the search still applied", got)
	}

	// Relating the file that matched moves it to its mod, which still matches
	press(t, s, "r", "j", "j", "enter") // Alpha Mod is after the pack... there is no loader here, so: the pack, Alpha Mod, Beta Mod
	if got := tree(t, s); len(got) != 2 || !strings.HasSuffix(got[1], "config/orphan.json") {
		t.Errorf("after relating the tree is %v, want the file under its new owner, still matching", got)
	}
}

func TestSearchKeysAreListedForHelp(t *testing.T) {
	s := newConfig(t)
	var keys []string
	for _, b := range s.keys() {
		keys = append(keys, b.Help().Key)
	}
	if !slices.Contains(keys, "/") {
		t.Errorf("the keys %v don't have /", keys)
	}
	press(t, s, "/")
	keys = nil
	for _, b := range s.keys() {
		if b.Help().Key == "" || b.Help().Desc == "" {
			t.Errorf("a key of the search has no help: %+v", b.Help())
		}
		keys = append(keys, b.Help().Key)
	}
	if !slices.Contains(keys, "enter") || !slices.Contains(keys, "esc") {
		t.Errorf("the keys while searching are %v, want enter and esc", keys)
	}
}

// What the search found is picked out, in the colour that the picker picks it out in
func TestSearchPicksOutWhatMatchedInColour(t *testing.T) {
	s := newConfig(t)
	search(t, s, "orph")
	press(t, s, "enter")

	plain := s.view()
	cmdtest.SetColor(t, ui.Always)
	coloured := s.view()
	if want := ui.Info.Sprint("orph"); !strings.Contains(coloured, want) {
		t.Errorf("the letters that matched aren't picked out as %q:\n%s", want, ui.Strip(coloured))
	}
	if ui.Strip(coloured) != plain {
		t.Errorf("colour changed what is said:\nplain:\n%s\nstripped:\n%s", plain, ui.Strip(coloured))
	}

	// And in the name of a group that a word was found in
	s.search.clear()
	s.searching = true
	typeText(t, s, "alpha")
	if want := ui.Bold.Sprint(ui.Info.Sprint("Alpha") + " Mod"); !strings.Contains(s.view(), want) {
		t.Errorf("the group's name doesn't have what matched picked out:\n%s", ui.Strip(s.view()))
	}
}

// The app leaves q and ? to the search, which they are letters of, as it does for the picker
func TestAppLeavesKeysToASearchBeingTyped(t *testing.T) {
	a := newTestApp(t, 100, 30)
	if appPress(t, a, "/", "q", "?") {
		t.Error("q quit from inside the search")
	}
	if a.help {
		t.Error("? opened help from inside the search")
	}
	out := lines(a.render())
	if got := out[len(out)-2]; got != "/ q?█" {
		t.Errorf("the line above the keys is %q, want q and ? typed into the search", got)
	}
	if footer := out[len(out)-1]; strings.Contains(footer, "quit") || !strings.Contains(footer, "enter") {
		t.Errorf("the footer %q offers the keys of the tree while a search is typed", footer)
	}
	if !appPress(t, a, "ctrl+c") {
		t.Error("ctrl+c didn't quit from inside the search")
	}
}
