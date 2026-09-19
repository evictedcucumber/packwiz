package modrinth

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestConfigDirIsOnlyConfiguredDefaults(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestProject(t, pack, &index, configuredDefaultsProjectID, "configured-defaults", "Configured Defaults")
	addTestProject(t, pack, &index, "AANobbMI", "sodium", "Sodium")

	mods, err := index.LoadAllMods()
	if err != nil {
		t.Fatalf("LoadAllMods() returned error: %v", err)
	}
	for _, mod := range mods {
		want := ""
		if mod.Name == "Configured Defaults" {
			want = "configureddefaults"
		}
		if got := (mrUpdater{}).ConfigDir(mod); got != want {
			t.Errorf("ConfigDir(%s) = %q, want %q", mod.Name, got, want)
		}
	}

	// A mod that isn't from Modrinth has no update data for it to go by
	if got := (mrUpdater{}).ConfigDir(&core.Mod{Name: "Local"}); got != "" {
		t.Errorf("ConfigDir(a mod with no update data) = %q, want none", got)
	}
}

// refreshedFiles adds files to a pack with the given projects, saves and reloads its index as a command would, refreshes
// it, and returns the paths of the metadata files and the other files it tracks
func refreshedFiles(t *testing.T, projects map[string]string, files ...string) (metaFiles, others []string) {
	t.Helper()
	pack, index := setupPackFixture(t)
	for id, slug := range projects {
		addTestProject(t, pack, &index, id, slug, slug)
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatalf("failed to create the folder of %s: %v", f, err)
		}
		if err := os.WriteFile(f, []byte("contents of "+f), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", f, err)
		}
	}
	if err := index.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	if err := index.Refresh(); err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}
	for _, p := range slices.Sorted(maps.Keys(index.Files)) {
		if index.Files[p].IsMetaFile() {
			metaFiles = append(metaFiles, p)
		} else {
			others = append(others, p)
		}
	}
	return metaFiles, others
}

func TestRefreshWithConfiguredDefaultsTracksOnlyItsFolder(t *testing.T) {
	metaFiles, others := refreshedFiles(t,
		map[string]string{configuredDefaultsProjectID: "configured-defaults", "AANobbMI": "sodium"},
		"configureddefaults/config/sodium.json", "configureddefaults/options.txt",
		"config/sodium.json", "options.txt", "shaderpacks/pack.zip",
	)

	if want := []string{"configureddefaults/config/sodium.json", "configureddefaults/options.txt"}; !reflect.DeepEqual(others, want) {
		t.Errorf("tracked files = %v, want %v", others, want)
	}
	if len(metaFiles) != 2 {
		t.Errorf("tracked metadata files = %v, want both mods", metaFiles)
	}
}

func TestRefreshWithoutConfiguredDefaultsTracksEveryFile(t *testing.T) {
	metaFiles, others := refreshedFiles(t,
		map[string]string{"AANobbMI": "sodium"},
		"configureddefaults/config/sodium.json", "config/sodium.json", "options.txt",
	)

	want := []string{"config/sodium.json", "configureddefaults/config/sodium.json", "options.txt"}
	if !reflect.DeepEqual(others, want) {
		t.Errorf("tracked files = %v, want %v", others, want)
	}
	if len(metaFiles) != 1 {
		t.Errorf("tracked metadata files = %v, want the one mod", metaFiles)
	}
}
