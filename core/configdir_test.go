package core

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// configDirSource is an updater for a mod that has the pack keep its files in a folder, standing in for Modrinth's
// Configured Defaults.
type configDirSource struct{ dir string }

func (configDirSource) ParseUpdate(raw map[string]interface{}) (interface{}, error) { return raw, nil }
func (configDirSource) CheckUpdate([]*Mod, Pack) ([]UpdateCheck, error)             { return nil, nil }
func (configDirSource) DoUpdate([]*Mod, []interface{}) error                        { return nil }
func (s configDirSource) ConfigDir(*Mod) string                                     { return s.dir }

func registerConfigDirSource(t *testing.T, name, dir string) {
	t.Helper()
	Updaters[name] = configDirSource{dir}
	t.Cleanup(func() { delete(Updaters, name) })
}

// refreshFixture writes the files, keyed by their path relative to the pack, into a temp pack and returns an empty
// index for it, and the pack's location
func refreshFixture(t *testing.T, files map[string]string) (*Index, string) {
	t.Helper()
	dir := t.TempDir()
	packFile := filepath.Join(dir, "pack.toml")
	indexFilePath := filepath.Join(dir, "index.toml")
	mustWriteFile(t, packFile, "name = \"Test\"\n")
	mustWriteFile(t, indexFilePath, "")
	for path, content := range files {
		mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(path)), content)
	}

	oldPackFile := viper.GetString("pack-file")
	viper.Set("pack-file", packFile)
	oldNoHashes := viper.GetBool("no-internal-hashes")
	viper.Set("no-internal-hashes", false)
	t.Cleanup(func() {
		viper.Set("pack-file", oldPackFile)
		viper.Set("no-internal-hashes", oldNoHashes)
	})

	return &Index{HashFormat: "sha256", indexFile: indexFilePath, packRoot: dir, Files: IndexFiles{}}, dir
}

func trackedPaths(idx *Index) []string {
	paths := make([]string, 0, len(idx.Files))
	for p := range idx.Files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	return paths
}

func mustRefresh(t *testing.T, idx *Index) {
	t.Helper()
	if err := idx.Refresh(); err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}
}

// refreshAgain saves the index and refreshes it as a later run of the command would, from what was saved. Refresh
// marks the entries it finds on the index itself, so one that is refreshed again in memory can't lose any.
func refreshAgain(t *testing.T, idx *Index) *Index {
	t.Helper()
	if err := idx.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	loaded, err := LoadIndex(idx.indexFile)
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	mustRefresh(t, &loaded)
	return &loaded
}

func TestIndexRefreshTracksOnlyTheConfigDir(t *testing.T) {
	registerConfigDirSource(t, "defaults", "configureddefaults")
	idx, _ := refreshFixture(t, map[string]string{
		"mods/defaults.pw.toml": modTOML("defaults", "", "defaults", "x"),
		"mods/plain.pw.toml":    modTOML("plain", "", "", "y"),

		"configureddefaults/config/sodium.json": "{}",
		"configureddefaults/options.txt":        "fov:90",
		// Still subject to .packwizignore inside the folder
		".packwizignore":                        "configureddefaults/private/**\n",
		"configureddefaults/private/secret.txt": "not for the pack",

		// Outside the folder, and so not the pack's to ship
		"config/sodium.json":                "{}",
		"options.txt":                       "fov:70",
		"mods/loose.jar":                    "jar",
		"configureddefaults-old/config.txt": "a different folder that starts the same",
	})

	mustRefresh(t, idx)

	want := []string{
		"configureddefaults/config/sodium.json",
		"configureddefaults/options.txt",
		"mods/defaults.pw.toml",
		"mods/plain.pw.toml",
	}
	if got := trackedPaths(idx); !reflect.DeepEqual(got, want) {
		t.Errorf("tracked = %v, want %v", got, want)
	}
	for _, p := range []string{"mods/defaults.pw.toml", "mods/plain.pw.toml"} {
		if !idx.Files[p].IsMetaFile() {
			t.Errorf("expected %s to be marked as a metadata file", p)
		}
	}
	if idx.Files["configureddefaults/options.txt"].IsMetaFile() {
		t.Error("did not expect configureddefaults/options.txt to be marked as a metadata file")
	}
}

func TestIndexRefreshTracksEverythingWhenNoModHasAConfigDir(t *testing.T) {
	// The source is known, but no mod uses it
	registerConfigDirSource(t, "defaults", "configureddefaults")
	idx, _ := refreshFixture(t, map[string]string{
		"mods/plain.pw.toml":                    modTOML("plain", "", "", "y"),
		"configureddefaults/config/sodium.json": "{}",
		"config/sodium.json":                    "{}",
		"options.txt":                           "fov:70",
	})

	mustRefresh(t, idx)

	want := []string{"config/sodium.json", "configureddefaults/config/sodium.json", "mods/plain.pw.toml", "options.txt"}
	if got := trackedPaths(idx); !reflect.DeepEqual(got, want) {
		t.Errorf("tracked = %v, want %v", got, want)
	}
}

func TestIndexRefreshFollowsTheModThatHasTheConfigDir(t *testing.T) {
	registerConfigDirSource(t, "defaults", "configureddefaults")
	idx, dir := refreshFixture(t, map[string]string{
		"mods/plain.pw.toml":                    modTOML("plain", "", "", "y"),
		"configureddefaults/config/sodium.json": "{}",
		"config/sodium.json":                    "{}",
	})

	mustRefresh(t, idx)
	if _, ok := idx.Files["config/sodium.json"]; !ok {
		t.Fatal("expected config/sodium.json to be tracked while no mod has a config folder")
	}

	// Adding the mod takes what is outside the folder out of the index...
	defaults := filepath.Join(dir, "mods", "defaults.pw.toml")
	mustWriteFile(t, defaults, modTOML("defaults", "", "defaults", "x"))
	idx = refreshAgain(t, idx)
	want := []string{"configureddefaults/config/sodium.json", "mods/defaults.pw.toml", "mods/plain.pw.toml"}
	if got := trackedPaths(idx); !reflect.DeepEqual(got, want) {
		t.Errorf("tracked with the mod = %v, want %v", got, want)
	}

	// ...and removing it puts it back
	if err := os.Remove(defaults); err != nil {
		t.Fatalf("failed to remove %s: %v", defaults, err)
	}
	idx = refreshAgain(t, idx)
	want = []string{"config/sodium.json", "configureddefaults/config/sodium.json", "mods/plain.pw.toml"}
	if got := trackedPaths(idx); !reflect.DeepEqual(got, want) {
		t.Errorf("tracked without the mod = %v, want %v", got, want)
	}
}

func TestIndexRefreshKeepsMetadataFilesThatTheIndexAlreadyMarks(t *testing.T) {
	registerConfigDirSource(t, "defaults", "configureddefaults")
	idx, _ := refreshFixture(t, map[string]string{
		"mods/defaults.pw.toml": modTOML("defaults", "", "defaults", "x"),
		// The old extension isn't recognised as a metadata file's, but a pack that has one marked keeps it so
		"mods/old.toml": modTOML("old", "", "", "z"),
	})
	idx.Files = IndexFiles{"mods/old.toml": &indexFile{File: "mods/old.toml", MetaFile: true}}

	mustRefresh(t, idx)

	want := []string{"mods/defaults.pw.toml", "mods/old.toml"}
	if got := trackedPaths(idx); !reflect.DeepEqual(got, want) {
		t.Errorf("tracked = %v, want %v", got, want)
	}
	if !idx.Files["mods/old.toml"].IsMetaFile() {
		t.Error("expected mods/old.toml to still be a metadata file")
	}
}

func TestIndexRefreshFailsWhenAMetadataFileCannotBeRead(t *testing.T) {
	idx, _ := refreshFixture(t, map[string]string{
		"mods/broken.pw.toml": "name = [",
	})

	err := idx.Refresh()
	if err == nil {
		t.Fatal("Refresh() returned no error for a metadata file that isn't valid TOML")
	}
	if !strings.Contains(err.Error(), "broken.pw.toml") {
		t.Errorf("Refresh() error = %q, want it to name the file", err)
	}
}

func TestConfigDir(t *testing.T) {
	registerConfigDirSource(t, "first", "one")
	registerConfigDirSource(t, "second", "two")
	registerConfigDirSource(t, "none", "")
	idx := versionFixture(t, map[string]string{
		"mods/a.pw.toml": modTOML("a", "", "first", "1"),
		"mods/b.pw.toml": modTOML("b", "", "second", "2"),
		"mods/c.pw.toml": modTOML("c", "", "none", "3"),
		"mods/d.pw.toml": modTOML("d", "", "", "4"),
	})
	path := func(name string) string { return idx.ResolveIndexPath("mods/" + name + ".pw.toml") }

	for _, tc := range []struct {
		name  string
		metas []string
		want  string
	}{
		{"no mods", nil, ""},
		{"mods with no config folder", []string{path("c"), path("d")}, ""},
		{"the first mod that has one", []string{path("d"), path("b"), path("a")}, "two"},
		{"in the order given", []string{path("a"), path("b")}, "one"},
	} {
		got, err := configDir(tc.metas)
		if err != nil {
			t.Errorf("%s: configDir() returned error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: configDir() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
