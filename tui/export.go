package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// The export screen is "packwiz modrinth export": it says where the pack is exported to and how, exports it (or its server
// pack), and shows what went into it.

var (
	keyExport       = key.NewBinding(key.WithKeys("enter", "e"), key.WithHelp("enter", "export"))
	keyExportFile   = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "choose the file"))
	keyExportDomain = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "allowed domains only"))
	keyExportServer = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "server pack"))
	keyExportBisect = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "Bisect Hosting pack"))
)

var exportKeys = []key.Binding{keyExport, keyExportFile, keyExportServer, keyExportBisect, keyExportDomain, keyUp, keyTop}

// exportMode is what the pack is exported as.
type exportMode int

const (
	modeMrpack exportMode = iota
	modeServer
	modeBisect
)

// exportInfoMsg is what the pack, and its server pack, would be exported to.
type exportInfoMsg struct {
	names exportNames
	err   error
}

// exportedMsg is the pack having been exported, or having failed to be.
type exportedMsg struct {
	result *modrinth.ExportResult
	err    error
}

// exportScreen exports the pack.
type exportScreen struct {
	page
	backend exportBackend

	// names are the files each kind of export goes to unless told another, and output is the file that was asked for, if
	// one was
	names  exportNames
	output string
	// restrict is whether files that aren't on the domains Modrinth allows are stored in the pack itself, as it is by default
	restrict bool
	// mode is whether it is the .mrpack, the server pack or the Bisect Hosting pack that is exported
	mode exportMode

	result *modrinth.ExportResult
	doc    report
	// pending is the options that the box that is open asks whether to export with
	pending *modrinth.ExportOptions
	// choosing is whether the box that is open asks for the file, rather than whether to overwrite it
	choosing bool
}

func newExportScreen(backend exportBackend) *exportScreen {
	return &exportScreen{backend: backend, restrict: true}
}

func (s *exportScreen) title() string { return "Export" }

func (s *exportScreen) about() string {
	return "export the pack as a .mrpack for Modrinth, or a server pack"
}

func (s *exportScreen) activate() tea.Cmd {
	backend := s.backend
	return exclusive(func() tea.Msg {
		names, err := backend.defaultExportName()
		return exportInfoMsg{names, err}
	})
}

func (s *exportScreen) keys() []key.Binding {
	if s.overlay != nil {
		return s.overlay.keys()
	}
	return exportKeys
}

// file is where the pack is exported to.
func (s *exportScreen) file() string {
	if s.output != "" {
		return s.output
	}
	return s.defaultFile()
}

// defaultFile is where the pack is exported to unless told another: the .mrpack or the server pack, whichever it is.
func (s *exportScreen) defaultFile() string {
	switch s.mode {
	case modeServer:
		return s.names.server
	case modeBisect:
		return s.names.bisect
	}
	return s.names.mrpack
}

func (s *exportScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case exportInfoMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.names = msg.names
	case exportedMsg:
		s.exported(msg)
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

func (s *exportScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	if s.doc.handleKey(msg, s.bodyHeight()-s.settingsHeight()) {
		return s, nil
	}
	switch {
	case key.Matches(msg, keyExportDomain):
		if s.mode != modeMrpack {
			s.status = infoStatus("Every file is in the server pack, wherever it comes from")
			return s, nil
		}
		s.restrict = !s.restrict
	case key.Matches(msg, keyExportServer):
		s.chooseMode(modeServer)
	case key.Matches(msg, keyExportBisect):
		s.chooseMode(modeBisect)
	case key.Matches(msg, keyExportFile):
		s.choosing = true
		s.open(newPromptBox("Export to which file?", s.output, nil,
			ui.Muted.Sprint("Leave it empty for "+s.defaultFile())))
	case key.Matches(msg, keyExport):
		return s.askOrExport()
	}
	return s, nil
}

// chooseMode exports as mode, or as the .mrpack if that is what it already was.
func (s *exportScreen) chooseMode(mode exportMode) {
	if s.mode == mode {
		mode = modeMrpack
	}
	s.mode = mode
	switch mode {
	case modeServer:
		s.status = infoStatus("Exporting the server pack")
	case modeBisect:
		s.status = infoStatus("Exporting the Bisect Hosting pack")
	default:
		s.status = infoStatus("Exporting the .mrpack")
	}
}

// askOrExport exports the pack, after asking if that would overwrite a file that is there.
func (s *exportScreen) askOrExport() (screen, tea.Cmd) {
	if s.running() {
		return s, nil
	}
	if s.file() == "" {
		s.status = warningStatus("The name of the file isn't known yet")
		return s, nil
	}
	options := modrinth.ExportOptions{Output: s.output, RestrictDomains: s.restrict, Server: s.mode == modeServer, Bisect: s.mode == modeBisect}
	if _, err := os.Stat(s.file()); err == nil {
		s.pending = &options
		s.open(newConfirmBox("Overwrite "+s.file()+"?", ui.Muted.Sprint("It is there already, and exporting replaces it.")))
		return s, nil
	}
	return s.startExport(options)
}

func (s *exportScreen) startExport(options modrinth.ExportOptions) (screen, tea.Cmd) {
	backend := s.backend
	return s, s.start("Exporting…", func(progress func(string)) tea.Msg {
		result, err := backend.exportPack(options, func(done, total int) {
			progress(fmt.Sprintf("Exporting… %d of %d files", done, total))
		})
		return exportedMsg{result, err}
	})
}

func (s *exportScreen) updateOverlay(msg tea.Msg) (screen, tea.Cmd) {
	box, result, _ := s.routeOverlay(msg)
	if result == overlayOpen {
		return s, nil
	}
	choosing, pending := s.choosing, s.pending
	s.choosing, s.pending = false, nil

	switch {
	case choosing && result == overlayConfirmed:
		if prompt, ok := box.(*promptBox); ok {
			s.output = strings.TrimSpace(prompt.text())
		}
	case !choosing && result == overlayConfirmed && pending != nil:
		return s.startExport(*pending)
	}
	return s, nil
}

// exported shows what exporting made.
func (s *exportScreen) exported(msg exportedMsg) {
	if msg.err != nil {
		s.status = errorStatus(msg.err)
		return
	}
	s.result = msg.result
	s.doc.set(exportReport(msg.result))
	s.status = successStatus("Exported to " + msg.result.Path)
}

// exportReport is the lines that say what went into the pack, for a width.
func exportReport(r *modrinth.ExportResult) func(width int) []string {
	return func(width int) []string {
		var out []string
		for _, line := range strings.Split(strings.TrimRight(r.Breakdown(), "\n"), "\n") {
			out = append(out, clip(line, width))
		}
		if len(r.Promotions) > 0 {
			out = append(out, "")
			for _, p := range r.Promotions {
				text := fmt.Sprintf("%s is only exported for the server, but %s needs it on the client; the Check screen says more", p.Mod, p.NeededBy)
				for _, line := range wrap(text, width, "  ") {
					out = append(out, ui.Warning.Sprint(line))
				}
			}
		}
		if len(r.Notices) > 0 {
			out = append(out, "")
			for _, n := range r.Notices {
				for _, line := range wrap(n, width, "  ") {
					out = append(out, ui.Muted.Sprint(line))
				}
			}
		}
		return out
	}
}

// settingsHeight is how many lines the settings take above what was exported.
func (s *exportScreen) settingsHeight() int { return 5 }

func (s *exportScreen) view() string {
	radio := func(mode exportMode, label string) string {
		if s.mode == mode {
			return "(•) " + label
		}
		return "( ) " + label
	}
	kind := radio(modeMrpack, ".mrpack") + "  " + radio(modeServer, "server pack") + "  " + radio(modeBisect, "Bisect Hosting") + "  "
	var domains, summary string
	switch s.mode {
	case modeServer:
		kind += ui.Muted.Sprint("a zip of the server's mods and files, with " + core.ServerConfigDir + "/")
		domains = ui.Muted.Sprint("every file is in the server pack")
		summary = " · a pack for a server"
	case modeBisect:
		kind += ui.Muted.Sprint("a server pack with the server installed, for Bisect Hosting to run")
		domains = ui.Muted.Sprint("every file is in the server pack")
		summary = " · a pack for Bisect Hosting"
	default:
		kind += ui.Muted.Sprint("for a launcher to install from Modrinth")
		domains = "[x] " + ui.Muted.Sprint("only files on the domains Modrinth allows are left for the launcher to download; the rest are stored in the pack")
		if !s.restrict {
			domains = "[ ] " + ui.Muted.Sprint("every file is left for the launcher to download, wherever it comes from")
		}
		summary = " · a pack for Modrinth"
	}
	lines := []string{
		field("File", s.file()),
		field("Kind", kind),
		field("Domains", domains),
		"",
	}
	room := s.bodyHeight() - len(lines) - 1
	switch {
	case s.result != nil:
		lines = append(lines, ui.Success.Sprint("Exported to "+s.result.Path))
		if s.result.Bisect && s.result.Instructions != "" {
			for _, line := range wrap(s.result.Instructions, s.width, "  ") {
				lines = append(lines, ui.Info.Sprint(line))
				room--
			}
		}
		lines = append(lines, s.doc.window(s.width, max(room, 0))...)
	default:
		lines = append(lines, messageBody("Press enter to export the pack.")...)
	}
	return s.render(ui.Bold.Sprint("Export")+ui.Muted.Sprint(summary), lines, "")
}
