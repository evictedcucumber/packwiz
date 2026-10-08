package tui

import (
	"archive/zip"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/modrinth"
	"github.com/jarcoal/httpmock"
	"github.com/spf13/viper"
)

// fakeExport is an exportBackend that says what a test tells it to, and records how it was asked to export.
type fakeExport struct {
	name    string
	nameErr error
	result  *modrinth.ExportResult
	err     error
	// instructions are what a Bisect export says to do with the pack
	instructions string
	// exported are the options it was asked to export with
	exported []modrinth.ExportOptions
}

func (f *fakeExport) defaultExportName() (exportNames, error) {
	base := strings.TrimSuffix(f.name, ".mrpack")
	return exportNames{f.name, base + "-server.zip", base + "-bisect.zip"}, f.nameErr
}

func (f *fakeExport) exportPack(options modrinth.ExportOptions, progress func(done, total int)) (*modrinth.ExportResult, error) {
	f.exported = append(f.exported, options)
	progress(1, 1)
	if options.Bisect && f.result != nil {
		// A Bisect export is a server pack that says what to do with it
		r := *f.result
		r.Server, r.Bisect, r.Instructions = true, true, f.instructions
		return &r, f.err
	}
	return f.result, f.err
}

func newFakeExport() *fakeExport {
	return &fakeExport{
		name:         "Test Pack-1.2.0.mrpack",
		instructions: "Upload Test Pack-1.2.0-bisect.zip to your Bisect Hosting server and set its start command to java -jar server.jar",
		result: &modrinth.ExportResult{
			Path: "Test Pack-1.2.0.mrpack",
			Files: []modrinth.ExportFile{
				{Name: "Alpha Mod", Path: "mods/alpha.jar", Client: "required", Server: "required", Size: 2048},
			},
			Promotions: []modrinth.ExportPromotion{{Mod: "Lib", NeededBy: "Alpha Mod"}},
			Notices:    []string{"Download of Beta Mod (beta.jar) failed: no network"},
		},
	}
}

func exportOn(t *testing.T, backend exportBackend) *exportScreen {
	t.Helper()
	setUpPack(t)
	s := newExportScreen(backend)
	s.setSize(100, 24)
	feed(t, s, s.activate()())
	return s
}

func TestExportShowsWhereThePackGoesAndHowUntilItIsExported(t *testing.T) {
	s := exportOn(t, newFakeExport())
	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{"File", "Test Pack-1.2.0.mrpack", "[x] only files on the domains Modrinth allows", "Press enter to export the pack."} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, out)
		}
	}
	press(t, s, "d")
	if out := s.view(); !strings.Contains(out, "[ ] every file is left for the launcher to download") {
		t.Errorf("d didn't allow every domain:\n%s", out)
	}
}

func TestExportExportsThePackAndShowsWhatWentIntoIt(t *testing.T) {
	f := newFakeExport()
	s := exportOn(t, f)
	press(t, s, "enter")

	if len(f.exported) != 1 || f.exported[0] != (modrinth.ExportOptions{Output: "", RestrictDomains: true}) {
		t.Fatalf("the pack was exported with %+v, want once, with the defaults", f.exported)
	}
	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{
		"Exported to Test Pack-1.2.0.mrpack", "Exported files:", "Alpha Mod", "mods/alpha.jar", "2.0 KiB",
		"Lib is only exported for the server, but Alpha Mod needs it on the client", "Download of Beta Mod (beta.jar) failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, out)
		}
	}
	if got := statusOf(s); got != "Exported to Test Pack-1.2.0.mrpack" {
		t.Errorf("the status line is %q, want it to say where the pack was exported to", got)
	}
}

func TestExportPassesWhatWasChosen(t *testing.T) {
	f := newFakeExport()
	s := exportOn(t, f)
	press(t, s, "d", "o")
	if !s.modal() {
		t.Fatal("the screen isn't modal while it asks for the file")
	}
	typeText(t, s, "my.mrpack")
	press(t, s, "enter")
	if out := s.view(); !strings.Contains(out, "my.mrpack") {
		t.Errorf("the screen doesn't show the file that was chosen:\n%s", out)
	}
	press(t, s, "enter")

	if len(f.exported) != 1 || f.exported[0] != (modrinth.ExportOptions{Output: "my.mrpack", RestrictDomains: false}) {
		t.Errorf("the pack was exported with %+v, want the file and the domains that were chosen", f.exported)
	}
}

func TestExportExportsTheServerPackWhenItIsChosen(t *testing.T) {
	f := newFakeExport()
	s := exportOn(t, f)
	press(t, s, "s")
	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{"Test Pack-1.2.0-server.zip", "(•) server pack", "every file is in the server pack"} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, out)
		}
	}
	// The domains are nothing to a server pack, so they are left as they were
	press(t, s, "d", "enter")

	if len(f.exported) != 1 || f.exported[0] != (modrinth.ExportOptions{RestrictDomains: true, Server: true}) {
		t.Fatalf("the pack was exported with %+v, want the server pack", f.exported)
	}

	press(t, s, "s")
	if out := s.view(); !strings.Contains(out, "Test Pack-1.2.0.mrpack") || !strings.Contains(out, "(•) .mrpack") {
		t.Errorf("s didn't go back to the .mrpack:\n%s", out)
	}
}

func TestExportExportsTheBisectPackWhenItIsChosen(t *testing.T) {
	f := newFakeExport()
	s := exportOn(t, f)
	press(t, s, "b")
	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{"Test Pack-1.2.0-bisect.zip", "(•) Bisect Hosting", "every file is in the server pack"} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, out)
		}
	}
	press(t, s, "enter")

	if len(f.exported) != 1 || f.exported[0] != (modrinth.ExportOptions{RestrictDomains: true, Bisect: true}) {
		t.Fatalf("the pack was exported with %+v, want the Bisect pack and not the server pack", f.exported)
	}
	out = strings.Join(body(t, s), "\n")
	if !strings.Contains(out, "Upload Test Pack-1.2.0-bisect.zip to your Bisect Hosting server") {
		t.Errorf("the screen doesn't give the instructions:\n%s", out)
	}
}

func TestExportDefaultFileFollowsTheMode(t *testing.T) {
	s := exportOn(t, newFakeExport())
	for _, step := range []struct{ key, want string }{
		{"s", "Test Pack-1.2.0-server.zip"},
		{"b", "Test Pack-1.2.0-bisect.zip"},
		{"s", "Test Pack-1.2.0-server.zip"},
		{"s", "Test Pack-1.2.0.mrpack"},
		{"b", "Test Pack-1.2.0-bisect.zip"},
		{"b", "Test Pack-1.2.0.mrpack"},
	} {
		press(t, s, step.key)
		if got := s.file(); got != step.want {
			t.Fatalf("after %s the file is %q, want %q", step.key, got, step.want)
		}
	}
}

func TestExportDoesNotShowInstructionsForOtherPacks(t *testing.T) {
	s := exportOn(t, newFakeExport())
	press(t, s, "s", "enter")
	if out := strings.Join(body(t, s), "\n"); strings.Contains(out, "Bisect Hosting server") {
		t.Errorf("the screen gives Bisect instructions for the server pack:\n%s", out)
	}
}

func TestExportLeavesTheFileAloneWhenTheBoxIsCancelled(t *testing.T) {
	s := exportOn(t, newFakeExport())
	press(t, s, "o")
	typeText(t, s, "other.mrpack")
	press(t, s, "esc")
	if out := s.view(); strings.Contains(out, "other.mrpack") {
		t.Errorf("the screen shows a file that was cancelled:\n%s", out)
	}
	if s.modal() {
		t.Error("the box is still open after esc")
	}
}

func TestExportAsksBeforeOverwritingAFileThatIsThere(t *testing.T) {
	f := newFakeExport()
	s := exportOn(t, f)
	writeFile(t, "Test Pack-1.2.0.mrpack", "old")

	press(t, s, "enter")
	if out := s.view(); !strings.Contains(out, "Overwrite Test Pack-1.2.0.mrpack?") {
		t.Errorf("the screen doesn't ask about overwriting:\n%s", out)
	}
	if len(f.exported) != 0 {
		t.Fatalf("the pack was exported before it was answered: %+v", f.exported)
	}
	press(t, s, "n")
	if len(f.exported) != 0 {
		t.Errorf("the pack was exported though the answer was no: %+v", f.exported)
	}
	press(t, s, "enter", "y")
	if len(f.exported) != 1 {
		t.Errorf("the pack was exported %d times, want once after yes", len(f.exported))
	}
}

func TestExportSaysWhenExportingFails(t *testing.T) {
	f := newFakeExport()
	f.err = errors.New("Error creating manifest: no minecraft version specified in modpack")
	s := exportOn(t, f)
	press(t, s, "enter")
	if got := statusOf(s); got != "Error creating manifest: no minecraft version specified in modpack" {
		t.Errorf("the status line is %q, want the reason", got)
	}
}

func TestExportSaysWhenTheNameOfTheFileCannotBeRead(t *testing.T) {
	f := newFakeExport()
	f.name, f.nameErr = "", errors.New("pack.toml is missing")
	s := exportOn(t, f)
	if got := statusOf(s); got != "pack.toml is missing" {
		t.Errorf("the status line is %q, want the reason", got)
	}
	press(t, s, "enter")
	if len(f.exported) != 0 {
		t.Errorf("the pack was exported without knowing where to: %+v", f.exported)
	}
}

func TestExportDoesNotExportTwiceAtOnce(t *testing.T) {
	f := newFakeExport()
	s := exportOn(t, f)
	first := s.start("Exporting…", func(func(string)) tea.Msg { return nil })
	press(t, s, "enter")
	if len(f.exported) != 0 {
		t.Errorf("a second export started while one was running: %+v", f.exported)
	}
	if !s.working() {
		t.Error("the screen isn't working, so q would quit in the middle of an export")
	}
	feed(t, s, first())
}

// exportFixture makes the pack that the tests work on have a mod that can be downloaded from a Modrinth that serves it.
func exportFixture(t *testing.T) {
	t.Helper()
	setUpPack(t)
	withLoader(t)
	httpmock.Activate(t)
	old := viper.GetString("cache.directory")
	viper.Set("cache.directory", t.TempDir())
	t.Cleanup(func() { viper.Set("cache.directory", old) })

	content := "bytes of gamma"
	sum := sha512.Sum512([]byte(content))
	url := "https://cdn.modrinth.com/data/gamma/versions/v1/gamma.jar"
	httpmock.RegisterResponder("GET", url, httpmock.NewStringResponder(200, content))
	writeFile(t, "mods/gamma.pw.toml", "name = \"Gamma Mod\"\nfilename = \"gamma.jar\"\nside = \"both\"\nconfig-files = []\n\n[download]\nurl = \""+url+"\"\nhash-format = \"sha512\"\nhash = \""+hex.EncodeToString(sum[:])+"\"\n")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
}

func TestExportWritesAPackThatHasTheMods(t *testing.T) {
	exportFixture(t)
	s := newExportScreen(packBackend{})
	s.setSize(100, 24)
	feed(t, s, s.activate()())

	out := cmdtest.CaptureStdout(t, func() { press(t, s, "enter") })
	if out != "" {
		t.Errorf("the export screen wrote %q to the terminal, which would be drawn over the screen", out)
	}
	if got := statusOf(s); got != "Exported to Test Pack-1.2.0.mrpack" {
		t.Fatalf("the status line is %q, want the pack exported", got)
	}
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "Gamma Mod") || !strings.Contains(out, "mods/gamma.jar") {
		t.Errorf("the screen doesn't list the mod:\n%s", out)
	}
	r, err := zip.OpenReader("Test Pack-1.2.0.mrpack")
	if err != nil {
		t.Fatalf("the pack wasn't written: %v", err)
	}
	defer func() { _ = r.Close() }()
	found := false
	for _, f := range r.File {
		found = found || f.Name == "modrinth.index.json"
	}
	if !found {
		t.Error("the pack has no manifest")
	}
	// Exporting refreshes the index, which it writes: it is as a refresh leaves it
	assertIndexIsConsistent(t)
}
