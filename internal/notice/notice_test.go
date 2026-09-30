package notice

import (
	"strings"
	"sync"
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestSayPrintsALineInTheStyleOfItsLevel(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	for level, style := range map[Level]ui.Style{
		Muted: ui.Muted, Info: ui.Info, Warning: ui.Warning, Error: ui.Error, Success: ui.Success,
	} {
		out := cmdtest.CaptureStdout(t, func() { Say(level, "%d things in %s", 3, "order") })
		if want := style.Sprint("3 things in order") + "\n"; out != want {
			t.Errorf("level %d printed %q, want %q", level, out, want)
		}
	}
}

func TestSayPrintsPlainTextWhenColourIsOff(t *testing.T) {
	cmdtest.SetColor(t, ui.Never)
	out := cmdtest.CaptureStdout(t, func() { Warnf("careful with %s", ui.Bold.Sprint("that")) })
	if out != "careful with that\n" {
		t.Errorf("printed %q, want the text as it is, without any styling", out)
	}
}

func TestCollectReturnsWhatWasSaidInsteadOfPrintingIt(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	var notices []Notice
	out := cmdtest.CaptureStdout(t, func() {
		notices = Collect(func() {
			Infof("one %s", ui.Bold.Sprint("bold"))
			Warnf("two")
			Errorf("three")
			Successf("four")
			Mutedf("five")
		})
	})

	if out != "" {
		t.Errorf("printed %q while collecting", out)
	}
	want := []Notice{{Info, "one bold"}, {Warning, "two"}, {Error, "three"}, {Success, "four"}, {Muted, "five"}}
	if len(notices) != len(want) {
		t.Fatalf("collected %v, want %v", notices, want)
	}
	for i := range want {
		if notices[i] != want[i] {
			t.Errorf("notice %d is %+v, want %+v: in plain text, whatever the text was styled with", i, notices[i], want[i])
		}
	}
}

func TestCollectingIsWhetherNoticesAreBeingCollected(t *testing.T) {
	if Collecting() {
		t.Fatal("Collecting() is true outside Collect")
	}
	Collect(func() {
		if !Collecting() {
			t.Error("Collecting() is false inside Collect")
		}
	})
	if Collecting() {
		t.Error("Collecting() is still true after Collect")
	}
}

func TestCollectsCanBeNestedAndTheOuterOneGetsWhatIsSaidAfterTheInnerEnds(t *testing.T) {
	var inner []Notice
	outer := Collect(func() {
		Infof("before")
		inner = Collect(func() { Infof("inside") })
		Infof("after")
	})
	if len(inner) != 1 || inner[0].Text != "inside" {
		t.Errorf("the inner collected %v, want only what was said inside it", inner)
	}
	var texts []string
	for _, n := range outer {
		texts = append(texts, n.Text)
	}
	if got := strings.Join(texts, ","); got != "before,after" {
		t.Errorf("the outer collected %q, want what was said outside the inner", got)
	}
}

func TestCollectReturnsNothingForWorkThatSaysNothing(t *testing.T) {
	if notices := Collect(func() {}); len(notices) != 0 {
		t.Errorf("collected %v, want nothing", notices)
	}
}

func TestSayingFromSeveralGoroutinesWhileCollectingIsSafe(t *testing.T) {
	notices := Collect(func() {
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				Infof("x")
			}()
		}
		wg.Wait()
	})
	if len(notices) != 20 {
		t.Errorf("collected %d notices, want all 20", len(notices))
	}
}
