package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// writeFile writes a file of the pack under test, making its folder if it needs one.
func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("failed to create the folder of %s: %v", name, err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

// writeMod writes the metadata file of a mod, and its jar, in the mods folder. configFiles is the mod's config-files
// as it is written in the file, or "" to leave it out; extra is anything else it should have.
func writeMod(t *testing.T, slug, name, configFiles, extra string) {
	t.Helper()
	content := "name = \"" + name + "\"\nfilename = \"" + slug + ".jar\"\n" + configFiles + "\n" + `
[download]
hash-format = "sha256"
hash = "abc"
url = "https://example.invalid/` + slug + `.jar"
` + extra
	writeFile(t, "mods/"+slug+".pw.toml", content)
	writeFile(t, "mods/"+slug+".jar", "jar of "+slug)
}

// setUpPack makes the pack that the tests work on, in a directory of its own that is the working directory, and turns
// colour off so that what is drawn can be compared as text. It has two mods:
//
//	Alpha Mod claims config/alpha.json, the folder config/alpha/ and config/gone.json, which doesn't exist
//	Beta Mod  claims nothing (its metadata file has no config-files at all)
//
// and config files that nothing claims: config/orphan.json, and config/other/a.json and b.json.
func setUpPack(t *testing.T) {
	t.Helper()
	cmdtest.Chdir(t)
	cmdtest.SetColor(t, ui.Never)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", Version: "1.2.0", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.21.1"},
	})
	writeFile(t, "index.toml", "hash-format = \"sha256\"\n")

	writeMod(t, "alpha", "Alpha Mod", `config-files = ["config/alpha.json", "config/alpha/", "config/gone.json"]`, "")
	writeMod(t, "beta", "Beta Mod", "", "")
	for _, f := range []string{
		"config/alpha.json", "config/alpha/sub.json", "config/alpha/deep/x.json",
		"config/orphan.json", "config/other/a.json", "config/other/b.json",
	} {
		writeFile(t, f, "{}")
	}
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("failed to build the index of the fixture pack: %v", err)
	}
}

// addMod adds a mod to the pack that setUpPack made, and refreshes the index so that it is in the pack.
func addMod(t *testing.T, slug, name string) {
	t.Helper()
	writeMod(t, slug, name, "", "")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
}

// newConfig makes a config screen on the pack that setUpPack made, as big as a terminal is likely to be.
func newConfig(t *testing.T) *configScreen {
	t.Helper()
	setUpPack(t)
	return configOn(t, packBackend{})
}

func configOn(t *testing.T, backend configBackend) *configScreen {
	t.Helper()
	data, err := backend.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	s := newConfigScreen(backend, data)
	s.setSize(100, 24)
	return s
}

// keyMsg makes the message that pressing a key sends. A key is written as it is in a key binding: "j", "R", "space",
// "enter", "esc", "tab", "up", "ctrl+c" and so on.
func keyMsg(t *testing.T, name string) tea.KeyPressMsg {
	t.Helper()
	special := map[string]tea.KeyPressMsg{
		"up":        {Code: tea.KeyUp},
		"down":      {Code: tea.KeyDown},
		"left":      {Code: tea.KeyLeft},
		"right":     {Code: tea.KeyRight},
		"enter":     {Code: tea.KeyEnter},
		"esc":       {Code: tea.KeyEscape},
		"tab":       {Code: tea.KeyTab},
		"backspace": {Code: tea.KeyBackspace},
		"delete":    {Code: tea.KeyDelete},
		"home":      {Code: tea.KeyHome},
		"end":       {Code: tea.KeyEnd},
		"pgup":      {Code: tea.KeyPgUp},
		"pgdown":    {Code: tea.KeyPgDown},
		"space":     {Code: tea.KeySpace, Text: " "},
		"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
		"ctrl+u":    {Code: 'u', Mod: tea.ModCtrl},
		"ctrl+w":    {Code: 'w', Mod: tea.ModCtrl},
	}
	if msg, ok := special[name]; ok {
		return msg
	}
	if r := []rune(name); len(r) == 1 {
		return tea.KeyPressMsg{Code: r[0], Text: name}
	}
	t.Fatalf("don't know how to press %q", name)
	return tea.KeyPressMsg{}
}

// feed sends a message to a screen, and then what the commands it starts come back with, and theirs in turn, as Bubble
// Tea runs a program. A command that returns nothing ends it.
func feed(t *testing.T, s screen, msg tea.Msg) {
	t.Helper()
	for range 20 {
		var cmd tea.Cmd
		s, cmd = s.update(msg)
		if cmd == nil {
			return
		}
		if msg = cmd(); msg == nil {
			return
		}
	}
	t.Fatal("commands kept starting more commands")
}

// press presses keys on a screen, one after another, running whatever they start.
func press(t *testing.T, s screen, keys ...string) {
	t.Helper()
	for _, k := range keys {
		feed(t, s, keyMsg(t, k))
	}
}

// typeText types text, a key for each character.
func typeText(t *testing.T, s screen, text string) {
	t.Helper()
	for _, r := range text {
		if r == ' ' {
			press(t, s, "space")
		} else {
			press(t, s, string(r))
		}
	}
}

// lines are the lines of what a screen draws, without trailing spaces.
func lines(view string) []string {
	out := strings.Split(view, "\n")
	for i := range out {
		out[i] = strings.TrimRight(out[i], " ")
	}
	return out
}

// tree is the rows of the config screen's tree, without the gutter that has the cursor and marks in it: what is between
// the summary and the status lines, less the blank lines after it.
func tree(t *testing.T, s *configScreen) []string {
	t.Helper()
	all := lines(s.view())
	if len(all) != s.height {
		t.Fatalf("the screen is %d lines, want %d", len(all), s.height)
	}
	body := all[1 : len(all)-1]
	for len(body) > 0 && body[len(body)-1] == "" {
		body = body[:len(body)-1]
	}
	out := make([]string, len(body))
	for i, l := range body {
		if len(l) < 4 {
			out[i] = l
		} else {
			out[i] = l[4:]
		}
	}
	return out
}

// cursorRow is what the cursor is on, as drawn.
func cursorRow(t *testing.T, s *configScreen) string {
	t.Helper()
	for _, l := range lines(s.view()) {
		if strings.HasPrefix(l, "> ") {
			return l[4:]
		}
	}
	t.Fatalf("no row has the cursor:\n%s", s.view())
	return ""
}

// statusOf is the last line of a screen, which says what was done.
func statusOf(s *configScreen) string {
	all := lines(s.view())
	return all[len(all)-1]
}

// claims reads what a mod's config-files has in it, from its metadata file.
func claims(t *testing.T, slug string) []string {
	t.Helper()
	mod, err := core.LoadMod("mods/" + slug + ".pw.toml")
	if err != nil {
		t.Fatalf("failed to read the metadata file of %s: %v", slug, err)
	}
	if mod.ConfigFiles == nil {
		return nil
	}
	return *mod.ConfigFiles
}

// assertIndexIsConsistent checks that what was written to the pack leaves its index and pack file as a refresh would:
// each file's hash is what it is now and the pack records the index's, so nothing that was changed was left out.
func assertIndexIsConsistent(t *testing.T) {
	t.Helper()
	indexBefore, err := os.ReadFile("index.toml")
	if err != nil {
		t.Fatalf("failed to read index.toml: %v", err)
	}
	packBefore, err := os.ReadFile("pack.toml")
	if err != nil {
		t.Fatalf("failed to read pack.toml: %v", err)
	}
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	indexAfter, _ := os.ReadFile("index.toml")
	packAfter, _ := os.ReadFile("pack.toml")
	if !bytes.Equal(indexBefore, indexAfter) {
		t.Errorf("index.toml wasn't what a refresh writes\nbefore:\n%s\nafter:\n%s", indexBefore, indexAfter)
	}
	if !bytes.Equal(packBefore, packAfter) {
		t.Errorf("pack.toml wasn't what a refresh writes\nbefore:\n%s\nafter:\n%s", packBefore, packAfter)
	}
}

// fakeBackend is a configBackend that works on data it is given, and can be told to fail, for what a screen does with
// a backend that isn't the pack.
type fakeBackend struct {
	data       configData
	loadErr    error
	relateErr  error
	refreshErr error
	notices    []string
	// related and unrelated are what relate and unrelate were called with: the keys of the owners, then the entries
	related   [][]string
	unrelated [][]string
	refreshes int
}

func (f *fakeBackend) load() (configData, error) { return f.data, f.loadErr }

func (f *fakeBackend) relate(owners []owner, entries []string) (relateResult, error) {
	var call []string
	for _, o := range owners {
		call = append(call, o.key())
	}
	f.related = append(f.related, append(call, entries...))
	return relateResult{added: len(owners) * len(entries), owners: len(owners)}, f.relateErr
}

func (f *fakeBackend) unrelate(o owner, entries []string) error {
	f.unrelated = append(f.unrelated, append([]string{o.key()}, entries...))
	return nil
}

func (f *fakeBackend) refresh() ([]string, error) {
	f.refreshes++
	return f.notices, f.refreshErr
}

// fakeMod makes a mod for a fakeBackend: its metadata file is at mods/<slug>.pw.toml.
func fakeMod(name, slug string, configFiles ...string) *core.Mod {
	m := &core.Mod{Name: name}
	if configFiles != nil {
		m.ConfigFiles = &configFiles
	}
	m.SetMetaPath("mods/" + slug + ".pw.toml")
	return m
}

// fakeOwner makes a mod as an owner, as fakeMod makes it as a mod.
func fakeOwner(name, slug string, configFiles ...string) owner {
	return modOwner(fakeMod(name, slug, configFiles...))
}

// withLoader makes the pack that setUpPack made have the mod loader neoforge, which the pack and its loader are
// owners of config files of.
func withLoader(t *testing.T) {
	t.Helper()
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	pack.Versions["neoforge"] = "21.1.0"
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
}

// packToml reads the pack's config-files, by owner, from pack.toml.
func packToml(t *testing.T) map[string][]string {
	t.Helper()
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	return pack.ConfigFiles
}

// readFileQuiet reads a file as text, for a test that compares it.
func readFileQuiet(name string) (string, error) {
	data, err := os.ReadFile(name)
	return string(data), err
}

// mustLoad loads what the config screen shows, for a test that needs some of it.
func mustLoad(t *testing.T) configData {
	t.Helper()
	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	return data
}

// windowSize is the message that tells a program how big its terminal is.
func windowSize(width, height int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: width, Height: height}
}
