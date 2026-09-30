package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestBackendLoadReadsThePack(t *testing.T) {
	setUpPack(t)
	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	if data.pack != "Test Pack 1.2.0" {
		t.Errorf("pack = %q, want its name and version", data.pack)
	}
	var names []string
	for _, m := range data.mods {
		names = append(names, m.Name)
	}
	if !slices.Equal(names, []string{"Alpha Mod", "Beta Mod"}) {
		t.Errorf("mods = %v, want every mod, even the one that claims nothing, by name", names)
	}
	if len(data.tree.Mods) != 1 || data.tree.Mods[0].Mod.Name != "Alpha Mod" {
		t.Errorf("the tree has mods %v, want only the one that claims something", data.tree.Mods)
	}
	if !slices.Equal(data.tree.Unclaimed, []string{"config/orphan.json", "config/other/a.json", "config/other/b.json"}) {
		t.Errorf("unclaimed = %v", data.tree.Unclaimed)
	}
	if data.configDir != "" {
		t.Errorf("configDir = %q, want none", data.configDir)
	}
}

func TestBackendLoadSortsModsByNameWhateverTheCase(t *testing.T) {
	setUpPack(t)
	addMod(t, "zeta", "zeta lower")
	addMod(t, "eta", "Eta Upper")
	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	var names []string
	for _, m := range data.mods {
		names = append(names, m.Name)
	}
	if want := []string{"Alpha Mod", "Beta Mod", "Eta Upper", "zeta lower"}; !slices.Equal(names, want) {
		t.Errorf("mods = %v, want %v", names, want)
	}
}

func TestBackendLoadFailsOutsideAPack(t *testing.T) {
	cmdtest.Chdir(t)
	cmdtest.SetViper(t, "pack-file", "pack.toml")
	if _, err := (packBackend{}).load(); err == nil {
		t.Error("load() returned no error in a folder that isn't a pack")
	}
}

func TestBackendRelateRecordsEntriesAndSaysWhatChanged(t *testing.T) {
	setUpPack(t)
	got, err := packBackend{}.relate(
		[]string{"mods/alpha.pw.toml", "mods/beta.pw.toml"},
		[]string{"config/alpha.json", "config/orphan.json"}, // Alpha has the first already
	)
	if err != nil {
		t.Fatalf("relate() returned error: %v", err)
	}
	if want := (relateResult{added: 3, existing: 1, mods: 2}); got != want {
		t.Errorf("relate() = %+v, want %+v", got, want)
	}
	if got := claims(t, "alpha"); !slices.Equal(got, []string{"config/alpha.json", "config/alpha/", "config/gone.json", "config/orphan.json"}) {
		t.Errorf("Alpha Mod's config-files is %v", got)
	}
	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/alpha.json", "config/orphan.json"}) {
		t.Errorf("Beta Mod's config-files is %v", got)
	}
	assertIndexIsConsistent(t)
}

func TestBackendRelateWritesNothingWhenThereIsNothingNew(t *testing.T) {
	setUpPack(t)
	indexBefore, _ := os.ReadFile("index.toml")
	alphaBefore, _ := os.ReadFile("mods/alpha.pw.toml")
	got, err := packBackend{}.relate([]string{"mods/alpha.pw.toml"}, []string{"config/alpha.json"})
	if err != nil {
		t.Fatalf("relate() returned error: %v", err)
	}
	if want := (relateResult{existing: 1}); got != want {
		t.Errorf("relate() = %+v, want %+v", got, want)
	}
	if after, _ := os.ReadFile("mods/alpha.pw.toml"); !slices.Equal(alphaBefore, after) {
		t.Error("the mod's file was rewritten")
	}
	if after, _ := os.ReadFile("index.toml"); !slices.Equal(indexBefore, after) {
		t.Error("the index was rewritten")
	}
}

// A mod that can't be read stops the rest, but what was saved before it has to be in the index, or the index would hold
// the hashes of files that have changed
func TestBackendRelateKeepsTheIndexRightWhenALaterModFails(t *testing.T) {
	setUpPack(t)
	got, err := packBackend{}.relate([]string{"mods/beta.pw.toml", "mods/missing.pw.toml"}, []string{"config/orphan.json"})
	if err == nil {
		t.Fatal("relate() returned no error for a mod that doesn't exist")
	}
	if got.mods != 1 || got.added != 1 {
		t.Errorf("relate() = %+v, want what was saved before the failure counted", got)
	}
	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/orphan.json"}) {
		t.Errorf("Beta Mod's config-files is %v, want what was saved before the failure", got)
	}
	assertIndexIsConsistent(t)
}

func TestBackendRelateFailsForAModWithoutAMetadataFile(t *testing.T) {
	setUpPack(t)
	if _, err := (packBackend{}).relate([]string{"mods/nope.pw.toml"}, []string{"config/a.json"}); err == nil {
		t.Error("relate() returned no error for a mod that doesn't exist")
	}
}

func TestBackendUnrelateTakesOutEntries(t *testing.T) {
	setUpPack(t)
	if err := (packBackend{}).unrelate("mods/alpha.pw.toml", []string{"config/alpha/", "config/gone.json"}); err != nil {
		t.Fatalf("unrelate() returned error: %v", err)
	}
	if got := claims(t, "alpha"); !slices.Equal(got, []string{"config/alpha.json"}) {
		t.Errorf("Alpha Mod's config-files is %v", got)
	}
	assertIndexIsConsistent(t)
}

func TestBackendUnrelateOfWhatIsAlreadyGoneWritesNothing(t *testing.T) {
	setUpPack(t)
	indexBefore, _ := os.ReadFile("index.toml")
	alphaBefore, _ := os.ReadFile("mods/alpha.pw.toml")
	if err := (packBackend{}).unrelate("mods/alpha.pw.toml", []string{"config/never-there.json"}); err != nil {
		t.Fatalf("unrelate() returned error: %v", err)
	}
	if after, _ := os.ReadFile("mods/alpha.pw.toml"); !slices.Equal(alphaBefore, after) {
		t.Error("the mod's file was rewritten")
	}
	if after, _ := os.ReadFile("index.toml"); !slices.Equal(indexBefore, after) {
		t.Error("the index was rewritten")
	}
}

func TestBackendUnrelateLeavesAnEmptyConfigFilesThere(t *testing.T) {
	setUpPack(t)
	if err := (packBackend{}).unrelate("mods/alpha.pw.toml", []string{"config/alpha.json", "config/alpha/", "config/gone.json"}); err != nil {
		t.Fatalf("unrelate() returned error: %v", err)
	}
	// Left as an empty list rather than taken out, so the field stays there to fill in (see core.Mod.UnclaimConfigFile)
	data, err := os.ReadFile("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("failed to read the mod's file: %v", err)
	}
	if want := "config-files = []"; !containsLine(string(data), want) {
		t.Errorf("the mod's file doesn't have %q:\n%s", want, data)
	}
}

func stripped(s string) string { return ui.Strip(s) }

func containsLine(text, line string) bool {
	return slices.Contains(lines(text), line)
}

func TestBackendRefreshBringsTheIndexUpToDate(t *testing.T) {
	setUpPack(t)
	writeFile(t, "config/fresh.json", "{}")
	if err := os.Remove(filepath.Join("config", "orphan.json")); err != nil {
		t.Fatalf("failed to delete the file: %v", err)
	}
	notices, err := packBackend{}.refresh()
	if err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none", notices)
	}
	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	if !slices.Equal(data.tree.Unclaimed, []string{"config/fresh.json", "config/other/a.json", "config/other/b.json"}) {
		t.Errorf("unclaimed = %v, want the new file in and the deleted one out", data.tree.Unclaimed)
	}
}

func TestBackendRefreshSaysWhenFilesLeaveTheIndex(t *testing.T) {
	setUpPack(t)
	cmdtest.RegisterConfigDirSource(t, "defaults", "configureddefaults")
	writeMod(t, "defaults", "Configured Defaults", "", "\n[update.defaults]\nversion = \"any\"\n")
	writeFile(t, "configureddefaults/config/x.json", "{}")

	notices, err := packBackend{}.refresh()
	if err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	if len(notices) != 1 || notices[0] == "" {
		t.Errorf("notices = %q, want the one notice that files left the index, as plain text", notices)
	}
	for _, n := range notices {
		if n != stripped(n) {
			t.Errorf("the notice %q has styling in it", n)
		}
	}
}
