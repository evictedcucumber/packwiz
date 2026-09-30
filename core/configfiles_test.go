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
	tree, err := idx.ConfigFileTree(mods, Pack{})
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
	tree, err := idx.ConfigFileTree(mods, Pack{})
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
	tree, err := idx.ConfigFileTree(mods, Pack{})
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
	tree, err := idx.ConfigFileTree(mods, Pack{})
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

	tree, err := idx.ConfigFileTree([]*Mod{defaultsMod, alphaMod}, Pack{})
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

	tree, err := idx.ConfigFileTree(nil, Pack{})
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
	tree, err := idx.ConfigFileTree(mods, Pack{})
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
	tree, err := idx.ConfigFileTree(mods, Pack{})
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
	tree, err := idx.ConfigFileTree(mods, Pack{})
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

	tree, err := idx.ConfigFileTree([]*Mod{defaultsMod, alphaMod}, Pack{})
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

func TestConfigEntry(t *testing.T) {
	tests := []struct {
		name      string
		rel       string
		isDir     bool
		configDir string
		want      string
	}{
		{"file", "config/sodium.json", false, "", "config/sodium.json"},
		{"folder gets a trailing slash", "config/sodium", true, "", "config/sodium/"},
		{"folder that has one already", "config/sodium/", true, "", "config/sodium/"},
		{"file in the config dir", "configureddefaults/config/sodium.json", false, "configureddefaults", "config/sodium.json"},
		{"folder in the config dir", "configureddefaults/config/sodium", true, "configureddefaults", "config/sodium/"},
		{"a path outside the config dir is left as it is", "config/sodium.json", false, "configureddefaults", "config/sodium.json"},
		{"the config dir itself has no entry", "configureddefaults", true, "configureddefaults", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfigEntry(tt.rel, tt.isDir, tt.configDir); got != tt.want {
				t.Errorf("ConfigEntry(%q, %v, %q) = %q, want %q", tt.rel, tt.isDir, tt.configDir, got, tt.want)
			}
		})
	}
}

func TestClaimingEntries(t *testing.T) {
	mod := modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar",
		"config/alpha.json", "config/alpha/", "config/alpha/sub.json", "config/other.json", "config/alpha/")

	tests := []struct {
		name string
		path string
		want []string
	}{
		{"an entry that names the file", "config/alpha.json", []string{"config/alpha.json"}},
		{"a folder entry", "config/alpha/other.json", []string{"config/alpha/"}},
		{"the file and its folder, each once", "config/alpha/sub.json", []string{"config/alpha/", "config/alpha/sub.json"}},
		{"a path nothing claims", "config/orphan.json", nil},
		{"a folder entry doesn't claim a file that only shares its name", "config/alpha-extra.json", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClaimingEntries(mod.ConfigEntries(), "", tt.path)
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ClaimingEntries(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}

	t.Run("a mod with no config-files claims nothing", func(t *testing.T) {
		if got := ClaimingEntries(modAt("Bare", "mods/bare.pw.toml", "bare.jar").ConfigEntries(), "", "config/alpha.json"); len(got) != 0 {
			t.Errorf("ClaimingEntries() = %v, want none", got)
		}
	})

	t.Run("entries are resolved against the config dir, and returned as written", func(t *testing.T) {
		got := ClaimingEntries(mod.ConfigEntries(), "configureddefaults", "configureddefaults/config/alpha/sub.json")
		slices.Sort(got)
		if want := []string{"config/alpha/", "config/alpha/sub.json"}; !slices.Equal(got, want) {
			t.Errorf("ClaimingEntries() = %v, want %v", got, want)
		}
		if got := ClaimingEntries(mod.ConfigEntries(), "configureddefaults", "config/alpha.json"); len(got) != 0 {
			t.Errorf("a path outside the config dir: ClaimingEntries() = %v, want none", got)
		}
	})
}

// ClaimingEntries has to find exactly the entries that make ConfigFileTree list a file under a mod, or unclaiming what it
// returns wouldn't stop the mod claiming the file
func TestClaimingEntriesAgreeWithConfigFileTree(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "config/alpha.json", "config/alpha/sub.json", "config/orphan.json")
	mod := modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/alpha.json", "config/alpha/")

	tree, err := idx.ConfigFileTree([]*Mod{mod}, Pack{})
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}
	for _, p := range append(slices.Clone(tree.Mods[0].Files), tree.Unclaimed...) {
		claimed := slices.Contains(tree.Mods[0].Files, p)
		if got := len(ClaimingEntries(mod.ConfigEntries(), "", p)) > 0; got != claimed {
			t.Errorf("%s: ClaimingEntries says claimed = %v, ConfigFileTree says %v", p, got, claimed)
		}
	}
}

func TestEntryClaims(t *testing.T) {
	tests := []struct {
		entry, configDir, path string
		want                   bool
	}{
		{"config/a.json", "", "config/a.json", true},
		{"config/a.json", "", "config/a.json.bak", false},
		{"config/a/", "", "config/a/b.json", true},
		{"config/a/", "", "config/ab.json", false},
		{"config/a/", "configureddefaults", "configureddefaults/config/a/b.json", true},
		{"config/a/", "configureddefaults", "config/a/b.json", false},
	}
	for _, tt := range tests {
		if got := EntryClaims(tt.entry, tt.configDir, tt.path); got != tt.want {
			t.Errorf("EntryClaims(%q, %q, %q) = %v, want %v", tt.entry, tt.configDir, tt.path, got, tt.want)
		}
	}
}

// packOf makes a pack that has the mod loader neoforge and these config files, by owner
func packOf(configFiles map[string][]string) Pack {
	return Pack{Versions: map[string]string{"minecraft": "1.21.1", "neoforge": "21.1.0"}, ConfigFiles: configFiles}
}

func TestConfigFileTreeHasTheOwnersThatAreNotModsFirst(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "options.txt", "config/neoforge-common.toml", "config/neoforge-client.toml",
		"config/alpha.json", "config/orphan.json")

	pack := packOf(map[string][]string{
		"neoforge": {"config/neoforge-common.toml", "config/neoforge-client.toml", "config/neoforge-old.toml"},
		"pack":     {"options.txt"},
	})
	mods := []*Mod{modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/alpha.json")}
	tree, err := idx.ConfigFileTree(mods, pack)
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}

	// The pack is first, whatever the keys sort as, then the loader, and the mods are in their own list
	if len(tree.Owners) != 2 || tree.Owners[0].Owner != "pack" || tree.Owners[1].Owner != "neoforge" {
		t.Fatalf("Owners = %+v, want the pack and then neoforge", tree.Owners)
	}
	if got := tree.Owners[0]; got.Name != "Pack" || !slices.Equal(got.Files, []string{"options.txt"}) || len(got.Missing) != 0 {
		t.Errorf("the pack is %+v, want options.txt claimed and nothing missing", got)
	}
	loader := tree.Owners[1]
	if loader.Name != "NeoForge" {
		t.Errorf("the loader is named %q, want it written for people", loader.Name)
	}
	if want := []string{"config/neoforge-client.toml", "config/neoforge-common.toml"}; !slices.Equal(loader.Files, want) {
		t.Errorf("the loader's files are %v, want %v, sorted", loader.Files, want)
	}
	if want := []string{"config/neoforge-old.toml"}; !slices.Equal(loader.Missing, want) {
		t.Errorf("the loader's missing entries are %v, want %v", loader.Missing, want)
	}
	if want := []string{"config/neoforge-common.toml", "config/neoforge-client.toml", "config/neoforge-old.toml"}; !slices.Equal(loader.Entries, want) {
		t.Errorf("the loader's entries are %v, want all of them as they are written", loader.Entries)
	}
	if len(tree.Mods) != 1 || tree.Mods[0].Mod.Name != "Alpha" {
		t.Errorf("Mods = %+v, want just Alpha", tree.Mods)
	}
	// Only what nothing claims is left
	if !slices.Equal(tree.Unclaimed, []string{"config/orphan.json"}) {
		t.Errorf("Unclaimed = %v, want only what nothing claims, not what the pack or the loader do", tree.Unclaimed)
	}
}

func TestConfigFileTreeOmitsOwnersWithNothingToShow(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "options.txt")
	tree, err := idx.ConfigFileTree(nil, packOf(nil))
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}
	if len(tree.Owners) != 0 {
		t.Errorf("Owners = %+v, want none: nothing claims anything", tree.Owners)
	}
	if !slices.Equal(tree.Unclaimed, []string{"options.txt"}) {
		t.Errorf("Unclaimed = %v", tree.Unclaimed)
	}
}

// A file can belong to the loader and to a mod that configures it, and then is listed under both
func TestConfigFileTreeListsAFileUnderEveryOwnerThatClaimsIt(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "mods/alpha.pw.toml", "config/shared.toml")
	tree, err := idx.ConfigFileTree(
		[]*Mod{modAt("Alpha", "mods/alpha.pw.toml", "alpha.jar", "config/shared.toml")},
		packOf(map[string][]string{"neoforge": {"config/shared.toml"}}))
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}
	if len(tree.Owners) != 1 || len(tree.Owners[0].Files) != 1 || len(tree.Mods) != 1 || len(tree.Mods[0].Files) != 1 {
		t.Errorf("tree = %+v, want the file under both the loader and the mod", tree)
	}
	if len(tree.Unclaimed) != 0 {
		t.Errorf("Unclaimed = %v, want none", tree.Unclaimed)
	}
}

// An owner that the pack doesn't have is shown as it is written, rather than its files looking as if nothing claimed them
func TestConfigFileTreeShowsAnOwnerThatThePackDoesNotHave(t *testing.T) {
	idx := newIndexFixture(t)
	track(t, idx, "config/fabric.toml")
	tree, err := idx.ConfigFileTree(nil, packOf(map[string][]string{"fabric": {"config/fabric.toml"}}))
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}
	if len(tree.Owners) != 1 || tree.Owners[0].Owner != "fabric" || tree.Owners[0].Name != "fabric" {
		t.Errorf("Owners = %+v, want fabric, named as it is written", tree.Owners)
	}
}

// Entries of the pack and its loader are written without the config folder too, as mods' are
func TestConfigFileTreeResolvesOwnersAgainstTheConfigDir(t *testing.T) {
	registerConfigDirSource(t, "defaults", "configureddefaults")
	idx := newIndexFixture(t)
	track(t, idx, "mods/defaults.pw.toml", "configureddefaults/options.txt", "configureddefaults/config/neoforge-common.toml")
	defaults := modAt("Defaults", "mods/defaults.pw.toml", "defaults.jar")
	defaults.Update = map[string]map[string]interface{}{"defaults": {"version": "any"}}

	tree, err := idx.ConfigFileTree([]*Mod{defaults}, packOf(map[string][]string{
		"pack":     {"options.txt"},
		"neoforge": {"config/neoforge-common.toml"},
	}))
	if err != nil {
		t.Fatalf("ConfigFileTree() returned error: %v", err)
	}
	if len(tree.Owners) != 2 || !slices.Equal(tree.Owners[0].Files, []string{"configureddefaults/options.txt"}) ||
		!slices.Equal(tree.Owners[1].Files, []string{"configureddefaults/config/neoforge-common.toml"}) {
		t.Errorf("Owners = %+v, want the files in the config folder claimed by entries without it", tree.Owners)
	}
	if len(tree.Unclaimed) != 0 {
		t.Errorf("Unclaimed = %v, want none", tree.Unclaimed)
	}
}

func TestClaimingEntriesWorksOnAnOwnersEntries(t *testing.T) {
	entries := []string{"options.txt", "config/", "config/neoforge-common.toml"}
	got := ClaimingEntries(entries, "", "config/neoforge-common.toml")
	slices.Sort(got)
	if want := []string{"config/", "config/neoforge-common.toml"}; !slices.Equal(got, want) {
		t.Errorf("ClaimingEntries() = %v, want %v", got, want)
	}
	if got := ClaimingEntries(nil, "", "options.txt"); len(got) != 0 {
		t.Errorf("ClaimingEntries() of nothing = %v", got)
	}
}
