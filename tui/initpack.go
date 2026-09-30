package tui

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// The init screen is "packwiz init": it is what the TUI opens on in a folder that has no pack, asks what the pack is, and
// makes it. Once it has, the TUI goes on with the pack as if it had been opened on it.

var (
	keyInitNext   = key.NewBinding(key.WithKeys("enter", "down", "tab"), key.WithHelp("enter", "next"))
	keyInitPrev   = key.NewBinding(key.WithKeys("up", "shift+tab"), key.WithHelp("↑", "previous"))
	keyInitChoose = key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "choose"))
	keyInitCreate = key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "create the pack"))
)

var initKeys = []key.Binding{keyInitNext, keyInitPrev, keyInitChoose, keyInitCreate}

// The fields of the screen, in the order they are in.
const (
	fieldName = iota
	fieldAuthor
	fieldVersion
	fieldMinecraft
	fieldLoader
	fieldLoaderVersion
	fieldCreate
	fieldCount
)

// defaultVersion is what the pack's version is unless it is told another.
const defaultVersion = "1.0.0"

// initVersionsMsg is the answer to looking up what versions of Minecraft there are.
type initVersionsMsg struct {
	versions minecraftVersions
	err      error
}

// initCreatedMsg is the pack having been made, or having failed to be.
type initCreatedMsg struct {
	err error
}

// packCreatedMsg says that there is a pack now, which the app opens the rest of the interface on.
type packCreatedMsg struct{}

// openFailedMsg says that the pack that was made couldn't be opened.
type openFailedMsg struct{ err error }

// initScreen asks what a pack is, and makes it.
type initScreen struct {
	page
	backend initBackend

	name, author, version, minecraft, loaderVersion input
	// loaders are the mod loaders that can be chosen, then "none", and loader is which is chosen
	loaders []string
	loader  int
	focus   int

	// versions are what versions of Minecraft there are, once they are looked up, and versionsErr why they couldn't be
	versions    *minecraftVersions
	versionsErr error
}

func newInitScreen(backend initBackend) *initScreen {
	s := &initScreen{backend: backend}
	s.loaders = append(slices.Sorted(maps.Keys(core.ModLoaders)), "none")
	s.name.insert(backend.defaultName())
	s.version.insert(defaultVersion)
	return s
}

func (s *initScreen) title() string { return "New pack" }

func (s *initScreen) about() string { return "make a pack in this folder" }

// activate looks up what versions of Minecraft there are, which is what it can suggest.
func (s *initScreen) activate() tea.Cmd {
	if s.versions != nil || s.running() {
		return nil
	}
	backend := s.backend
	return exclusive(func() tea.Msg {
		versions, err := backend.minecraftVersions()
		return initVersionsMsg{versions, err}
	})
}

// modal is always true: a field is being typed in, or chosen from, so the keys are the screen's.
func (s *initScreen) modal() bool { return true }

func (s *initScreen) keys() []key.Binding { return initKeys }

// inputs are the fields that are typed in, by the field they are for.
func (s *initScreen) input(field int) *input {
	switch field {
	case fieldName:
		return &s.name
	case fieldAuthor:
		return &s.author
	case fieldVersion:
		return &s.version
	case fieldMinecraft:
		return &s.minecraft
	case fieldLoaderVersion:
		return &s.loaderVersion
	}
	return nil
}

// loaderName is the mod loader that is chosen, or "" for none.
func (s *initScreen) loaderName() string {
	if name := s.loaders[s.loader]; name != "none" {
		return name
	}
	return ""
}

// skipped is whether a field is left out, as there is nothing to ask: the version of a mod loader when there is none.
func (s *initScreen) skipped(field int) bool {
	return field == fieldLoaderVersion && s.loaderName() == ""
}

// moveFocus goes to the next field (or the previous, for a negative delta) that isn't left out.
func (s *initScreen) moveFocus(delta int) {
	for range fieldCount {
		s.focus = (s.focus + delta + fieldCount) % fieldCount
		if !s.skipped(s.focus) {
			return
		}
	}
}

func (s *initScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if result, next, mine := s.receive(msg); mine {
		if result == nil {
			return s, next
		}
		msg = result
	}
	switch msg := msg.(type) {
	case initVersionsMsg:
		if msg.err != nil {
			s.versionsErr = msg.err
			s.status = warningStatus("Couldn't look up the versions of Minecraft: " + msg.err.Error())
			return s, nil
		}
		s.versions, s.versionsErr = &msg.versions, nil
	case initCreatedMsg:
		if msg.err != nil {
			s.status = errorStatus(msg.err)
			return s, nil
		}
		s.status = successStatus("Created pack.toml")
		return s, func() tea.Msg { return packCreatedMsg{} }
	case openFailedMsg:
		s.status = errorStatus(fmt.Errorf("the pack was made, but couldn't be opened: %w", msg.err))
	case tea.KeyPressMsg:
		return s.updateKey(msg)
	case tea.PasteMsg:
		if in := s.input(s.focus); in != nil && !s.running() {
			in.handle(msg)
		}
	}
	return s, nil
}

func (s *initScreen) updateKey(msg tea.KeyPressMsg) (screen, tea.Cmd) {
	s.status = status{}
	if s.running() {
		return s, nil
	}
	switch {
	case key.Matches(msg, keyInitCreate):
		return s.startCreate()
	case key.Matches(msg, keyInitPrev):
		s.moveFocus(-1)
	case key.Matches(msg, keyInitNext) && msg.Key().Text == "":
		// enter on the last field makes the pack, as there is nothing after it
		if s.focus == fieldCreate && key.Matches(msg, keyOK) {
			return s.startCreate()
		}
		s.moveFocus(1)
	case s.focus == fieldLoader && key.Matches(msg, keyInitChoose):
		step := 1
		if msg.String() == "left" {
			step = -1
		}
		s.loader = (s.loader + step + len(s.loaders)) % len(s.loaders)
	default:
		if in := s.input(s.focus); in != nil {
			in.handle(msg)
		}
	}
	return s, nil
}

// startCreate checks what was asked for, and makes the pack. Checking it needs the network, so it is done with the rest.
func (s *initScreen) startCreate() (screen, tea.Cmd) {
	name := strings.TrimSpace(s.name.String())
	if name == "" {
		s.status = warningStatus("The pack needs a name")
		s.focus = fieldName
		return s, nil
	}
	pack := cmd.NewPack{
		Name: name, Author: strings.TrimSpace(s.author.String()), Version: strings.TrimSpace(s.version.String()),
		MCVersion: strings.TrimSpace(s.minecraft.String()), Loader: s.loaderName(), LoaderVersion: strings.TrimSpace(s.loaderVersion.String()),
		IndexFile: "index.toml",
	}
	if pack.Version == "" {
		pack.Version = defaultVersion
	}
	backend, versions := s.backend, s.versions
	return s, s.start("Creating the pack…", func(func(string)) tea.Msg {
		return initCreatedMsg{createWith(backend, versions, pack)}
	})
}

// createWith checks the versions that were chosen and makes the pack. A version that wasn't chosen is the latest.
func createWith(backend initBackend, versions *minecraftVersions, pack cmd.NewPack) error {
	switch {
	case versions == nil:
		return errors.New("can't check the Minecraft version, as the versions there are couldn't be looked up")
	case pack.MCVersion == "":
		pack.MCVersion = versions.latest
	case !versions.valid(pack.MCVersion):
		return fmt.Errorf("%q isn't a version of Minecraft", pack.MCVersion)
	}
	if pack.Loader != "" {
		version, err := backend.loaderVersion(pack.Loader, pack.MCVersion, pack.LoaderVersion)
		if err != nil {
			return err
		}
		pack.LoaderVersion = version
	}
	return backend.createPack(pack)
}

func (s *initScreen) view() string {
	return s.render(ui.Bold.Sprint("Create a pack")+ui.Muted.Sprint(" · in this folder"), s.bodyLines(), s.hint())
}

// hint says what the field that is focused is for.
func (s *initScreen) hint() string {
	switch s.focus {
	case fieldName:
		return ui.Muted.Sprint("What the pack is called: it is in pack.toml, and names the exported file")
	case fieldMinecraft:
		return ui.Muted.Sprint("Leave it empty for the latest release")
	case fieldLoader:
		return ui.Muted.Sprint("Left and right choose. Mods go with the loader that runs them")
	case fieldLoaderVersion:
		return ui.Muted.Sprint("Leave it empty for the latest version that suits the Minecraft version")
	case fieldCreate:
		return ui.Muted.Sprint("Press enter to create the pack")
	}
	return ""
}

func (s *initScreen) bodyLines() []string {
	lines := []string{ui.Muted.Sprint("No pack.toml is here, so this sets one up. You can change all of it later, in pack.toml."), ""}
	row := func(field int, label, value, suffix string) {
		cursor := "  "
		if s.focus == field {
			cursor = ui.Info.Sprint("> ")
			if in := s.input(field); in != nil {
				value += ui.Muted.Sprint("█")
			}
			label = ui.Bold.Sprint(padRight(label, 16))
		} else {
			label = ui.Muted.Sprint(padRight(label, 16))
		}
		lines = append(lines, cursor+label+" "+value+suffix)
	}

	row(fieldName, "Name", s.name.String(), "")
	row(fieldAuthor, "Author", s.author.String(), "")
	row(fieldVersion, "Version", s.version.String(), "")
	row(fieldMinecraft, "Minecraft", s.minecraft.String(), s.minecraftNote())

	loaders := make([]string, len(s.loaders))
	for i, name := range s.loaders {
		label := core.LoaderName(name)
		if name == "none" {
			label = "none"
		}
		if i == s.loader {
			label = ui.Info.Sprint("[" + label + "]")
		}
		loaders[i] = label
	}
	row(fieldLoader, "Mod loader", strings.Join(loaders, " "), "")
	if !s.skipped(fieldLoaderVersion) {
		row(fieldLoaderVersion, core.LoaderName(s.loaderName())+" version", s.loaderVersion.String(), "")
	}
	lines = append(lines, "")
	create := "[ Create the pack ]"
	if s.focus == fieldCreate {
		create = ui.Bold.Sprint(ui.Info.Sprint(create))
	}
	lines = append(lines, "  "+create)
	return lines
}

// minecraftNote says what is known of the Minecraft version that was typed, or of the ones there are when none was.
func (s *initScreen) minecraftNote() string {
	text := strings.TrimSpace(s.minecraft.String())
	switch {
	case s.versionsErr != nil:
		return ui.Warning.Sprint("  can't look up the versions (" + s.versionsErr.Error() + ")")
	case s.versions == nil:
		return ui.Muted.Sprint("  looking up the versions…")
	case text == "":
		return ui.Muted.Sprint("  the latest release is " + s.versions.latest)
	case !s.versions.valid(text):
		return ui.Warning.Sprint("  isn't a version of Minecraft")
	}
	return ""
}
