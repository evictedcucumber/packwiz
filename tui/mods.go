package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// The mods screen is "packwiz list" with what is done to a mod one at a time: its mods, which can be searched and filtered
// by where they run and by whether they were added on their own, and keys to pin one, mark it as a dependency, update it
// and see all that the pack knows of it.

var (
	keyModPin    = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pin/unpin"))
	keyModDep    = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "mark as dependency/main"))
	keyModUpdate = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "update"))
	keyModInfo   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details"))
	keyModSide   = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "filter by side"))
	keyModKind   = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "filter main/dependencies"))
	keyModList   = key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "write "+core.ModListFile))
)

// modsKeys are the bindings of the mods screen, in the order they are shown in help.
var modsKeys = []key.Binding{keyModPin, keyModUpdate, keyModInfo, keySearch, keyModSide, keyModKind, keyModList, keyRefresh, keyUp, keyTop}

// sideFilter is which mods the mods screen shows, by where they run, as "packwiz list --side" does: those on the side and
// those on both.
type sideFilter int

const (
	anySide sideFilter = iota
	clientSide
	serverSide
)

func (f sideFilter) String() string {
	switch f {
	case clientSide:
		return "client"
	case serverSide:
		return "server"
	}
	return "all sides"
}

func (f sideFilter) next() sideFilter { return (f + 1) % 3 }

func (f sideFilter) shows(side string) bool {
	switch f {
	case clientSide:
		return side == core.ClientSide || side == core.UniversalSide
	case serverSide:
		return side == core.ServerSide || side == core.UniversalSide
	}
	return true
}

// kindFilter is which mods the mods screen shows, by whether they were added on their own or for another mod, as
// "packwiz list --only" does.
type kindFilter int

const (
	anyKind kindFilter = iota
	mainKind
	dependencyKind
)

func (f kindFilter) String() string {
	switch f {
	case mainKind:
		return "main mods"
	case dependencyKind:
		return "dependencies"
	}
	return "main mods and dependencies"
}

func (f kindFilter) next() kindFilter { return (f + 1) % 3 }

func (f kindFilter) shows(dependency bool) bool {
	switch f {
	case mainKind:
		return !dependency
	case dependencyKind:
		return dependency
	}
	return true
}

// modsLoadedMsg is the pack's mods having been read.
type modsLoadedMsg struct {
	data modsData
	err  error
}

// modsChangedMsg is a change to the pack having been made, or having failed; either way the mods are read again.
type modsChangedMsg struct {
	text    string
	notices []string
	err     error
}

// modsCheckedMsg is the answer to looking for an update to a mod.
type modsCheckedMsg struct {
	name  string
	found updatesFound
	err   error
}

// modsUpdatedMsg is a mod having been updated, or the update having failed.
type modsUpdatedMsg struct {
	outcome updateOutcome
	err     error
}

// modLine is a mod in the list that the filters and the search let through.
type modLine struct {
	row *modRow
	// score is how well the search matched, which is what the list is in the order of while there is one
	score int
	// namePositions and slugPositions are the characters of the name and of the slug that the search matched
	namePositions, slugPositions []int
}

// modsScreen shows the pack's mods.
type modsScreen struct {
	page
	backend modsBackend
	data    modsData

	side   sideFilter
	kind   kindFilter
	search searchBox

	// loaded is whether the pack's mods have been read, which what is shown before that says
	loaded bool
	lines  []modLine
	scroll scroller
	// pending are the updates that the box that is open asks whether to make
	pending []updateOffer
}

func newModsScreen(backend modsBackend) *modsScreen {
	return &modsScreen{backend: backend}
}

func (s *modsScreen) title() string { return "Mods" }

func (s *modsScreen) about() string { return "pin, mark and update what the pack has" }

func (s *modsScreen) activate() tea.Cmd { return s.loadCmd() }

func (s *modsScreen) loadCmd() tea.Cmd {
	backend := s.backend
	return exclusive(func() tea.Msg {
		data, err := backend.loadMods()
		return modsLoadedMsg{data, err}
	})
}

func (s *modsScreen) modal() bool { return s.page.modal() || s.search.typing }

func (s *modsScreen) keys() []key.Binding {
	switch {
	case s.overlay != nil:
		return s.overlay.keys()
	case s.search.typing:
		return searchKeys
	}
	return modsKeys
}

func (s *modsScreen) setSize(width, height int) {
	s.page.setSize(width, height)
	s.scroll.clamp(len(s.lines), s.bodyHeight())
}

// current is the mod the cursor is on.
func (s *modsScreen) current() (*modRow, bool) {
	if s.scroll.cursor < 0 || s.scroll.cursor >= len(s.lines) {
		return nil, false
	}
	return s.lines[s.scroll.cursor].row, true
}

// rebuild makes the list again from the mods, the filters and the search, keeping the cursor on its mod if it is still
// there. With a new search (fresh) the cursor goes to the mod that matches best instead, which is first.
func (s *modsScreen) rebuild(fresh bool) {
	var prev string
	if row, ok := s.current(); ok {
		prev = row.path
	}

	q := s.search.query()
	lines := make([]modLine, 0, len(s.data.rows))
	for i := range s.data.rows {
		row := &s.data.rows[i]
		if !s.side.shows(row.side) || !s.kind.shows(row.dependency) {
			continue
		}
		line := modLine{row: row}
		if !q.Empty() {
			m, ok := q.Match(row.name, row.slug)
			if !ok {
				continue
			}
			line.score, line.namePositions, line.slugPositions = m.Score, m.Positions[0], m.Positions[1]
		}
		lines = append(lines, line)
	}
	if !q.Empty() {
		// As fzf orders them: the best match first, and of matches as good the shorter name
		slices.SortStableFunc(lines, func(a, b modLine) int {
			if a.score != b.score {
				return b.score - a.score
			}
			return len([]rune(a.row.name)) - len([]rune(b.row.name))
		})
	}
	s.lines = lines

	s.scroll.cursor = 0
	if !fresh {
		if i := slices.IndexFunc(lines, func(l modLine) bool { return l.row.path == prev }); i >= 0 {
			s.scroll.cursor = i
		}
	}
	s.scroll.clamp(len(s.lines), s.bodyHeight())
}

func (s *modsScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case modsLoadedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.data, s.loaded = msg.data, true
		s.rebuild(false)
	case modsChangedMsg:
		switch {
		case msg.err != nil:
			s.status = errorStatus(msg.err)
		case len(msg.notices) > 0:
			s.status = warningStatus(msg.text + ". " + strings.Join(msg.notices, " "))
		default:
			s.status = successStatus(msg.text)
		}
		return s, s.loadCmd()
	case modsCheckedMsg:
		return s.checked(msg)
	case modsUpdatedMsg:
		s.status = updateStatus(msg.outcome, msg.err)
		return s, s.loadCmd()
	case tea.KeyPressMsg:
		if s.overlay != nil {
			return s.updateOverlay(msg)
		}
		return s.updateKey(msg)
	case tea.PasteMsg:
		if s.overlay != nil {
			return s.updateOverlay(msg)
		}
		if s.search.paste(msg) {
			s.rebuild(true)
		}
	}
	return s, nil
}

// updateStatus says what updating mods did.
func updateStatus(outcome updateOutcome, err error) status {
	switch {
	case err != nil:
		return errorStatus(err)
	case len(outcome.failed) > 0 && len(outcome.updated) == 0:
		return errorStatus(fmt.Errorf("nothing was updated: %s", strings.Join(outcome.failed, "; ")))
	case len(outcome.failed) > 0:
		return warningStatus(fmt.Sprintf("Updated %s, but not everything: %s", entriesLine(outcome.updated), strings.Join(outcome.failed, "; ")))
	case len(outcome.updated) == 1:
		return successStatus("Updated " + outcome.updated[0])
	}
	return successStatus(fmt.Sprintf("Updated %s", count(len(outcome.updated), "mod", "mods")))
}

func (s *modsScreen) move(delta int) {
	s.scroll.move(delta, len(s.lines), s.bodyHeight())
}

func (s *modsScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	if s.search.typing {
		switch s.search.handleKey(msg) {
		case searchChanged:
			s.rebuild(true)
		case searchLeft:
			s.rebuild(false)
		default:
			s.moveKey(msg)
		}
		return s, nil
	}
	if s.moveKey(msg) {
		return s, nil
	}
	switch {
	case key.Matches(msg, keySearch):
		s.search.start()
	case key.Matches(msg, keyUnmark):
		// A search that is left on is what is in the way of the list
		if s.search.active() {
			s.search.clear()
			s.rebuild(false)
		}
	case key.Matches(msg, keyModSide):
		s.side = s.side.next()
		s.rebuild(false)
		s.status = infoStatus("Showing mods on " + s.side.String())
	case key.Matches(msg, keyModKind):
		s.kind = s.kind.next()
		s.rebuild(false)
		s.status = infoStatus("Showing " + s.kind.String())
	case key.Matches(msg, keyModInfo):
		return s.showDetails()
	case key.Matches(msg, keyModPin):
		return s.togglePin()
	case key.Matches(msg, keyModDep):
		return s.toggleDependency()
	case key.Matches(msg, keyModUpdate):
		return s.startCheck()
	case key.Matches(msg, keyModList):
		return s.startSaveList()
	case key.Matches(msg, keyRefresh):
		return s.startRefresh()
	}
	return s, nil
}

// moveKey moves the cursor if the key is one that does, and says whether it was.
func (s *modsScreen) moveKey(msg tea.KeyPressMsg) bool {
	switch {
	case key.Matches(msg, keyMoveUp):
		s.move(-1)
	case key.Matches(msg, keyMoveDown):
		s.move(1)
	case key.Matches(msg, keyPageUp):
		s.move(-s.bodyHeight())
	case key.Matches(msg, keyPageDown):
		s.move(s.bodyHeight())
	case key.Matches(msg, keyTop):
		s.move(-len(s.lines))
	case key.Matches(msg, keyBottom):
		s.move(len(s.lines))
	default:
		return false
	}
	return true
}

// change begins a job that changes the pack, unless one is running, which is what is waited for first. What the job makes
// of the pack is a modsChangedMsg.
func (s *modsScreen) change(text string, work func() modsChangedMsg) (screen, tea.Cmd) {
	if s.running() {
		return s, nil
	}
	return s, s.start(text, func(func(string)) tea.Msg { return work() })
}

func (s *modsScreen) togglePin() (screen, tea.Cmd) {
	row, ok := s.current()
	if !ok {
		return s, nil
	}
	backend, path, name, pin := s.backend, row.path, row.name, !row.pinned
	verb := "Pinned "
	if !pin {
		verb = "Unpinned "
	}
	return s.change("Saving…", func() modsChangedMsg {
		return modsChangedMsg{text: verb + name, err: backend.setPinned(path, pin)}
	})
}

func (s *modsScreen) toggleDependency() (screen, tea.Cmd) {
	row, ok := s.current()
	if !ok {
		return s, nil
	}
	backend, path, name, dep := s.backend, row.path, row.name, !row.dependency
	text := "Marked " + name + " as a dependency"
	if !dep {
		text = "Marked " + name + " as a main mod"
	}
	return s.change("Saving…", func() modsChangedMsg {
		return modsChangedMsg{text: text, err: backend.setDependency(path, dep)}
	})
}

func (s *modsScreen) startSaveList() (screen, tea.Cmd) {
	backend := s.backend
	return s.change("Writing the list…", func() modsChangedMsg {
		path, err := backend.saveList()
		return modsChangedMsg{text: "Wrote " + path, err: err}
	})
}

func (s *modsScreen) startRefresh() (screen, tea.Cmd) {
	backend := s.backend
	return s.change("Refreshing the index…", func() modsChangedMsg {
		notices, err := backend.refresh()
		return modsChangedMsg{text: "Index refreshed", notices: notices, err: err}
	})
}

// startCheck looks for an update to the mod the cursor is on.
func (s *modsScreen) startCheck() (screen, tea.Cmd) {
	row, ok := s.current()
	if !ok || s.running() {
		return s, nil
	}
	backend, path, name := s.backend, row.path, row.name
	return s, s.start("Looking for an update to "+name+"…", func(func(string)) tea.Msg {
		found, err := backend.checkUpdate(path)
		return modsCheckedMsg{name: name, found: found, err: err}
	})
}

// checked says what looking for an update found, and if there is one asks whether to make it.
func (s *modsScreen) checked(msg modsCheckedMsg) (screen, tea.Cmd) {
	switch {
	case msg.err != nil:
		s.status = errorStatus(msg.err)
	case len(msg.found.failures) > 0:
		s.status = errorStatus(fmt.Errorf("failed to check for updates to %s: %s", msg.name, msg.found.failures[0].err))
	case len(msg.found.pinned) > 0:
		s.status = warningStatus(msg.name + " has an update, but it is pinned: press p to unpin it")
	case len(msg.found.unsupported) > 0:
		s.status = warningStatus("Nothing can update " + msg.name)
	case len(msg.found.offers) == 0:
		s.status = successStatus(msg.name + " is up to date")
	default:
		offer := msg.found.offers[0]
		lines := []string{cmd.StyleUpdate(offer.change)}
		for _, n := range msg.found.notices {
			lines = append(lines, ui.Muted.Sprint(n))
		}
		s.open(newConfirmBox("Update "+offer.name+"?", lines...))
		s.pending = []updateOffer{offer}
	}
	return s, nil
}

func (s *modsScreen) updateOverlay(msg tea.Msg) (screen, tea.Cmd) {
	_, result, _ := s.routeOverlay(msg)
	if result == overlayConfirmed && len(s.pending) > 0 {
		offers := s.pending
		s.pending = nil
		backend := s.backend
		return s, s.start("Updating…", func(func(string)) tea.Msg {
			outcome, err := backend.applyUpdates(offers)
			return modsUpdatedMsg{outcome, err}
		})
	}
	if result != overlayOpen {
		s.pending = nil
	}
	return s, nil
}

// showDetails opens a box with everything the pack knows of the mod the cursor is on.
func (s *modsScreen) showDetails() (screen, tea.Cmd) {
	row, ok := s.current()
	if !ok {
		return s, nil
	}
	yesNo := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	kind := "a main mod"
	if row.dependency {
		kind = "added as a dependency"
	}
	lines := []string{
		field("slug", row.slug),
		field("metadata", row.path),
		field("file", row.file),
		field("version", row.version),
		field("side", row.side),
		field("pinned", yesNo(row.pinned)),
		field("added as", kind),
	}
	if row.optional {
		lines = append(lines, field("optional", "yes"))
	}
	if len(row.updaters) > 0 {
		source := strings.Join(row.updaters, ", ")
		if row.project != "" {
			source += " (project " + row.project + ")"
		}
		lines = append(lines, field("updates from", source))
	}
	lines = append(lines, field("config files", fmt.Sprintf("%d %s in its config-files", row.configEntries, pluralWord(row.configEntries, "entry", "entries"))))
	if len(row.deps) > 0 {
		lines = append(lines, "", ui.Bold.Sprint("Depends on"))
		for _, d := range row.deps {
			where := ui.Warning.Sprint("not in the pack")
			if d.inPack {
				where = ui.Success.Sprint("in the pack")
			}
			lines = append(lines, fmt.Sprintf("  %s %s  %s", padRight("["+d.kind+"]", 14), d.name, where))
		}
	}
	s.open(newInfoBox(row.name, lines...))
	return s, nil
}

// field is a line of a box that says what something is: "name   value", the name in a column of its own.
func field(name, value string) string {
	return ui.Muted.Sprint(padRight(name, 13)) + " " + value
}

// pluralWord is one or many of something, for a number.
func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *modsScreen) view() string {
	return s.render(s.summary(), s.bodyLines(), s.detailLine())
}

// summary is the line above the list: what it shows, and how much of it there is.
func (s *modsScreen) summary() string {
	pinned, deps := 0, 0
	for _, row := range s.data.rows {
		if row.pinned {
			pinned++
		}
		if row.dependency {
			deps++
		}
	}
	left := ui.Bold.Sprint("Mods") + ui.Muted.Sprintf(" · %d of %d shown", len(s.lines), len(s.data.rows))
	if s.side != anySide {
		left += ui.Muted.Sprint(" · " + s.side.String())
	}
	if s.kind != anyKind {
		left += ui.Muted.Sprint(" · " + s.kind.String())
	}
	if pinned > 0 {
		left += "  " + ui.Warning.Sprintf("%d pinned", pinned)
	}
	if deps > 0 {
		left += "  " + ui.Muted.Sprintf("%d dependencies", deps)
	}
	return spread(left, ui.Muted.Sprint(s.scroll.position(len(s.lines))), s.width)
}

// detailLine is the status line when nothing has been done: the search if there is one, else where the mod under the
// cursor is.
func (s *modsScreen) detailLine() string {
	if line := s.search.line(); line != "" {
		return line
	}
	if row, ok := s.current(); ok {
		return ui.Muted.Sprint(row.path + " · " + row.file)
	}
	return ""
}

func (s *modsScreen) bodyLines() []string {
	if len(s.lines) == 0 {
		return s.emptyMessage()
	}
	h := s.bodyHeight()
	from, to := s.scroll.visible(len(s.lines), h)
	cols := s.columns()
	out := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		out = append(out, s.renderLine(s.lines[i], i == s.scroll.cursor, cols))
	}
	return out
}

func (s *modsScreen) emptyMessage() []string {
	switch {
	case !s.loaded:
		return messageBody("Reading the pack…")
	case !s.search.input.empty():
		return messageBody(fmt.Sprintf("No mod matches %q.", s.search.input.String()), "esc clears the search.")
	case len(s.data.rows) == 0:
		return messageBody("The pack has no mods yet.", "Press 3 to add one.")
	}
	return messageBody("No mod is shown with these filters.", "Press s and m to change them.")
}

// modColumns are how wide the parts of a line of the list are, for the room the screen has: what doesn't fit is left out,
// the version first, and the flags of a mod (whether it is pinned, and a dependency) fall back to a letter each.
type modColumns struct {
	name, version, side int
	// flags is how wide the flags are written out in words, or 0 if they are letters after the side
	flags int
}

func (s *modsScreen) columns() modColumns {
	const gutter = 2
	cols := modColumns{side: len("server")}
	longest, longestName := 0, 0
	for _, row := range s.data.rows {
		longest = max(longest, ansi.StringWidth(row.version))
		longestName = max(longestName, ansi.StringWidth(row.name))
	}
	room := s.width - gutter - cols.side - 2
	if s.width >= 84 {
		cols.flags = len("pinned dep opt")
		room -= cols.flags + 2
	} else {
		room -= 3
	}
	if s.width >= 70 {
		cols.version = min(longest, 26)
		if cols.version > 0 {
			room -= cols.version + 2
		}
	}
	// No wider than the longest name, so that what goes with it is close to it, and what has to be cut is cut
	cols.name = max(min(room, longestName), 8)
	return cols
}

func (s *modsScreen) renderLine(l modLine, selected bool, cols modColumns) string {
	row := l.row
	cursor := "  "
	if selected {
		cursor = ui.Info.Sprint("> ")
	}

	name := highlight(row.name, l.namePositions, ui.Info)
	if selected {
		name = ui.Bold.Sprint(name)
	}
	if len(l.slugPositions) > 0 && cols.name > 24 {
		// The name didn't have all of what was searched for, so the slug that did is shown after it
		name += "  " + ui.Muted.Sprint(highlight(row.slug, l.slugPositions, ui.Info))
	}
	parts := []string{padRight(clip(name, cols.name), cols.name)}
	if cols.version > 0 {
		parts = append(parts, padRight(clip(ui.Muted.Sprint(row.version), cols.version), cols.version))
	}

	side := padRight(row.side, cols.side)
	switch row.side {
	case core.ClientSide:
		side = ui.Info.Sprint(side)
	case core.ServerSide:
		side = ui.Warning.Sprint(side)
	default:
		side = ui.Muted.Sprint(side)
	}
	parts = append(parts, side)

	var flags []string
	if cols.flags > 0 {
		if row.pinned {
			flags = append(flags, ui.Warning.Sprint("pinned"))
		}
		if row.dependency {
			flags = append(flags, ui.Muted.Sprint("dep"))
		}
		if row.optional {
			flags = append(flags, ui.Muted.Sprint("opt"))
		}
		parts = append(parts, padRight(strings.Join(flags, " "), cols.flags))
	} else {
		letters := ""
		if row.pinned {
			letters += ui.Warning.Sprint("P")
		}
		if row.dependency {
			letters += ui.Muted.Sprint("D")
		}
		parts = append(parts, padRight(letters, 2))
	}
	return cursor + strings.Join(parts, "  ")
}
