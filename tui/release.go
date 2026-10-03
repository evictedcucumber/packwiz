package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/viper"
)

// The release screen is "packwiz changelog", "packwiz changelog release", "packwiz commit" and "packwiz git release":
// the release that the pack's changes would make, what hasn't been committed yet, and keys to commit, to release, and to
// write the changelog.

var (
	keyRelease        = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "release"))
	keyReleaseCommit  = key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "commit"))
	keyReleaseVersion = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "choose the version"))
	keyReleaseTag     = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "commit and tag"))
	keyReleaseSave    = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "write CHANGELOG.md"))
)

var releaseKeys = []key.Binding{keyRelease, keyReleaseCommit, keyReleaseVersion, keyReleaseTag, keyReleaseSave, keyRecheck, keyUp, keyTop}

// releaseLoadedMsg is the answer to working out the next release.
type releaseLoadedMsg struct {
	data releaseData
	err  error
}

// releaseCommittedMsg is the pack having been committed, or having failed to be.
type releaseCommittedMsg struct {
	committed []string
	notices   []string
	err       error
}

// releaseMadeMsg is a release having been made, or having failed to be.
type releaseMadeMsg struct {
	outcome releaseOutcome
	err     error
}

// releaseSavedMsg is the changelog having been written, or having failed to be.
type releaseSavedMsg struct {
	path    string
	notices []string
	err     error
}

// releaseScreen shows the next release, and commits and releases.
type releaseScreen struct {
	page
	backend releaseBackend

	ran  bool
	data *releaseData
	doc  report
	// version is the version that was asked for, or "" for the one that is worked out from the changes
	version string
	// tag is whether releasing also commits the release and tags it, as "packwiz git release" does. It is on when the pack is in a
	// repository, until it is turned off.
	tag    bool
	tagSet bool

	// pending says what the box that is open asks: to commit, to release, or for a version
	pending releaseAsk
}

// releaseAsk is what a box of the release screen is for.
type releaseAsk int

const (
	askNothing releaseAsk = iota
	askCommit
	askRelease
	askVersion
)

func newReleaseScreen(backend releaseBackend) *releaseScreen {
	return &releaseScreen{backend: backend}
}

func (s *releaseScreen) title() string { return "Release" }

func (s *releaseScreen) about() string { return "see what the next release is, commit, and release" }

// activate works out the release the first time the screen is shown. After that it shows what it found, which is worked out
// again when asked to, as it reads the log and asks the network about mods that don't record their version.
func (s *releaseScreen) activate() tea.Cmd {
	if s.ran || s.running() {
		return nil
	}
	s.ran = true
	return s.startLoad()
}

func (s *releaseScreen) keys() []key.Binding {
	if s.overlay != nil {
		return s.overlay.keys()
	}
	return releaseKeys
}

func (s *releaseScreen) startLoad() tea.Cmd {
	backend, version := s.backend, s.version
	return s.start("Working out the next release…", func(func(string)) tea.Msg {
		data, err := backend.loadRelease(version)
		return releaseLoadedMsg{data, err}
	})
}

// tagging is whether releasing commits and tags the release.
func (s *releaseScreen) tagging() bool {
	return s.data != nil && s.data.preview.InRepository && s.tag
}

func (s *releaseScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case releaseLoadedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			// A version that can't be used isn't kept, or every look at the release would fail the same way
			if s.version != "" {
				s.version = ""
				s.status = errorStatus(fmt.Errorf("%v (going back to the version the changes make)", msg.err))
			}
			return s, nil
		}
		s.data = &msg.data
		if !s.tagSet {
			s.tag = msg.data.preview.InRepository
		}
		s.doc.set(releaseReport(msg.data))
		if len(msg.data.notices) > 0 {
			s.status = infoStatus(strings.Join(msg.data.notices, " "))
		}
	case releaseCommittedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
		} else {
			s.status = successStatus(commitSummary(msg.committed))
		}
		return s, s.startLoad()
	case releaseMadeMsg:
		return s.made(msg)
	case releaseSavedMsg:
		switch {
		case msg.err != nil:
			s.status = errorStatus(msg.err)
		case len(msg.notices) > 0:
			s.status = warningStatus("Wrote " + msg.path + ". " + strings.Join(msg.notices, " "))
		default:
			s.status = successStatus("Wrote " + msg.path)
		}
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

// commitSummary says what committing did.
func commitSummary(committed []string) string {
	switch len(committed) {
	case 0:
		return "Nothing to commit"
	case 1:
		return "Committed: " + firstLine(committed[0])
	}
	return fmt.Sprintf("Made %s", count(len(committed), "commit", "commits"))
}

// firstLine is the first line of a commit message, which is its subject.
func firstLine(message string) string {
	first, _, _ := strings.Cut(message, "\n")
	return first
}

func (s *releaseScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	if s.doc.handleKey(msg, s.bodyHeight()) {
		return s, nil
	}
	if s.running() {
		return s, nil
	}
	switch {
	case key.Matches(msg, keyRecheck):
		return s, s.startLoad()
	case key.Matches(msg, keyReleaseTag):
		if s.data != nil && !s.data.preview.InRepository {
			s.status = warningStatus("The pack isn't in a git repository, so there is nothing to commit or tag")
			return s, nil
		}
		s.tag, s.tagSet = !s.tag, true
		if s.tag {
			s.status = infoStatus("Releasing will commit the release and tag it")
		} else {
			s.status = infoStatus("Releasing will only record the release, without committing it or tagging it")
		}
	case key.Matches(msg, keyReleaseVersion):
		s.pending = askVersion
		s.open(newPromptBox("Release which version?", s.version, checkVersion,
			ui.Muted.Sprint("Leave it empty for the version the changes make")))
	case key.Matches(msg, keyReleaseCommit):
		return s.askCommit()
	case key.Matches(msg, keyRelease):
		return s.askRelease()
	case key.Matches(msg, keyReleaseSave):
		backend := s.backend
		return s, s.start("Writing the changelog…", func(func(string)) tea.Msg {
			path, notices, err := backend.saveChangelog()
			return releaseSavedMsg{path, notices, err}
		})
	}
	return s, nil
}

// checkVersion says what is wrong with a version that was typed, or "" if it will do.
func checkVersion(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if _, err := changelog.ParseVersion(text); err != nil {
		return "That isn't a version like 1.2.3"
	}
	return ""
}

func (s *releaseScreen) askCommit() (screen, tea.Cmd) {
	switch {
	case s.data == nil:
		s.status = warningStatus("The release hasn't been worked out yet")
	case !s.data.preview.InRepository:
		s.status = warningStatus("The pack isn't in a git repository, so there is nothing to commit: " + s.data.preview.Reason)
	case len(s.data.pending) == 0:
		s.status = infoStatus("Nothing to commit")
	default:
		s.pending = askCommit
		var lines []string
		for _, m := range s.data.pending {
			lines = append(lines, styleCommit(m))
		}
		s.open(newConfirmBox(fmt.Sprintf("Make %s?", count(len(lines), "commit", "commits")), lines...))
	}
	return s, nil
}

func (s *releaseScreen) askRelease() (screen, tea.Cmd) {
	switch {
	case s.data == nil:
		s.status = warningStatus("The release hasn't been worked out yet")
	case !s.data.preview.HasChanges:
		s.status = infoStatus(s.data.preview.NoChanges)
	case s.data.preview.InRepository && len(s.data.pending) > 0:
		s.status = warningStatus(count(len(s.data.pending), "change isn't", "changes aren't") + " committed yet; commit first")
	default:
		s.pending = askRelease
		p := s.data.preview
		lines := []string{ui.Bold.Sprint(p.Release.Version) + ui.Muted.Sprintf(" (%s)", releaseKind(p))}
		records := "the release in " + changelog.HistoryFile + " and " + changelog.MarkdownFile
		if changelog.HasServerPack(filepath.Dir(viper.GetString("pack-file"))) {
			records = "the release in " + changelog.HistoryFile + ", " + changelog.MarkdownFile + " and " + changelog.ServerMarkdownFile
		}
		lines = append(lines, field("records", records))
		lines = append(lines, field("pack.toml", "gets the version "+p.Release.Version))
		if s.tagging() {
			lines = append(lines, field("then", "commits it as chore(release): "+p.Release.Version+" and tags it v"+p.Release.Version))
		}
		s.open(newConfirmBox("Release "+p.Release.Version+"?", lines...))
	}
	return s, nil
}

// releaseKind says what makes the release: how far its changes raise the version.
func releaseKind(p changelog.Preview) string {
	if p.Last == "" {
		return "first release"
	}
	return p.Release.Bump.String() + " bump"
}

func (s *releaseScreen) updateOverlay(msg tea.Msg) (screen, tea.Cmd) {
	box, result, _ := s.routeOverlay(msg)
	if result == overlayOpen {
		return s, nil
	}
	ask := s.pending
	s.pending = askNothing
	if result != overlayConfirmed {
		return s, nil
	}

	backend := s.backend
	switch ask {
	case askVersion:
		if prompt, ok := box.(*promptBox); ok {
			s.version = strings.TrimSpace(prompt.text())
			return s, s.startLoad()
		}
	case askCommit:
		return s, s.start("Committing…", func(func(string)) tea.Msg {
			committed, notices, err := backend.commit()
			return releaseCommittedMsg{committed, notices, err}
		})
	case askRelease:
		version, tag := s.version, s.tagging()
		return s, s.start("Releasing…", func(func(string)) tea.Msg {
			outcome, err := backend.release(version, tag)
			return releaseMadeMsg{outcome, err}
		})
	}
	return s, nil
}

// made says what releasing did, and works out the next release, which is of what has changed since.
func (s *releaseScreen) made(msg releaseMadeMsg) (screen, tea.Cmd) {
	o := msg.outcome
	switch {
	case msg.err != nil:
		s.status = errorStatus(msg.err)
	case !o.made:
		s.status = infoStatus(o.noChanges)
	case o.tag != "":
		s.status = successStatus("Released " + o.version + ", committed and tagged " + o.tag)
	default:
		s.status = successStatus("Released " + o.version)
	}
	// The version that was asked for was for that release
	s.version = ""
	return s, s.startLoad()
}

// styleCommit shows a commit message: its subject, in bold, and the rest as it is.
func styleCommit(message string) string {
	subject, body, hasBody := strings.Cut(message, "\n")
	if !hasBody {
		return ui.Bold.Sprint(subject)
	}
	return ui.Bold.Sprint(subject) + "\n" + strings.TrimSpace(body)
}

// releaseReport is the lines that say where the pack is, what isn't committed, and what the release would be.
func releaseReport(d releaseData) func(width int) []string {
	return func(width int) []string {
		p := d.preview
		var out []string
		add := func(line string) { out = append(out, wrap(line, width, strings.Repeat(" ", 14))...) }
		if p.Last != "" {
			add(field("Last release", p.Last))
		} else {
			add(field("Last release", "none yet"))
		}
		if p.InRepository {
			add(field("Repository", "git: the release is made from the commits since the last"))
		} else {
			add(field("Repository", "none: "+p.Reason))
			add(ui.Muted.Sprint("              The release is what differs from the pack as it was at the last one."))
		}
		if len(d.pending) > 0 {
			out = append(out, "", ui.Bold.Sprintf("Not committed yet (%d)", len(d.pending)))
			for _, m := range d.pending {
				out = append(out, wrap("  "+ui.Muted.Sprint(firstLine(m)), width, "    ")...)
			}
		}
		out = append(out, "")
		for _, line := range strings.Split(strings.TrimRight(p.Text(), "\n"), "\n") {
			out = append(out, wrap(line, width, "  ")...)
		}
		return out
	}
}

func (s *releaseScreen) view() string {
	var body []string
	switch {
	case s.data == nil && s.running():
		body = messageBody(s.worker.text)
	case s.data == nil:
		body = messageBody("The release hasn't been worked out yet.", "Press c to work it out.")
	default:
		body = s.doc.window(s.width, s.bodyHeight())
	}
	return s.render(s.summary(), body, "")
}

func (s *releaseScreen) summary() string {
	left := ui.Bold.Sprint("Release")
	if s.data != nil {
		p := s.data.preview
		switch {
		case p.HasChanges:
			left += "  " + ui.Info.Sprint(p.Release.Version) + ui.Muted.Sprintf(" (%s)", releaseKind(p))
		default:
			left += ui.Muted.Sprint("  nothing to release")
		}
		if s.tagging() {
			left += ui.Muted.Sprint(" · commits and tags")
		}
	}
	return spread(left, ui.Muted.Sprint(s.doc.position(s.bodyHeight())), s.width)
}
