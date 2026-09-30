package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// The add screen is "packwiz modrinth add": give it the address of a project's page on Modrinth, or its slug, or search for
// it by name, and it says what adding it would do, which mods it needs, and asks.

var (
	keyAddSearch      = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search/add"))
	keyAddEdit        = key.NewBinding(key.WithKeys("/", "i"), key.WithHelp("/", "type"))
	keyAddKind        = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "kind of project"))
	keyAddKindTab     = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "kind of project"))
	keyAddRelease     = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "release type"))
	keyAddReleaseCtrl = key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "release type"))
	keyAddLeave       = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "leave the box"))
	keyAddWithout     = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "add without dependencies"))
)

var (
	addKeys     = []key.Binding{keyAddEdit, keyAddSearch, keyAddKind, keyAddRelease, keyUp, keyTop}
	addTypeKeys = []key.Binding{keyAddSearch, keyAddKindTab, keyAddReleaseCtrl, keyAddLeave, keyPickerMove}
)

// addKinds are the kinds of project that can be searched for, in the order t goes through them, and what they are called.
var addKinds = []struct{ kind, name string }{
	{modrinth.KindMod, "mods"},
	{modrinth.KindResourcePack, "resource packs"},
	{modrinth.KindShader, "shaders"},
}

// addReleaseTypes are what r goes through: what the pack says by default, and then the least stable version to accept.
var addReleaseTypes = []string{"", core.ReleaseTypeRelease, core.ReleaseTypeBeta, core.ReleaseTypeAlpha}

// addSearchedMsg is the answer to a search.
type addSearchedMsg struct {
	query   string
	results *modrinth.SearchResults
	err     error
}

// addPlannedMsg is what adding a project would do.
type addPlannedMsg struct {
	plan *modrinth.AddPlan
	err  error
}

// addDoneMsg is a project having been added, or having failed to be.
type addDoneMsg struct {
	result *modrinth.AddResult
	err    error
}

// addScreen looks for projects and adds them.
type addScreen struct {
	page
	backend addBackend

	query input
	// typing is whether keys go to the query, which they do until esc
	typing  bool
	kind    int
	release int

	// results are what was found for searched, and what is being searched for is query
	results  *modrinth.SearchResults
	searched string
	scroll   scroller

	// pending is what the box that is open asks whether to do
	pending *modrinth.AddPlan
}

func newAddScreen(backend addBackend) *addScreen {
	return &addScreen{backend: backend}
}

func (s *addScreen) title() string { return "Add" }

func (s *addScreen) about() string { return "add a mod, resource pack or shader from Modrinth" }

// activate does nothing: the query is typed after / as in the other screens, as a screen that took the keys as soon as it was
// shown would keep tab and the numbers from going through the screens.
func (s *addScreen) activate() tea.Cmd { return nil }

func (s *addScreen) modal() bool { return s.page.modal() || s.typing }

func (s *addScreen) keys() []key.Binding {
	switch {
	case s.overlay != nil:
		return s.overlay.keys()
	case s.typing:
		return addTypeKeys
	}
	return addKeys
}

func (s *addScreen) setSize(width, height int) {
	s.page.setSize(width, height)
	s.scroll.clamp(s.resultCount(), s.listHeight())
}

// listHeight is how many results are shown: what is left of the body under the line that has the query.
func (s *addScreen) listHeight() int { return max(s.bodyHeight()-2, 1) }

func (s *addScreen) resultCount() int {
	if s.results == nil {
		return 0
	}
	return len(s.results.Found)
}

// current is the result the cursor is on.
func (s *addScreen) current() (modrinth.Found, bool) {
	if s.results == nil || s.scroll.cursor >= len(s.results.Found) {
		return modrinth.Found{}, false
	}
	return s.results.Found[s.scroll.cursor], true
}

// releaseType is the release type that was picked, or "" for the pack's.
func (s *addScreen) releaseType() string { return addReleaseTypes[s.release] }

func (s *addScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case addSearchedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.results, s.searched = msg.results, msg.query
		s.scroll = scroller{}
		switch {
		case len(msg.results.Found) == 0:
			s.status = warningStatus(fmt.Sprintf("Nothing on Modrinth matches %q for this pack", msg.query))
		case msg.results.Total > len(msg.results.Found):
			s.status = infoStatus(fmt.Sprintf("Showing %d of %d matches: enter adds the one under the cursor", len(msg.results.Found), msg.results.Total))
		default:
			s.status = infoStatus("enter adds the one under the cursor")
		}
	case addPlannedMsg:
		return s.planned(msg)
	case addDoneMsg:
		return s.done(msg)
	case tea.KeyPressMsg:
		if s.overlay != nil {
			return s.updateOverlay(msg)
		}
		return s.updateKey(msg)
	case tea.PasteMsg:
		if s.overlay != nil {
			return s.updateOverlay(msg)
		}
		if s.typing {
			s.query.handle(msg)
		}
	}
	return s, nil
}

func (s *addScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	if s.typing {
		return s.updateTyping(msg)
	}
	if s.moveResults(msg) {
		return s, nil
	}
	switch {
	case key.Matches(msg, keyAddEdit):
		s.typing = true
	case key.Matches(msg, keyAddKind):
		s.nextKind()
	case key.Matches(msg, keyAddRelease):
		s.nextRelease()
	case key.Matches(msg, keyAddSearch):
		return s.startPlanOfResult()
	}
	return s, nil
}

// moveResults moves the cursor through the results if the key is one that does, and says whether it was.
func (s *addScreen) moveResults(msg tea.KeyPressMsg) bool {
	n, h := s.resultCount(), s.listHeight()
	switch {
	case key.Matches(msg, keyMoveUp):
		s.scroll.move(-1, n, h)
	case key.Matches(msg, keyMoveDown):
		s.scroll.move(1, n, h)
	case key.Matches(msg, keyPageUp):
		s.scroll.move(-h, n, h)
	case key.Matches(msg, keyPageDown):
		s.scroll.move(h, n, h)
	case key.Matches(msg, keyTop):
		s.scroll.move(-n, n, h)
	case key.Matches(msg, keyBottom):
		s.scroll.move(n, n, h)
	default:
		return false
	}
	return true
}

// updateTyping handles a key while the query is being typed.
func (s *addScreen) updateTyping(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	switch {
	case key.Matches(msg, keyAddLeave):
		s.typing = false
	case key.Matches(msg, keyAddKindTab):
		s.nextKind()
	case key.Matches(msg, keyAddReleaseCtrl):
		s.nextRelease()
	case key.Matches(msg, keyAddSearch):
		return s.enter()
	case key.Matches(msg, keyMoveUp, keyMoveDown, keyPageUp, keyPageDown) && msg.Key().Text == "":
		// Only the keys that aren't text move, as j and k are letters of what is being typed
		s.moveResults(msg)
	default:
		s.query.handle(msg)
	}
	return s, nil
}

func (s *addScreen) nextKind() {
	s.kind = (s.kind + 1) % len(addKinds)
	// What was found was for another kind, so it isn't shown as if it were for this one
	s.results, s.searched = nil, ""
	s.status = infoStatus("Looking for " + addKinds[s.kind].name)
}

func (s *addScreen) nextRelease() {
	s.release = (s.release + 1) % len(addReleaseTypes)
	if s.releaseType() == "" {
		s.status = infoStatus("Accepting what the pack says is stable enough")
	} else {
		s.status = infoStatus("Accepting versions that are " + s.releaseType() + " or more stable")
	}
}

// enter does what the query and the results ask for: adds a project whose address was typed, or the result the cursor is on
// if it is for what is typed, or else searches for what is typed.
func (s *addScreen) enter() (screen, tea.Cmd) {
	text := strings.TrimSpace(s.query.String())
	switch {
	case s.running():
		return s, nil
	case text == "":
		return s.startPlanOfResult()
	case strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://"):
		return s.startPlan(text)
	case s.results != nil && s.searched == text && len(s.results.Found) > 0:
		return s.startPlanOfResult()
	}
	return s.startSearch(text)
}

func (s *addScreen) startSearch(text string) (screen, tea.Cmd) {
	backend, kind := s.backend, addKinds[s.kind]
	return s, s.start("Searching for "+kind.name+"…", func(func(string)) tea.Msg {
		results, err := backend.search(text, kind.kind)
		return addSearchedMsg{text, results, err}
	})
}

// startPlanOfResult works out what adding the result under the cursor would do.
func (s *addScreen) startPlanOfResult() (screen, tea.Cmd) {
	found, ok := s.current()
	if !ok {
		s.status = warningStatus("Nothing is picked: type what to search for, or the address of a project, and press enter")
		return s, nil
	}
	return s.startPlan(found.ID)
}

func (s *addScreen) startPlan(ref string) (screen, tea.Cmd) {
	if s.running() {
		return s, nil
	}
	backend, release := s.backend, s.releaseType()
	return s, s.start("Finding out what adding it would do…", func(func(string)) tea.Msg {
		plan, err := backend.planAdd(ref, release)
		return addPlannedMsg{plan, err}
	})
}

// planned says what adding the project would do, and asks whether to.
func (s *addScreen) planned(msg addPlannedMsg) (screen, tea.Cmd) {
	if msg.err != nil {
		s.status = errorStatus(msg.err)
		return s, nil
	}
	plan := msg.plan
	switch e := plan.Existing; {
	case e != nil && e.UpToDate:
		s.status = successStatus(plan.Project + " is already added and up to date")
		return s, nil
	case e != nil && e.Pinned:
		s.status = errorStatus(fmt.Errorf("%s is pinned; unpin it to allow updating", plan.Project))
		return s, nil
	}
	s.pending = plan
	s.open(&addConfirm{confirmBox: *newConfirmBox(addTitle(plan), addLines(plan)...), dependencies: len(plan.Dependencies) > 0})
	return s, nil
}

// addTitle is the question that is asked about a plan.
func addTitle(plan *modrinth.AddPlan) string {
	if plan.Existing != nil {
		return "Update " + plan.Project + "?"
	}
	return "Add " + plan.Project + "?"
}

// addLines say what adding a project would do.
func addLines(plan *modrinth.AddPlan) []string {
	lines := []string{
		ui.Bold.Sprint(plan.Project) + " " + plan.Version + ui.Muted.Sprintf(" (%s)", plan.ReleaseType),
		field("file", plan.File),
		field("goes in", plan.Folder+"/"),
		field("runs on", plan.Side),
	}
	if e := plan.Existing; e != nil {
		lines = append(lines, field("now at", ui.Transition(e.Current, plan.Version)))
	}
	if len(plan.Dependencies) > 0 {
		lines = append(lines, "", ui.Bold.Sprint("It requires, which the pack doesn't have"))
		for _, d := range plan.Dependencies {
			lines = append(lines, "  "+d.Name+" "+d.Version+ui.Muted.Sprintf(" (%s)", d.File))
		}
	}
	if len(plan.Notices) > 0 {
		lines = append(lines, "")
		for _, n := range plan.Notices {
			lines = append(lines, ui.Muted.Sprint(n))
		}
	}
	return lines
}

// addConfirm is the question of whether to add a project. It is a yes or no, with one more answer when the project has
// dependencies: to add it without them.
type addConfirm struct {
	confirmBox
	dependencies bool
	// without is whether the answer was to leave the dependencies out
	without bool
}

func (a *addConfirm) update(msg tea.Msg) (overlay, overlayResult) {
	if k, ok := msg.(tea.KeyPressMsg); ok && a.dependencies && key.Matches(k, keyAddWithout) {
		a.without = true
		return a, overlayConfirmed
	}
	_, result := a.confirmBox.update(msg)
	return a, result
}

func (a *addConfirm) keys() []key.Binding {
	bindings := []key.Binding{keyConfirm}
	if a.dependencies {
		bindings = append(bindings, keyAddWithout)
	}
	return append(bindings, keyDecline)
}

func (s *addScreen) updateOverlay(msg tea.Msg) (screen, tea.Cmd) {
	box, result, _ := s.routeOverlay(msg)
	plan := s.pending
	if result != overlayOpen {
		s.pending = nil
	}
	if result != overlayConfirmed || plan == nil {
		return s, nil
	}
	without := false
	if confirm, ok := box.(*addConfirm); ok {
		without = confirm.without
	}
	backend := s.backend
	return s, s.start("Adding "+plan.Project+"…", func(func(string)) tea.Msg {
		result, err := backend.applyAdd(plan, !without)
		return addDoneMsg{result, err}
	})
}

// done says what adding did, and gets the screen ready for the next project.
func (s *addScreen) done(msg addDoneMsg) (screen, tea.Cmd) {
	if msg.err != nil {
		s.status = errorStatus(msg.err)
		return s, nil
	}
	r := msg.result
	verb := "Added "
	if r.Updated {
		verb = "Updated "
	}
	text := verb + r.Project + " (" + r.File + ")"
	if len(r.Dependencies) > 0 {
		text += " with " + entriesLine(r.Dependencies)
	}
	if len(r.Notices) > 0 {
		s.status = warningStatus(text + ". " + strings.Join(r.Notices, " "))
	} else {
		s.status = successStatus(text)
	}
	// What is found is marked as being in the pack, which it now is
	if s.results != nil {
		for i := range s.results.Found {
			if s.results.Found[i].Title == r.Project {
				s.results.Found[i].InPack = true
			}
		}
	}
	return s, nil
}

func (s *addScreen) view() string {
	return s.render(s.summary(), s.bodyLines(), s.hint())
}

func (s *addScreen) summary() string {
	left := ui.Bold.Sprint("Add") + ui.Muted.Sprint(" · "+addKinds[s.kind].name)
	if rt := s.releaseType(); rt != "" {
		left += ui.Muted.Sprint(" · " + rt + " or more stable")
	}
	return spread(left, ui.Muted.Sprint(s.scroll.position(s.resultCount())), s.width)
}

// hint is what the status line says when nothing has been done.
func (s *addScreen) hint() string {
	if s.typing {
		return ui.Muted.Sprint("Type a name to search for, or paste the address of a project on Modrinth, then press enter")
	}
	return ""
}

func (s *addScreen) bodyLines() []string {
	prompt := ui.Bold.Sprint("Search ") + s.query.String()
	if s.typing {
		prompt += ui.Muted.Sprint("█")
	} else if s.query.empty() {
		prompt += ui.Muted.Sprint("press / to type a name, or the address of a project")
	}
	lines := []string{prompt, ""}

	switch {
	case s.results == nil:
		lines = append(lines, messageBody(
			"Search Modrinth by name, or give the address of a project's page:",
			"  https://modrinth.com/mod/sodium",
			"  sodium  (its slug or ID also works)",
		)...)
	case len(s.results.Found) == 0:
		lines = append(lines, messageBody("Nothing matches.")...)
	default:
		from, to := s.scroll.visible(len(s.results.Found), s.listHeight())
		for i := from; i < to; i++ {
			lines = append(lines, s.renderResult(s.results.Found[i], i == s.scroll.cursor))
		}
	}
	return lines
}

func (s *addScreen) renderResult(f modrinth.Found, selected bool) string {
	cursor := "  "
	if selected {
		cursor = ui.Info.Sprint("> ")
	}
	title := ui.Bold.Sprint(f.Title)
	titleWidth := min(max(s.width/4, 12), 28)
	parts := []string{padRight(clip(title, titleWidth), titleWidth)}
	if s.width >= 60 {
		parts = append(parts, ui.Muted.Sprint(padRight(clip("by "+f.Author, 18), 18)))
	}
	if s.width >= 50 {
		parts = append(parts, ui.Muted.Sprint(padLeft(downloads(f.Downloads), 6)))
	}
	line := cursor + strings.Join(parts, "  ")
	if f.InPack {
		line += "  " + ui.Success.Sprint("in the pack")
	}
	if room := s.width - ansi.StringWidth(line) - 2; room > 12 && f.Description != "" {
		line += "  " + ui.Muted.Sprint(clip(f.Description, room))
	}
	return line
}

// padLeft adds spaces before s until it is width columns wide.
func padLeft(s string, width int) string {
	if w := ansi.StringWidth(s); w < width {
		return strings.Repeat(" ", width-w) + s
	}
	return s
}

// downloads writes how many times something was downloaded in few characters: "233M", "1.2k".
func downloads(n int) string {
	switch {
	case n >= 10_000_000:
		return fmt.Sprintf("%dM", n/1_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
