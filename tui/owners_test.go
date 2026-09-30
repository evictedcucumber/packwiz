package tui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/fuzzy"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// setUpOwnerFiles makes the pack that setUpPack made have the mod loader neoforge, and adds the files that belong to it
// and to the pack as a whole rather than to a mod: neoforge's config/neoforge-common.toml and config/neoforge-client.toml,
// and options.txt. Nothing claims them yet.
func setUpOwnerFiles(t *testing.T) {
	t.Helper()
	setUpPack(t)
	withLoader(t)
	for _, f := range []string{"options.txt", "config/neoforge-common.toml", "config/neoforge-client.toml"} {
		writeFile(t, f, "x")
	}
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
}

// claimInPack records what the pack or its loader owns in pack.toml, as relating would.
func claimInPack(t *testing.T, owner string, entries ...string) {
	t.Helper()
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	for _, e := range entries {
		pack.ClaimConfigFile(owner, e)
	}
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
}

// pickerRows are the owners the picker lists, in order, as drawn: each line of the box's list without its cursor and box.
func pickerRows(t *testing.T, s *configScreen) []string {
	t.Helper()
	var rows []string
	for _, l := range lines(s.view()) {
		if i := strings.Index(l, "] "); i >= 0 && strings.Contains(l, "│") && (strings.Contains(l, "[ ]") || strings.Contains(l, "[x]")) {
			rows = append(rows, strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l[i+2:]), "│")))
		}
	}
	return rows
}

func TestBuildRowsHasThePackAndItsLoaderBeforeTheMods(t *testing.T) {
	tree := core.ConfigFileTree{
		Owners: []core.OwnerConfigFiles{
			{Owner: "pack", Name: "Pack", Entries: []string{"options.txt"}, Files: []string{"options.txt"}},
			{Owner: "neoforge", Name: "NeoForge", Entries: []string{"config/a.toml", "config/gone.toml"}, Files: []string{"config/a.toml"}, Missing: []string{"config/gone.toml"}},
		},
		Mods:      []core.ModConfigFiles{{Mod: fakeMod("Alpha", "alpha"), Files: []string{"config/alpha.json"}}},
		Unclaimed: []string{"config/orphan.json"},
	}
	got := describe(buildRows(tree, allStates, nil, fuzzy.Query{}))
	want := []string{
		"> Pack", "options.txt",
		"> NeoForge", "config/a.toml", "missing:config/gone.toml",
		"> Alpha", "config/alpha.json",
		"> Invalid", "config/orphan.json",
	}
	if !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}

	// The filter applies to them as it does to the mods
	if got, want := describe(buildRows(tree, missingState, nil, fuzzy.Query{})), []string{"> NeoForge", "missing:config/gone.toml"}; !slices.Equal(got, want) {
		t.Errorf("missing: rows = %v, want %v", got, want)
	}
	if got, want := describe(buildRows(tree, invalidState, nil, fuzzy.Query{})), []string{"> Invalid", "config/orphan.json"}; !slices.Equal(got, want) {
		t.Errorf("invalid: rows = %v, want %v", got, want)
	}

	if counts := countStates(tree); counts != (stateCounts{valid: 3, invalid: 1, missing: 1}) {
		t.Errorf("countStates() = %+v, want what the pack and its loader claim counted too", counts)
	}
}

func TestBuildRowsGiveEachOwnerItsKindAndWhatItClaims(t *testing.T) {
	tree := core.ConfigFileTree{
		Owners: []core.OwnerConfigFiles{
			{Owner: "pack", Name: "Pack", Entries: []string{"options.txt"}, Files: []string{"options.txt"}},
			{Owner: "neoforge", Name: "NeoForge", Entries: []string{"config/"}, Files: []string{"config/a.toml"}},
		},
		Mods:      []core.ModConfigFiles{{Mod: fakeMod("Alpha", "alpha", "config/x"), Files: []string{"config/x"}}},
		Unclaimed: []string{"config/orphan.json"},
	}
	rows := buildRows(tree, allStates, nil, fuzzy.Query{})
	kinds := map[string]ownerKind{}
	for _, r := range rows {
		if r.kind == groupRow {
			kinds[r.title] = r.owner.kind
		}
	}
	want := map[string]ownerKind{"Pack": ownerPack, "NeoForge": ownerLoader, "Alpha": ownerMod, "Invalid": ownerNone}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}
	if got := rows[0].owner.entries; !slices.Equal(got, []string{"options.txt"}) {
		t.Errorf("the pack's entries are %v, want what pack.toml has for it, for unrelating", got)
	}
	// No two rows share a key, nor groups an id, whoever owns them
	seen := map[rowKey]bool{}
	for _, r := range rows {
		if seen[r.key()] {
			t.Errorf("two rows have the key %+v", r.key())
		}
		seen[r.key()] = true
	}
}

func TestOwnerKeysAreDifferentForEveryOwner(t *testing.T) {
	owners := []owner{
		{kind: ownerPack, id: "pack"}, {kind: ownerLoader, id: "neoforge"},
		modOwner(fakeMod("Pack", "pack")), modOwner(fakeMod("NeoForge", "neoforge")),
	}
	seen := map[string]bool{}
	for _, o := range owners {
		if seen[o.key()] {
			t.Errorf("the key %q is shared", o.key())
		}
		seen[o.key()] = true
	}
	if k := (owner{}).key(); seen[k] {
		t.Errorf("the key of nothing, %q, is an owner's", k)
	}
	if invalidGroup == (owner{kind: ownerPack, id: "pack"}).key() {
		t.Error("the Invalid group has the pack's id")
	}
}

func TestOwnerNotes(t *testing.T) {
	for _, tt := range []struct {
		o    owner
		want string
	}{
		{owner{kind: ownerPack}, "the pack as a whole"},
		{owner{kind: ownerLoader}, "mod loader"},
		{owner{kind: ownerMod}, ""},
		{owner{}, ""},
	} {
		if got := tt.o.note(); got != tt.want {
			t.Errorf("note() of %v = %q, want %q", tt.o.kind, got, tt.want)
		}
	}
}

func TestConfigScreenShowsWhatThePackAndItsLoaderClaimBeforeTheMods(t *testing.T) {
	setUpOwnerFiles(t)
	claimInPack(t, core.ConfigOwnerPack, "options.txt")
	claimInPack(t, "neoforge", "config/neoforge-common.toml", "config/neoforge-client.toml", "config/neoforge-gone.toml")
	s := configOn(t, packBackend{})

	want := []string{
		"▾ Pack (1)", "└── options.txt",
		"▾ NeoForge (3)", "├── config/neoforge-client.toml", "├── config/neoforge-common.toml", "└── config/neoforge-gone.toml (missing)",
		"▾ Alpha Mod (4)", "├── config/alpha.json", "├── config/alpha/deep/x.json", "├── config/alpha/sub.json", "└── config/gone.json (missing)",
		"▾ Invalid (3)", "├── config/orphan.json", "├── config/other/a.json", "└── config/other/b.json",
	}
	s.setSize(100, 30)
	if got := tree(t, s); !slices.Equal(got, want) {
		t.Errorf("the tree is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := lines(s.view())[0]; !strings.Contains(got, "6 valid") || !strings.Contains(got, "2 missing") {
		t.Errorf("the summary %q doesn't count what the pack and its loader claim", got)
	}
}

func TestPickerOffersThePackAndItsLoaderBeforeTheMods(t *testing.T) {
	setUpOwnerFiles(t)
	s := configOn(t, packBackend{})
	goToFile(t, s, "options.txt")
	press(t, s, "r")

	if got, want := pickerRows(t, s), []string{"Pack  the pack as a whole", "NeoForge  mod loader", "Alpha Mod", "Beta Mod"}; !slices.Equal(got, want) {
		t.Errorf("the picker lists %q, want %q", got, want)
	}

	// A pack that has no loader has none to offer
	s.overlay = nil
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	delete(pack.Versions, "neoforge")
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	s = configOn(t, packBackend{})
	goToFile(t, s, "options.txt")
	press(t, s, "r")
	if got, want := pickerRows(t, s), []string{"Pack  the pack as a whole", "Alpha Mod", "Beta Mod"}; !slices.Equal(got, want) {
		t.Errorf("the picker lists %q for a pack with no loader, want %q", got, want)
	}
}

func TestRelateOptionsTxtToThePackAsAWhole(t *testing.T) {
	setUpOwnerFiles(t)
	s := configOn(t, packBackend{})
	goToFile(t, s, "options.txt")
	press(t, s, "r", "enter") // the pack is first, so it is what is picked

	if got := packToml(t); !reflect.DeepEqual(got, map[string][]string{"pack": {"options.txt"}}) {
		t.Errorf("pack.toml's config-files = %v, want options.txt for the pack", got)
	}
	if got := statusOf(s); got != "Pack now claims options.txt" {
		t.Errorf("the status is %q", got)
	}
	got := tree(t, s)
	if got[0] != "▾ Pack (1)" || got[1] != "└── options.txt" {
		t.Errorf("the tree starts %v, want the pack with options.txt under it", got[:2])
	}
	if slices.ContainsFunc(got[2:], func(l string) bool { return strings.HasSuffix(l, "options.txt") }) {
		t.Errorf("options.txt is still listed below the pack, as unclaimed: %v", got)
	}
	if got := cursorRow(t, s); got != "└── options.txt" {
		t.Errorf("the cursor is on %q, want it on the file, under the pack", got)
	}
	assertIndexIsConsistent(t)
}

func TestRelateNeoForgesFilesToTheModLoader(t *testing.T) {
	setUpOwnerFiles(t)
	s := configOn(t, packBackend{})
	goToFile(t, s, "config/neoforge-client.toml")
	press(t, s, "space")
	goToFile(t, s, "config/neoforge-common.toml")
	press(t, s, "space", "r")
	// Searched for, rather than counted to
	press(t, s, "/")
	typeText(t, s, "neo")
	press(t, s, "enter", "enter")

	want := map[string][]string{"neoforge": {"config/neoforge-client.toml", "config/neoforge-common.toml"}}
	if got := packToml(t); !reflect.DeepEqual(got, want) {
		t.Errorf("pack.toml's config-files = %v, want %v", got, want)
	}
	if got := statusOf(s); got != "Related 2 entries to 1 owner" {
		t.Errorf("the status is %q", got)
	}
	if got := tree(t, s); !slices.Contains(got, "▾ NeoForge (2)") {
		t.Errorf("the tree is %v, want NeoForge with both files", got)
	}
	assertIndexIsConsistent(t)
}

// What is related to the pack, its loader and a mod in one go is saved once, with each going where it belongs
func TestRelateToThePackItsLoaderAndAModTogether(t *testing.T) {
	setUpOwnerFiles(t)
	s := configOn(t, packBackend{})
	goToFile(t, s, "config/orphan.json")
	press(t, s, "r", "space", "j", "space", "j", "space", "enter") // the pack, its loader, and Alpha Mod

	got := packToml(t)
	if !slices.Equal(got["pack"], []string{"config/orphan.json"}) || !slices.Equal(got["neoforge"], []string{"config/orphan.json"}) {
		t.Errorf("pack.toml's config-files = %v, want the file for both the pack and its loader", got)
	}
	if got := claims(t, "alpha"); !slices.Contains(got, "config/orphan.json") {
		t.Errorf("Alpha Mod's config-files is %v, want the file in it", got)
	}
	if got := statusOf(s); got != "Related 1 entry to 3 owners" {
		t.Errorf("the status is %q", got)
	}
	// The mod's change saved the index with the pack, which was changed too, so both changes are in pack.toml
	if pack, err := core.LoadPack(); err != nil || pack.Index.Hash == "" {
		t.Errorf("pack.toml = %+v, %v, want it to record the index's hash as well", pack, err)
	}
	assertIndexIsConsistent(t)
}

func TestRelateTheFolderOfAFileToTheLoader(t *testing.T) {
	setUpOwnerFiles(t)
	s := configOn(t, packBackend{})
	goToFile(t, s, "config/neoforge-client.toml")
	press(t, s, "r", "tab", "j", "enter") // the folder is config/, then the loader, which is second
	if got := packToml(t); !reflect.DeepEqual(got, map[string][]string{"neoforge": {"config/"}}) {
		t.Errorf("pack.toml's config-files = %v, want the folder for the loader", got)
	}
}

func TestRelateOptionsTxtToThePackWhenThePackKeepsItsFilesInAConfigDir(t *testing.T) {
	setUpPack(t)
	cmdtest.RegisterConfigDirSource(t, "defaults", "configureddefaults")
	writeMod(t, "defaults", "Configured Defaults", "", "\n[update.defaults]\nversion = \"any\"\n")
	writeFile(t, "configureddefaults/options.txt", "x")
	if _, err := (packBackend{}).refresh(); err != nil {
		t.Fatalf("refresh() returned error: %v", err)
	}
	s := configOn(t, packBackend{})
	goToFile(t, s, "configureddefaults/options.txt")
	press(t, s, "r", "enter")

	// Written as it would be without the folder, as every entry is, so it reads the same wherever the pack keeps its files
	if got := packToml(t); !reflect.DeepEqual(got, map[string][]string{"pack": {"options.txt"}}) {
		t.Errorf("pack.toml's config-files = %v, want the entry without the config dir", got)
	}
	if got := tree(t, s); got[0] != "▾ Pack (1)" || got[1] != "└── configureddefaults/options.txt" {
		t.Errorf("the tree starts %v, want the file under the pack", got[:2])
	}

	// And taking it out finds the entry as it is written
	press(t, s, "x")
	if view := s.view(); !strings.Contains(view, "Remove from Pack's config-files?") || !strings.Contains(view, "options.txt") {
		t.Errorf("the prompt doesn't ask about the entry:\n%s", view)
	}
	press(t, s, "y")
	if got := packToml(t); len(got) != 0 {
		t.Errorf("pack.toml's config-files = %v, want nothing once the entry is taken out", got)
	}
}

func TestUnrelateAFileFromThePack(t *testing.T) {
	setUpOwnerFiles(t)
	claimInPack(t, core.ConfigOwnerPack, "options.txt")
	s := configOn(t, packBackend{})
	goToFile(t, s, "options.txt")
	if got := cursorRow(t, s); got != "└── options.txt" {
		t.Fatalf("the cursor is on %q, want the file under the pack", got)
	}

	press(t, s, "x")
	if view := s.view(); !strings.Contains(view, "Remove from Pack's config-files?") {
		t.Errorf("the prompt doesn't say whose config-files:\n%s", view)
	}
	press(t, s, "y")

	// The last of the pack's entries is taken out, and with it the pack's place in pack.toml
	if got := packToml(t); len(got) != 0 {
		t.Errorf("pack.toml's config-files = %v, want none left", got)
	}
	if data, err := readFileQuiet("pack.toml"); err != nil || strings.Contains(data, "config-files") {
		t.Errorf("pack.toml still has a config-files table: %v\n%s", err, data)
	}
	if got := statusOf(s); got != "Pack no longer claims options.txt" {
		t.Errorf("the status is %q", got)
	}
	if got := tree(t, s); slices.Contains(got, "▾ Pack (1)") {
		t.Errorf("the tree is %v, want no group for the pack once it claims nothing", got)
	}
	// It is unclaimed again, and the cursor went with it
	if got := cursorRow(t, s); got != "└── options.txt" || !slices.Contains(tree(t, s), "▾ Invalid (6)") {
		t.Errorf("the cursor is on %q and the tree is %v, want the file among those nothing claims", got, tree(t, s))
	}
	assertIndexIsConsistent(t)
}

func TestUnrelateAnEntryOfTheLoaderThatMatchesNoFile(t *testing.T) {
	setUpOwnerFiles(t)
	claimInPack(t, "neoforge", "config/neoforge-common.toml", "config/neoforge-gone.toml")
	s := configOn(t, packBackend{})
	goToMissing(t, s, "config/neoforge-gone.toml")
	press(t, s, "x")
	if view := s.view(); !strings.Contains(view, "Remove from NeoForge's config-files?") || !strings.Contains(view, "config/neoforge-gone.toml  (matches no file in the pack)") {
		t.Errorf("the prompt doesn't ask about the entry that matches no file:\n%s", view)
	}
	press(t, s, "y")
	if got := packToml(t); !reflect.DeepEqual(got, map[string][]string{"neoforge": {"config/neoforge-common.toml"}}) {
		t.Errorf("pack.toml's config-files = %v, want only the entry that has a file", got)
	}
}

func TestUnrelateDeclinedLeavesPackTomlAlone(t *testing.T) {
	setUpOwnerFiles(t)
	claimInPack(t, core.ConfigOwnerPack, "options.txt")
	before, _ := readFileQuiet("pack.toml")
	s := configOn(t, packBackend{})
	goToFile(t, s, "options.txt")
	press(t, s, "x", "n")
	if after, _ := readFileQuiet("pack.toml"); after != before {
		t.Errorf("pack.toml changed:\n%s", after)
	}
}

func TestUnrelateAFolderEntryOfThePackSaysHowManyFilesItCovers(t *testing.T) {
	setUpOwnerFiles(t)
	claimInPack(t, core.ConfigOwnerPack, "config/other/")
	s := configOn(t, packBackend{})
	goToFile(t, s, "config/other/a.json")
	press(t, s, "x")
	if view := s.view(); !strings.Contains(view, "config/other/  (covers 2 files)") {
		t.Errorf("the prompt doesn't say that the folder covers two files:\n%s", view)
	}
}

func TestColoursOfThePackAndItsLoaderAreThoseOfTheMods(t *testing.T) {
	setUpOwnerFiles(t)
	claimInPack(t, core.ConfigOwnerPack, "options.txt")
	claimInPack(t, "neoforge", "config/neoforge-common.toml")
	a := newApp(mustLoad(t).pack, configOn(t, packBackend{}))
	a.Update(windowSize(100, 30))
	cmdtest.SetColor(t, ui.Always)
	out := a.render()
	for what, want := range map[string]string{
		"the pack's name":    ui.Bold.Sprint("Pack"),
		"the loader's name":  ui.Bold.Sprint("NeoForge"),
		"a file of the pack": ui.Success.Sprint("config/neoforge-common.toml"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%s isn't drawn as %q:\n%s", what, want, ui.Strip(out))
		}
	}
}

// What is related to the pack is kept even if a mod later in the same change can't be saved, as it is only in pack.toml
func TestBackendRelateKeepsWhatThePackClaimsWhenAModFails(t *testing.T) {
	setUpOwnerFiles(t)
	pack := owner{kind: ownerPack, id: "pack", name: "Pack"}
	result, err := packBackend{}.relate([]owner{pack, fakeOwner("Missing", "missing")}, []string{"options.txt"})
	if err == nil {
		t.Fatal("relate() returned no error for a mod that doesn't exist")
	}
	if result.owners != 1 || result.added != 1 {
		t.Errorf("relate() = %+v, want what was saved before the failure counted", result)
	}
	if got := packToml(t); !reflect.DeepEqual(got, map[string][]string{"pack": {"options.txt"}}) {
		t.Errorf("pack.toml's config-files = %v, want what was claimed before the failure", got)
	}
}

func TestBackendRelateToThePackWritesOnlyPackToml(t *testing.T) {
	setUpOwnerFiles(t)
	indexBefore, _ := readFileQuiet("index.toml")
	alphaBefore, _ := readFileQuiet("mods/alpha.pw.toml")
	result, err := packBackend{}.relate(
		[]owner{{kind: ownerPack, id: "pack", name: "Pack"}, {kind: ownerLoader, id: "neoforge", name: "NeoForge"}},
		[]string{"options.txt", "config/neoforge-common.toml"})
	if err != nil {
		t.Fatalf("relate() returned error: %v", err)
	}
	if want := (relateResult{added: 4, owners: 2}); result != want {
		t.Errorf("relate() = %+v, want %+v", result, want)
	}
	// What the pack and its loader own is in pack.toml, which isn't in the index, so nothing else is touched
	if after, _ := readFileQuiet("index.toml"); after != indexBefore {
		t.Error("the index was rewritten")
	}
	if after, _ := readFileQuiet("mods/alpha.pw.toml"); after != alphaBefore {
		t.Error("a mod's file was rewritten")
	}

	// Again, and it finds it all there
	result, err = packBackend{}.relate(
		[]owner{{kind: ownerPack, id: "pack", name: "Pack"}}, []string{"options.txt"})
	if err != nil || result != (relateResult{existing: 1}) {
		t.Errorf("relate() again = %+v, %v, want only that it was there already", result, err)
	}
}

func TestBackendRelateRefusesAnOwnerThatIsNothing(t *testing.T) {
	setUpPack(t)
	if _, err := (packBackend{}).relate([]owner{{kind: ownerNone, name: "Invalid"}}, []string{"a"}); err == nil {
		t.Error("relate() accepted an owner that is nothing")
	}
	if err := (packBackend{}).unrelate(owner{kind: ownerNone, name: "Invalid"}, []string{"a"}); err == nil {
		t.Error("unrelate() accepted an owner that is nothing")
	}
}

func TestBackendUnrelateOfThePackThatIsNotThereWritesNothing(t *testing.T) {
	setUpOwnerFiles(t)
	before, _ := readFileQuiet("pack.toml")
	if err := (packBackend{}).unrelate(owner{kind: ownerPack, id: "pack", name: "Pack"}, []string{"options.txt"}); err != nil {
		t.Fatalf("unrelate() returned error: %v", err)
	}
	if after, _ := readFileQuiet("pack.toml"); after != before {
		t.Errorf("pack.toml changed:\n%s", after)
	}
}
