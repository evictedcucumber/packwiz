package core

import (
	"slices"
	"strings"
	"testing"
)

// modAt builds a Mod with its metadata file at metaPath, destination fileName, and the given config-files
func modAt(metaPath, fileName string, configFiles ...string) *Mod {
	m := &Mod{FileName: fileName, ConfigFiles: configFiles}
	m.SetMetaPath(metaPath)
	return m
}

func newIndexFixture(t *testing.T) *Index {
	t.Helper()
	idx := &Index{HashFormat: "sha256", packRoot: "."}
	return idx
}

func TestConfigFilesExcludesMetaFilesAndAModsOwnDestFile(t *testing.T) {
	idx := newIndexFixture(t)
	if err := idx.RefreshFileWithHash("mods/alpha.pw.toml", "sha256", "h", true); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}
	if err := idx.RefreshFileWithHash("mods/alpha.jar", "sha256", "h", false); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}

	mods := []*Mod{modAt("mods/alpha.pw.toml", "alpha.jar")}
	files, err := idx.ConfigFiles(mods)
	if err != nil {
		t.Fatalf("ConfigFiles() returned error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("ConfigFiles() = %v, want none (the metadata file and the mod's own jar shouldn't be listed)", files)
	}
}

func TestConfigFilesReportsWhatIsClaimed(t *testing.T) {
	idx := newIndexFixture(t)
	for _, p := range []string{
		"mods/alpha.pw.toml", "mods/alpha.jar",
		"config/alpha.json", "config/alpha/sub.json", "config/orphan.json",
	} {
		meta := p == "mods/alpha.pw.toml"
		if err := idx.RefreshFileWithHash(p, "sha256", "h", meta); err != nil {
			t.Fatalf("RefreshFileWithHash(%q) returned error: %v", p, err)
		}
	}

	mods := []*Mod{modAt("mods/alpha.pw.toml", "alpha.jar", "config/alpha.json", "config/alpha/")}
	files, err := idx.ConfigFiles(mods)
	if err != nil {
		t.Fatalf("ConfigFiles() returned error: %v", err)
	}

	got := make(map[string]bool)
	for _, f := range files {
		got[f.Path] = f.Claimed
	}
	want := map[string]bool{
		"config/alpha.json":     true,  // claimed exactly
		"config/alpha/sub.json": true,  // claimed by the "config/alpha/" folder entry
		"config/orphan.json":    false, // nothing claims it
	}
	if !mapsEqual(got, want) {
		t.Errorf("ConfigFiles() = %v, want %v", got, want)
	}
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestConfigFilesResolvedAgainstTheConfigDir checks that config-files entries are written as if the pack kept its
// files at the root of the game directory (as it normally does), and are resolved against a mod's ConfigDir (e.g.
// Configured Defaults) when the pack has one: "config/alpha.json" claims "configureddefaults/config/alpha.json".
func TestConfigFilesResolvedAgainstTheConfigDir(t *testing.T) {
	registerConfigDirSource(t, "defaults", "configureddefaults")

	idx := newIndexFixture(t)
	for _, p := range []string{
		"mods/defaults.pw.toml", "mods/alpha.pw.toml",
		"configureddefaults/config/alpha.json", "configureddefaults/config/alpha/sub.json",
		"configureddefaults/config/orphan.json",
	} {
		meta := strings.HasSuffix(p, ".pw.toml")
		if err := idx.RefreshFileWithHash(p, "sha256", "h", meta); err != nil {
			t.Fatalf("RefreshFileWithHash(%q) returned error: %v", p, err)
		}
	}

	defaultsMod := &Mod{Update: map[string]map[string]interface{}{"defaults": {}}}
	defaultsMod.SetMetaPath("mods/defaults.pw.toml")
	alphaMod := modAt("mods/alpha.pw.toml", "alpha.jar", "config/alpha.json", "config/alpha/")

	files, err := idx.ConfigFiles([]*Mod{defaultsMod, alphaMod})
	if err != nil {
		t.Fatalf("ConfigFiles() returned error: %v", err)
	}

	got := make(map[string]bool)
	for _, f := range files {
		got[f.Path] = f.Claimed
	}
	want := map[string]bool{
		"configureddefaults/config/alpha.json":     true,
		"configureddefaults/config/alpha/sub.json": true,
		"configureddefaults/config/orphan.json":    false,
	}
	if !mapsEqual(got, want) {
		t.Errorf("ConfigFiles() = %v, want %v", got, want)
	}
}

func TestConfigFilesSortedByPath(t *testing.T) {
	idx := newIndexFixture(t)
	for _, p := range []string{"config/z.json", "config/a.json", "config/m.json"} {
		if err := idx.RefreshFileWithHash(p, "sha256", "h", false); err != nil {
			t.Fatalf("RefreshFileWithHash(%q) returned error: %v", p, err)
		}
	}

	files, err := idx.ConfigFiles(nil)
	if err != nil {
		t.Fatalf("ConfigFiles() returned error: %v", err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	want := []string{"config/a.json", "config/m.json", "config/z.json"}
	if !slices.Equal(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}
