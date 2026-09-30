package tui

import (
	"testing"

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
