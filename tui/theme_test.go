package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

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

func TestWrapBreaksAtSpacesAndIndentsWhatFollows(t *testing.T) {
	got := wrap("one two three four five six", 12, "  ")
	want := []string{"one two", "  three four", "  five six"}
	if !slices.Equal(got, want) {
		t.Errorf("wrap() = %q, want %q", got, want)
	}
}

func TestWrapLeavesWhatFitsAsItIs(t *testing.T) {
	for _, text := range []string{"", "short", "exactly ten"} {
		if got := wrap(text, 11, "  "); !slices.Equal(got, []string{text}) {
			t.Errorf("wrap(%q) = %q, want it as it is", text, got)
		}
	}
}

func TestWrapKeepsEachLineOfTextThatHasSeveral(t *testing.T) {
	got := wrap("first line\nsecond line is longer than the room", 12, "> ")
	want := []string{"first line", "second line", "> is longer", "> than the", "> room"}
	if !slices.Equal(got, want) {
		t.Errorf("wrap() = %q, want %q", got, want)
	}
}

func TestWrapKeepsStylingAndDoesNotCountIt(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	text := ui.Bold.Sprint("bold words") + " and then some plain ones"
	for _, line := range wrap(text, 14, "  ") {
		if w := ansi.StringWidth(line); w > 14 {
			t.Errorf("the line %q is %d columns wide, want no more than 14", ui.Strip(line), w)
		}
	}
	if got := ui.Strip(strings.Join(wrap(text, 14, ""), " ")); got != "bold words and then some plain ones" {
		t.Errorf("wrapping lost or changed text: %q", got)
	}
}

func TestWrapGivesUpOnAWidthWithNoRoomForTheIndent(t *testing.T) {
	if got := wrap("a b c d e f g h", 3, "    "); len(got) != 1 {
		t.Errorf("wrap() = %q, want the text as one line when there is no room to indent what follows", got)
	}
	if got := wrap("x", 0, ""); got != nil {
		t.Errorf("wrap() = %q, want nothing for no width", got)
	}
}
