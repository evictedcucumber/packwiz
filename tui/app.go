package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// screen is one page of the TUI: the pack's mods, its config files, and so on. The app shows one at a time, giving it the
// room that is left once it has drawn its own header and footer, and the keys that aren't its own.
type screen interface {
	title() string
	// activate is called whenever the screen is switched to, and returns what it needs to run then, if anything: a
	// screen that shows what is in the pack reads it again, as another screen may have changed it
	activate() tea.Cmd
	// setSize tells the screen how much room it has, which it is told before it is drawn and whenever that changes, for
	// every screen and not only the one that is shown
	setSize(width, height int)
	// update is given every message that isn't a key or a paste that another screen is the one for: what a job that a
	// screen started comes back with is for that screen, whichever is shown when it does
	update(tea.Msg) (screen, tea.Cmd)
	// view draws the screen in the room it was given: exactly that many lines, each no wider
	view() string
	// keys are the bindings the screen has now, most useful first, for the footer and for help
	keys() []key.Binding
	// modal is whether the screen has something open that takes keys itself, such as a box that is asking something or
	// text being typed: the app leaves the keys to it, so a q that is typed doesn't quit.
	modal() bool
	// working is whether the screen has a job running that changes the pack or something else that shouldn't be cut
	// short, so that q doesn't quit until it is done
	working() bool
}

var (
	keyQuit    = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	keyHelp    = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
	keyBack    = key.NewBinding(key.WithKeys("?", "esc", "q"), key.WithHelp("?/esc", "back"))
	keyNext    = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next screen"))
	keyPrev    = key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous screen"))
	keyScreens = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "screens"))
	keyJump    = key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1-9", "go to a screen"))
)

// The smallest a terminal can be for the TUI to be drawn in it.
const (
	minWidth  = 40
	minHeight = 10
)

// chrome is how many lines the app draws itself: the header, its row of screens, and the footer.
const chrome = 3

// app is the TUI: a header, the screens of the interface one at a time, and a footer, and the keys that work on every
// screen.
type app struct {
	// pack is what the pack is called, shown in the header
	pack    string
	screens []screen
	// screen is the one that is shown, which is one of screens
	screen screen
	help   bool
	// notice is something the footer says instead of the keys, until the next key is pressed
	notice string
	// opened, if it is set, is how to open the rest of the interface once a pack has been made, as the app is opened without
	// one: it returns what the pack is called and the screens that work on it
	opened func() (pack string, screens []screen, err error)

	width, height int
}

func newApp(pack string, screens ...screen) *app {
	return &app{pack: pack, screens: screens, screen: screens[0]}
}

func (a *app) Init() tea.Cmd {
	return a.screen.activate()
}

// goTo shows the screen at index i, and starts what it has to do then.
func (a *app) goTo(i int) tea.Cmd {
	n := len(a.screens)
	i = (i%n + n) % n
	if a.screens[i] == a.screen {
		return nil
	}
	a.screen = a.screens[i]
	return a.screen.activate()
}

// index is where the screen that is shown is in screens.
func (a *app) index() int {
	for i, s := range a.screens {
		if s == a.screen {
			return i
		}
	}
	return 0
}

// working is whether any screen has a job running that mustn't be cut short.
func (a *app) working() bool {
	for _, s := range a.screens {
		if s.working() {
			return true
		}
	}
	return false
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		for _, s := range a.screens {
			s.setSize(a.width, a.height-chrome)
		}
		return a, nil
	case tea.KeyPressMsg:
		a.notice = ""
		// Whatever is open, ctrl+c gets out
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		if a.help {
			if key.Matches(msg, keyBack) {
				a.help = false
			}
			return a, nil
		}
		if !a.screen.modal() {
			switch {
			case key.Matches(msg, keyQuit):
				if a.working() {
					a.notice = "Still working: wait for it to finish, or press ctrl+c to quit now"
					return a, nil
				}
				return a, tea.Quit
			case key.Matches(msg, keyHelp):
				a.help = true
				return a, nil
			case key.Matches(msg, keyNext):
				return a, a.goTo(a.index() + 1)
			case key.Matches(msg, keyPrev):
				return a, a.goTo(a.index() - 1)
			case key.Matches(msg, keyJump):
				if n, err := strconv.Atoi(msg.String()); err == nil && n >= 1 && n <= len(a.screens) {
					return a, a.goTo(n - 1)
				}
			}
		}
		return a.toScreen(msg)
	case tea.PasteMsg:
		return a.toScreen(msg)
	case packCreatedMsg:
		return a.openPack()
	}

	// What a job comes back with is for the screen that started it, which isn't necessarily the one that is shown
	var cmds []tea.Cmd
	for i, s := range a.screens {
		updated, cmd := s.update(msg)
		a.screens[i] = updated
		if s == a.screen {
			a.screen = updated
		}
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// openPack replaces the screens that made a pack with the ones that work on it.
func (a *app) openPack() (tea.Model, tea.Cmd) {
	if a.opened == nil {
		return a, nil
	}
	pack, screens, err := a.opened()
	if err != nil {
		return a.toScreen(openFailedMsg{err})
	}
	a.pack, a.screens, a.screen, a.help = pack, screens, screens[0], false
	for _, s := range screens {
		s.setSize(a.width, a.height-chrome)
	}
	return a, a.screen.activate()
}

// toScreen gives a key or a paste to the screen that is shown.
func (a *app) toScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	i := a.index()
	updated, cmd := a.screen.update(msg)
	a.screens[i], a.screen = updated, updated
	return a, cmd
}

func (a *app) View() tea.View {
	v := tea.NewView(a.render())
	v.AltScreen = true
	return v
}

// render draws the whole screen as text.
func (a *app) render() string {
	if a.width == 0 || a.height == 0 {
		return ""
	}
	if a.width < minWidth || a.height < minHeight {
		return clip("Terminal too small", a.width) + "\n" + clip("Needs at least 40x10", a.width)
	}

	body := a.screen.view()
	if a.help {
		body = a.helpView()
	}
	return strings.Join([]string{a.header(), a.tabs(), body, a.footer()}, "\n")
}

func (a *app) header() string {
	line := ui.Bold.Sprint("packwiz")
	if a.pack != "" {
		line += ui.Muted.Sprint("  " + a.pack)
	}
	return clip(line, a.width)
}

// tabs is the row of the screens, with the one that is shown picked out: in brackets, as well as in bold. There may not
// be room for the names of all of them, and then the others are only their numbers.
func (a *app) tabs() string {
	current := a.index()
	build := func(compact bool) string {
		parts := make([]string, len(a.screens))
		for i, s := range a.screens {
			label := strconv.Itoa(i+1) + " " + s.title()
			switch {
			case i == current:
				parts[i] = ui.Bold.Sprint(ui.Info.Sprint("[" + label + "]"))
			case compact:
				parts[i] = ui.Muted.Sprint(strconv.Itoa(i + 1))
			default:
				parts[i] = ui.Muted.Sprint(label)
			}
		}
		return strings.Join(parts, " ")
	}
	if full := build(false); ansi.StringWidth(full) <= a.width {
		return full
	}
	return clip(build(true), a.width)
}

// footer is the keys the screen has, as many as fit, and always those for help and quitting.
func (a *app) footer() string {
	if a.notice != "" {
		return clip(ui.Warning.Sprint(a.notice), a.width)
	}
	if a.help {
		return clip(hints([]key.Binding{keyBack}, a.width), a.width)
	}
	closing := []key.Binding{keyScreens, keyHelp, keyQuit}
	if a.screen.modal() {
		// Quit isn't a key while a box is open, nor is going to another screen, so they aren't offered
		closing = []key.Binding{keyHelp}
	}
	closingText := hints(closing, a.width)
	room := a.width - ansi.StringWidth(closingText) - 2
	front := hints(a.screen.keys(), room)
	if front == "" {
		return clip(closingText, a.width)
	}
	return clip(front+"  "+closingText, a.width)
}

// helpView is every key the screen has, one to a line, in the room the screen has.
func (a *app) helpView() string {
	height := a.height - chrome
	lines := []string{ui.Bold.Sprint(a.screen.title() + " keys"), ""}
	bindings := append([]key.Binding{}, a.screen.keys()...)
	if !a.screen.modal() {
		bindings = append(bindings, keyNext, keyPrev, keyJump)
	}
	for _, b := range append(bindings, keyHelp, keyQuit) {
		if !b.Enabled() {
			continue
		}
		help := b.Help()
		lines = append(lines, "  "+ui.Bold.Sprint(padRight(help.Key, 9))+" "+help.Desc)
	}
	lines = append(lines, "", ui.Muted.Sprint("Press ? or esc to go back"))

	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = clip(line, a.width)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
