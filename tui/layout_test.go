package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// newLongApp is newTestApp for a pack with names and paths that are too long for a terminal, and enough mods to fill a
// picker, so that what is drawn has to be cut to fit.
func newLongApp(t *testing.T, width, height int) *app {
	t.Helper()
	setUpPack(t)
	writeMod(t, "long", "A mod whose name goes on and on and on, well past the edge of any terminal anyone has", `config-files = ["config/a-folder-with-a-long-name/another-long-folder-name/"]`, "")
	for i := range 12 {
		writeMod(t, fmt.Sprintf("extra-%02d", i), fmt.Sprintf("Extra mod number %02d", i), "", "")
	}
	writeFile(t, "config/a-folder-with-a-long-name/another-long-folder-name/and-a-file-with-a-name-that-is-long-as-well.json", "{}")
	writeFile(t, "config/orphan-with-a-long-name-so-that-it-cannot-fit-in-forty-columns.json", "{}")
	// The pack and its loader own files too, which are drawn as groups of their own
	withLoader(t)
	writeFile(t, "options.txt", "x")
	writeFile(t, "config/neoforge-with-a-long-name-so-that-it-cannot-fit-in-forty-columns-either.toml", "x")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	claimInPack(t, core.ConfigOwnerPack, "options.txt")
	claimInPack(t, "neoforge", "config/neoforge-with-a-long-name-so-that-it-cannot-fit-in-forty-columns-either.toml")
	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	a := newApp(data.pack, newConfigScreen(packBackend{}, data))
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return a
}

// states are the things the app can be showing, each a way to get to it from a new app.
var states = map[string][]string{
	"the tree":            nil,
	"the end of the tree": {"G"},
	"a folded group":      {"left"},
	"a status message":    {"f"},
	"the picker":          {"G", "r"},
	"the picker's filter": {"G", "r", "/", "e", "x"},
	"the picker's folder": {"G", "r", "tab", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j"},
	"a prompt":            {"j", "x"},
	"help":                {"?"},
	"marked files":        {"G", "space", "k", "space"},
	// The search is a line of its own below the tree, and what matched is picked out in it
	"a search being typed":        {"/", "o", "r", "p"},
	"a search that is left on":    {"/", "j", "s", "o", "n", "enter"},
	"a search of a long name":     {"/", "l", "o", "n", "g"},
	"a search that finds nothing": {"/", "z", "z", "z"},
	"a search of a group's name":  {"/", "a", "l", "p", "h", "a"},
}

func TestEverythingDrawnFitsTheTerminal(t *testing.T) {
	sizes := [][2]int{{40, 10}, {41, 11}, {45, 40}, {60, 12}, {80, 24}, {100, 30}, {200, 50}}
	for _, mode := range []ui.Mode{ui.Never, ui.Always} {
		for _, size := range sizes {
			for name, keys := range states {
				t.Run(fmt.Sprintf("%d-%dx%d/%s", mode, size[0], size[1], name), func(t *testing.T) {
					a := newLongApp(t, size[0], size[1])
					cmdtest.SetColor(t, mode)
					appPress(t, a, keys...)

					out := strings.Split(a.render(), "\n")
					if len(out) != size[1] {
						t.Errorf("drew %d lines, want %d:\n%s", len(out), size[1], ui.Strip(strings.Join(out, "\n")))
					}
					for i, l := range out {
						if w := ansi.StringWidth(l); w > size[0] {
							t.Errorf("line %d is %d columns wide in a terminal of %d: %q", i+1, w, size[0], ui.Strip(l))
						}
					}
				})
			}
		}
	}
}

// Everything drawn says the same with colour on, which only adds to it (see package ui)
func TestColourOnlyAddsToWhatIsDrawn(t *testing.T) {
	for name, keys := range states {
		t.Run(name, func(t *testing.T) {
			a := newLongApp(t, 100, 30)
			appPress(t, a, keys...)

			cmdtest.SetColor(t, ui.Never)
			plain := a.render()
			if ui.Strip(plain) != plain {
				t.Errorf("there is colour with colour off: %q", plain)
			}
			cmdtest.SetColor(t, ui.Always)
			coloured := a.render()
			if coloured == plain {
				t.Error("there is no colour with colour on")
			}
			if stripped := ui.Strip(coloured); stripped != plain {
				t.Errorf("taking the colour out doesn't give what is drawn without it\nplain:\n%s\nstripped:\n%s", plain, stripped)
			}
		})
	}
}

// What things are is told by their colour, in the roles that the commands use for the same things
func TestTreeIsColouredForWhatEachRowIs(t *testing.T) {
	a := newTestApp(t, 100, 30)
	cmdtest.SetColor(t, ui.Always)
	out := a.render()
	for what, want := range map[string]string{
		"a valid file":          ui.Success.Sprint("config/alpha.json"),
		"a file nothing claims": ui.Warning.Sprint("config/orphan.json"),
		"an entry that matches": ui.Warning.Sprint("config/gone.json (missing)"),
		"how many are valid":    ui.Success.Sprint("3 valid"),
		"a mod's name":          ui.Bold.Sprint("Alpha Mod"),
		"the Invalid group":     ui.Bold.Sprint(ui.Warning.Sprint("Invalid")),
		"the tool's name":       ui.Bold.Sprint("packwiz"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%s isn't drawn as %q:\n%s", what, want, out)
		}
	}
}
