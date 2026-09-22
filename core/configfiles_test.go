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
