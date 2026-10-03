package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestModsListsTheModsByName(t *testing.T) {
	s := newMods(t)
	rows := modLines(t, s)
	if len(rows) != 2 || !strings.Contains(rows[0], "Alpha Mod") || !strings.Contains(rows[1], "Beta Mod") {
		t.Errorf("the list is %q, want Alpha Mod and then Beta Mod", rows)
	}
	if summary := lines(s.view())[0]; !strings.Contains(summary, "2 of 2 shown") {
		t.Errorf("the summary is %q, want it to say how many mods are shown", summary)
	}
	if !strings.HasPrefix(rows[0], "> ") {
		t.Errorf("the first row is %q, want the cursor on it", rows[0])
	}
}

func TestModsShowsWhereAModIsAndWhatItInstallsForTheModUnderTheCursor(t *testing.T) {
	s := newMods(t)
	if got := statusOf(s); got != "mods/alpha.pw.toml · alpha.jar" {
		t.Errorf("the status line is %q, want where the mod's file and its jar are", got)
	}
	press(t, s, "j")
	if got := statusOf(s); got != "mods/beta.pw.toml · beta.jar" {
		t.Errorf("after moving the status line is %q, want the other mod's", got)
	}
}

func TestModsPinsAndUnpinsAMod(t *testing.T) {
	s := newMods(t)
	press(t, s, "p")

	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if !mod.Pin {
		t.Error("the mod isn't pinned in its file after p")
	}
	if got := statusOf(s); got != "Pinned Alpha Mod" {
		t.Errorf("the status line is %q, want it to say what was done", got)
	}
	if row := modLines(t, s)[0]; !strings.Contains(row, "pinned") {
		t.Errorf("the row is %q, want it to say the mod is pinned", row)
	}
	assertIndexIsConsistent(t)

	press(t, s, "p")
	mod, _ = core.LoadMod("mods/alpha.pw.toml")
	if mod.Pin {
		t.Error("the mod is still pinned after p again")
	}
	if got := statusOf(s); got != "Unpinned Alpha Mod" {
		t.Errorf("the status line is %q, want it to say the mod was unpinned", got)
	}
	assertIndexIsConsistent(t)
}

func TestModsMarksAModAsADependencyAndAsAMainMod(t *testing.T) {
	s := newMods(t)
	press(t, s, "j", "d")

	mod, _ := core.LoadMod("mods/beta.pw.toml")
	if !mod.AddedAsDependency {
		t.Error("the mod isn't marked as a dependency in its file after d")
	}
	if got := statusOf(s); got != "Marked Beta Mod as a dependency" {
		t.Errorf("the status line is %q, want it to say what was done", got)
	}
	if row := modLines(t, s)[1]; !strings.Contains(row, "dep") {
		t.Errorf("the row is %q, want it to say the mod is a dependency", row)
	}
	assertIndexIsConsistent(t)

	press(t, s, "d")
	mod, _ = core.LoadMod("mods/beta.pw.toml")
	if mod.AddedAsDependency {
		t.Error("the mod is still marked as a dependency after d again")
	}
	if got := statusOf(s); got != "Marked Beta Mod as a main mod" {
		t.Errorf("the status line is %q, want it to say the mod is a main mod", got)
	}
}

func TestModsFiltersBySideAsListDoes(t *testing.T) {
	setUpPack(t)
	writeModAt(t, "client-only", "Client Only", `side = "client"`)
	writeModAt(t, "server-only", "Server Only", `side = "server"`)
	addMod(t, "both-sides", "Both Sides")
	s := modsOn(t, packBackend{})

	names := func() string {
		var out []string
		for _, row := range modLines(t, s) {
			out = append(out, strings.Fields(row[2:])[0]+" "+strings.Fields(row[2:])[1])
		}
		return strings.Join(out, ",")
	}
	if got := names(); got != "Alpha Mod,Beta Mod,Both Sides,Client Only,Server Only" {
		t.Errorf("all the mods are %q, want them all in order", got)
	}
	press(t, s, "s")
	if got := names(); got != "Alpha Mod,Beta Mod,Both Sides,Client Only" {
		t.Errorf("the mods on the client are %q, want those on it and those on both, as list --side client has", got)
	}
	press(t, s, "s")
	if got := names(); got != "Alpha Mod,Beta Mod,Both Sides,Server Only" {
		t.Errorf("the mods on the server are %q, want those on it and those on both, as list --side server has", got)
	}
	press(t, s, "s")
	if got := names(); got != "Alpha Mod,Beta Mod,Both Sides,Client Only,Server Only" {
		t.Errorf("with the filter off the mods are %q, want them all again", got)
	}
}

func TestModsFiltersByMainModsAndDependencies(t *testing.T) {
	setUpPack(t)
	writeModAt(t, "gamma", "Gamma Lib", `added-as-dependency = true`)
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := modsOn(t, packBackend{})

	press(t, s, "m")
	if rows := modLines(t, s); len(rows) != 2 || strings.Contains(strings.Join(rows, "\n"), "Gamma") {
		t.Errorf("the main mods are %q, want the two that weren't added as dependencies", rows)
	}
	press(t, s, "m")
	if rows := modLines(t, s); len(rows) != 1 || !strings.Contains(rows[0], "Gamma Lib") {
		t.Errorf("the dependencies are %q, want only Gamma Lib", rows)
	}
	press(t, s, "m")
	if rows := modLines(t, s); len(rows) != 3 {
		t.Errorf("with the filter off the mods are %q, want all three", rows)
	}
}

func TestModsSearchesFuzzilyAndPutsTheCursorOnTheBestMatch(t *testing.T) {
	setUpPack(t)
	addMod(t, "sodium", "Sodium")
	addMod(t, "sodium-extra", "Sodium Extra")
	s := modsOn(t, packBackend{})

	press(t, s, "/")
	if !s.modal() {
		t.Error("the screen isn't modal while a search is typed, so a q would quit")
	}
	typeText(t, s, "sdm")
	rows := modLines(t, s)
	if len(rows) != 2 {
		t.Fatalf("the rows are %q, want Sodium and Sodium Extra", rows)
	}
	if !strings.HasPrefix(rows[0], "> ") || !strings.Contains(rows[0], "Sodium") || strings.Contains(rows[0], "Extra") {
		t.Errorf("the first row is %q, want the cursor on Sodium, the shorter name, as it is the better match", rows[0])
	}
	if got := statusOf(s); got != "/ sdm█" {
		t.Errorf("the status line is %q, want the search being typed", got)
	}

	// Left on, the search leaves the keys to the screen again
	press(t, s, "enter")
	if s.modal() {
		t.Error("the screen is modal once the search is left on")
	}
	if got := statusOf(s); got != "/ sdm  (esc clears)" {
		t.Errorf("the status line is %q, want the search that is left on", got)
	}
	press(t, s, "esc")
	if rows := modLines(t, s); len(rows) != 4 {
		t.Errorf("after esc the rows are %q, want every mod", rows)
	}
}

func TestModsSearchFindsNothingAndSaysSo(t *testing.T) {
	s := newMods(t)
	press(t, s, "/")
	typeText(t, s, "zzz")
	if out := s.view(); !strings.Contains(out, `No mod matches "zzz".`) {
		t.Errorf("the screen doesn't say nothing matches:\n%s", out)
	}
}

func TestModsDetailsShowWhatThePackKnowsOfAMod(t *testing.T) {
	setUpPack(t)
	writeMod(t, "gamma", "Gamma Mod", `side = "client"
version = "3.1.4"`, `
[update]
[update.stub]
id = "x"

[[dependencies]]
id = "alpha-project"
type = "required"

[[dependencies]]
id = "unknown-project"
type = "optional"
`)
	useUpdater(t, &stubUpdater{})
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := modsOn(t, packBackend{})
	press(t, s, "j", "j", "enter")

	out := s.view()
	for _, want := range []string{"Gamma Mod", "slug", "gamma", "mods/gamma.pw.toml", "gamma.jar", "3.1.4", "client", "stub", "Depends on", "[required]", "[optional]", "unknown-project"} {
		if !strings.Contains(out, want) {
			t.Errorf("the details don't have %q:\n%s", want, out)
		}
	}
	if !s.modal() {
		t.Error("the screen isn't modal while the details are open")
	}
	press(t, s, "esc")
	if s.modal() {
		t.Error("esc didn't close the details")
	}
}

func TestModsWritesTheServersMarkdownList(t *testing.T) {
	s := newMods(t)
	press(t, s, "W")

	if got := statusOf(s); !strings.HasPrefix(got, "Wrote ") || !strings.HasSuffix(got, filepath.FromSlash(core.ServerModListFile)) {
		t.Errorf("the status line is %q, want it to say where the list was written", got)
	}
	text, err := readFileQuiet(core.ServerModListFile)
	if err != nil {
		t.Fatalf("the list wasn't written: %v", err)
	}
	for _, want := range []string{"Alpha Mod", "Beta Mod"} {
		if !strings.Contains(text, want) {
			t.Errorf("the list doesn't have %q, which is on both sides:\n%s", want, text)
		}
	}
	if _, err := readFileQuiet(core.ModListFile); err == nil {
		t.Errorf("%s was written as well", core.ModListFile)
	}
	// The server's files aren't in the index, so writing it leaves the index as it was
	assertIndexIsConsistent(t)
}

func TestModsWritesTheMarkdownList(t *testing.T) {
	s := newMods(t)
	press(t, s, "w")

	if got := statusOf(s); !strings.HasPrefix(got, "Wrote ") || !strings.HasSuffix(got, core.ModListFile) {
		t.Errorf("the status line is %q, want it to say where the list was written", got)
	}
	text, err := readFileQuiet(core.ModListFile)
	if err != nil {
		t.Fatalf("the list wasn't written: %v", err)
	}
	for _, want := range []string{"Alpha Mod", "Beta Mod"} {
		if !strings.Contains(text, want) {
			t.Errorf("the list doesn't have %q:\n%s", want, text)
		}
	}
	// The list isn't part of the pack, so writing it leaves the index as it was
	assertIndexIsConsistent(t)
}

func TestModsRefreshesTheIndex(t *testing.T) {
	s := newMods(t)
	writeMod(t, "gamma", "Gamma Mod", "", "")
	press(t, s, "R")

	if got := statusOf(s); got != "Index refreshed" {
		t.Errorf("the status line is %q, want it to say the index was refreshed", got)
	}
	if rows := modLines(t, s); len(rows) != 3 {
		t.Errorf("the rows are %q, want the mod that was added to show up", rows)
	}
}

func TestModsUpdatesAModOnceYouSayYes(t *testing.T) {
	setUpPack(t)
	writeMod(t, "gamma", "Gamma Mod", "", stubTable)
	u := useUpdater(t, &stubUpdater{checks: map[string]core.UpdateCheck{
		"Gamma Mod": {UpdateAvailable: true, UpdateString: "gamma.jar -> gamma-2.jar", CachedState: "gamma-2.jar"},
	}})
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := modsOn(t, packBackend{})
	press(t, s, "j", "j", "u")

	out := s.view()
	for _, want := range []string{"Update Gamma Mod?", "gamma.jar -> gamma-2.jar"} {
		if !strings.Contains(out, want) {
			t.Errorf("the question doesn't have %q:\n%s", want, out)
		}
	}
	if len(u.updated) != 0 {
		t.Fatalf("the mod was updated before it was asked: %v", u.updated)
	}

	press(t, s, "y")
	mod, _ := core.LoadMod("mods/gamma.pw.toml")
	if mod.FileName != "gamma-2.jar" || mod.Version != "2.0" {
		t.Errorf("the mod is %q at %q, want it updated", mod.FileName, mod.Version)
	}
	if got := statusOf(s); got != "Updated Gamma Mod" {
		t.Errorf("the status line is %q, want it to say what was updated", got)
	}
	assertIndexIsConsistent(t)
}

func TestModsLeavesAModAloneWhenYouSayNo(t *testing.T) {
	setUpPack(t)
	writeMod(t, "gamma", "Gamma Mod", "", stubTable)
	u := useUpdater(t, &stubUpdater{checks: map[string]core.UpdateCheck{
		"Gamma Mod": {UpdateAvailable: true, UpdateString: "a -> b", CachedState: "gamma-2.jar"},
	}})
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := modsOn(t, packBackend{})
	press(t, s, "j", "j", "u", "n")

	if len(u.updated) != 0 {
		t.Errorf("the mod was updated though the answer was no: %v", u.updated)
	}
	if s.modal() {
		t.Error("the question is still open after n")
	}
}

func TestModsSaysWhyAModIsNotUpdated(t *testing.T) {
	for name, tc := range map[string]struct {
		check   core.UpdateCheck
		updater error
		pinned  bool
		want    string
	}{
		"it is up to date":  {check: core.UpdateCheck{}, want: "Gamma Mod is up to date"},
		"checking it fails": {check: core.UpdateCheck{Error: errors.New("boom")}, want: "failed to check for updates to Gamma Mod: boom"},
		"its source fails":  {updater: errors.New("no network"), want: "failed to check for updates to Gamma Mod: no network"},
		"it is pinned":      {check: core.UpdateCheck{UpdateAvailable: true, UpdateString: "a -> b"}, pinned: true, want: "Gamma Mod has an update, but it is pinned: press p to unpin it"},
	} {
		t.Run(name, func(t *testing.T) {
			setUpPack(t)
			top := ""
			if tc.pinned {
				top = "pin = true"
			}
			writeMod(t, "gamma", "Gamma Mod", top, stubTable)
			useUpdater(t, &stubUpdater{err: tc.updater, checks: map[string]core.UpdateCheck{"Gamma Mod": tc.check}})
			if _, err := (packBackend{}).refresh(); err != nil {
				t.Fatalf("refresh() returned error: %v", err)
			}
			s := modsOn(t, packBackend{})
			press(t, s, "j", "j", "u")

			if got := statusOf(s); got != tc.want {
				t.Errorf("the status line is %q, want %q", got, tc.want)
			}
			if s.modal() {
				t.Error("a question is open, though there is nothing to update")
			}
		})
	}
}

func TestModsSaysWhenAnUpdateFails(t *testing.T) {
	setUpPack(t)
	writeMod(t, "gamma", "Gamma Mod", "", stubTable)
	useUpdater(t, &stubUpdater{doErr: errors.New("disk full"), checks: map[string]core.UpdateCheck{
		"Gamma Mod": {UpdateAvailable: true, UpdateString: "a -> b", CachedState: "b.jar"},
	}})
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := modsOn(t, packBackend{})
	press(t, s, "j", "j", "u", "y")

	if got := statusOf(s); got != "nothing was updated: disk full" {
		t.Errorf("the status line is %q, want it to say why nothing was updated", got)
	}
	mod, _ := core.LoadMod("mods/gamma.pw.toml")
	if mod.FileName != "gamma.jar" {
		t.Errorf("the mod's file is %q after the update failed, want it as it was", mod.FileName)
	}
}

func TestModsDoesNotUpdateAModThatWasPinnedSinceItWasChecked(t *testing.T) {
	setUpPack(t)
	writeMod(t, "gamma", "Gamma Mod", "", stubTable)
	u := useUpdater(t, &stubUpdater{checks: map[string]core.UpdateCheck{
		"Gamma Mod": {UpdateAvailable: true, UpdateString: "a -> b", CachedState: "b.jar"},
	}})
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := modsOn(t, packBackend{})
	press(t, s, "j", "j", "u")
	// While the question is open, something else pins the mod
	if err := (packBackend{}).setPinned("mods/gamma.pw.toml", true); err != nil {
		t.Fatalf("setPinned() returned error: %v", err)
	}
	press(t, s, "y")

	if len(u.updated) != 0 {
		t.Errorf("a pinned mod was updated: %v", u.updated)
	}
	if got := statusOf(s); got != "nothing was updated: Gamma Mod is pinned" {
		t.Errorf("the status line is %q, want it to say the mod is pinned", got)
	}
}

func TestModsWritesNothingToTheTerminal(t *testing.T) {
	setUpPack(t)
	writeMod(t, "gamma", "Gamma Mod", "", stubTable)
	useUpdater(t, &stubUpdater{checks: map[string]core.UpdateCheck{
		"Gamma Mod": {UpdateAvailable: true, UpdateString: "a -> b", CachedState: "b.jar"},
	}})
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	out := cmdtest.CaptureStdout(t, func() {
		s := modsOn(t, packBackend{})
		press(t, s, "j", "j", "p", "d", "w", "R", "u", "y")
	})
	if out != "" {
		t.Errorf("the mods screen wrote %q to the terminal, which would be drawn over the screen", out)
	}
	_ = os.Remove(core.ModListFile)
}

func TestModsKeepsTheCursorOnItsModWhenThePackChanges(t *testing.T) {
	s := newMods(t)
	press(t, s, "j")
	addMod(t, "aardvark", "Aardvark Mod")
	feed(t, s, s.activate()())

	if rows := modLines(t, s); !strings.HasPrefix(rows[2], "> ") || !strings.Contains(rows[2], "Beta Mod") {
		t.Errorf("the rows are %q, want the cursor still on Beta Mod, now the third", rows)
	}
}

func TestModsColourOnlyAddsToWhatIsDrawn(t *testing.T) {
	setUpPack(t)
	writeModAt(t, "gamma", "Gamma Mod", "pin = true\nside = \"client\"\nadded-as-dependency = true")
	addMod(t, "delta", "Delta Mod")
	s := modsOn(t, packBackend{})
	press(t, s, "/", "m")

	cmdtest.SetColor(t, ui.Never)
	plain := s.view()
	cmdtest.SetColor(t, ui.Always)
	coloured := s.view()
	if coloured == plain {
		t.Error("there is no colour with colour on")
	}
	if stripped := ui.Strip(coloured); stripped != plain {
		t.Errorf("taking the colour out doesn't give what is drawn without it\nplain:\n%s\nstripped:\n%s", plain, stripped)
	}
}

// fakeMods is a modsBackend that fails as a test tells it to, and otherwise is the pack
type fakeMods struct {
	packBackend
	loadErr, pinErr, depErr, saveErr, refreshErr error
}

func (f fakeMods) loadMods() (modsData, error) {
	if f.loadErr != nil {
		return modsData{}, f.loadErr
	}
	return f.packBackend.loadMods()
}

func (f fakeMods) setPinned(path string, pinned bool) error {
	if f.pinErr != nil {
		return f.pinErr
	}
	return f.packBackend.setPinned(path, pinned)
}

func (f fakeMods) setDependency(path string, dependency bool) error {
	if f.depErr != nil {
		return f.depErr
	}
	return f.packBackend.setDependency(path, dependency)
}

func (f fakeMods) saveList(server bool) (string, error) {
	if f.saveErr != nil {
		return "", f.saveErr
	}
	return f.packBackend.saveList(server)
}

func (f fakeMods) refresh() ([]string, error) {
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	return f.packBackend.refresh()
}

func TestModsSaysWhenThePackCannotBeRead(t *testing.T) {
	setUpPack(t)
	s := modsOn(t, fakeMods{loadErr: errors.New("pack.toml is unreadable")})
	if got := statusOf(s); got != "pack.toml is unreadable" {
		t.Errorf("the status line is %q, want the reason", got)
	}
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "Reading the pack…") {
		t.Errorf("the screen says %q, want it to go on saying it is reading the pack, as it hasn't", out)
	}
}

func TestModsSaysWhenAChangeFails(t *testing.T) {
	for name, tc := range map[string]struct {
		backend fakeMods
		keys    []string
		want    string
	}{
		"pinning":                 {fakeMods{pinErr: errors.New("read only")}, []string{"p"}, "read only"},
		"marking as a dependency": {fakeMods{depErr: errors.New("disk full")}, []string{"d"}, "disk full"},
		"writing the list":        {fakeMods{saveErr: errors.New("no room")}, []string{"w"}, "no room"},
		"refreshing the index":    {fakeMods{refreshErr: errors.New("can't read")}, []string{"R"}, "can't read"},
	} {
		t.Run(name, func(t *testing.T) {
			setUpPack(t)
			s := modsOn(t, tc.backend)
			press(t, s, tc.keys...)
			if got := statusOf(s); got != tc.want {
				t.Errorf("the status line is %q, want %q", got, tc.want)
			}
		})
	}
}

func TestModsIgnoresKeysThatChangeThePackWhileAJobIsRunning(t *testing.T) {
	s := newMods(t)
	first := s.start("Saving…", func(func(string)) tea.Msg { return nil })
	for _, k := range []string{"p", "d", "w", "R", "u"} {
		if _, next := s.updateKey(keyMsg(t, k)); next != nil {
			t.Errorf("%s started a job while one was running", k)
		}
	}
	feed(t, s, first())
	if mod, _ := core.LoadMod("mods/alpha.pw.toml"); mod.Pin {
		t.Error("a mod was pinned while a job was running")
	}
}

func TestModsWaitsForAModToBeThereBeforeActingOnIt(t *testing.T) {
	setUpPack(t)
	mustRemove(t, "mods/alpha.pw.toml")
	mustRemove(t, "mods/beta.pw.toml")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := modsOn(t, packBackend{})
	press(t, s, "p", "d", "u", "enter", "j", "k", "G", "g")
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "The pack has no mods yet.") {
		t.Errorf("the screen doesn't say the pack has no mods:\n%s", out)
	}
}
