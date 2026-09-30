package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func newOverview(t *testing.T) *overviewScreen {
	t.Helper()
	setUpPack(t)
	s := newOverviewScreen(packBackend{})
	s.setSize(100, 24)
	feed(t, s, s.activate()())
	return s
}

func TestOverviewSaysWhatThePackIsAndWhatIsInIt(t *testing.T) {
	s := newOverview(t)
	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{
		"Test Pack 1.2.0", "Minecraft", "1.21.1", "Index", "index.toml, tracking", "Mods", "2 (2 main)",
		"Sides", "2 both", "Config files", "3 claimed", "3 unclaimed", "1 missing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the overview doesn't say %q:\n%s", want, out)
		}
	}
}

func TestOverviewSaysWhatThePackIsForAndWhoMadeIt(t *testing.T) {
	setUpPack(t)
	withLoader(t)
	pack, _ := core.LoadPack()
	pack.Author, pack.Description = "Someone", "A pack for trying things"
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	s := newOverviewScreen(packBackend{})
	s.setSize(100, 24)
	feed(t, s, s.activate()())

	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{"Test Pack 1.2.0  by Someone", "A pack for trying things", "NeoForge", "21.1.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("the overview doesn't say %q:\n%s", want, out)
		}
	}
	// Minecraft is what everything else is for, so it comes first
	if strings.Index(out, "Minecraft") > strings.Index(out, "NeoForge") {
		t.Errorf("the versions are in the wrong order:\n%s", out)
	}
}

func TestOverviewCountsWhatAreDependenciesAndWhatIsPinnedAndWhereTheyRun(t *testing.T) {
	setUpPack(t)
	writeModAt(t, "gamma", "Gamma Lib", "added-as-dependency = true\npin = true\nside = \"client\"")
	writeModAt(t, "delta", "Delta Mod", `side = "server"`)
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := newOverviewScreen(packBackend{})
	s.setSize(100, 24)
	feed(t, s, s.activate()())

	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{"4 (3 main, 1 dependency, 1 pinned)", "2 both, 1 client, 1 server"} {
		if !strings.Contains(out, want) {
			t.Errorf("the overview doesn't say %q:\n%s", want, out)
		}
	}
}

func TestOverviewSaysWhenThePackHasNoMods(t *testing.T) {
	setUpPack(t)
	mustRemove(t, "mods/alpha.pw.toml")
	mustRemove(t, "mods/beta.pw.toml")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := newOverviewScreen(packBackend{})
	s.setSize(100, 24)
	feed(t, s, s.activate()())
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "none yet") {
		t.Errorf("the overview doesn't say there are no mods:\n%s", out)
	}
}

func TestOverviewListsWhereToGoWhenItIsGivenTheScreens(t *testing.T) {
	a := newNavigableApp(t, 100, 40)
	out := a.render()
	for i, s := range a.screens {
		if i == 0 {
			continue
		}
		d, _ := s.(describer)
		if want := strings.TrimSpace(d.about()); !strings.Contains(out, want) {
			t.Errorf("the overview doesn't say what %s is for (%q):\n%s", s.title(), want, out)
		}
	}
}

func TestOverviewDrawsNothingMoreThanItsNameUntilThePackIsRead(t *testing.T) {
	setUpPack(t)
	s := newOverviewScreen(packBackend{})
	s.setSize(100, 24)
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "Reading the pack…") {
		t.Errorf("the overview doesn't say it is reading the pack:\n%s", out)
	}
}

func TestOverviewRefreshesTheIndex(t *testing.T) {
	s := newOverview(t)
	writeFile(t, "config/new.json", "{}")
	out := cmdtest.CaptureStdout(t, func() { press(t, s, "R") })

	if out != "" {
		t.Errorf("the overview wrote %q to the terminal", out)
	}
	if got := statusOf(s); got != "Index refreshed" {
		t.Errorf("the status line is %q, want it to say the index was refreshed", got)
	}
	if body := strings.Join(body(t, s), "\n"); !strings.Contains(body, "4 unclaimed") {
		t.Errorf("the overview doesn't count the file that was added:\n%s", body)
	}
	assertIndexIsConsistent(t)
}

func TestOverviewSaysWhatARefreshHadToSay(t *testing.T) {
	s := newOverview(t)
	s.backend = fakeOverview{notices: []string{"Notice: 2 files are no longer tracked"}, data: s.data}
	press(t, s, "R")
	if got := statusOf(s); got != "Index refreshed. Notice: 2 files are no longer tracked" {
		t.Errorf("the status line is %q, want what the refresh said", got)
	}
}

func TestOverviewSaysWhenThePackCannotBeRead(t *testing.T) {
	setUpPack(t)
	s := newOverviewScreen(fakeOverview{loadErr: errors.New("pack.toml is unreadable")})
	s.setSize(100, 24)
	feed(t, s, s.activate()())
	if got := statusOf(s); got != "pack.toml is unreadable" {
		t.Errorf("the status line is %q, want the reason", got)
	}
}

func TestOverviewSaysWhenARefreshFails(t *testing.T) {
	s := newOverview(t)
	s.backend = fakeOverview{refreshErr: errors.New("disk full"), data: s.data}
	press(t, s, "R")
	if got := statusOf(s); got != "disk full" {
		t.Errorf("the status line is %q, want the reason", got)
	}
}

// fakeOverview is an overviewBackend that says what a test tells it to
type fakeOverview struct {
	data       overviewData
	loadErr    error
	notices    []string
	refreshErr error
}

func (f fakeOverview) loadOverview() (overviewData, error) { return f.data, f.loadErr }

func (f fakeOverview) refresh() ([]string, error) { return f.notices, f.refreshErr }
