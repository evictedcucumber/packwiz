package tui

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
)

// modrinthTable is what a mod's metadata needs to come from Modrinth, at a version, for writeMod's extra; deps follow it.
func modrinthTable(project, version string) string {
	return "\n[update]\n[update.modrinth]\nmod-id = \"" + project + "\"\nversion = \"" + version + "\"\n"
}

// dependencyTable is a dependency, for writeMod's extra after modrinthTable.
func dependencyTable(id, kind string) string {
	return "\n[[dependencies]]\nid = \"" + id + "\"\ntype = \"" + kind + "\"\n"
}

// newDeps makes a dependencies screen on a pack that has Alpha Mod, which is Modrinth's alpha-id, and Gamma Mod, which
// requires a library that the pack doesn't have and has Alpha Mod as an optional one. Modrinth knows the library.
func newDeps(t *testing.T) *depsScreen {
	t.Helper()
	setUpPack(t)
	writeMod(t, "alpha", "Alpha Mod", `config-files = ["config/alpha.json"]`, modrinthTable("alpha-id", "av1"))
	writeMod(t, "gamma", "Gamma Mod", "", modrinthTable("gamma-id", "gv1")+dependencyTable("lib-id", "required")+dependencyTable("alpha-id", "optional"))
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewJsonResponderOrPanic(200, []map[string]string{{"id": "lib-id", "title": "Library"}}))
	s := newDepsScreen(packBackend{})
	s.setSize(100, 24)
	return s
}

func TestDepsReportsWhatTheModsNeedTheFirstTimeItIsShown(t *testing.T) {
	s := newDeps(t)
	// Alpha Mod records no dependencies, so Modrinth is asked for its version's
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/av1`,
		httpmock.NewStringResponder(200, `{"id":"av1","dependencies":[]}`))
	feed(t, s, s.activate()())

	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{"Gamma Mod:", "[required] Library (missing)", "[optional] Alpha Mod (in the pack)"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report doesn't say %q:\n%s", want, out)
		}
	}
	if summary := lines(s.view())[0]; !strings.Contains(summary, "2 mods from Modrinth") || !strings.Contains(summary, "1 required missing") {
		t.Errorf("the summary is %q, want it to say how many mods and how many dependencies are missing", summary)
	}
}

func TestDepsDoesNotReportAgainWhenSwitchedBackTo(t *testing.T) {
	s := newDeps(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/av1`,
		httpmock.NewStringResponder(200, `{"id":"av1","dependencies":[]}`))
	feed(t, s, s.activate()())
	if cmd := s.activate(); cmd != nil {
		t.Error("the dependencies were read again when the screen was shown a second time, which asks the network")
	}
}

func TestDepsSavesWhatItLookedUpWhenAsked(t *testing.T) {
	s := newDeps(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/av1`,
		httpmock.NewStringResponder(200, `{"id":"av1","dependencies":[{"project_id":"gamma-id","dependency_type":"optional"}]}`))
	feed(t, s, s.activate()())

	if got := statusOf(s); got != "Looked up the dependencies of 1 mod on Modrinth: press s to save them to the pack" {
		t.Errorf("the status line is %q, want it to say what was looked up and how to save it", got)
	}
	if alpha, _ := core.LoadMod("mods/alpha.pw.toml"); len(alpha.Dependencies) != 0 {
		t.Fatalf("Alpha Mod has dependencies %v before they were saved, want it left alone", alpha.Dependencies)
	}

	press(t, s, "s")
	alpha, _ := core.LoadMod("mods/alpha.pw.toml")
	if len(alpha.Dependencies) != 1 || alpha.Dependencies[0].ID != "gamma-id" {
		t.Errorf("Alpha Mod's dependencies are %v, want the one that was looked up", alpha.Dependencies)
	}
	if got := statusOf(s); got != "Saved the dependencies to the pack" {
		t.Errorf("the status line is %q, want it to say they were saved", got)
	}
	if summary := lines(s.view())[0]; strings.Contains(summary, "unsaved") {
		t.Errorf("the summary %q still says something is unsaved", summary)
	}
	assertIndexIsConsistent(t)

	press(t, s, "s")
	if got := statusOf(s); got != "Nothing was looked up that isn't saved" {
		t.Errorf("after saving again the status line is %q, want it to say there is nothing to save", got)
	}
}

func TestDepsSaysWhichModsCouldNotBeLookedUp(t *testing.T) {
	s := newDeps(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/av1`, httpmock.NewErrorResponder(errors.New("no network")))
	feed(t, s, s.activate()())

	if got := statusOf(s); got != "Couldn't look up the dependencies of Alpha Mod" {
		t.Errorf("the status line is %q, want it to say which mod couldn't be looked up", got)
	}
}

func TestDepsSaysWhenNoModComesFromModrinth(t *testing.T) {
	setUpPack(t)
	httpmock.Activate(t)
	s := newDepsScreen(packBackend{})
	s.setSize(100, 24)
	feed(t, s, s.activate()())

	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "None of the pack's mods come from Modrinth") {
		t.Errorf("the screen doesn't say no mod comes from Modrinth:\n%s", out)
	}
}

func TestDepsLooksEverythingUpAgainWhenAsked(t *testing.T) {
	s := newDeps(t)
	asked := 0
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/`, func(*http.Request) (*http.Response, error) {
		asked++
		return httpmock.NewStringResponse(200, `{"id":"v","dependencies":[]}`), nil
	})
	feed(t, s, s.activate()())
	first := asked
	press(t, s, "r")
	// Every mod is looked up, not only the one that recorded nothing
	if asked-first != 2 {
		t.Errorf("Modrinth was asked about %d versions after r, want both mods' versions", asked-first)
	}
}

func TestDepsWritesNothingToTheTerminal(t *testing.T) {
	s := newDeps(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/av1`,
		httpmock.NewStringResponder(200, `{"id":"av1","dependencies":[]}`))
	out := cmdtest.CaptureStdout(t, func() {
		feed(t, s, s.activate()())
		press(t, s, "s", "r", "c")
	})
	if out != "" {
		t.Errorf("the dependencies screen wrote %q to the terminal, which would be drawn over the screen", out)
	}
}
