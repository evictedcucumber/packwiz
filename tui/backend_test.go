package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
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
	if got := ownerNames(data); !slices.Equal(got, []string{"Pack", "Alpha Mod", "Beta Mod"}) {
		t.Errorf("owners = %v, want the pack, then every mod, even the one that claims nothing, by name", got)
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
	if got, want := ownerNames(data), []string{"Pack", "Alpha Mod", "Beta Mod", "Eta Upper", "zeta lower"}; !slices.Equal(got, want) {
		t.Errorf("owners = %v, want %v", got, want)
	}
}

func ownerNames(data configData) []string {
	names := make([]string, len(data.owners))
	for i, o := range data.owners {
		names[i] = o.name
	}
	return names
}

// The pack and its loader are owners of config files, and what they already own is read from pack.toml
func TestBackendLoadHasThePackAndItsLoaderAsOwners(t *testing.T) {
	setUpPack(t)
	withLoader(t)
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	pack.ClaimConfigFile(core.ConfigOwnerPack, "options.txt")
	pack.ClaimConfigFile("neoforge", "config/neoforge-common.toml")
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	data, err := packBackend{}.load()
	if err != nil {
		t.Fatalf("load() returned error: %v", err)
	}
	if got := ownerNames(data); !slices.Equal(got, []string{"Pack", "NeoForge", "Alpha Mod", "Beta Mod"}) {
		t.Errorf("owners = %v, want the pack, its loader and then the mods", got)
	}
	want := []owner{
		{kind: ownerPack, id: "pack", name: "Pack", slug: "pack", entries: []string{"options.txt"}},
		{kind: ownerLoader, id: "neoforge", name: "NeoForge", slug: "neoforge", entries: []string{"config/neoforge-common.toml"}},
	}
	if !reflect.DeepEqual(data.owners[:2], want) {
		t.Errorf("the first owners are %+v, want %+v", data.owners[:2], want)
	}
	// Neither file is in the index, so what they own is in the tree as entries that match no file
	if len(data.tree.Owners) != 2 || len(data.tree.Owners[0].Files) != 0 ||
		!slices.Equal(data.tree.Owners[0].Missing, []string{"options.txt"}) || !slices.Equal(data.tree.Owners[1].Missing, []string{"config/neoforge-common.toml"}) {
		t.Errorf("Owners = %+v, want the pack and its loader, with what they own as missing", data.tree.Owners)
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
		[]owner{fakeOwner("Alpha Mod", "alpha"), fakeOwner("Beta Mod", "beta")},
		[]string{"config/alpha.json", "config/orphan.json"}, // Alpha has the first already
	)
	if err != nil {
		t.Fatalf("relate() returned error: %v", err)
	}
	if want := (relateResult{added: 3, existing: 1, owners: 2}); got != want {
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
	got, err := packBackend{}.relate([]owner{fakeOwner("Alpha Mod", "alpha")}, []string{"config/alpha.json"})
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
	got, err := packBackend{}.relate([]owner{fakeOwner("Beta Mod", "beta"), fakeOwner("Missing", "missing")}, []string{"config/orphan.json"})
	if err == nil {
		t.Fatal("relate() returned no error for a mod that doesn't exist")
	}
	if got.owners != 1 || got.added != 1 {
		t.Errorf("relate() = %+v, want what was saved before the failure counted", got)
	}
	if got := claims(t, "beta"); !slices.Equal(got, []string{"config/orphan.json"}) {
		t.Errorf("Beta Mod's config-files is %v, want what was saved before the failure", got)
	}
	assertIndexIsConsistent(t)
}

func TestBackendRelateFailsForAModWithoutAMetadataFile(t *testing.T) {
	setUpPack(t)
	if _, err := (packBackend{}).relate([]owner{fakeOwner("Nope", "nope")}, []string{"config/a.json"}); err == nil {
		t.Error("relate() returned no error for a mod that doesn't exist")
	}
}

func TestBackendUnrelateTakesOutEntries(t *testing.T) {
	setUpPack(t)
	if err := (packBackend{}).unrelate(fakeOwner("Alpha Mod", "alpha"), []string{"config/alpha/", "config/gone.json"}); err != nil {
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
	if err := (packBackend{}).unrelate(fakeOwner("Alpha Mod", "alpha"), []string{"config/never-there.json"}); err != nil {
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
	if err := (packBackend{}).unrelate(fakeOwner("Alpha Mod", "alpha"), []string{"config/alpha.json", "config/alpha/", "config/gone.json"}); err != nil {
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
