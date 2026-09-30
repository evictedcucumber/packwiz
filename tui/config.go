package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/fuzzy"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// The config screen is "packwiz config list" and "packwiz config relate" on one screen: the pack's config files as a
// tree of the mod that claims each, with the files nothing claims under "Invalid", and keys to record who owns what.

var (
	keyUp       = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "move"))
	keyDown     = key.NewBinding(key.WithKeys("down", "j"))
	keyPgUp     = key.NewBinding(key.WithKeys("pgup"))
	keyPgDown   = key.NewBinding(key.WithKeys("pgdown"))
	keyTop      = key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g/G", "top/bottom"))
	keyBottom   = key.NewBinding(key.WithKeys("end", "G"))
	keyFold     = key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/→", "fold"))
	keyUnfold   = key.NewBinding(key.WithKeys("right", "l"))
	keyEnter    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "relate/fold"))
	keyMark     = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "mark"))
	keyUnmark   = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "unmark all"))
	keyRelate   = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "relate"))
	keyUnrelate = key.NewBinding(key.WithKeys("x", "delete"), key.WithHelp("x", "unrelate"))
	keyFilter   = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter"))
	keySearch   = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	keyRefresh  = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh index"))
)

// configKeys are the bindings of the config screen, in the order they are shown in help.
var configKeys = []key.Binding{keyRelate, keyUnrelate, keyMark, keySearch, keyFilter, keyRefresh, keyEnter, keyFold, keyUp, keyTop, keyUnmark}

// searchKeys are the bindings while a search is being typed.
var searchKeys = []key.Binding{keyFilterDone, keyFilterClear, keyPickerMove}

// loadedMsg is the pack's config files having been read again.
type loadedMsg struct {
	data configData
	err  error
}

// reloadedMsg is the pack's config files having been read again because the screen was switched to.
type reloadedMsg struct {
	data configData
	err  error
}

// changedMsg is a change to the pack having been made, or failed. Either way the pack is read again, as a change that
// failed can have been made in part.
type changedMsg struct {
	// text says what was done, if it was
	text string
	err  error
}

// refreshedMsg is the index having been refreshed, or failed to be.
type refreshedMsg struct {
	// notices are what refreshing had to say, such as that files have left the index
	notices []string
	err     error
}

// configScreen shows the pack's config files and changes who owns them.
type configScreen struct {
	backend configBackend
	data    configData

	filter stateFilter
	// search is what is being searched for, which leaves only the files that match it (see buildRows), and searching is
	// whether it is being typed, when keys go to it rather than being commands
	search    input
	searching bool
	// folded are the groups that are folded away, by id
	folded map[string]bool
	// marked are the files picked to be related together, by path
	marked map[string]bool

	rows   []row
	cursor int
	offset int

	overlay overlay
	// busy says what is being waited for, if something is
	busy   string
	status status

	width, height int
}

func newConfigScreen(backend configBackend, data configData) *configScreen {
	s := &configScreen{backend: backend, data: data, folded: map[string]bool{}, marked: map[string]bool{}}
	s.rebuild()
	return s
}

func (s *configScreen) title() string { return "Config" }

func (s *configScreen) about() string { return "say which mod, loader or pack owns each config file" }

// activate reads the pack again, as another screen may have changed what it says about config files (taking out entries
// that match no file, say). It isn't a job: it doesn't stop keys, and what it finds is only shown.
func (s *configScreen) activate() tea.Cmd {
	backend := s.backend
	return exclusive(func() tea.Msg {
		data, err := backend.load()
		return reloadedMsg{data, err}
	})
}

// working is whether the pack is being changed, or read after it was.
func (s *configScreen) working() bool { return s.busy != "" }

func (s *configScreen) modal() bool { return s.overlay != nil || s.searching }

func (s *configScreen) keys() []key.Binding {
	if s.overlay != nil {
		return s.overlay.keys()
	}
	if s.searching {
		return searchKeys
	}
	return configKeys
}

func (s *configScreen) setSize(width, height int) {
	s.width, s.height = width, height
	s.clampScroll()
	if s.overlay != nil {
		s.overlay.setSize(width, s.listHeight())
	}
}

// open shows a box over the tree.
func (s *configScreen) open(o overlay) {
	o.setSize(s.width, s.listHeight())
	s.overlay = o
}

// listHeight is how many rows are shown at once: the screen less its summary line and its status line.
func (s *configScreen) listHeight() int {
	return max(s.height-2, 1)
}

// current is the row the cursor is on.
func (s *configScreen) current() (row, bool) {
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return row{}, false
	}
	return s.rows[s.cursor], true
}

// rebuild makes the rows again, from the data, the filter, the search and what is folded, keeping the cursor on the row
// it was on.
func (s *configScreen) rebuild() {
	prev, had := s.current()
	s.rows = buildRows(s.data.tree, s.filter, s.folded, fuzzy.Parse(s.search.String()))
	s.cursor = s.locate(prev, had)
	s.clampScroll()
}

// rebuildForSearch is rebuild for when what is searched for has changed: the cursor goes to the file that matches it
// best, as that is the one that is being looked for, and it is where relating and marking start from. The first of the
// files that match as well as each other is the one, so that the cursor doesn't move about among them. With nothing
// searched for any more it is where it was.
func (s *configScreen) rebuildForSearch() {
	if s.search.empty() {
		s.rebuild()
		return
	}
	s.rows = buildRows(s.data.tree, s.filter, s.folded, fuzzy.Parse(s.search.String()))
	best := -1
	for i, r := range s.rows {
		if r.kind != groupRow && (best < 0 || r.score > s.rows[best].score) {
			best = i
		}
	}
	s.cursor = max(best, 0)
	s.clampScroll()
}

// locate finds where the cursor should be in the rows, given the row it was on: on it if it is there, else on the same
// file (which is what a file that has been claimed is, in another group), else on its group, else as near as it was.
func (s *configScreen) locate(prev row, had bool) int {
	if !had {
		return 0
	}
	if i := slices.IndexFunc(s.rows, func(r row) bool { return r.key() == prev.key() }); i >= 0 {
		return i
	}
	if prev.kind == fileRow {
		if i := slices.IndexFunc(s.rows, func(r row) bool { return r.kind == fileRow && r.path == prev.path }); i >= 0 {
			return i
		}
	}
	if i := slices.IndexFunc(s.rows, func(r row) bool { return r.kind == groupRow && r.group == prev.group }); i >= 0 {
		return i
	}
	return max(min(s.cursor, len(s.rows)-1), 0)
}

// clampScroll scrolls so that the cursor is on screen, and no further than the rows go.
func (s *configScreen) clampScroll() {
	h := s.listHeight()
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+h {
		s.offset = s.cursor - h + 1
	}
	s.offset = max(min(s.offset, len(s.rows)-h), 0)
}

func (s *configScreen) move(delta int) {
	if len(s.rows) == 0 {
		return
	}
	s.cursor = max(min(s.cursor+delta, len(s.rows)-1), 0)
	s.clampScroll()
}

func (s *configScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case reloadedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.applyData(msg.data)
	case loadedMsg:
		s.busy = ""
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.applyData(msg.data)
	case changedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
		} else {
			s.status = successStatus(msg.text)
			// What was marked has been done, or is what the change was about, whichever it was
			clear(s.marked)
		}
		s.busy = "Reading the pack again…"
		return s, s.loadCmd()
	case refreshedMsg:
		switch {
		case msg.err != nil:
			s.status = errorStatus(msg.err)
		case len(msg.notices) > 0:
			s.status = warningStatus("Index refreshed. " + strings.Join(msg.notices, " "))
		default:
			s.status = successStatus("Index refreshed")
		}
		s.busy = "Reading the pack again…"
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
		if s.searching && s.search.handle(msg) {
			s.rebuildForSearch()
		}
	}
	return s, nil
}

// applyData shows data that was read, in place of what was there. Files that are marked and no longer there are
// unmarked, as they can't be related.
func (s *configScreen) applyData(data configData) {
	s.data = data
	present := map[string]bool{}
	for _, m := range data.tree.Mods {
		for _, f := range m.Files {
			present[f] = true
		}
	}
	for _, f := range data.tree.Unclaimed {
		present[f] = true
	}
	for f := range s.marked {
		if !present[f] {
			delete(s.marked, f)
		}
	}
	s.rebuild()
}

func (s *configScreen) loadCmd() tea.Cmd {
	backend := s.backend
	return exclusive(func() tea.Msg {
		data, err := backend.load()
		return loadedMsg{data, err}
	})
}

func (s *configScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	if s.searching {
		return s.updateSearching(msg)
	}
	switch {
	case key.Matches(msg, keyUp):
		s.move(-1)
	case key.Matches(msg, keyDown):
		s.move(1)
	case key.Matches(msg, keyPgUp):
		s.move(-s.listHeight())
	case key.Matches(msg, keyPgDown):
		s.move(s.listHeight())
	case key.Matches(msg, keyTop):
		s.move(-len(s.rows))
	case key.Matches(msg, keyBottom):
		s.move(len(s.rows))
	case key.Matches(msg, keyFold):
		s.fold()
	case key.Matches(msg, keyUnfold):
		s.setFolded(false)
	case key.Matches(msg, keyEnter):
		if r, ok := s.current(); ok && r.kind == groupRow {
			s.setFolded(!r.folded)
		} else {
			return s.openRelate()
		}
	case key.Matches(msg, keyMark):
		s.mark()
	case key.Matches(msg, keyUnmark):
		// A search that is left on goes first, as it is what is in the way of the tree
		if !s.search.empty() {
			s.search.clear()
			s.rebuild()
		} else if n := len(s.marked); n > 0 {
			clear(s.marked)
			s.status = infoStatus(fmt.Sprintf("Unmarked %s", count(n, "file", "files")))
		}
	case key.Matches(msg, keySearch):
		s.searching = true
	case key.Matches(msg, keyRelate):
		return s.openRelate()
	case key.Matches(msg, keyUnrelate):
		return s.openUnrelate()
	case key.Matches(msg, keyFilter):
		s.filter = s.filter.next()
		s.rebuild()
		s.status = infoStatus("Showing " + s.filter.String())
	case key.Matches(msg, keyRefresh):
		return s.startRefresh()
	}
	return s, nil
}

// updateSearching handles a key while a search is being typed: text goes into it, and what matches is shown as it does;
// the arrow keys move through what matches, enter leaves what was typed as the search, and esc takes it away.
func (s *configScreen) updateSearching(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	switch {
	case key.Matches(msg, keyFilterClear):
		s.search.clear()
		s.searching = false
		s.rebuild()
	case key.Matches(msg, keyFilterDone):
		s.searching = false
	case key.Matches(msg, keyMoveUp, keyMoveDown, keyPageUp, keyPageDown) && msg.Key().Text == "":
		// Only the keys that aren't text move, as j and k are letters of a name here
		switch {
		case key.Matches(msg, keyMoveUp):
			s.move(-1)
		case key.Matches(msg, keyMoveDown):
			s.move(1)
		case key.Matches(msg, keyPageUp):
			s.move(-s.listHeight())
		default:
			s.move(s.listHeight())
		}
	default:
		if s.search.handle(msg) {
			s.rebuildForSearch()
		}
	}
	return s, nil
}

// count writes a number of things: "1 file", "3 files".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// fold folds the group the cursor is on. On a file it moves the cursor to the row of the file's group instead, as going
// left does in any tree.
func (s *configScreen) fold() {
	r, ok := s.current()
	if !ok {
		return
	}
	if r.kind != groupRow {
		if i := slices.IndexFunc(s.rows, func(x row) bool { return x.kind == groupRow && x.group == r.group }); i >= 0 {
			s.cursor = i
			s.clampScroll()
		}
		return
	}
	s.setFolded(true)
}

func (s *configScreen) setFolded(folded bool) {
	r, ok := s.current()
	if !ok || r.kind != groupRow {
		return
	}
	s.folded[r.group] = folded
	s.rebuild()
}

// mark picks the file the cursor is on to be related together with others, or unpicks it. On a group's row it picks
// all of the files in the group, unless they all are already, and then it unpicks them.
func (s *configScreen) mark() {
	r, ok := s.current()
	if !ok {
		return
	}
	switch r.kind {
	case fileRow:
		if s.marked[r.path] {
			delete(s.marked, r.path)
		} else {
			s.marked[r.path] = true
		}
	case groupRow:
		all := true
		for _, f := range r.files {
			all = all && s.marked[f]
		}
		for _, f := range r.files {
			if all {
				delete(s.marked, f)
			} else {
				s.marked[f] = true
			}
		}
	default:
		s.status = warningStatus("That entry matches no file, so there is nothing to mark")
	}
}

// relateSubjects are the files that relating would be about: the ones that are marked, or if none is, what the cursor is
// on (a file, or all the files of a group).
func (s *configScreen) relateSubjects() []string {
	if len(s.marked) > 0 {
		return slices.Sorted(maps.Keys(s.marked))
	}
	r, ok := s.current()
	if !ok {
		return nil
	}
	switch r.kind {
	case fileRow:
		return []string{r.path}
	case groupRow:
		return r.files
	}
	return nil
}

func (s *configScreen) openRelate() (screen, tea.Cmd) {
	if s.busy != "" {
		// What is being waited for is on the status line, and it will be a moment
		return s, nil
	}
	subjects := s.relateSubjects()
	switch {
	case len(subjects) == 0:
		s.status = warningStatus("Nothing to relate here: move to a file or a group of files")
	default:
		s.open(newPicker(subjects, s.data.owners, s.data.configDir))
	}
	return s, nil
}

func (s *configScreen) openUnrelate() (screen, tea.Cmd) {
	if s.busy != "" {
		// What is being waited for is on the status line, and it will be a moment
		return s, nil
	}
	r, ok := s.current()
	if !ok || r.owner.kind == ownerNone || r.kind == groupRow {
		s.status = warningStatus("Nothing to unrelate here: move to a file that something claims")
		return s, nil
	}

	prompt := &unrelatePrompt{owner: r.owner}
	if r.kind == missingRow {
		prompt.entries = []string{r.path}
		prompt.lines = []string{r.path + ui.Muted.Sprint("  (matches no file in the pack)")}
	} else {
		prompt.entries = core.ClaimingEntries(r.owner.entries, s.data.configDir, r.path)
		for _, entry := range prompt.entries {
			line := entry
			if n := s.coveredBy(r.owner, entry); n > 1 {
				line += ui.Muted.Sprintf("  (covers %s)", count(n, "file", "files"))
			}
			prompt.lines = append(prompt.lines, line)
		}
	}
	if len(prompt.entries) == 0 {
		s.status = warningStatus("No entry of " + r.owner.name + "'s config-files claims that file")
		return s, nil
	}
	s.open(prompt)
	return s, nil
}

// coveredBy is how many of the tracked files that an owner claims one of its config-files entries claims.
func (s *configScreen) coveredBy(o owner, entry string) int {
	var files []string
	if o.kind == ownerMod {
		for _, m := range s.data.tree.Mods {
			if m.Mod.GetFilePath() == o.id {
				files = m.Files
			}
		}
	} else {
		for _, x := range s.data.tree.Owners {
			if x.Owner == o.id {
				files = x.Files
			}
		}
	}
	n := 0
	for _, f := range files {
		if core.EntryClaims(entry, s.data.configDir, f) {
			n++
		}
	}
	return n
}

func (s *configScreen) updateOverlay(msg tea.Msg) (screen, tea.Cmd) {
	o, result := s.overlay.update(msg)
	s.overlay = o
	switch result {
	case overlayCancelled:
		s.overlay = nil
	case overlayConfirmed:
		s.overlay = nil
		switch o := o.(type) {
		case *picker:
			return s.startRelate(o)
		case *unrelatePrompt:
			return s.startUnrelate(o)
		}
	}
	return s, nil
}

func (s *configScreen) startRelate(p *picker) (screen, tea.Cmd) {
	owners, entries := p.result()
	s.busy = "Saving…"
	backend := s.backend
	return s, func() tea.Msg {
		result, err := backend.relate(owners, entries)
		return changedMsg{text: relateSummary(result, owners, entries), err: err}
	}
}

// relateSummary says what relating did.
func relateSummary(result relateResult, owners []owner, entries []string) string {
	switch {
	case result.added == 0:
		if len(owners) == 1 {
			return owners[0].name + " already claims " + entriesLine(entries)
		}
		return "Already claimed: nothing changed"
	case len(owners) == 1 && len(entries) == 1:
		return owners[0].name + " now claims " + entries[0]
	}
	// They are mods, unless the pack or its loader is among them
	one, many := "mod", "mods"
	for _, o := range owners {
		if o.kind != ownerMod {
			one, many = "owner", "owners"
		}
	}
	text := fmt.Sprintf("Related %s to %s", count(len(entries), "entry", "entries"), count(len(owners), one, many))
	if result.existing > 0 {
		text += fmt.Sprintf(" (%d already claimed)", result.existing)
	}
	return text
}

func (s *configScreen) startUnrelate(u *unrelatePrompt) (screen, tea.Cmd) {
	s.busy = "Saving…"
	backend := s.backend
	return s, func() tea.Msg {
		if err := backend.unrelate(u.owner, u.entries); err != nil {
			return changedMsg{err: err}
		}
		return changedMsg{text: fmt.Sprintf("%s no longer claims %s", u.owner.name, entriesLine(u.entries))}
	}
}

func (s *configScreen) startRefresh() (screen, tea.Cmd) {
	if s.busy != "" {
		// What is being waited for is on the status line, and it will be a moment
		return s, nil
	}
	s.busy = "Refreshing the index…"
	backend := s.backend
	return s, func() tea.Msg {
		notices, err := backend.refresh()
		return refreshedMsg{notices: notices, err: err}
	}
}

func (s *configScreen) view() string {
	if s.width <= 0 {
		return ""
	}
	list := s.listLines()
	lines := make([]string, 0, s.height)
	lines = append(lines, clip(s.summary(), s.width))
	for _, line := range list {
		lines = append(lines, clip(line, s.width))
	}
	lines = append(lines, clip(s.statusLine(), s.width))
	return strings.Join(lines, "\n")
}

// summary is the line above the tree: what it shows, and how much of it there is.
func (s *configScreen) summary() string {
	counts := countStates(s.data.tree)
	left := ui.Bold.Sprint("Config files") + ui.Muted.Sprint(" · showing "+s.filter.String()) + "  " +
		ui.Success.Sprintf("%d valid", counts.valid) + "  " +
		ui.Warning.Sprintf("%d invalid", counts.invalid) + "  " +
		ui.Warning.Sprintf("%d missing", counts.missing)
	if n := len(s.marked); n > 0 {
		left += "  " + ui.Info.Sprintf("%d marked", n)
	}
	if len(s.rows) == 0 {
		return left
	}
	return spread(left, ui.Muted.Sprintf("%d/%d", s.cursor+1, len(s.rows)), s.width)
}

func (s *configScreen) statusLine() string {
	switch {
	case s.busy != "":
		return ui.Muted.Sprint(s.busy)
	case s.status.text != "":
		return s.status.render()
	case s.searching:
		return ui.Bold.Sprint("/") + " " + s.search.String() + ui.Muted.Sprint("█")
	case !s.search.empty():
		return ui.Bold.Sprint("/") + " " + s.search.String() + ui.Muted.Sprint("  (esc clears)")
	}
	return ""
}

// listLines are the lines between the summary and the status: the tree, or the box that is open over it. There are
// always as many as there is room for.
func (s *configScreen) listLines() []string {
	h := s.listHeight()
	if s.overlay != nil {
		return centre(s.overlay.view(), s.width, h)
	}
	if len(s.rows) == 0 {
		return append(s.emptyMessage(), make([]string, h)...)[:h]
	}
	lines := make([]string, 0, h)
	for i := s.offset; i < min(s.offset+h, len(s.rows)); i++ {
		lines = append(lines, s.renderRow(s.rows[i], i == s.cursor))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}

func (s *configScreen) emptyMessage() []string {
	if !s.search.empty() {
		return []string{
			ui.Muted.Sprint("No config file matches " + fmt.Sprintf("%q", s.search.String()) + "."),
			ui.Muted.Sprint("esc clears the search."),
		}
	}
	if s.filter == allStates {
		return []string{
			ui.Muted.Sprint("No config files are tracked in this pack."),
			ui.Muted.Sprint("Add files under config/, then press R to refresh the index."),
		}
	}
	return []string{
		ui.Muted.Sprint("No config files are " + s.filter.String() + "."),
		ui.Muted.Sprint("Press f to show others."),
	}
}

func (s *configScreen) renderRow(r row, selected bool) string {
	cursor, mark := "  ", "  "
	if selected {
		cursor = ui.Info.Sprint("> ")
	}
	if r.kind == fileRow && s.marked[r.path] {
		mark = ui.Info.Sprint("* ")
	}

	var text string
	switch r.kind {
	case groupRow:
		arrow := "▾"
		if r.folded {
			arrow = "▸"
		}
		// What a search found in the name is picked out
		title := ui.Bold.Sprint(highlight(r.title, r.positions, ui.Info))
		if r.owner.kind == ownerNone {
			title = ui.Bold.Sprint(ui.Warning.Sprint(highlight(r.title, r.positions, ui.Info)))
		}
		text = arrow + " " + title + " " + ui.Muted.Sprintf("(%d)", r.count)
	case fileRow:
		style := ui.Success
		if r.owner.kind == ownerNone {
			style = ui.Warning
		}
		text = ui.Muted.Sprint(branch(r.last)) + style.Sprint(highlight(r.path, r.positions, ui.Info))
	case missingRow:
		text = ui.Muted.Sprint(branch(r.last)) + ui.Warning.Sprint(highlight(r.path, r.positions, ui.Info)+" (missing)")
	}

	line := cursor + mark + text
	if selected {
		line = ui.Bold.Sprint(line)
	}
	return line
}

func branch(last bool) string {
	if last {
		return "└── "
	}
	return "├── "
}
