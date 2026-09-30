package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// The overview is what the TUI opens on: what the pack is, what is in it, and where to go from here.

// overviewLoadedMsg is what the pack is having been read.
type overviewLoadedMsg struct {
	data overviewData
	err  error
}

// overviewRefreshedMsg is the index having been refreshed, or having failed to be.
type overviewRefreshedMsg struct {
	notices []string
	err     error
}

// guideEntry is a screen, and what it is for, in the overview's list of where to go.
type guideEntry struct {
	number int
	title  string
	about  string
}

// describer is what a screen implements to say what it is for, in a few words, for the overview's guide.
type describer interface {
	about() string
}

// guideFor lists what the screens are for, in the order they are in.
func guideFor(screens []screen) []guideEntry {
	var guide []guideEntry
	for i, s := range screens {
		if d, ok := s.(describer); ok {
			guide = append(guide, guideEntry{number: i + 1, title: s.title(), about: d.about()})
		}
	}
	return guide
}

// overviewScreen shows the pack.
type overviewScreen struct {
	page
	backend overviewBackend
	data    overviewData
	loaded  bool
	guide   []guideEntry
}

func newOverviewScreen(backend overviewBackend) *overviewScreen {
	return &overviewScreen{backend: backend}
}

func (s *overviewScreen) title() string { return "Overview" }

func (s *overviewScreen) activate() tea.Cmd {
	backend := s.backend
	return exclusive(func() tea.Msg {
		data, err := backend.loadOverview()
		return overviewLoadedMsg{data, err}
	})
}

func (s *overviewScreen) keys() []key.Binding { return []key.Binding{keyRefresh} }

func (s *overviewScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case overviewLoadedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.data, s.loaded = msg.data, true
	case overviewRefreshedMsg:
		switch {
		case msg.err != nil:
			s.status = errorStatus(msg.err)
		case len(msg.notices) > 0:
			s.status = warningStatus("Index refreshed. " + strings.Join(msg.notices, " "))
		default:
			s.status = successStatus("Index refreshed")
		}
		return s, s.activate()
	case tea.KeyPressMsg:
		s.status = status{}
		if key.Matches(msg, keyRefresh) && !s.running() {
			backend := s.backend
			return s, s.start("Refreshing the index…", func(func(string)) tea.Msg {
				notices, err := backend.refresh()
				return overviewRefreshedMsg{notices, err}
			})
		}
	}
	return s, nil
}

func (s *overviewScreen) view() string {
	return s.render(ui.Bold.Sprint("Overview"), s.bodyLines(), "")
}

func (s *overviewScreen) bodyLines() []string {
	if !s.loaded {
		return messageBody("Reading the pack…")
	}
	d := s.data
	name := ui.Bold.Sprint(d.name)
	if d.version != "" {
		name += " " + ui.Info.Sprint(d.version)
	}
	if d.author != "" {
		name += ui.Muted.Sprint("  by " + d.author)
	}
	lines := []string{name}
	if d.description != "" {
		lines = append(lines, ui.Muted.Sprint(d.description))
	}
	lines = append(lines, "")
	for _, v := range d.versions {
		lines = append(lines, field(v.name, v.version))
	}
	lines = append(lines,
		field("Index", fmt.Sprintf("%s, tracking %s", d.indexFile, count(d.files, "file", "files"))),
		field("Mods", d.describeMods()),
	)
	if sides := d.describeSides(); sides != "" {
		lines = append(lines, field("Sides", sides))
	}
	config := fmt.Sprintf("%s · %s · %s",
		ui.Success.Sprintf("%d claimed", d.config.valid), ui.Warning.Sprintf("%d unclaimed", d.config.invalid), ui.Warning.Sprintf("%d missing", d.config.missing))
	lines = append(lines, field("Config files", config))

	if len(s.guide) > 0 {
		lines = append(lines, "", ui.Bold.Sprint("Where to go"))
		for _, g := range s.guide {
			lines = append(lines, fmt.Sprintf("  %s %s %s", ui.Bold.Sprint(g.number), padRight(g.title, 9), ui.Muted.Sprint(g.about)))
		}
	}
	return lines
}
