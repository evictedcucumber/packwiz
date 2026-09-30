package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// screen is one page of the TUI: the config files, and whatever comes after. The app runs one, giving it the room that
// is left once it has drawn its own header and footer, and the keys that aren't its own.
type screen interface {
	title() string
	// init returns what the screen needs to run when it opens, if anything
	init() tea.Cmd
	// setSize tells the screen how much room it has, which it is told before it is drawn and whenever that changes
	setSize(width, height int)
	update(tea.Msg) (screen, tea.Cmd)
	// view draws the screen in the room it was given: exactly that many lines, each no wider
	view() string
	// keys are the bindings the screen has now, most useful first, for the footer and for help
	keys() []key.Binding
	// modal is whether the screen has something open that takes keys itself, such as a box that is asking something or
	// text being typed: the app leaves the keys to it, so a q that is typed doesn't quit.
	modal() bool
}

var (
	keyQuit = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	keyHelp = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
	keyBack = key.NewBinding(key.WithKeys("?", "esc", "q"), key.WithHelp("?/esc", "back"))
)

// The smallest a terminal can be for the TUI to be drawn in it.
const (
	minWidth  = 40
	minHeight = 10
)

// app is the TUI: a header, a screen and a footer, and the keys that work on every screen.
type app struct {
	// pack is what the pack is called, shown in the header
	pack   string
	screen screen
	help   bool

	width, height int
}

func newApp(pack string, s screen) *app {
	return &app{pack: pack, screen: s}
}

func (a *app) Init() tea.Cmd {
	return a.screen.init()
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.screen.setSize(a.width, a.height-2)
		return a, nil
	case tea.KeyPressMsg:
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
				return a, tea.Quit
			case key.Matches(msg, keyHelp):
				a.help = true
				return a, nil
			}
		}
	}
	var cmd tea.Cmd
	a.screen, cmd = a.screen.update(msg)
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
	return strings.Join([]string{a.header(), body, a.footer()}, "\n")
}

func (a *app) header() string {
	line := ui.Bold.Sprint("packwiz")
	if a.pack != "" {
		line += ui.Muted.Sprint("  " + a.pack)
	}
	line += ui.Muted.Sprint("  ›  ") + a.screen.title()
	return clip(line, a.width)
}

// footer is the keys the screen has, as many as fit, and always those for help and quitting.
func (a *app) footer() string {
	if a.help {
		return clip(hints([]key.Binding{keyBack}, a.width), a.width)
	}
	closing := hints([]key.Binding{keyHelp, keyQuit}, a.width)
	if a.screen.modal() {
		// Quit isn't a key while a box is open, so it isn't offered
		closing = hints([]key.Binding{keyHelp}, a.width)
	}
	room := a.width - len("? help  q quit") - 2
	front := hints(a.screen.keys(), room)
	if front == "" {
		return clip(closing, a.width)
	}
	return clip(front+"  "+closing, a.width)
}

// helpView is every key the screen has, one to a line, in the room the screen has.
func (a *app) helpView() string {
	height := a.height - 2
	lines := []string{ui.Bold.Sprint(a.screen.title() + " keys"), ""}
	for _, b := range append(append([]key.Binding{}, a.screen.keys()...), keyHelp, keyQuit) {
		if !b.Enabled() {
			continue
		}
		help := b.Help()
		lines = append(lines, "  "+ui.Bold.Sprint(padRight(help.Key, 8))+" "+help.Desc)
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
