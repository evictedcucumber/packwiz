package tui

import (
	"fmt"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// The updates screen is "packwiz update --all": it looks for an update to every mod, lists the ones it finds, and updates
// those that are picked.

var (
	keyCheck       = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "look for updates"))
	keyUpdateAll   = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "pick all/none"))
	keyUpdateApply = key.NewBinding(key.WithKeys("enter", "u"), key.WithHelp("enter", "update picked"))
	keyUpdatePick  = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "pick"))
)

var updatesKeys = []key.Binding{keyCheck, keyUpdateApply, keyUpdatePick, keyUpdateAll, keyUp, keyTop}

// updatesFoundMsg is the answer to looking for updates.
type updatesFoundMsg struct {
	found updatesFound
	err   error
}

// updatesAppliedMsg is the updates having been made, or having failed.
type updatesAppliedMsg struct {
	outcome updateOutcome
	err     error
}

// updatesRow is a line of the list: an update that can be picked, or something else that was found.
type updatesRow struct {
	// offer is set for an update
	offer *updateOffer
	// text is what is said of anything else, and kind how to style it
	text string
	kind statusKind
	// heading is a line that names the rows after it
	heading bool
}

// updatesScreen looks for updates and makes them.
type updatesScreen struct {
	page
	backend updatesBackend

	// checked is whether updates have been looked for, as what is shown is said differently before that
	checked bool
	found   updatesFound
	// picked are the updates that will be made, by the path of the mod's metadata file
	picked map[string]bool

	rows   []updatesRow
	scroll scroller
	// pending are the updates that the box that is open asks whether to make
	pending []updateOffer
}

func newUpdatesScreen(backend updatesBackend) *updatesScreen {
	return &updatesScreen{backend: backend, picked: map[string]bool{}}
}

func (s *updatesScreen) title() string { return "Updates" }

func (s *updatesScreen) about() string { return "look for new versions of the mods, and update them" }

// activate does nothing: looking for updates asks the network, which is done when it is asked for.
func (s *updatesScreen) activate() tea.Cmd { return nil }

func (s *updatesScreen) keys() []key.Binding {
	if s.overlay != nil {
		return s.overlay.keys()
	}
	return updatesKeys
}

func (s *updatesScreen) setSize(width, height int) {
	s.page.setSize(width, height)
	s.scroll.clamp(len(s.rows), s.bodyHeight())
}

// rebuild makes the rows from what was found.
func (s *updatesScreen) rebuild() {
	var rows []updatesRow
	for i := range s.found.offers {
		rows = append(rows, updatesRow{offer: &s.found.offers[i]})
	}
	section := func(title string, kind statusKind, lines []string) {
		if len(lines) == 0 {
			return
		}
		rows = append(rows, updatesRow{heading: true, text: fmt.Sprintf("%s (%d)", title, len(lines))})
		for _, line := range lines {
			rows = append(rows, updatesRow{text: line, kind: kind})
		}
	}
	section("Pinned, with an update that isn't offered", statusInfo, prefixAll(s.found.pinned, "", " is pinned"))
	failed := make([]string, len(s.found.failures))
	for i, f := range s.found.failures {
		failed[i] = f.name + ": " + f.err
	}
	section("Couldn't be checked", statusError, failed)
	section("Nothing can update these", statusWarning, s.found.unsupported)
	section("Notes", statusInfo, s.found.notices)
	s.rows = rows
	s.scroll.clamp(len(rows), s.bodyHeight())
}

// prefixAll wraps each of items in a prefix and a suffix.
func prefixAll(items []string, prefix, suffix string) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = prefix + item + suffix
	}
	return out
}

func (s *updatesScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case updatesFoundMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.checked, s.found = true, msg.found
		s.picked = map[string]bool{}
		for _, o := range msg.found.offers {
			// Updating all of them is what is usually wanted, and what "packwiz update --all" offers
			s.picked[o.path] = true
		}
		s.scroll = scroller{}
		s.rebuild()
		s.status = s.foundStatus()
	case updatesAppliedMsg:
		s.status = updateStatus(msg.outcome, msg.err)
		// What was updated has nothing newer to offer
		s.found.offers = slices.DeleteFunc(s.found.offers, func(o updateOffer) bool {
			return slices.Contains(msg.outcome.updatedPaths, o.path)
		})
		for _, p := range msg.outcome.updatedPaths {
			delete(s.picked, p)
		}
		s.rebuild()
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

// foundStatus says what looking for updates found.
func (s *updatesScreen) foundStatus() status {
	f := s.found
	switch {
	case len(f.offers) > 0:
		return successStatus(fmt.Sprintf("%s found", count(len(f.offers), "update", "updates")))
	case len(f.failures) > 0:
		// Not knowing whether a mod has an update isn't its being up to date
		return warningStatus(fmt.Sprintf("No updates found, but %s could not be checked", count(f.failedChecks, "mod", "mods")))
	}
	return successStatus("Everything is up to date")
}

func (s *updatesScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	switch {
	case key.Matches(msg, keyMoveUp):
		s.scroll.move(-1, len(s.rows), s.bodyHeight())
	case key.Matches(msg, keyMoveDown):
		s.scroll.move(1, len(s.rows), s.bodyHeight())
	case key.Matches(msg, keyPageUp):
		s.scroll.move(-s.bodyHeight(), len(s.rows), s.bodyHeight())
	case key.Matches(msg, keyPageDown):
		s.scroll.move(s.bodyHeight(), len(s.rows), s.bodyHeight())
	case key.Matches(msg, keyTop):
		s.scroll.move(-len(s.rows), len(s.rows), s.bodyHeight())
	case key.Matches(msg, keyBottom):
		s.scroll.move(len(s.rows), len(s.rows), s.bodyHeight())
	case key.Matches(msg, keyUpdatePick):
		s.pick()
	case key.Matches(msg, keyUpdateAll):
		s.pickAll()
	case key.Matches(msg, keyCheck):
		return s.startCheck()
	case key.Matches(msg, keyUpdateApply):
		return s.openConfirm()
	}
	return s, nil
}

// pick picks the update the cursor is on, or unpicks it.
func (s *updatesScreen) pick() {
	if s.scroll.cursor >= len(s.rows) {
		return
	}
	row := s.rows[s.scroll.cursor]
	if row.offer == nil {
		return
	}
	if s.picked[row.offer.path] {
		delete(s.picked, row.offer.path)
	} else {
		s.picked[row.offer.path] = true
	}
}

// pickAll picks every update, or if they all are picked, none.
func (s *updatesScreen) pickAll() {
	if len(s.picked) == len(s.found.offers) {
		s.picked = map[string]bool{}
		return
	}
	for _, o := range s.found.offers {
		s.picked[o.path] = true
	}
}

func (s *updatesScreen) startCheck() (screen, tea.Cmd) {
	if s.running() {
		return s, nil
	}
	backend := s.backend
	return s, s.start("Looking for updates…", func(progress func(string)) tea.Msg {
		found, err := backend.findUpdates(func(source string) { progress("Asking " + source + " for updates…") })
		return updatesFoundMsg{found, err}
	})
}

// openConfirm asks whether to make the updates that are picked.
func (s *updatesScreen) openConfirm() (screen, tea.Cmd) {
	if s.running() {
		return s, nil
	}
	var offers []updateOffer
	var lines []string
	for _, o := range s.found.offers {
		if s.picked[o.path] {
			offers = append(offers, o)
			lines = append(lines, ui.Bold.Sprint(o.name)+": "+cmd.StyleUpdate(o.change))
		}
	}
	if len(offers) == 0 {
		s.status = warningStatus("Nothing is picked: press space to pick an update, or c to look for some")
		return s, nil
	}
	s.pending = offers
	s.open(newConfirmBox("Update "+count(len(offers), "mod", "mods")+"?", lines...))
	return s, nil
}

func (s *updatesScreen) updateOverlay(msg tea.Msg) (screen, tea.Cmd) {
	_, result, _ := s.routeOverlay(msg)
	offers := s.pending
	if result != overlayOpen {
		s.pending = nil
	}
	if result == overlayConfirmed && len(offers) > 0 {
		backend := s.backend
		return s, s.start("Updating…", func(func(string)) tea.Msg {
			outcome, err := backend.applyUpdates(offers)
			return updatesAppliedMsg{outcome, err}
		})
	}
	return s, nil
}

func (s *updatesScreen) view() string {
	return s.render(s.summary(), s.bodyLines(), "")
}

func (s *updatesScreen) summary() string {
	left := ui.Bold.Sprint("Updates")
	if !s.checked {
		return left
	}
	f := s.found
	left += "  " + ui.Success.Sprintf("%d available", len(f.offers))
	if n := len(s.picked); n > 0 {
		left += ui.Muted.Sprintf(" · %d picked", n)
	}
	left += "  " + ui.Muted.Sprintf("%d up to date", f.upToDate)
	if f.failedChecks > 0 {
		left += "  " + ui.Error.Sprintf("%d failed", f.failedChecks)
	}
	return spread(left, ui.Muted.Sprint(s.scroll.position(len(s.rows))), s.width)
}

func (s *updatesScreen) bodyLines() []string {
	switch {
	case !s.checked:
		return messageBody("Nothing has been looked for yet.", "Press c to look for newer versions of the mods.")
	case len(s.rows) == 0:
		return messageBody("Everything is up to date.")
	}
	from, to := s.scroll.visible(len(s.rows), s.bodyHeight())
	nameWidth := 0
	for _, o := range s.found.offers {
		nameWidth = max(nameWidth, len([]rune(o.name)))
	}
	nameWidth = min(nameWidth, max(s.width/3, 12))

	out := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		out = append(out, s.renderRow(s.rows[i], i == s.scroll.cursor, nameWidth))
	}
	return out
}

func (s *updatesScreen) renderRow(r updatesRow, selected bool, nameWidth int) string {
	cursor := "  "
	if selected {
		cursor = ui.Info.Sprint("> ")
	}
	var text string
	switch {
	case r.heading:
		text = ui.Bold.Sprint(r.text)
	case r.offer != nil:
		box := "[ ] "
		if s.picked[r.offer.path] {
			box = ui.Info.Sprint("[x] ")
		}
		text = box + padRight(clip(ui.Bold.Sprint(r.offer.name), nameWidth), nameWidth) + "  " + cmd.StyleUpdate(r.offer.change)
	default:
		style := status{text: r.text, kind: r.kind}
		text = "  " + style.render()
	}
	return cursor + text
}
