package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
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
		if appFeed(t, a, keyMsg(t, k)) {
			return true
		}
	}
	return false
}

// appFeed gives the app a message, and then what the commands it starts come back with, as a program does, and says
// whether that ended in it quitting.
func appFeed(t *testing.T, a *app, msg tea.Msg) (quit bool) {
	t.Helper()
	queue := []tea.Msg{msg}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 1000 {
			t.Fatal("commands kept starting more commands")
		}
		next := queue[0]
		queue = queue[1:]
		switch m := next.(type) {
		case nil:
			continue
		case tea.QuitMsg:
			return true
		case tea.BatchMsg:
			for _, cmd := range m {
				if cmd != nil {
					queue = append(queue, cmd())
				}
			}
			continue
		}
		_, cmd := a.Update(next)
		if cmd != nil {
			queue = append(queue, cmd())
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
	if want := "packwiz  Test Pack 1.2.0"; out[0] != want {
		t.Errorf("the header is %q, want %q", out[0], want)
	}
	if want := "[1 Config]"; out[1] != want {
		t.Errorf("the row of screens is %q, want %q", out[1], want)
	}
	if !strings.HasPrefix(out[2], "Config files") {
		t.Errorf("the third line is %q, want the screen's own first line", out[2])
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
	if cs.width != 90 || cs.height != 27 {
		t.Errorf("the screen was given %dx%d, want 90x27 (the terminal less a header, a row of screens and a footer)", cs.width, cs.height)
	}
	a.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if cs.width != 70 || cs.height != 17 {
		t.Errorf("after resizing the screen was given %dx%d, want 70x17", cs.width, cs.height)
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

// newNavigableApp makes the app with the screens the TUI has, on the pack that setUpPack makes.
func newNavigableApp(t *testing.T, width, height int) *app {
	t.Helper()
	setUpPack(t)
	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	a := newApp(data.pack, newScreens(packBackend{}, data)...)
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	appFeed(t, a, a.Init()())
	return a
}

func TestAppStartsOnTheOverview(t *testing.T) {
	a := newNavigableApp(t, 100, 30)
	out := lines(a.render())
	if !strings.HasPrefix(out[1], "[1 Overview] 2 Mods 3 ") {
		t.Errorf("the row of screens is %q, want the overview picked out, in brackets, with the rest after it", out[1])
	}
	if !strings.Contains(strings.Join(out, "\n"), "Test Pack") {
		t.Errorf("the overview doesn't say what the pack is:\n%s", strings.Join(out, "\n"))
	}
}

func TestAppGoesToAScreenByNumberAndWithTab(t *testing.T) {
	a := newNavigableApp(t, 100, 30)
	title := func() string { return a.screen.title() }
	titleAt := func(i int) string { return a.screens[i].title() }

	appPress(t, a, "2")
	if title() != titleAt(1) {
		t.Errorf("2 went to %s, want the second screen, %s", title(), titleAt(1))
	}
	appPress(t, a, "tab")
	if title() != titleAt(2) {
		t.Errorf("tab went to %s, want the next screen, %s", title(), titleAt(2))
	}
	appPress(t, a, "shift+tab", "shift+tab")
	if title() != titleAt(0) {
		t.Errorf("shift+tab twice went to %s, want %s", title(), titleAt(0))
	}
	appPress(t, a, "shift+tab")
	if last := len(a.screens) - 1; title() != titleAt(last) {
		t.Errorf("shift+tab from the first screen went to %s, want it to wrap round to the last, %s", title(), titleAt(last))
	}
	appPress(t, a, "tab")
	if title() != titleAt(0) {
		t.Errorf("tab from the last screen went to %s, want it to wrap round to %s", title(), titleAt(0))
	}
	appPress(t, a, "9")
	if len(a.screens) < 9 && title() != titleAt(0) {
		t.Errorf("9 went to %s, but there is no ninth screen, so it should have stayed", title())
	}
}

func TestAppGoesThroughEveryScreenWithTabWithoutGettingStuck(t *testing.T) {
	a := newNavigableApp(t, 100, 30)
	for i := 1; i <= len(a.screens); i++ {
		appPress(t, a, "tab")
		if want := a.screens[i%len(a.screens)].title(); a.screen.title() != want {
			t.Fatalf("tab number %d went to %s, want %s: a screen took the key and kept it", i, a.screen.title(), want)
		}
		if a.screen.modal() {
			t.Fatalf("%s is modal as soon as it is shown, which takes tab and the numbers from the keys that go through the screens", a.screen.title())
		}
	}
}

func TestAppLeavesTabAndNumbersToAScreenThatIsTakingText(t *testing.T) {
	a := newNavigableApp(t, 100, 30)
	appPress(t, a, "2", "/")
	if !a.screen.modal() {
		t.Fatal("the search isn't being typed")
	}
	appPress(t, a, "3", "tab", "q")
	if a.screen.title() != "Mods" {
		t.Errorf("a number or tab changed screens while text was being typed, which went to %s", a.screen.title())
	}
	if got := a.screen.(*modsScreen).search.input.String(); got != "3q" {
		t.Errorf("the search is %q, want the characters that were typed", got)
	}
}

func TestAppReadsThePackAgainWhenAScreenIsSwitchedTo(t *testing.T) {
	a := newNavigableApp(t, 100, 30)
	appPress(t, a, "2")
	writeMod(t, "gamma", "Gamma Mod", "", "")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	if strings.Contains(a.render(), "Gamma Mod") {
		t.Fatal("the screen shows a mod that was added after it was read")
	}

	appPress(t, a, "1", "2")
	if !strings.Contains(a.render(), "Gamma Mod") {
		t.Errorf("after coming back the screen doesn't show the mod that was added:\n%s", a.render())
	}
}

func TestAppDeliversWhatAJobComesBackWithToTheScreenThatStartedIt(t *testing.T) {
	a := newNavigableApp(t, 100, 30)
	appPress(t, a, "2")
	mods := a.screen.(*modsScreen)

	// Pinning starts a job; the answer comes back after another screen has been gone to
	_, cmd := a.Update(keyMsg(t, "p"))
	appPress(t, a, "1")
	appFeed(t, a, cmd())

	if got := mods.status.text; got != "Pinned Alpha Mod" {
		t.Errorf("the mods screen says %q, want it to have heard that its job was done", got)
	}
	if mods.running() {
		t.Error("the mods screen is still waiting for its job")
	}
}

func TestAppDoesNotQuitWhileAScreenIsWorking(t *testing.T) {
	a := newNavigableApp(t, 100, 30)
	appPress(t, a, "2")
	_, cmd := a.Update(keyMsg(t, "p"))
	if cmd == nil || !a.working() {
		t.Fatal("pinning didn't start a job")
	}

	if appPress(t, a, "q") {
		t.Error("q quit while the pack was being changed")
	}
	if !strings.Contains(a.render(), "Still working") {
		t.Errorf("the footer doesn't say why q didn't quit:\n%s", a.render())
	}
	if !appPress(t, a, "ctrl+c") {
		t.Error("ctrl+c didn't quit, which it must whatever is happening")
	}

	appFeed(t, a, cmd())
	if appPress(t, a, "q") == false {
		t.Error("q didn't quit once the job was done")
	}
}

// namedScreen is a screen that only has a name, for what the app does with many of them.
type namedScreen struct {
	page
	name string
}

func (s *namedScreen) title() string                    { return s.name }
func (s *namedScreen) activate() tea.Cmd                { return nil }
func (s *namedScreen) update(tea.Msg) (screen, tea.Cmd) { return s, nil }
func (s *namedScreen) view() string                     { return s.render(s.name, nil, "") }
func (s *namedScreen) keys() []key.Binding              { return nil }

func TestAppTabsFallBackToNumbersWhenTheNamesDontFit(t *testing.T) {
	var screens []screen
	for _, name := range []string{"Overview", "Mods", "Updates", "Add", "Config", "Check", "Export", "Release", "Git"} {
		screens = append(screens, &namedScreen{name: name})
	}
	a := newApp("Pack", screens...)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	appPress(t, a, "4")

	row := lines(a.render())[1]
	if want := "1 2 3 [4 Add] 5 6 7 8 9"; row != want {
		t.Errorf("the row of screens is %q, want %q: the one that is shown named, and the others as numbers as their names don't fit", row, want)
	}

	a.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	if row := lines(a.render())[1]; !strings.Contains(row, "Release") || !strings.Contains(row, "[4 Add]") {
		t.Errorf("in a wide terminal the row of screens is %q, want every name", row)
	}
}

func TestAppHelpListsTheKeysOfEveryScreen(t *testing.T) {
	a := newNavigableApp(t, 100, 40)
	appPress(t, a, "2", "?")
	out := a.render()
	for _, want := range []string{"Mods keys", "pin/unpin", "next screen", "go to a screen", "quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("help doesn't say %q:\n%s", want, out)
		}
	}
}
