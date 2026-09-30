package tui

import (
	"slices"
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestParseQuerySplitsIntoLowercaseWords(t *testing.T) {
	q := parseQuery("  Sod   EX\tfoo ")
	if len(q.terms) != 3 || string(q.terms[0]) != "sod" || string(q.terms[1]) != "ex" || string(q.terms[2]) != "foo" {
		t.Errorf("terms = %q, want the three words in lowercase", q.terms)
	}
	for _, blank := range []string{"", "   ", "\t"} {
		if !parseQuery(blank).empty() {
			t.Errorf("parseQuery(%q) isn't empty", blank)
		}
	}
	if parseQuery("a").empty() {
		t.Error("a query with a word is empty")
	}
}

func TestMatchTermFindsCharactersInOrderButNotTogether(t *testing.T) {
	score, positions, ok := matchTerm([]rune("sdm"), "Sodium")
	if !ok || score <= 0 {
		t.Fatalf("matchTerm() = %d, %v, want a match: sdm is in Sodium in that order", score, ok)
	}
	if want := []int{0, 2, 5}; !slices.Equal(positions, want) {
		t.Errorf("positions = %v, want %v", positions, want)
	}

	// Out of order, and characters that aren't there, don't match
	for _, term := range []string{"mds", "sodiumm", "xyz", "soz"} {
		if _, _, ok := matchTerm([]rune(term), "Sodium"); ok {
			t.Errorf("%q matched Sodium", term)
		}
	}
}

func TestMatchTermIgnoresCase(t *testing.T) {
	for _, text := range []string{"Sodium", "SODIUM", "sodium"} {
		if _, _, ok := matchTerm([]rune("sod"), text); !ok {
			t.Errorf("sod didn't match %q", text)
		}
	}
}

func TestMatchTermScoresAWordStartAboveTheMiddleOfAWord(t *testing.T) {
	word, _, _ := matchTerm([]rune("mod"), "Zed Mod")
	middle, _, _ := matchTerm([]rune("mod"), "Amod")
	if word <= middle {
		t.Errorf("a match at the start of a word scores %d, and one inside a word %d, want the first higher", word, middle)
	}
	together, _, _ := matchTerm([]rune("sod"), "Sodium")
	apart, _, _ := matchTerm([]rune("sod"), "Sea Orchid Dye")
	if together <= apart {
		t.Errorf("a match of characters together scores %d, and one of characters apart %d, want the first higher", together, apart)
	}
}

// Positions are of characters, so that what is shown is what matched, however many bytes it takes to write the text
func TestMatchTermPositionsAreOfRunesAndIgnoreAccents(t *testing.T) {
	_, positions, ok := matchTerm([]rune("mod"), "日本 Mod")
	if !ok || !slices.Equal(positions, []int{3, 4, 5}) {
		t.Errorf("positions = %v, %v, want 3 4 5: the characters after the two that take three bytes each", positions, ok)
	}
	_, positions, ok = matchTerm([]rune("cafe"), "Café Mod")
	if !ok || !slices.Equal(positions, []int{0, 1, 2, 3}) {
		t.Errorf("cafe in Café Mod: positions = %v, %v, want an é to be found by an e", positions, ok)
	}
}

func TestMergePositionsIsInOrderWithoutRepeats(t *testing.T) {
	if got := mergePositions([]int{5, 1, 3}, nil, []int{3, 2, 9}); !slices.Equal(got, []int{1, 2, 3, 5, 9}) {
		t.Errorf("mergePositions() = %v", got)
	}
	if got := mergePositions(); len(got) != 0 {
		t.Errorf("mergePositions() of nothing = %v", got)
	}
}

func TestHighlightStylesRunsOfMatchedCharacters(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	got := highlight("Sodium Extra", []int{0, 1, 2, 7, 8}, ui.Info)
	want := ui.Info.Sprint("Sod") + "ium " + ui.Info.Sprint("Ex") + "tra"
	if got != want {
		t.Errorf("highlight() = %q, want %q", got, want)
	}

	// Characters are indexes of runes, not bytes
	got = highlight("日本 Mod", []int{0, 3}, ui.Info)
	want = ui.Info.Sprint("日") + "本 " + ui.Info.Sprint("M") + "od"
	if got != want {
		t.Errorf("highlight() of multi-byte text = %q, want %q", got, want)
	}

	if got := highlight("Sodium", nil, ui.Info); got != "Sodium" {
		t.Errorf("highlight() with nothing to show = %q, want the text as it is", got)
	}
	// Whatever is matched, what was said is the same: colour only adds
	if got := ui.Strip(highlight("Sodium Extra", []int{0, 7, 11}, ui.Info)); got != "Sodium Extra" {
		t.Errorf("without its colour highlight() = %q, want the text", got)
	}
	cmdtest.SetColor(t, ui.Never)
	if got := highlight("Sodium Extra", []int{0, 7, 11}, ui.Info); got != "Sodium Extra" {
		t.Errorf("with colour off highlight() = %q, want the text", got)
	}
}
