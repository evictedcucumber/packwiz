package tui

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// overlayResult says whether an overlay is still open after a message, and if not, how it ended.
type overlayResult int

const (
	overlayOpen overlayResult = iota
	// overlayConfirmed is when what the overlay asks for was answered yes, or chosen
	overlayConfirmed
	overlayCancelled
)

// overlay is a box that a screen shows over its content to ask for something, and that takes the keys while it is open.
type overlay interface {
	// setSize tells the overlay how much room it has: it is told when it is opened and whenever that changes
	setSize(width, height int)
	update(tea.Msg) (overlay, overlayResult)
	// view returns the lines of the box, no wider or higher than the room it was given
	view() []string
	// keys are the bindings that are in use while it is open
	keys() []key.Binding
}

var (
	keyConfirm = key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "yes"))
	keyDecline = key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "no"))

	keyPickerMove   = key.NewBinding(key.WithKeys("up", "down", "k", "j", "pgup", "pgdown"), key.WithHelp("↑/↓", "move"))
	keyPickerToggle = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select"))
	keyPickerScope  = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "file/folder"))
	keyPickerFilter = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	keyPickerAccept = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "relate"))
	keyPickerCancel = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))

	keyFilterDone  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "done"))
	keyFilterClear = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear"))

	keyMoveUp   = key.NewBinding(key.WithKeys("up", "k"))
	keyMoveDown = key.NewBinding(key.WithKeys("down", "j"))
	keyPageUp   = key.NewBinding(key.WithKeys("pgup"))
	keyPageDown = key.NewBinding(key.WithKeys("pgdown"))
)

// picker asks who to relate config files to: the pack, its mod loader and the mods. They are a list to choose from, that can be filtered by typing
// part of a name, and the files can be claimed one by one or as the folders that they are in.
type picker struct {
	// subjects are the files being related: paths relative to the pack, as the index has them
	subjects  []string
	owners    []owner
	configDir string

	// files are the entries that claim the subjects themselves, and folders the ones that claim the folders they are in
	// (a subject that isn't in one is claimed itself). Each is in the order of subjects, without repeats.
	files, folders []string
	// folder is whether the folders are claimed rather than the files
	folder bool

	// chosen are the owners that are picked, by their key
	chosen map[string]bool
	// visible are the owners that the filter lets through, the best match first
	visible []pickerEntry
	filter  input
	// filtering is whether keys go to the filter, rather than being commands
	filtering bool
	cursor    int
	offset    int

	width, height int
}

// pickerEntry is a mod in the picker's list, and how the filter matched it.
type pickerEntry struct {
	owner owner
	// score is how well the filter matched, which is what the list is in the order of while there is one
	score int
	// namePositions and slugPositions are the characters of the mod's name and of its slug that the filter matched, as
	// indexes of their runes, to show them. The slug is only shown when it was matched.
	namePositions, slugPositions []int
}

func newPicker(subjects []string, owners []owner, configDir string) *picker {
	p := &picker{subjects: subjects, owners: owners, configDir: configDir, chosen: make(map[string]bool)}
	for _, s := range subjects {
		file := core.ConfigEntry(s, false, configDir)
		folder := file
		if dir := path.Dir(s); dir != "." {
			if e := core.ConfigEntry(dir, true, configDir); e != "" && e != "/" {
				folder = e
			}
		}
		if !slices.Contains(p.files, file) {
			p.files = append(p.files, file)
		}
		if !slices.Contains(p.folders, folder) {
			p.folders = append(p.folders, folder)
		}
	}
	p.refilter()
	return p
}

// entries are what would be recorded in each mod's config-files: the files, or the folders they are in.
func (p *picker) entries() []string {
	if p.folder {
		return p.folders
	}
	return p.files
}

// canChooseScope is whether claiming the folders is different from claiming the files, as it isn't for files that aren't
// in a folder.
func (p *picker) canChooseScope() bool {
	return !slices.Equal(p.files, p.folders)
}

// refilter finds the owners that the filter matches, and puts the cursor at the first. With nothing typed that is every
// owner in the order they are in; with a filter it is those that it matches fuzzily, the best match first, and of matches
// that are as good the shorter name, as fzf orders them.
func (p *picker) refilter() {
	q := parseQuery(p.filter.String())
	p.visible = p.visible[:0]
	for _, o := range p.owners {
		if q.empty() {
			p.visible = append(p.visible, pickerEntry{owner: o})
		} else if e, ok := matchOwner(q, o); ok {
			p.visible = append(p.visible, e)
		}
	}
	if !q.empty() {
		slices.SortStableFunc(p.visible, func(a, b pickerEntry) int {
			if a.score != b.score {
				return b.score - a.score
			}
			return len([]rune(a.owner.name)) - len([]rune(b.owner.name))
		})
	}
	p.cursor, p.offset = 0, 0
}

// matchOwner matches a query against an owner, which it has to do for every word of it, each against the owner's name or,
// if that doesn't have it, its slug.
func matchOwner(q query, o owner) (pickerEntry, bool) {
	e := pickerEntry{owner: o}
	for _, term := range q.terms {
		if score, positions, ok := matchTerm(term, o.name); ok {
			e.score += score
			e.namePositions = append(e.namePositions, positions...)
		} else if score, positions, ok := matchTerm(term, o.slug); ok {
			e.score += score
			e.slugPositions = append(e.slugPositions, positions...)
		} else {
			return pickerEntry{}, false
		}
	}
	e.namePositions = mergePositions(e.namePositions)
	e.slugPositions = mergePositions(e.slugPositions)
	return e, true
}

// claimsAll is whether an owner's config-files has all of the entries already.
func claimsAll(o owner, entries []string) bool {
	for _, e := range entries {
		if !slices.Contains(o.entries, e) {
			return false
		}
	}
	return true
}

// result is what was chosen: the owners, in the order of the list, and the entries to record for each.
func (p *picker) result() (owners []owner, entries []string) {
	for _, o := range p.owners {
		if p.chosen[o.key()] {
			owners = append(owners, o)
		}
	}
	return owners, slices.Clone(p.entries())
}

// titleLines are the lines of the box before the list: what is being related, what would be claimed, the filter and a
// gap. The border is around all of it.
const titleLines = 4

func (p *picker) setSize(width, height int) {
	p.width, p.height = width, height
	p.scroll()
}

// listHeight is how many mods are shown at once: as many as there are, or as many as there is room for.
func (p *picker) listHeight() int {
	return max(min(len(p.visible), p.height-2-titleLines), 1)
}

// scroll scrolls the list so that the cursor is in it, and no further than the mods go.
func (p *picker) scroll() {
	h := p.listHeight()
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+h {
		p.offset = p.cursor - h + 1
	}
	p.offset = max(min(p.offset, len(p.visible)-h), 0)
}

func (p *picker) move(delta int) {
	if len(p.visible) == 0 {
		return
	}
	p.cursor = min(max(p.cursor+delta, 0), len(p.visible)-1)
	p.scroll()
}

func (p *picker) update(msg tea.Msg) (overlay, overlayResult) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if p.filtering {
			p.filter.handle(msg)
			p.refilter()
		}
	case tea.KeyPressMsg:
		if p.filtering {
			return p.updateFiltering(msg)
		}
		return p.updateBrowsing(msg)
	}
	return p, overlayOpen
}

// move applies a key that moves the cursor, and reports whether it was one.
func (p *picker) moveKey(msg tea.KeyPressMsg) bool {
	switch {
	case key.Matches(msg, keyMoveUp):
		p.move(-1)
	case key.Matches(msg, keyMoveDown):
		p.move(1)
	case key.Matches(msg, keyPageUp):
		p.move(-p.listHeight())
	case key.Matches(msg, keyPageDown):
		p.move(p.listHeight())
	default:
		return false
	}
	return true
}

func (p *picker) updateFiltering(msg tea.KeyPressMsg) (overlay, overlayResult) {
	switch {
	case key.Matches(msg, keyFilterClear):
		p.filter.clear()
		p.filtering = false
		p.refilter()
	case key.Matches(msg, keyFilterDone):
		p.filtering = false
	case key.Matches(msg, keyMoveUp, keyMoveDown, keyPageUp, keyPageDown) && msg.Key().Text == "":
		// Only the keys that aren't text move the cursor, as j and k are letters of a name here
		p.moveKey(msg)
	default:
		if p.filter.handle(msg) {
			p.refilter()
		}
	}
	return p, overlayOpen
}

func (p *picker) updateBrowsing(msg tea.KeyPressMsg) (overlay, overlayResult) {
	if p.moveKey(msg) {
		return p, overlayOpen
	}
	switch {
	case key.Matches(msg, keyPickerToggle):
		if len(p.visible) > 0 {
			id := p.visible[p.cursor].owner.key()
			if p.chosen[id] {
				delete(p.chosen, id)
			} else {
				p.chosen[id] = true
			}
		}
	case key.Matches(msg, keyPickerScope):
		if p.canChooseScope() {
			p.folder = !p.folder
		}
	case key.Matches(msg, keyPickerFilter):
		p.filtering = true
	case key.Matches(msg, keyPickerAccept):
		if len(p.chosen) == 0 {
			// Nothing was picked, so what is under the cursor is what is wanted
			if len(p.visible) == 0 {
				return p, overlayOpen
			}
			p.chosen[p.visible[p.cursor].owner.key()] = true
		}
		return p, overlayConfirmed
	case key.Matches(msg, keyPickerCancel):
		if !p.filter.empty() {
			// A filter that is applied goes first, as it is what is in the way of the list
			p.filter.clear()
			p.refilter()
			return p, overlayOpen
		}
		return p, overlayCancelled
	}
	return p, overlayOpen
}

func (p *picker) keys() []key.Binding {
	if p.filtering {
		return []key.Binding{keyFilterDone, keyFilterClear, keyPickerMove}
	}
	bindings := []key.Binding{keyPickerAccept, keyPickerToggle}
	if p.canChooseScope() {
		bindings = append(bindings, keyPickerScope)
	}
	return append(bindings, keyPickerFilter, keyPickerMove, keyPickerCancel)
}

// subjectsTitle says what is being related: the file, or how many files.
func subjectsTitle(subjects []string) string {
	if len(subjects) == 1 {
		return subjects[0]
	}
	return fmt.Sprintf("%d files", len(subjects))
}

// entriesLine lists entries on a line, with how many more there are if there are too many to be worth showing.
func entriesLine(entries []string) string {
	const shown = 3
	if len(entries) <= shown {
		return strings.Join(entries, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(entries[:shown], ", "), len(entries)-shown)
}

func (p *picker) view() []string {
	inner := min(p.width-4, 68)

	filter := ui.Muted.Sprint("Filter: press / to search")
	if p.filtering {
		filter = "Filter: " + p.filter.String() + ui.Muted.Sprint("█")
	} else if !p.filter.empty() {
		filter = "Filter: " + p.filter.String() + ui.Muted.Sprint("  (esc clears)")
	}

	// Where the cursor is, when there are more mods than fit, so that it is known that the list goes on
	if len(p.visible) > p.listHeight() {
		filter = spread(filter, ui.Muted.Sprintf("%d/%d", p.cursor+1, len(p.visible)), inner)
	}

	lines := []string{
		ui.Bold.Sprint("Relate " + subjectsTitle(p.subjects) + " to"),
		ui.Muted.Sprint("Claims: ") + entriesLine(p.entries()),
		filter,
		"",
	}

	if len(p.visible) == 0 {
		lines = append(lines, ui.Muted.Sprint("No mod matches"))
	}
	for i := p.offset; i < min(p.offset+p.listHeight(), len(p.visible)); i++ {
		e := p.visible[i]
		cursor, box := "  ", "[ ]"
		if p.chosen[e.owner.key()] {
			box = "[x]"
		}
		// What the filter found is picked out, in the colour of the cursor as it is what is being looked for
		name := highlight(e.owner.name, e.namePositions, ui.Info)
		if i == p.cursor {
			cursor = ui.Info.Sprint("> ")
			name = ui.Bold.Sprint(name)
		}
		line := cursor + box + " " + name
		if note := e.owner.note(); note != "" {
			line += ui.Muted.Sprint("  " + note)
		}
		if len(e.slugPositions) > 0 {
			// The name didn't have all of it, so what did is shown
			line += "  " + ui.Muted.Sprint(highlight(e.owner.slug, e.slugPositions, ui.Info))
		}
		if claimsAll(e.owner, p.entries()) {
			line += ui.Muted.Sprint("  (already claims)")
		}
		lines = append(lines, line)
	}
	return frame(lines, inner)
}

// unrelatePrompt asks whether to take entries out of an owner's config-files.
type unrelatePrompt struct {
	// owner is who the entries are taken out of the config-files of
	owner   owner
	entries []string
	// lines say what is being removed, one for each entry
	lines []string

	width, height int
}

func (u *unrelatePrompt) update(msg tea.Msg) (overlay, overlayResult) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, keyConfirm):
			return u, overlayConfirmed
		case key.Matches(msg, keyDecline):
			return u, overlayCancelled
		}
	}
	return u, overlayOpen
}

func (u *unrelatePrompt) keys() []key.Binding {
	return []key.Binding{keyConfirm, keyDecline}
}

func (u *unrelatePrompt) setSize(width, height int) {
	u.width, u.height = width, height
}

func (u *unrelatePrompt) view() []string {
	lines := []string{ui.Bold.Sprint("Remove from " + u.owner.name + "'s config-files?"), ""}
	// A file can be claimed by an entry for each folder it is in as well as its own, which is more than a small
	// terminal has room for: what doesn't fit is counted, as all of it is removed
	room := max(u.height-2-len(lines), 1)
	if len(u.lines) <= room {
		lines = append(lines, u.lines...)
	} else {
		lines = append(lines, u.lines[:room-1]...)
		lines = append(lines, ui.Muted.Sprintf("and %d more", len(u.lines)-(room-1)))
	}
	return frame(lines, min(u.width-4, 68))
}
