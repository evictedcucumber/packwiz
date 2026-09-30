package cmd

import (
	"errors"
	"os"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// loadFixtureMods reads the mods of the pack that setUpUpdateFixture made, by name.
func loadFixtureMods(t *testing.T, pack core.Pack) (map[string]*core.Mod, core.Index) {
	t.Helper()
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		t.Fatalf("LoadAllMods() returned error: %v", err)
	}
	byName := make(map[string]*core.Mod)
	for _, m := range mods {
		byName[m.Name] = m
	}
	return byName, index
}

func TestFindUpdatesOffersWhatHasAnUpdateAndSaysWhatDoesNot(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{checks: map[string]core.UpdateCheck{
		"Alpha": {UpdateAvailable: true, UpdateString: "a.jar -> a2.jar", CachedState: "state-a"},
		"Beta":  {Error: errors.New("boom")},
		"Gamma": {UpdateAvailable: true, UpdateString: "g.jar -> g2.jar"},
	}}, "Alpha", "Beta", "Gamma", "Delta")
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	mods, _ := loadFixtureMods(t, pack)
	mods["Gamma"].Pin = true
	all := []*core.Mod{mods["Alpha"], mods["Beta"], mods["Gamma"], mods["Delta"]}

	var sources []string
	out := cmdtest.CaptureStdout(t, func() {
		search := FindUpdates(pack, all, func(source string) { sources = append(sources, source) })
		if len(search.Offers) != 1 || search.Offers[0].Mod.Name != "Alpha" || search.Offers[0].Change != "a.jar -> a2.jar" || search.Offers[0].Source != "fake" {
			t.Errorf("the offers are %+v, want only Alpha's update", search.Offers)
		}
		if len(search.Failures) != 1 || search.Failures[0].Name != "Beta" || search.Failures[0].Err.Error() != "boom" || search.Failures[0].Count != 1 {
			t.Errorf("the failures are %+v, want Beta's check", search.Failures)
		}
		if len(search.Pinned) != 1 || search.Pinned[0].Name != "Gamma" {
			t.Errorf("the pinned are %v, want Gamma, which has an update that isn't offered", search.Pinned)
		}
		if len(search.Unsupported) != 0 {
			t.Errorf("the unsupported are %v, want none", search.Unsupported)
		}
		if search.Checked != 4 || search.FailedChecks() != 1 {
			t.Errorf("checked %d and failed %d, want all four checked and one failed", search.Checked, search.FailedChecks())
		}
	})
	if out != "" {
		t.Errorf("FindUpdates() wrote %q to the terminal", out)
	}
	if len(sources) != 1 || sources[0] != "fake" {
		t.Errorf("the sources asked are %v, want the one, said as it was asked", sources)
	}
}

func TestFindUpdatesCountsEveryModOfASourceThatFailedAsAWhole(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{err: errors.New("no network")}, "Alpha", "Beta")
	pack, _ := core.LoadPack()
	mods, _ := loadFixtureMods(t, pack)

	search := FindUpdates(pack, []*core.Mod{mods["Alpha"], mods["Beta"]}, nil)
	if len(search.Failures) != 1 || search.Failures[0].Name != "fake" || search.Failures[0].Count != 2 || search.FailedChecks() != 2 {
		t.Errorf("the failures are %+v, want one for the source, for both of its mods", search.Failures)
	}
}

func TestFindUpdatesSaysWhichModsNothingCanUpdate(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{}, "Alpha")
	pack, _ := core.LoadPack()
	mods, _ := loadFixtureMods(t, pack)
	mods["Alpha"].Update = nil

	search := FindUpdates(pack, []*core.Mod{mods["Alpha"]}, nil)
	if len(search.Unsupported) != 1 || search.Unsupported[0].Name != "Alpha" || len(search.Offers) != 0 || search.Checked != 0 {
		t.Errorf("the search is %+v, want Alpha unsupported and nothing checked", search)
	}
}

func TestFindUpdatesOfNothingFindsNothing(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{}, "Alpha")
	pack, _ := core.LoadPack()
	search := FindUpdates(pack, nil, nil)
	if len(search.Offers)+len(search.Failures)+len(search.Pinned)+len(search.Unsupported) != 0 || search.Checked != 0 {
		t.Errorf("the search is %+v, want nothing", search)
	}
}

// doingUpdater updates a mod by giving it the file its check said, and can be told to fail
type doingUpdater struct {
	fakeUpdater
	err error
}

func (u doingUpdater) DoUpdate(mods []*core.Mod, state []interface{}) error {
	if u.err != nil {
		return u.err
	}
	for i, mod := range mods {
		mod.FileName = state[i].(string)
	}
	return nil
}

func TestApplyUpdatesWritesTheModsAndPutsThemInTheIndex(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{checks: map[string]core.UpdateCheck{
		"Alpha": {UpdateAvailable: true, UpdateString: "a", CachedState: "alpha-2.jar"},
		"Beta":  {UpdateAvailable: true, UpdateString: "b", CachedState: "beta-2.jar"},
	}}, "Alpha", "Beta")
	core.Updaters["fake"] = doingUpdater{fakeUpdater: core.Updaters["fake"].(fakeUpdater)}
	pack, _ := core.LoadPack()
	mods, index := loadFixtureMods(t, pack)
	search := FindUpdates(pack, []*core.Mod{mods["Alpha"], mods["Beta"]}, nil)
	if len(search.Offers) != 2 {
		t.Fatalf("the offers are %+v, want both", search.Offers)
	}

	var updated []*core.Mod
	var errs []error
	out := cmdtest.CaptureStdout(t, func() { updated, errs = ApplyUpdates(&index, search.Offers) })

	if len(errs) != 0 || len(updated) != 2 {
		t.Fatalf("ApplyUpdates() = %d updated, %v, want both and no errors", len(updated), errs)
	}
	if out != "" {
		t.Errorf("ApplyUpdates() wrote %q to the terminal", out)
	}
	for name, file := range map[string]string{"Alpha": "alpha-2.jar", "Beta": "beta-2.jar"} {
		mod, err := core.LoadMod("mods/" + map[string]string{"Alpha": "alpha", "Beta": "beta"}[name] + ".pw.toml")
		if err != nil || mod.FileName != file {
			t.Errorf("%s is %+v (%v), want it on %s", name, mod.FileName, err, file)
		}
	}
	// What was updated is in the index, which is for the caller to write along with the pack
	if err := pack.SaveIndex(index); err != nil {
		t.Fatalf("SaveIndex() returned error: %v", err)
	}
	if data, _ := os.ReadFile("index.toml"); len(data) == 0 {
		t.Error("the index is empty after it was saved")
	}
}

func TestApplyUpdatesGoesOnAfterASourceThatFails(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{checks: map[string]core.UpdateCheck{
		"Alpha": {UpdateAvailable: true, UpdateString: "a", CachedState: "alpha-2.jar"},
	}}, "Alpha")
	core.Updaters["fake"] = doingUpdater{fakeUpdater: core.Updaters["fake"].(fakeUpdater), err: errors.New("disk full")}
	pack, _ := core.LoadPack()
	mods, index := loadFixtureMods(t, pack)
	search := FindUpdates(pack, []*core.Mod{mods["Alpha"]}, nil)

	updated, errs := ApplyUpdates(&index, search.Offers)
	if len(updated) != 0 || len(errs) != 1 || errs[0].Error() != "disk full" {
		t.Errorf("ApplyUpdates() = %d updated, %v, want nothing updated and the reason", len(updated), errs)
	}
}

func TestFilterByKindKeepsMainModsOrDependencies(t *testing.T) {
	fresh := func() []*core.Mod {
		return []*core.Mod{{Name: "Main"}, {Name: "Lib", AddedAsDependency: true}, {Name: "Other"}}
	}
	names := func(mods []*core.Mod) []string {
		var out []string
		for _, m := range mods {
			out = append(out, m.Name)
		}
		return out
	}
	if got := names(FilterByKind(fresh(), OnlyMain)); len(got) != 2 || got[0] != "Main" || got[1] != "Other" {
		t.Errorf("the main mods are %v, want Main and Other", got)
	}
	if got := names(FilterByKind(fresh(), OnlyDependencies)); len(got) != 1 || got[0] != "Lib" {
		t.Errorf("the dependencies are %v, want Lib", got)
	}
}

func TestFilterBySideKeepsWhatRunsThereAndWhatRunsEverywhere(t *testing.T) {
	mods := func() []*core.Mod {
		return []*core.Mod{
			{Name: "C", Side: core.ClientSide}, {Name: "S", Side: core.ServerSide},
			{Name: "B", Side: core.UniversalSide}, {Name: "E", Side: core.EmptySide},
		}
	}
	names := func(mods []*core.Mod) string {
		out := ""
		for _, m := range mods {
			out += m.Name
		}
		return out
	}
	for side, want := range map[string]string{core.ClientSide: "CBE", core.ServerSide: "SBE", core.UniversalSide: "CSBE"} {
		if got := names(FilterBySide(mods(), side)); got != want {
			t.Errorf("FilterBySide(%q) = %q, want %q", side, got, want)
		}
	}
}

func TestSortModsOrdersByNameIgnoringCase(t *testing.T) {
	mods := []*core.Mod{{Name: "beta"}, {Name: "Alpha"}, {Name: "Gamma"}}
	SortMods(mods)
	if mods[0].Name != "Alpha" || mods[1].Name != "beta" || mods[2].Name != "Gamma" {
		t.Errorf("the mods are %v, want them in alphabetical order whatever the case", []string{mods[0].Name, mods[1].Name, mods[2].Name})
	}
}

func TestSaveModListWritesTheListInThePacksFolderAndSaysNothing(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{}, "Alpha", "Beta")
	pack, _ := core.LoadPack()
	mods, index := loadFixtureMods(t, pack)
	all := []*core.Mod{mods["Alpha"], mods["Beta"]}

	var path string
	var err error
	out := cmdtest.CaptureStdout(t, func() { path, err = SaveModList(pack, index, all) })
	if err != nil {
		t.Fatalf("SaveModList() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("SaveModList() wrote %q to the terminal", out)
	}
	if path != core.ModListFile {
		t.Errorf("SaveModList() = %q, want the default file in the pack's folder", path)
	}
	text, _ := os.ReadFile(path)
	want, _ := RenderModList(pack, index, all)
	if string(text) != want || want == "" {
		t.Errorf("the file has %q, want what RenderModList makes: %q", text, want)
	}
}
