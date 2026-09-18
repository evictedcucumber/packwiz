package modrinth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/spf13/viper"
)

func boolPtr(b bool) *bool { return &b }

// setupPackFixture creates a minimal pack.toml + index.toml on disk, points
// viper at them, and restores all touched viper keys on cleanup.
func setupPackFixture(t *testing.T) (core.Pack, core.Index) {
	t.Helper()
	cmdtest.Chdir(t)
	// pack-file is kept relative (matching the CLI's own default of
	// "pack.toml"), since createFileMeta joins meta-folder-base/folder as a
	// relative path and expects it to resolve against the same root as the
	// index's packRoot (itself derived from pack-file).
	packFile := "pack.toml"

	for _, kv := range []struct{ key, value string }{
		{"pack-file", packFile},
		{"meta-folder-base", ""},
		{"meta-folder", ""},
	} {
		old := viper.GetString(kv.key)
		viper.Set(kv.key, kv.value)
		t.Cleanup(func(key, oldValue string) func() {
			return func() { viper.Set(key, oldValue) }
		}(kv.key, old))
	}

	pack := core.Pack{
		Name:       "Test Pack",
		PackFormat: core.CurrentPackFormat,
		Versions:   map[string]string{"minecraft": "1.20.1"},
	}
	pack.Index.File = "index.toml"
	if err := pack.Write(); err != nil {
		t.Fatalf("failed to write pack.toml fixture: %v", err)
	}

	if err := os.WriteFile("index.toml", []byte("hash-format = \"sha256\"\n"), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}
	// Loaded the same way the CLI itself would (relative to pack-file's
	// directory), so its packRoot stays relative like pack-file/meta-folder
	// do - keeping filepath.Rel happy when createFileMeta later resolves a
	// relative meta-folder path against it.
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	return pack, index
}

func testProjectAndFile() (*modrinthApi.Project, *modrinthApi.Version, *modrinthApi.File) {
	project := &modrinthApi.Project{
		ID: strPtr("p1"), Title: strPtr("Test Project"), Slug: strPtr("test-project"),
		ProjectType: strPtr("mod"), ClientSide: strPtr("required"), ServerSide: strPtr("required"),
	}
	version := &modrinthApi.Version{ID: strPtr("v1"), VersionNumber: strPtr("1.2.3")}
	file := &modrinthApi.File{
		URL: strPtr("https://example.com/test.jar"), Filename: strPtr("test.jar"), Primary: boolPtr(true),
		Hashes: map[string]string{"sha512": "deadbeef"},
	}
	version.Files = []*modrinthApi.File{file}
	return project, version, file
}

func TestCreateFileMetaWritesModAndUpdatesIndex(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, file := testProjectAndFile()

	if err := createFileMeta(project, version, file, pack, &index, "", false); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}

	dir := filepath.Dir(viper.GetString("pack-file"))
	metaPath := filepath.Join(dir, "mods", "test-project"+core.MetaExtension)
	mod, err := core.LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.Name != "Test Project" {
		t.Errorf("Name = %q, want %q", mod.Name, "Test Project")
	}
	if mod.FileName != "test.jar" {
		t.Errorf("FileName = %q, want %q", mod.FileName, "test.jar")
	}
	if mod.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", mod.Version, "1.2.3")
	}
	if mod.Side != core.UniversalSide {
		t.Errorf("Side = %q, want %q", mod.Side, core.UniversalSide)
	}
	if mod.Download.HashFormat != "sha512" || mod.Download.Hash != "deadbeef" {
		t.Errorf("Download = %+v, want HashFormat=sha512 Hash=deadbeef", mod.Download)
	}
	if mod.AddedAsDependency {
		t.Error("AddedAsDependency = true, want false")
	}

	relPath, err := index.RelIndexPath(metaPath)
	if err != nil {
		t.Fatalf("RelIndexPath() returned error: %v", err)
	}
	if _, ok := index.Files[relPath]; !ok {
		t.Errorf("expected %q to be tracked in the index after createFileMeta, got %v", relPath, index.Files)
	}
}

func TestCreateFileMetaWithoutVersionNumber(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, file := testProjectAndFile()
	version.VersionNumber = nil

	if err := createFileMeta(project, version, file, pack, &index, "", false); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}

	dir := filepath.Dir(viper.GetString("pack-file"))
	mod, err := core.LoadMod(filepath.Join(dir, "mods", "test-project"+core.MetaExtension))
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.Version != "" {
		t.Errorf("Version = %q, want empty when Modrinth reports no version number", mod.Version)
	}
}

func TestCreateFileMetaAsDependency(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, file := testProjectAndFile()

	if err := createFileMeta(project, version, file, pack, &index, "", true); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}

	dir := filepath.Dir(viper.GetString("pack-file"))
	metaPath := filepath.Join(dir, "mods", "test-project"+core.MetaExtension)
	mod, err := core.LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if !mod.AddedAsDependency {
		t.Error("AddedAsDependency = false, want true")
	}
}

func TestCreateFileMetaNoHashReturnsError(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, _ := testProjectAndFile()
	file := &modrinthApi.File{URL: strPtr("https://example.com/test.jar"), Filename: strPtr("test.jar")}

	if err := createFileMeta(project, version, file, pack, &index, "", false); err == nil {
		t.Error("expected an error when the file has no hashes, got nil")
	}
}

func TestInstallVersionNoFilesReturnsError(t *testing.T) {
	project := &modrinthApi.Project{ID: strPtr("p1"), Title: strPtr("Test Project")}
	version := &modrinthApi.Version{ID: strPtr("v1")}

	if err := installVersion(project, version, "", core.Pack{}, &core.Index{}, ""); err == nil {
		t.Error("expected an error when the version has no files, got nil")
	}
}

func TestInstallVersionNoDependenciesWritesPackAndIndex(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, _ := testProjectAndFile()

	if err := installVersion(project, version, "", pack, &index, ""); err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	reloaded, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if reloaded.Index.Hash == "" {
		t.Error("Index.Hash is empty after installVersion(); expected pack.UpdateIndexHash() to have run")
	}

	dir := filepath.Dir(viper.GetString("pack-file"))
	metaPath := filepath.Join(dir, "mods", "test-project"+core.MetaExtension)
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("expected mod metadata file to exist at %s: %v", metaPath, err)
	}
}
