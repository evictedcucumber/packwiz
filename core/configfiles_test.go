package core

import (
	"slices"
	"strings"
	"testing"
)

// modAt builds a Mod with a name, its metadata file at metaPath, destination fileName, and the given config-files
func modAt(name, metaPath, fileName string, configFiles ...string) *Mod {
	m := &Mod{Name: name, FileName: fileName}
	if len(configFiles) > 0 {
		m.ConfigFiles = &configFiles
	}
	m.SetMetaPath(metaPath)
	return m
}

func newIndexFixture(t *testing.T) *Index {
	t.Helper()
	idx := &Index{HashFormat: "sha256", packRoot: "."}
	return idx
}

// track adds paths to the index, given as metadata file paths (by their ".pw.toml" extension) or plain files
func track(t *testing.T, idx *Index, paths ...string) {
	t.Helper()
	for _, p := range paths {
		meta := strings.HasSuffix(p, ".pw.toml")
		if err := idx.RefreshFileWithHash(p, "sha256", "h", meta); err != nil {
			t.Fatalf("RefreshFileWithHash(%q) returned error: %v", p, err)
		}
	}
}

func TestConfigFileTreeExcludesMetaFilesAndAModsOwnDestFile(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "mods/alpha.jar")

	mods := []*Mod{modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar")}
	tree, err := idx.ConfigFileTree(mods)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}
	if len(tree.Mods) != 0 || len(tree.Unclaimed) != 0 {
		t.Errorf("ConfigFileTree() = %+v, want none (the metadata file and the mod's own jar shouldn't be listed)", tree)
	}
}

func TestConfigFileTreeGroupsFilesByMod(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "mods/alpha.jar", "config/alpha.json", "config/alpha/sub.json", "config/orphan.json")

	mods := []*Mod{modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/alpha.json", "config/alpha/")}
	tree, err := idx.ConfigFileTree(mods)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	if len(tree.Mods) != 1 || tree.Mods[0].Mod.Name != "Alpha" {
		t.Fatalf("Mods = %+v, want just Alpha", tree.Mods)
	}
	want := []string{"config/alpha.json", "config/alpha/sub.json"}
	if !slices.Equal(tree.Mods[0].Files, want) {
		t.Errorf("Alpha's files = %v, want %v", tree.Mods[0].Files, want)
	}
	if want := []string{"config/orphan.json"}; !slices.Equal(tree.Unclaimed, want) {
		t.Errorf("Unclaimed = %v, want %v", tree.Unclaimed, want)
	}
}

func TestConfigFileTreeSortsModsByName(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/zeta.pw.toml", "mods/alpha.pw.toml", "config/zeta.json", "config/alpha.json")

	mods := []*Mod{
		modAt("Zeta Mod", "mods/zeta.pw.toml", "zeta.jar", "config/zeta.json"),
		modAt("alpha mod", "mods/alpha.pw.toml", "alpha.jar", "config/alpha.json"),
	}
	tree, err := idx.ConfigFileTree(mods)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	if len(tree.Mods) != 2 {
		t.Fatalf("Mods = %+v, want 2", tree.Mods)
	}
	if tree.Mods[0].Mod.Name != "alpha mod" || tree.Mods[1].Mod.Name != "Zeta Mod" {
		t.Errorf("Mods in order = %q, %q, want alpha mod then Zeta Mod (case-insensitive)", tree.Mods[0].Mod.Name, tree.Mods[1].Mod.Name)
	}
}

func TestConfigFileTreeListsAFileUnderEveryModThatClaimsIt(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "mods/beta.pw.toml", "config/shared.json")

	mods := []*Mod{
		modAt("Beta", "mods/beta.pw.toml", "beta.jar", "config/shared.json"),
		modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/shared.json"),
	}
	tree, err := idx.ConfigFileTree(mods)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	if len(tree.Mods) != 2 || tree.Mods[0].Mod.Name != "Alpha" || tree.Mods[1].Mod.Name != "Beta" {
		t.Fatalf("Mods = %+v, want the file to go to both Alpha and Beta, in name order", tree.Mods)
	}
	want := []string{"config/shared.json"}
	if !slices.Equal(tree.Mods[0].Files, want) {
		t.Errorf("Alpha's files = %v, want %v", tree.Mods[0].Files, want)
	}
	if !slices.Equal(tree.Mods[1].Files, want) {
		t.Errorf("Beta's files = %v, want %v", tree.Mods[1].Files, want)
	}
	if len(tree.Unclaimed) != 0 {
		t.Errorf("Unclaimed = %v, want none", tree.Unclaimed)
	}
}

// TestConfigFileTreeResolvedAgainstTheConfigDir checks that config-files entries are written as if the pack kept its
// files at the root of the game directory (as it normally does), and are resolved against a mod's ConfigDir (e.g.
// Configured Defaults) when the pack has one: "config/alpha.json" claims "configureddefaults/config/alpha.json".
func TestConfigFileTreeResolvedAgainstTheConfigDir(t *testing.T) {
	registerConfigDirSource(t, "defaults", "configureddefaults")

	idx := newIndexFixture(t)
	track(t, idx,
		"mods/defaults.pw.toml", "mods/alpha.pw.toml",
		"configureddefaults/config/alpha.json", "configureddefaults/config/alpha/sub.json",
		"configureddefaults/config/orphan.json",
	)

	defaultsMod := &Mod{Name: "Configured Defaults", Update: map[string]map[string]interface{}{"defaults": {}}}
	defaultsMod.SetMetaPath("mods/defaults.pw.toml")
	alphaMod := modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/alpha.json", "config/alpha/")

	tree, err := idx.ConfigFileTree([]*Mod{defaultsMod, alphaMod})
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	if len(tree.Mods) != 1 || tree.Mods[0].Mod.Name != "Alpha" {
		t.Fatalf("Mods = %+v, want just Alpha", tree.Mods)
	}
	want := []string{"configureddefaults/config/alpha.json", "configureddefaults/config/alpha/sub.json"}
	if !slices.Equal(tree.Mods[0].Files, want) {
		t.Errorf("Alpha's files = %v, want %v", tree.Mods[0].Files, want)
	}
	if want := []string{"configureddefaults/config/orphan.json"}; !slices.Equal(tree.Unclaimed, want) {
		t.Errorf("Unclaimed = %v, want %v", tree.Unclaimed, want)
	}
}

func TestConfigFileTreeUnclaimedSortedByPath(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "config/z.json", "config/a.json", "config/m.json")

	tree, err := idx.ConfigFileTree(nil)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}
	want := []string{"config/a.json", "config/m.json", "config/z.json"}
	if !slices.Equal(tree.Unclaimed, want) {
		t.Errorf("Unclaimed = %v, want %v", tree.Unclaimed, want)
	}
}

func TestConfigFileTreeReportsEntriesThatMatchNoTrackedFile(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "config/alpha.json", "config/kept/sub.json")

	mods := []*Mod{modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar",
		"config/z-gone.json", "config/alpha.json", "config/a-gone/", "config/kept/", "config/z-gone.json")}
	tree, err := idx.ConfigFileTree(mods)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	if len(tree.Mods) != 1 {
		t.Fatalf("Mods = %+v, want just Alpha", tree.Mods)
	}
	if want := []string{"config/alpha.json", "config/kept/sub.json"}; !slices.Equal(tree.Mods[0].Files, want) {
		t.Errorf("Alpha's files = %v, want %v", tree.Mods[0].Files, want)
	}
	// A folder with nothing under it is as missing as a file that isn't there, and each is only reported once
	if want := []string{"config/a-gone/", "config/z-gone.json"}; !slices.Equal(tree.Mods[0].Missing, want) {
		t.Errorf("Alpha's missing = %v, want %v", tree.Mods[0].Missing, want)
	}
	if len(tree.Unclaimed) != 0 {
		t.Errorf("Unclaimed = %v, want none", tree.Unclaimed)
	}
}

// A mod whose config files are all gone is still in the tree, with nothing but what is missing
func TestConfigFileTreeIncludesAModWithOnlyMissingEntries(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "mods/beta.pw.toml", "config/beta.json")

	mods := []*Mod{
		modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/alpha.json"),
		modAt("Beta", "mods/beta.pw.toml", "beta.jar", "config/beta.json"),
		modAt("Gamma", "mods/gamma.pw.toml", "gamma.jar"),
	}
	tree, err := idx.ConfigFileTree(mods)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	if len(tree.Mods) != 2 || tree.Mods[0].Mod.Name != "Alpha" || tree.Mods[1].Mod.Name != "Beta" {
		t.Fatalf("Mods = %+v, want Alpha and Beta (Gamma claims nothing)", tree.Mods)
	}
	if got := tree.Mods[0]; len(got.Files) != 0 || !slices.Equal(got.Missing, []string{"config/alpha.json"}) {
		t.Errorf("Alpha = %+v, want no files and config/alpha.json missing", got)
	}
	if got := tree.Mods[1]; !slices.Equal(got.Files, []string{"config/beta.json"}) || len(got.Missing) != 0 {
		t.Errorf("Beta = %+v, want config/beta.json and nothing missing", got)
	}
}

// A file that only another mod claims is still there for a mod that claims it too
func TestConfigFileTreeMissingIsPerMod(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "mods/beta.pw.toml", "config/shared.json")

	mods := []*Mod{
		modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/shared.json", "config/alpha.json"),
		modAt("Beta", "mods/beta.pw.toml", "beta.jar", "config/shared.json"),
	}
	tree, err := idx.ConfigFileTree(mods)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	if len(tree.Mods) != 2 {
		t.Fatalf("Mods = %+v, want Alpha and Beta", tree.Mods)
	}
	if want := []string{"config/alpha.json"}; !slices.Equal(tree.Mods[0].Missing, want) {
		t.Errorf("Alpha's missing = %v, want %v", tree.Mods[0].Missing, want)
	}
	if len(tree.Mods[1].Missing) != 0 {
		t.Errorf("Beta's missing = %v, want none", tree.Mods[1].Missing)
	}
}

// What is missing is reported as it is written in the mod, without the pack's config folder, but it is looked for in it
func TestConfigFileTreeMissingIsLookedForInTheConfigDir(t *testing.T) {
	registerConfigDirSource(t, "defaults", "configureddefaults")

	idx := newIndexFixture(t)
	track(t, idx, "mods/defaults.pw.toml", "mods/alpha.pw.toml", "configureddefaults/config/alpha.json", "config/gone.json")

	defaultsMod := &Mod{Name: "Configured Defaults", Update: map[string]map[string]interface{}{"defaults": {}}}
	defaultsMod.SetMetaPath("mods/defaults.pw.toml")
	alphaMod := modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/alpha.json", "config/gone.json")

	tree, err := idx.ConfigFileTree([]*Mod{defaultsMod, alphaMod})
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	if len(tree.Mods) != 1 || tree.Mods[0].Mod.Name != "Alpha" {
		t.Fatalf("Mods = %+v, want just Alpha", tree.Mods)
	}
	if want := []string{"configureddefaults/config/alpha.json"}; !slices.Equal(tree.Mods[0].Files, want) {
		t.Errorf("Alpha's files = %v, want %v", tree.Mods[0].Files, want)
	}
	if want := []string{"config/gone.json"}; !slices.Equal(tree.Mods[0].Missing, want) {
		t.Errorf("Alpha's missing = %v, want %v", tree.Mods[0].Missing, want)
	}
}
