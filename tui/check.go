package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// The check screen is "packwiz validate" and "packwiz fix" on one screen: what is wrong with the pack, by mod, and a key
// that works out what could be fixed, shows it and asks.

var (
	keyRecheck = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "check again"))
	keyFix     = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fix what can be fixed"))
)

var checkKeys = []key.Binding{keyFix, keyRecheck, keyUp, keyTop}

// checkDoneMsg is the answer to checking the pack.
type checkDoneMsg struct {
	check *modrinth.Check
	err   error
}

// fixesPlannedMsg is the answer to working out what could be fixed.
type fixesPlannedMsg struct {
	fixes *modrinth.Fixes
	err   error
}

// fixesAppliedMsg is the fixes having been made, or having failed.
type fixesAppliedMsg struct {
	changed int
	err     error
}

// checkScreen checks the pack and fixes what it can.
type checkScreen struct {
	page
	backend checkBackend

	// ran is whether the pack has been checked yet, which it is the first time the screen is shown
	ran   bool
	check *modrinth.Check
	doc   report
	// pending are the fixes that the box that is open asks whether to make
	pending *modrinth.Fixes
	// fixed says what fixing did, to be shown along with what checking again found
	fixed status
}

func newCheckScreen(backend checkBackend) *checkScreen {
	return &checkScreen{backend: backend}
}

func (s *checkScreen) title() string { return "Check" }

func (s *checkScreen) about() string { return "find what is wrong with the pack, and fix it" }

// activate checks the pack the first time the screen is shown. After that it shows what it found, which is checked again
// when asked to: the pack may have changed, but checking it asks the network.
func (s *checkScreen) activate() tea.Cmd {
	if s.ran || s.running() {
		return nil
	}
	s.ran = true
	return s.startCheck()
}

func (s *checkScreen) keys() []key.Binding {
	if s.overlay != nil {
		return s.overlay.keys()
	}
	return checkKeys
}

func (s *checkScreen) startCheck() tea.Cmd {
	backend := s.backend
	return s.start("Checking the pack…", func(func(string)) tea.Msg {
		check, err := backend.checkPack()
		return checkDoneMsg{check, err}
	})
}

func (s *checkScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case checkDoneMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			s.fixed = status{}
			return s, nil
		}
		s.check = msg.check
		s.doc.set(checkReport(msg.check))
		s.status = s.checkStatus()
		s.fixed = status{}
	case fixesPlannedMsg:
		return s.planned(msg)
	case fixesAppliedMsg:
		switch {
		case msg.err != nil && msg.changed == 0:
			s.status = errorStatus(msg.err)
			return s, nil
		case msg.err != nil:
			s.fixed = warningStatus(fmt.Sprintf("Changed %s, but not everything: %v", count(msg.changed, "file", "files"), msg.err))
		default:
			s.fixed = successStatus(fmt.Sprintf("Changed %s", count(msg.changed, "file", "files")))
		}
		// Checking again is what says whether what was fixed was
		return s, s.startCheck()
	case tea.KeyPressMsg:
		if s.overlay != nil {
			return s.updateOverlay(msg)
		}
		return s.updateKey(msg)
	case tea.PasteMsg:
		if s.overlay != nil {
			return s.updateOverlay(msg)
		}
	}
	return s, nil
}

// checkStatus says what checking found, after what fixing did if it was just done.
func (s *checkScreen) checkStatus() status {
	c := s.check
	var st status
	switch {
	case c.Errors > 0:
		st = errorStatus(fmt.Errorf("Found %s and %s in %s", count(c.Errors, "error", "errors"), count(c.Warnings, "warning", "warnings"), count(c.Mods, "mod", "mods")))
	case c.Warnings > 0:
		st = warningStatus(fmt.Sprintf("No errors, but %s", count(c.Warnings, "warning", "warnings")))
	case c.Mods == 0:
		st = infoStatus("The pack has no mods to check")
	case c.Mods == 1:
		st = successStatus("The mod is valid!")
	default:
		st = successStatus(fmt.Sprintf("All %d mods are valid!", c.Mods))
	}
	if s.fixed.text != "" {
		st.text = s.fixed.text + ". " + st.text
		if s.fixed.kind == statusWarning && st.kind == statusSuccess {
			st.kind = statusWarning
		}
	}
	return st
}

func (s *checkScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	if s.doc.handleKey(msg, s.bodyHeight()) {
		return s, nil
	}
	switch {
	case key.Matches(msg, keyRecheck):
		if !s.running() {
			return s, s.startCheck()
		}
	case key.Matches(msg, keyFix):
		return s.startPlan()
	}
	return s, nil
}

// startPlan works out what could be fixed.
func (s *checkScreen) startPlan() (screen, tea.Cmd) {
	if s.running() {
		return s, nil
	}
	if s.check == nil {
		s.status = warningStatus("The pack hasn't been checked yet")
		return s, nil
	}
	backend, check := s.backend, s.check
	return s, s.start("Working out what can be fixed…", func(func(string)) tea.Msg {
		fixes, err := backend.planFixes(check)
		return fixesPlannedMsg{fixes, err}
	})
}

// planned shows what could be fixed and asks whether to fix it.
func (s *checkScreen) planned(msg fixesPlannedMsg) (screen, tea.Cmd) {
	switch {
	case msg.err != nil:
		s.status = errorStatus(msg.err)
	case msg.fixes.Empty() && len(msg.fixes.Skipped) > 0:
		// What could have been fixed but couldn't is the reason there is nothing to do
		s.status = warningStatus(strings.Join(msg.fixes.Skipped, " "))
	case msg.fixes.Empty() && len(s.check.Findings) == 0:
		s.status = successStatus("Nothing to fix")
	case msg.fixes.Empty():
		s.status = infoStatus("Nothing that was found can be fixed automatically")
	default:
		s.pending = msg.fixes
		s.open(newConfirmBox("Make these changes?", fixLines(msg.fixes)...))
	}
	return s, nil
}

// fixLines describe the changes that fixing would make, by file.
func fixLines(f *modrinth.Fixes) []string {
	var lines []string
	for _, skipped := range f.Skipped {
		lines = append(lines, ui.Warning.Sprint(skipped))
	}
	for _, file := range f.Files {
		lines = append(lines, ui.Bold.Sprint(file.Name)+" "+ui.Muted.Sprintf("(%s)", file.Path)+":")
		for _, l := range file.Lines {
			lines = append(lines, "  "+l)
		}
	}
	if len(f.Pack) > 0 {
		lines = append(lines, ui.Bold.Sprint("pack.toml")+":")
		for _, l := range f.Pack {
			lines = append(lines, "  "+l)
		}
	}
	return lines
}

func (s *checkScreen) updateOverlay(msg tea.Msg) (screen, tea.Cmd) {
	_, result, _ := s.routeOverlay(msg)
	fixes := s.pending
	if result != overlayOpen {
		s.pending = nil
	}
	if result == overlayConfirmed && fixes != nil {
		backend := s.backend
		return s, s.start("Making the changes…", func(func(string)) tea.Msg {
			changed, err := backend.applyFixes(fixes)
			return fixesAppliedMsg{changed, err}
		})
	}
	return s, nil
}

// checkReport is the lines that say what was found, by mod, for a width.
func checkReport(c *modrinth.Check) func(width int) []string {
	return func(width int) []string {
		var out []string
		for _, f := range c.Findings {
			heading := ui.Bold.Sprint(f.Name)
			if f.Path != "" && f.Path != f.Name {
				heading += " " + ui.Muted.Sprintf("(%s)", f.Path)
			}
			out = append(out, heading)
			for _, p := range f.Problems {
				label, style := "warning: ", ui.Warning
				if p.Error {
					label, style = "error: ", ui.Error
				}
				for _, line := range wrap("  "+label+p.Message, width, "      ") {
					out = append(out, style.Sprint(line))
				}
			}
			out = append(out, "")
		}
		if len(c.Notices) > 0 {
			out = append(out, ui.Bold.Sprint("Notes"))
			for _, n := range c.Notices {
				for _, line := range wrap("  "+n, width, "    ") {
					out = append(out, ui.Muted.Sprint(line))
				}
			}
		}
		// What separates one mod from the next isn't needed after the last
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		return out
	}
}

func (s *checkScreen) view() string {
	var body []string
	switch {
	case s.check == nil && !s.running():
		body = messageBody("The pack hasn't been checked yet.", "Press c to check it.")
	case s.check == nil:
		body = messageBody("Checking the pack…")
	case len(s.check.Findings) == 0 && len(s.check.Notices) == 0 && s.doc.empty():
		body = messageBody("Nothing is wrong.")
	default:
		body = s.doc.window(s.width, s.bodyHeight())
		if len(body) == 0 {
			body = messageBody("Nothing is wrong with the pack.")
		}
	}
	return s.render(s.summary(), body, "")
}

func (s *checkScreen) summary() string {
	left := ui.Bold.Sprint("Check")
	if s.check == nil {
		return left
	}
	c := s.check
	left += ui.Muted.Sprintf(" · %s", count(c.Mods, "mod", "mods"))
	if c.Errors > 0 {
		left += "  " + ui.Error.Sprintf("%s", count(c.Errors, "error", "errors"))
	}
	if c.Warnings > 0 {
		left += "  " + ui.Warning.Sprintf("%s", count(c.Warnings, "warning", "warnings"))
	}
	if c.Errors == 0 && c.Warnings == 0 {
		left += "  " + ui.Success.Sprint("valid")
	}
	return spread(left, ui.Muted.Sprint(s.doc.position(s.bodyHeight())), s.width)
}
