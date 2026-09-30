package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// The dependencies screen is "packwiz modrinth deps": what each of the pack's mods needs, and whether the pack has it, with
// a key to save what it had to look up on Modrinth so that it needn't again.

var (
	keyDepsRefetch = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "look everything up again"))
	keyDepsSave    = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save what was looked up"))
)

var depsKeys = []key.Binding{keyDepsSave, keyRecheck, keyDepsRefetch, keyUp, keyTop}

// depsLoadedMsg is the answer to reporting the dependencies.
type depsLoadedMsg struct {
	report *modrinth.DependencyReport
	err    error
}

// depsSavedMsg is the dependencies having been saved, or having failed to be.
type depsSavedMsg struct {
	failures []string
	err      error
}

// depsScreen reports what the mods depend on.
type depsScreen struct {
	page
	backend depsBackend

	ran    bool
	report *modrinth.DependencyReport
	doc    report
}

func newDepsScreen(backend depsBackend) *depsScreen {
	return &depsScreen{backend: backend}
}

func (s *depsScreen) title() string { return "Deps" }

func (s *depsScreen) about() string { return "see what the mods need, and what the pack lacks" }

// activate reports the dependencies the first time the screen is shown. Mods that record none are looked up on Modrinth,
// which is why after that it waits to be asked.
func (s *depsScreen) activate() tea.Cmd {
	if s.ran || s.running() {
		return nil
	}
	s.ran = true
	return s.startLoad(false)
}

func (s *depsScreen) keys() []key.Binding {
	if s.overlay != nil {
		return s.overlay.keys()
	}
	return depsKeys
}

func (s *depsScreen) startLoad(refresh bool) tea.Cmd {
	backend := s.backend
	text := "Reading what the mods depend on…"
	if refresh {
		text = "Looking up what every mod depends on…"
	}
	return s.start(text, func(func(string)) tea.Msg {
		report, err := backend.loadDependencies(refresh)
		return depsLoadedMsg{report, err}
	})
}

func (s *depsScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case depsLoadedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.report = msg.report
		s.doc.set(dependencyReport(msg.report))
		s.status = s.loadedStatus()
	case depsSavedMsg:
		switch {
		case msg.err != nil:
			s.status = errorStatus(msg.err)
		case len(msg.failures) > 0:
			s.status = warningStatus("Saved, but not everything: " + strings.Join(msg.failures, "; "))
		default:
			s.status = successStatus("Saved the dependencies to the pack")
		}
		if s.report != nil {
			s.doc.set(dependencyReport(s.report))
		}
	case tea.KeyPressMsg:
		return s.updateKey(msg)
	}
	return s, nil
}

// loadedStatus says what was found that needs doing something about.
func (s *depsScreen) loadedStatus() status {
	r := s.report
	switch {
	case len(r.FetchFailed) > 0:
		return warningStatus("Couldn't look up the dependencies of " + entriesLine(r.FetchFailed))
	case r.Fetched > 0:
		return infoStatus(fmt.Sprintf("Looked up the dependencies of %s on Modrinth: press s to save them to the pack", count(r.Fetched, "mod", "mods")))
	case len(r.Notices) > 0:
		return warningStatus(strings.Join(r.Notices, " "))
	}
	return status{}
}

func (s *depsScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	if s.doc.handleKey(msg, s.bodyHeight()) {
		return s, nil
	}
	if s.running() {
		return s, nil
	}
	switch {
	case key.Matches(msg, keyRecheck):
		return s, s.startLoad(false)
	case key.Matches(msg, keyDepsRefetch):
		return s, s.startLoad(true)
	case key.Matches(msg, keyDepsSave):
		if s.report == nil || s.report.Fetched == 0 {
			s.status = infoStatus("Nothing was looked up that isn't saved")
			return s, nil
		}
		backend, report := s.backend, s.report
		return s, s.start("Saving…", func(func(string)) tea.Msg {
			failures, err := backend.saveDependencies(report)
			return depsSavedMsg{failures, err}
		})
	}
	return s, nil
}

// dependencyReport is the lines that say what the mods depend on, for a width.
func dependencyReport(r *modrinth.DependencyReport) func(width int) []string {
	return func(width int) []string {
		var out []string
		for _, g := range r.Groups {
			out = append(out, ui.Bold.Sprint(g.Mod)+":")
			for _, d := range g.Dependencies {
				status := ui.Success.Sprint("in the pack")
				if !d.InPack {
					missing := ui.Warning
					if d.Kind == "required" {
						// Only a dependency that is required is a problem when it is missing
						missing = ui.Error
					}
					status = missing.Sprint("missing")
				}
				out = append(out, wrap("  "+styleKind(d.Kind)+" "+d.Name+" ("+status+")", width, "      ")...)
			}
			out = append(out, "")
		}
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		return out
	}
}

// styleKind shows how a dependency is needed, as "packwiz modrinth deps" does.
func styleKind(kind string) string {
	tag := "[" + kind + "]"
	switch kind {
	case "required":
		return tag
	case "incompatible":
		return ui.Error.Sprint(tag)
	}
	return ui.Muted.Sprint(tag)
}

func (s *depsScreen) view() string {
	var body []string
	switch {
	case s.report == nil && s.running():
		body = messageBody(s.worker.text)
	case s.report == nil:
		body = messageBody("Nothing has been read yet.", "Press c to read what the mods depend on.")
	case s.report.Mods == 0:
		body = messageBody("None of the pack's mods come from Modrinth, so there are no dependencies to report.")
	case len(s.report.Groups) == 0:
		body = messageBody("None of the mods depends on anything.")
	default:
		body = s.doc.window(s.width, s.bodyHeight())
	}
	return s.render(s.summary(), body, "")
}

func (s *depsScreen) summary() string {
	left := ui.Bold.Sprint("Dependencies")
	if s.report == nil {
		return left
	}
	r := s.report
	left += ui.Muted.Sprintf(" · %s from Modrinth", count(r.Mods, "mod", "mods"))
	if r.MissingRequired > 0 {
		left += "  " + ui.Error.Sprintf("%d required missing", r.MissingRequired)
	} else if len(r.Groups) > 0 {
		left += "  " + ui.Success.Sprint("all required there")
	}
	if r.Fetched > 0 {
		left += "  " + ui.Info.Sprintf("%d unsaved", r.Fetched)
	}
	return spread(left, ui.Muted.Sprint(s.doc.position(s.bodyHeight())), s.width)
}
