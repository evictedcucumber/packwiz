package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newTestApp makes the app on the pack that setUpPack makes, in a terminal of the given size.
func newTestApp(t *testing.T, width, height int) *app {
	t.Helper()
	setUpPack(t)
	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	a := newApp(data.pack, newConfigScreen(packBackend{}, data))
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return a
}

// appPress presses keys on the app, running what they start, and says whether one of them quit.
func appPress(t *testing.T, a *app, keys ...string) (quit bool) {
	t.Helper()
	for _, k := range keys {
		msg := tea.Msg(keyMsg(t, k))
		for range 20 {
			_, cmd := a.Update(msg)
			if cmd == nil {
				break
			}
			if msg = cmd(); msg == nil {
				break
			}
			if _, ok := msg.(tea.QuitMsg); ok {
				return true
			}
		}
	}
	return false
}

func TestAppDrawsAHeaderAScreenAndAFooterThatFillTheTerminal(t *testing.T) {
	a := newTestApp(t, 100, 26)
	out := lines(a.render())

	if len(out) != 26 {
		t.Errorf("the app is %d lines, want as many as the terminal has", len(out))
	}
	if want := "packwiz  Test Pack 1.2.0  ›  Config"; out[0] != want {
		t.Errorf("the header is %q, want %q", out[0], want)
	}
	if !strings.HasPrefix(out[1], "Config files") {
		t.Errorf("the second line is %q, want the screen's own first line", out[1])
	}
	footer := out[len(out)-1]
	for _, want := range []string{"r relate", "x unrelate", "? help", "q quit"} {
		if !strings.Contains(footer, want) {
			t.Errorf("the footer %q doesn't have %q", footer, want)
		}
	}
}

func TestAppFooterKeepsHelpAndQuitWhenTheKeysDontAllFit(t *testing.T) {
	a := newTestApp(t, 44, 12)
	out := lines(a.render())
	footer := out[len(out)-1]
	if !strings.HasSuffix(footer, "? help  q quit") {
		t.Errorf("the footer %q doesn't end with help and quit", footer)
	}
	if !strings.HasPrefix(footer, "r relate") {
		t.Errorf("the footer %q doesn't start with the most useful key", footer)
	}
	if strings.Contains(footer, "refresh index") {
		t.Errorf("the footer %q has every key though it is narrow", footer)
	}
}

func TestAppGivesTheScreenTheRoomThatIsLeft(t *testing.T) {
	a := newTestApp(t, 90, 30)
	cs := a.screen.(*configScreen)
	if cs.width != 90 || cs.height != 28 {
		t.Errorf("the screen was given %dx%d, want 90x28 (the terminal less a header and a footer)", cs.width, cs.height)
	}
	a.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if cs.width != 70 || cs.height != 18 {
		t.Errorf("after resizing the screen was given %dx%d, want 70x18", cs.width, cs.height)
	}
}

func TestAppDrawsNothingBeforeItKnowsTheSize(t *testing.T) {
	setUpPack(t)
	data, _ := packBackend{}.load()
	a := newApp(data.pack, newConfigScreen(packBackend{}, data))
	if got := a.render(); got != "" {
		t.Errorf("the app drew %q before the terminal's size was known", got)
	}
}

func TestAppSaysWhenTheTerminalIsTooSmall(t *testing.T) {
	for _, size := range [][2]int{{39, 24}, {80, 9}, {30, 5}} {
		out := newTestApp(t, size[0], size[1]).render()
		if !strings.Contains(out, "Terminal too small") {
			t.Errorf("%dx%d: the app drew %q, want it to say the terminal is too small", size[0], size[1], out)
		}
	}
	// However small, what is drawn isn't wider than the terminal, which would wrap and make a mess of it
	for _, size := range [][2]int{{39, 24}, {80, 9}, {10, 3}, {1, 1}} {
		for _, l := range lines(newTestApp(t, size[0], size[1]).render()) {
			if len([]rune(l)) > size[0] {
				t.Errorf("%dx%d: the line %q is wider than the terminal", size[0], size[1], l)
			}
		}
	}
	// And draws when it isn't
	if out := newTestApp(t, 40, 10).render(); strings.Contains(out, "too small") {
		t.Errorf("40x10 is big enough, but the app drew %q", out)
	}
}

func TestAppUsesTheAlternateScreen(t *testing.T) {
	a := newTestApp(t, 80, 24)
	if !a.View().AltScreen {
		t.Error("the app doesn't ask for the alternate screen, so it would be drawn over the user's terminal history")
	}
}

func TestAppQuitsOnQAndCtrlC(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		if !appPress(t, newTestApp(t, 80, 24), k) {
			t.Errorf("%s didn't quit", k)
		}
	}
}

func TestAppLeavesKeysToABoxThatIsOpen(t *testing.T) {
	a := newTestApp(t, 100, 30)
	appPress(t, a, "j", "j", "j", "j", "j", "j", "r")
	if !a.screen.modal() {
		t.Fatal("r didn't open the picker")
	}

	// q and ? are letters of a mod's name when the filter is being typed in
	if appPress(t, a, "/", "q", "?") {
		t.Error("q quit from inside the picker")
	}
	if a.help {
		t.Error("? opened help from inside the picker")
	}
	if out := a.render(); !strings.Contains(out, "Filter: q?█") {
		t.Errorf("q and ? weren't typed into the filter:\n%s", out)
	}
	// And the footer doesn't offer a key that doesn't work now
	footer := lines(a.render())
	if strings.Contains(footer[len(footer)-1], "quit") {
		t.Errorf("the footer %q offers quit while the picker is open", footer[len(footer)-1])
	}

	// ctrl+c gets out from anywhere
	if !appPress(t, a, "ctrl+c") {
		t.Error("ctrl+c didn't quit from inside the picker")
	}
}

func TestAppHelpListsTheKeysAndIsLeftWithQuestionMarkEscOrQ(t *testing.T) {
	for _, closeKey := range []string{"?", "esc", "q"} {
		t.Run(closeKey, func(t *testing.T) {
			a := newTestApp(t, 100, 30)
			if appPress(t, a, "?") {
				t.Fatal("? quit")
			}
			out := a.render()
			for _, want := range []string{"Config keys", "relate", "unrelate", "refresh index", "quit", "Press ? or esc to go back"} {
				if !strings.Contains(out, want) {
					t.Errorf("help doesn't have %q:\n%s", want, out)
				}
			}
			if got := lines(out); len(got) != 30 || !strings.Contains(got[len(got)-1], "?/esc back") {
				t.Errorf("help is %d lines with a footer of %q, want the terminal filled and a footer for going back", len(got), got[len(got)-1])
			}

			// Keys that aren't for help do nothing to what is behind it
			appPress(t, a, "j", "r", "x")
			if a.screen.modal() || !a.help {
				t.Error("keys that aren't for help did something while it was open")
			}

			if appPress(t, a, closeKey) {
				t.Errorf("%s quit, rather than closing help", closeKey)
			}
			if a.help {
				t.Errorf("%s didn't close help", closeKey)
			}
			if out := a.render(); strings.Contains(out, "Config keys") {
				t.Errorf("help is still drawn:\n%s", out)
			}
		})
	}
}

func TestAppCtrlCQuitsFromHelp(t *testing.T) {
	a := newTestApp(t, 80, 24)
	appPress(t, a, "?")
	if !appPress(t, a, "ctrl+c") {
		t.Error("ctrl+c didn't quit from help")
	}
}

func TestAppHelpFitsASmallTerminal(t *testing.T) {
	a := newTestApp(t, 40, 10)
	appPress(t, a, "?")
	out := lines(a.render())
	if len(out) != 10 {
		t.Errorf("help is %d lines in a terminal of 10", len(out))
	}
	for _, l := range out {
		if len([]rune(l)) > 40 {
			t.Errorf("the line %q is wider than the terminal", l)
		}
	}
}
