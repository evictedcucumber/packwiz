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
	"github.com/jarcoal/httpmock"
)

const longText = "a line of text that goes on well past the edge of any terminal that anyone has, to be cut or wrapped"

// newFullApp makes the app with every screen the TUI has, on a pack with names and paths that are too long, where what needs
// the network has something to say without it, so that each screen has what to draw: what is long enough to fill it, and
// what has to be cut or wrapped to fit.
func newFullApp(t *testing.T, width, height int) *app {
	t.Helper()
	setUpLongPack(t)
	httpmock.Activate(t)
	useUpdater(t, &stubUpdater{checks: map[string]core.UpdateCheck{
		"Extra mod number 03": {UpdateAvailable: true, UpdateString: "extra-03.jar -> " + longText + ".jar", CachedState: "new.jar"},
		"Extra mod number 04": {Error: fmt.Errorf("%s", longText)},
	}})
	for i := range 6 {
		writeMod(t, fmt.Sprintf("extra-%02d", i), fmt.Sprintf("Extra mod number %02d", i), "", stubTable+dependencyTable("needed-"+longText, "required"))
	}
	// Mods that come from Modrinth, that record what they need
	for i := range 4 {
		writeMod(t, fmt.Sprintf("long-dep-%d", i), fmt.Sprintf("A mod from Modrinth number %d with a name too long for a narrow terminal", i), "",
			modrinthTable(fmt.Sprintf("proj-%d", i), "v1")+dependencyTable("lib-id", "required")+dependencyTable("proj-0", "optional"))
	}
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewJsonResponderOrPanic(200, []map[string]string{{"id": "lib-id", "title": "A library with a name that is long, too"}}))
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}

	add := newFakeAdd()
	add.results.Found[0].Title = "Sodium with a title too long for the column it is in, to be cut"
	add.results.Found[0].Description = longText
	add.plan.Notices = []string{longText, longText}
	add.plan.Project = "A project with a name that goes on and on and on, well past the edge"

	export := newFakeExport()
	export.result.Files[0].Name = "A mod with a long name that goes past the edge of the terminal"
	export.result.Promotions[0].Mod = longText
	export.result.Notices = []string{longText}

	release := newFakeRelease()
	release.data.pending = append(release.data.pending, longText, longText)
	release.data.notices = []string{longText}
	release.data.preview.Notes = []string{longText}

	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	overview := newOverviewScreen(packBackend{})
	screens := []screen{
		overview, newModsScreen(packBackend{}), newAddScreen(add), newUpdatesScreen(packBackend{}), newCheckScreen(packBackend{}),
		newDepsScreen(packBackend{}), newConfigScreen(packBackend{}, data), newExportScreen(export), newReleaseScreen(release),
	}
	overview.guide = guideFor(screens)
	a := newApp(data.pack, screens...)
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	appFeed(t, a, a.Init()())
	return a
}

// goToScreen goes to the screen with a name.
func goToScreen(t *testing.T, a *app, title string) {
	t.Helper()
	for i, s := range a.screens {
		if s.title() == title {
			appPress(t, a, fmt.Sprint(i+1))
			return
		}
	}
	t.Fatalf("there is no screen called %s", title)
}

// screenStates are the things each screen can be showing, each a way to get to it from when the screen is first shown.
var screenStates = map[string]map[string][]string{
	"Overview": {
		"the overview": nil,
		"help":         {"?"},
		"a refresh":    {"R"},
	},
	"Mods": {
		"the list":                    nil,
		"the end of the list":         {"G"},
		"a search being typed":        {"/", "l", "o", "n", "g"},
		"a search that is left on":    {"/", "m", "o", "d", "enter"},
		"a search that finds nothing": {"/", "z", "z", "z"},
		"details":                     {"enter"},
		"details of a mod with deps":  {"G", "enter"},
		"a side filter":               {"s"},
		"a kind filter":               {"m"},
		"a pin":                       {"p"},
		"a question about an update":  {"j", "j", "u"},
		"help":                        {"?"},
	},
	"Add": {
		"the start":                nil,
		"a query being typed":      {"/", "s", "o", "d", "i", "u", "m"},
		"the results":              {"/", "s", "o", "d", "enter"},
		"the results, left":        {"/", "s", "o", "d", "enter", "esc"},
		"the question about a mod": {"/", "s", "enter", "enter"},
		"another kind of project":  {"/", "tab"},
		"a release type":           {"/", "ctrl+r"},
		"help":                     {"?"},
	},
	"Updates": {
		"nothing looked for yet":     nil,
		"what was found":             {"c"},
		"what is picked":             {"c", "space", "j", "space"},
		"the question about updates": {"c", "enter"},
		"the end":                    {"c", "G"},
		"help":                       {"?"},
	},
	"Check": {
		"what was found":      nil,
		"the end":             {"G"},
		"a question about it": {"f"},
		"after fixing":        {"f", "y"},
		"help":                {"?"},
	},
	"Deps": {
		"what the mods need": nil,
		"the end":            {"G"},
		"help":               {"?"},
	},
	"Config": states,
	"Export": {
		"before exporting":      nil,
		"a file being chosen":   {"o", "a", "b"},
		"the server pack":       {"s"},
		"a question":            {"d", "enter"},
		"what was exported":     {"enter"},
		"the end of the report": {"enter", "G"},
		"help":                  {"?"},
	},
	"Release": {
		"the release":           nil,
		"the end":               {"G"},
		"a question":            {"r"},
		"a question to commit":  {"C"},
		"a version being typed": {"v", "1", "."},
		"help":                  {"?"},
	},
}

func TestEveryScreenDrawnFitsTheTerminal(t *testing.T) {
	sizes := [][2]int{{40, 10}, {60, 12}, {100, 30}, {200, 50}}
	for _, mode := range []ui.Mode{ui.Never, ui.Always} {
		for _, size := range sizes {
			// Colour doesn't change how wide anything is, so it is only drawn again at the sizes that are most likely to go
			// wrong: the smallest, and an ordinary one
			if mode == ui.Always && size[0] != 40 && size[0] != 100 {
				continue
			}
			for title, states := range screenStates {
				for name, keys := range states {
					t.Run(fmt.Sprintf("%d-%dx%d/%s/%s", mode, size[0], size[1], title, name), func(t *testing.T) {
						a := newFullApp(t, size[0], size[1])
						cmdtest.SetColor(t, mode)
						goToScreen(t, a, title)
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
}

// Everything drawn says the same with colour on, which only adds to it (see package ui)
func TestColourOnlyAddsToEveryScreen(t *testing.T) {
	for title, states := range screenStates {
		for name, keys := range states {
			t.Run(title+"/"+name, func(t *testing.T) {
				a := newFullApp(t, 100, 30)
				goToScreen(t, a, title)
				appPress(t, a, keys...)

				cmdtest.SetColor(t, ui.Never)
				plain := a.render()
				if ui.Strip(plain) != plain {
					t.Errorf("there is colour with colour off: %q", plain)
				}
				cmdtest.SetColor(t, ui.Always)
				coloured := a.render()
				if stripped := ui.Strip(coloured); stripped != plain {
					t.Errorf("taking the colour out doesn't give what is drawn without it\nplain:\n%s\nstripped:\n%s", plain, stripped)
				}
			})
		}
	}
}

// The screens that are shown in a state say something, rather than being blank
func TestNoScreenIsBlank(t *testing.T) {
	a := newFullApp(t, 100, 30)
	for i, s := range a.screens {
		appPress(t, a, fmt.Sprint(i+1))
		if body := strings.TrimSpace(ui.Strip(s.view())); body == "" {
			t.Errorf("%s draws nothing", s.title())
		}
	}
}
