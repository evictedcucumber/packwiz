package fuzzy

import (
	"slices"
	"testing"
)

func TestParseSplitsIntoLowercaseWords(t *testing.T) {
	q := Parse("  Sod   EX\tfoo ")
	if len(q.terms) != 3 || string(q.terms[0]) != "sod" || string(q.terms[1]) != "ex" || string(q.terms[2]) != "foo" {
		t.Errorf("terms = %q, want the three words in lowercase", q.terms)
	}
	for _, blank := range []string{"", "   ", "\t"} {
		if !Parse(blank).Empty() {
			t.Errorf("Parse(%q) isn't empty", blank)
		}
	}
	if Parse("a").Empty() {
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

func TestMatchNeedsEveryWordInAnyOrder(t *testing.T) {
	q := Parse("extra sod")
	if _, ok := q.Match("Sodium Extra"); !ok {
		t.Error("'extra sod' didn't match Sodium Extra: both words, in either order")
	}
	for _, text := range []string{"Sodium", "Extra", "Lexicon"} {
		if _, ok := q.Match(text); ok {
			t.Errorf("'extra sod' matched %q, which doesn't have both words", text)
		}
	}
}

// Each word is in the first text that has it, which can be a different one for each
func TestMatchFindsEachWordInTheFirstTextThatHasIt(t *testing.T) {
	name, slug := "Plain", "zzz-long-slug"

	m, ok := Parse("longslug").Match(name, slug)
	if !ok || len(m.Positions[0]) != 0 || len(m.Positions[1]) == 0 {
		t.Fatalf("Match() = %+v, %v, want it in the slug only", m, ok)
	}

	m, ok = Parse("plain slug").Match(name, slug)
	if !ok || len(m.Positions[0]) == 0 || len(m.Positions[1]) == 0 {
		t.Errorf("Match() = %+v, %v, want one word in the name and one in the slug", m, ok)
	}

	// A word that both have is in the name, which comes first, and isn't shown twice
	m, ok = Parse("pl").Match(name, "plain")
	if !ok || !slices.Equal(m.Positions[0], []int{0, 1}) || len(m.Positions[1]) != 0 {
		t.Errorf("Match() = %+v, %v, want it in the first text only", m, ok)
	}

	if _, ok := Parse("plain nope").Match(name, slug); ok {
		t.Error("a word in neither text matched")
	}
}

func TestMatchPositionsAreInOrderWithoutRepeats(t *testing.T) {
	// The words overlap: s in "sod" and in "sm", and the characters are shown once
	m, ok := Parse("sm sod").Match("Sodium")
	if !ok {
		t.Fatal("no match")
	}
	if !slices.IsSorted(m.Positions[0]) || len(slices.Compact(slices.Clone(m.Positions[0]))) != len(m.Positions[0]) {
		t.Errorf("positions = %v, want them in order without repeats", m.Positions[0])
	}
}

func TestEmptyQueryMatchesEverythingWithNothingToShow(t *testing.T) {
	m, ok := Parse("  ").Match("anything", "else")
	if !ok || m.Score != 0 || len(m.Positions) != 2 || len(m.Positions[0]) != 0 || len(m.Positions[1]) != 0 {
		t.Errorf("Match() = %+v, %v, want a match with no score and nothing to show", m, ok)
	}
	if m, ok := Parse("").Match(); !ok || len(m.Positions) != 0 {
		t.Errorf("Match() of no texts = %+v, %v", m, ok)
	}
}

// A match with nothing to match against is no match, unless there was nothing to find
func TestMatchWithNoTextsMatchesNothing(t *testing.T) {
	if _, ok := Parse("a").Match(); ok {
		t.Error("a word matched with no text to find it in")
	}
}

func TestMatchScoresABetterMatchHigher(t *testing.T) {
	q := Parse("mod")
	better, _ := q.Match("Zed Mod")
	worse, _ := q.Match("Amod")
	if better.Score <= worse.Score {
		t.Errorf("scores are %d and %d, want a match at the start of a word to be higher", better.Score, worse.Score)
	}
}
