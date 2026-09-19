package modrinth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/jarcoal/httpmock"
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

// testVersion is a version of the test project (see testProjectAndFile), whose one file is named after its number
func testVersion(id, number string) *modrinthApi.Version {
	return &modrinthApi.Version{
		ID: strPtr(id), ProjectID: strPtr("p1"), VersionNumber: strPtr(number), VersionType: strPtr("release"),
		DatePublished: timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		Files: []*modrinthApi.File{{
			URL: strPtr("https://example.com/test-" + number + ".jar"), Filename: strPtr("test-" + number + ".jar"),
			Primary: boolPtr(true), Hashes: map[string]string{"sha512": "hash-" + id},
		}},
	}
}

// testMetaPath is where the test project's metadata goes
var testMetaPath = filepath.Join("mods", "test-project"+core.MetaExtension)

// addTestVersion adds version of the test project to the pack, as 'mr add' would
func addTestVersion(t *testing.T, pack core.Pack, index *core.Index, version *modrinthApi.Version, releaseType string) {
	t.Helper()
	project, _, _ := testProjectAndFile()
	if err := installVersion(project, version, "", pack, index, releaseType); err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}

func loadTestMod(t *testing.T) core.Mod {
	t.Helper()
	mod, err := core.LoadMod(testMetaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	return mod
}

// editTestMod changes the test project's metadata on disk, as the user (or another command) might have
func editTestMod(t *testing.T, edit func(*core.Mod)) {
	t.Helper()
	mod := loadTestMod(t)
	edit(&mod)
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
}

// addAgain adds version of the test project the way 'mr add' does when it has been given a version, returning
// what it printed and the error it failed with
func addAgain(t *testing.T, pack core.Pack, index *core.Index, version *modrinthApi.Version, releaseType string) (string, error) {
	t.Helper()
	project, _, _ := testProjectAndFile()
	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installVersion(project, version, "", pack, index, releaseType) })
	return out, err
}

func TestInstallVersionAlreadyUpToDateChangesNothing(t *testing.T) {
	pack, index := setupPackFixture(t)
	v1 := testVersion("v1", "1.0.0")
	addTestVersion(t, pack, &index, v1, "")
	before := readFile(t, testMetaPath)
	cmdtest.SetStdin(t, "y\n")

	out, err := addAgain(t, pack, &index, v1, "")

	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
	if !strings.Contains(out, "already added and up to date") {
		t.Errorf("output = %q, want it to say the mod is already added and up to date", out)
	}
	if strings.Contains(out, "Would you like") {
		t.Errorf("output = %q, want no prompt when there is nothing to update", out)
	}
	if after := readFile(t, testMetaPath); after != before {
		t.Errorf("metadata changed although the mod is up to date:\n%s\nwas:\n%s", after, before)
	}
}

func TestInstallVersionUpdatesAddedMod(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	// What the user has set on the mod since it was added, which an update has to leave alone
	editTestMod(t, func(m *core.Mod) {
		m.AddedAsDependency = true
		m.Option = &core.ModOption{Optional: true, Description: "kept"}
	})
	cmdtest.SetStdin(t, "y\n")

	out, err := addAgain(t, pack, &index, testVersion("v2", "2.0.0"), "")

	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
	if !strings.Contains(out, "Update available: 1.0.0 -> 2.0.0") {
		t.Errorf("output = %q, want it to say what the update is", out)
	}
	if !strings.Contains(out, "successfully updated") {
		t.Errorf("output = %q, want it to say the mod was updated, not added", out)
	}

	mod := loadTestMod(t)
	if mod.FileName != "test-2.0.0.jar" || mod.Version != "2.0.0" {
		t.Errorf("FileName, Version = %q, %q; want test-2.0.0.jar, 2.0.0", mod.FileName, mod.Version)
	}
	if mod.Download.URL != "https://example.com/test-2.0.0.jar" || mod.Download.Hash != "hash-v2" {
		t.Errorf("Download = %+v, want the new version's file", mod.Download)
	}
	data, _ := modrinthUpdateData(&mod)
	if data.ProjectID != "p1" || data.InstalledVersion != "v2" {
		t.Errorf("update data = %+v, want project p1 at version v2", data)
	}
	if !mod.AddedAsDependency {
		t.Error("AddedAsDependency = false, want the update to keep it")
	}
	if mod.Option == nil || mod.Option.Description != "kept" {
		t.Errorf("Option = %+v, want the update to keep it", mod.Option)
	}

	// The index on disk knows about the one mod, and its hash
	reloaded, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	onDisk, err := reloaded.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	if len(onDisk.Files) != 1 {
		t.Errorf("index has %d files, want only the one mod: %v", len(onDisk.Files), onDisk.Files)
	}
}

func TestInstallVersionDeclinedUpdateChangesNothing(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	before := readFile(t, testMetaPath)
	cmdtest.SetStdin(t, "n\n")

	out, err := addAgain(t, pack, &index, testVersion("v2", "2.0.0"), "")

	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
	if !strings.Contains(out, "Would you like to update it?") || !strings.Contains(out, "Cancelled!") {
		t.Errorf("output = %q, want it to ask, and say it was cancelled", out)
	}
	if after := readFile(t, testMetaPath); after != before {
		t.Errorf("metadata changed although the update was declined:\n%s", after)
	}
}

func TestInstallVersionUpdateShowsFileNamesForModsWithoutAVersion(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	editTestMod(t, func(m *core.Mod) { m.Version = "" }) // as added before versions were recorded
	cmdtest.SetStdin(t, "n\n")

	out, _ := addAgain(t, pack, &index, testVersion("v2", "2.0.0"), "")

	if !strings.Contains(out, "test-1.0.0.jar -> test-2.0.0.jar") {
		t.Errorf("output = %q, want the file names, as it has no version to show", out)
	}
}

func TestInstallVersionPinnedModIsNotUpdated(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	editTestMod(t, func(m *core.Mod) { m.Pin = true })
	before := readFile(t, testMetaPath)
	// Even -y, which answers every prompt, must not get an update past a pin
	cmdtest.SetViperBool(t, "non-interactive", true)

	out, err := addAgain(t, pack, &index, testVersion("v2", "2.0.0"), "")

	if err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Errorf("error = %v, want one saying the mod is pinned", err)
	}
	if strings.Contains(out, "Would you like") {
		t.Errorf("output = %q, want no prompt for a pinned mod", out)
	}
	if after := readFile(t, testMetaPath); after != before {
		t.Errorf("metadata of a pinned mod changed:\n%s", after)
	}
}

func TestInstallVersionPinnedModAtThatVersionIsUpToDate(t *testing.T) {
	pack, index := setupPackFixture(t)
	v1 := testVersion("v1", "1.0.0")
	addTestVersion(t, pack, &index, v1, "")
	editTestMod(t, func(m *core.Mod) { m.Pin = true })

	out, err := addAgain(t, pack, &index, v1, "")

	if err != nil {
		t.Errorf("installVersion() returned error %v, want none: a pin only stops changes, and there is none", err)
	}
	if !strings.Contains(out, "already added and up to date") {
		t.Errorf("output = %q, want it to say the mod is up to date", out)
	}
}

// The mod is found by its project, so it is updated where it is rather than added a second time under the name the
// project has now
func TestInstallVersionUpdatesModAddedUnderAnotherName(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	cmdtest.SetStdin(t, "y\n")

	project, _, _ := testProjectAndFile()
	project.Slug = strPtr("renamed-project")
	var err error
	cmdtest.CaptureStdout(t, func() { err = installVersion(project, testVersion("v2", "2.0.0"), "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
	if mod := loadTestMod(t); mod.FileName != "test-2.0.0.jar" {
		t.Errorf("FileName = %q, want the existing metadata updated", mod.FileName)
	}
	if _, err := os.Stat(filepath.Join("mods", "renamed-project"+core.MetaExtension)); !os.IsNotExist(err) {
		t.Errorf("stat of a second metadata file = %v, want it not to exist", err)
	}
}

func TestInstallVersionUpdateRecordsReleaseType(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	cmdtest.SetStdin(t, "y\n")

	if _, err := addAgain(t, pack, &index, testVersion("v2", "2.0.0"), "beta"); err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	mod := loadTestMod(t)
	if data, _ := modrinthUpdateData(&mod); data.ReleaseType != "beta" {
		t.Errorf("ReleaseType = %q, want the --release-type it was updated with, so 'packwiz update' keeps to it", data.ReleaseType)
	}
}

func TestInstallVersionUpdateKeepsReleaseType(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "beta")
	cmdtest.SetStdin(t, "y\n")

	if _, err := addAgain(t, pack, &index, testVersion("v2", "2.0.0"), ""); err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	mod := loadTestMod(t)
	if data, _ := modrinthUpdateData(&mod); data.ReleaseType != "beta" {
		t.Errorf("ReleaseType = %q, want the one the mod already had", data.ReleaseType)
	}
}

// versionWithNewDependency is version 2 of the test project, which needs a mod that version 1 didn't and that isn't in
// the pack, and Modrinth serving that mod
func versionWithNewDependency(t *testing.T) *modrinthApi.Version {
	t.Helper()
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewStringResponder(200, `[{"id":"dep1","slug":"dep-mod","title":"Dep Mod","project_type":"mod","client_side":"required","server_side":"required"}]`))
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/dep1/version`,
		httpmock.NewStringResponder(200, `[{"id":"dv1","project_id":"dep1","version_number":"1.0.0","version_type":"release","date_published":"2024-01-01T00:00:00Z",
			"files":[{"url":"https://example.com/dep.jar","filename":"dep.jar","primary":true,"hashes":{"sha512":"depdead"}}]}]`))

	v2 := testVersion("v2", "2.0.0")
	v2.Dependencies = []*modrinthApi.Dependency{{ProjectID: strPtr("dep1"), DependencyType: strPtr("required")}}
	return v2
}

var depMetaPath = filepath.Join("mods", "dep-mod"+core.MetaExtension)

// Updating can bring in a dependency the old version didn't have, and 'mr add' offers those as it does for a new mod.
// It is a second prompt after the one for the update, and both are answered from the same piped input.
func TestInstallVersionUpdateOffersNewDependencies(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	v2 := versionWithNewDependency(t)
	cmdtest.SetStdin(t, "y\ny\n") // yes to the update, and yes to the dependency

	if _, err := addAgain(t, pack, &index, v2, ""); err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	dep, err := core.LoadMod(depMetaPath)
	if err != nil {
		t.Fatalf("expected the new dependency to be added: %v", err)
	}
	if !dep.AddedAsDependency {
		t.Error("AddedAsDependency = false, want the dependency marked as one")
	}
	if mod := loadTestMod(t); mod.Version != "2.0.0" {
		t.Errorf("Version = %q, want the mod itself updated too", mod.Version)
	}
}

func TestInstallVersionUpdateWithoutNewDependencies(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	v2 := versionWithNewDependency(t)
	cmdtest.SetStdin(t, "y\nn\n") // yes to the update, and no to the dependency

	if _, err := addAgain(t, pack, &index, v2, ""); err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	if _, err := os.Stat(depMetaPath); !os.IsNotExist(err) {
		t.Errorf("stat of the dependency's metadata = %v, want it not to exist as it was declined", err)
	}
	if mod := loadTestMod(t); mod.Version != "2.0.0" {
		t.Errorf("Version = %q, want the mod updated anyway", mod.Version)
	}
}

// serveProjectVersions answers Modrinth's version listing for the test project with versions
func serveProjectVersions(t *testing.T, versions ...*modrinthApi.Version) {
	t.Helper()
	httpmock.Activate(t)
	body, err := json.Marshal(versions)
	if err != nil {
		t.Fatalf("failed to encode versions: %v", err)
	}
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/p1/version`, httpmock.NewBytesResponder(200, body))
}

func TestInstallProjectLatestAlreadyAdded(t *testing.T) {
	pack, index := setupPackFixture(t)
	v1 := testVersion("v1", "1.0.0")
	addTestVersion(t, pack, &index, v1, "")
	before := readFile(t, testMetaPath)
	serveProjectVersions(t, v1)
	project, _, _ := testProjectAndFile()

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installProject(project, "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installProject() returned error: %v", err)
	}
	if !strings.Contains(out, "already added and up to date") {
		t.Errorf("output = %q, want it to say the mod is already added and up to date", out)
	}
	if after := readFile(t, testMetaPath); after != before {
		t.Errorf("metadata changed although the latest version is the one added:\n%s", after)
	}
}

func TestInstallProjectOffersUpdateToLatest(t *testing.T) {
	pack, index := setupPackFixture(t)
	addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "")
	v2 := testVersion("v2", "2.0.0")
	v2.DatePublished = timePtr(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	serveProjectVersions(t, testVersion("v1", "1.0.0"), v2)
	cmdtest.SetStdin(t, "y\n")
	project, _, _ := testProjectAndFile()

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installProject(project, "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installProject() returned error: %v", err)
	}
	if !strings.Contains(out, "Update available: 1.0.0 -> 2.0.0") {
		t.Errorf("output = %q, want it to offer the latest version", out)
	}
	if mod := loadTestMod(t); mod.Version != "2.0.0" {
		t.Errorf("Version = %q, want the mod updated to the latest", mod.Version)
	}
}

// A mod added with --release-type beta is checked for updates with it by 'packwiz update', and so must 'mr add' be, or
// it would offer the older release as an "update"
func TestInstallProjectLooksForUpdatesWithTheReleaseTypeItWasAddedWith(t *testing.T) {
	pack, index := setupPackFixture(t)
	older := testVersion("v1", "1.0.0")
	newerBeta := testVersion("v2", "2.0.0-beta")
	newerBeta.VersionType = strPtr("beta")
	newerBeta.DatePublished = timePtr(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	addTestVersion(t, pack, &index, newerBeta, "beta")
	serveProjectVersions(t, older, newerBeta)
	cmdtest.SetStdin(t, "n\n")
	project, _, _ := testProjectAndFile()

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installProject(project, "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installProject() returned error: %v", err)
	}
	if !strings.Contains(out, "already added and up to date") {
		t.Errorf("output = %q, want the beta it has to count as the latest", out)
	}
}

func TestGetLatestVersionWarnsWhenTheNewestHasALowerVersionNumber(t *testing.T) {
	pack, _ := setupPackFixture(t)
	major := testVersion("v1", "3.0.0")
	major.DatePublished = timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	backport := testVersion("v2", "2.9.1")
	backport.DatePublished = timePtr(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	serveProjectVersions(t, major, backport)

	var latest *modrinthApi.Version
	var err error
	out := cmdtest.CaptureStdout(t, func() { latest, err = getLatestVersion("p1", "Test Project", pack, "") })

	if err != nil {
		t.Fatalf("getLatestVersion() returned error: %v", err)
	}
	if *latest.ID != "v2" {
		t.Errorf("latest = %s, want the newest version, v2", *latest.ID)
	}
	want := "Warning: using the newest version of Test Project, 2.9.1 (release, published 2024-02-01), although 3.0.0 (release, published 2024-01-01) has a higher version number"
	if !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
}

// Amendments tagged its version numbers with the loader, "neoforge_1.21-2.0.8", and later stopped: that isn't a
// version that is higher than the newest
func TestGetLatestVersionDoesNotWarnWhenOnlyTheLoaderTagChanged(t *testing.T) {
	pack, _ := setupPackFixture(t)
	tagged := testVersion("v1", "neoforge_1.21-2.0.8")
	tagged.Loaders = []string{"neoforge"}
	tagged.DatePublished = timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	untagged := testVersion("v2", "1.21-2.1.10")
	untagged.Loaders = []string{"neoforge"}
	untagged.DatePublished = timePtr(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	serveProjectVersions(t, tagged, untagged)

	var latest *modrinthApi.Version
	var err error
	out := cmdtest.CaptureStdout(t, func() { latest, err = getLatestVersion("p1", "Test Project", pack, "") })

	if err != nil {
		t.Fatalf("getLatestVersion() returned error: %v", err)
	}
	if *latest.ID != "v2" {
		t.Errorf("latest = %s, want the newest version, v2", *latest.ID)
	}
	if strings.Contains(out, "Warning") {
		t.Errorf("output = %q, want no warning", out)
	}
}

// Create went from "mc1.21.1-6.0.9" to "6.0.10+mc1.21.1": 6.0.10 is higher, however FlexVer ranks the two
func TestGetLatestVersionDoesNotWarnWhenVersionNumbersAreWrittenDifferently(t *testing.T) {
	pack, _ := setupPackFixture(t)
	older := testVersion("v1", "mc1.21.1-6.0.9")
	older.Loaders = []string{"neoforge"}
	older.DatePublished = timePtr(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	newer := testVersion("v2", "6.0.10+mc1.21.1")
	newer.Loaders = []string{"neoforge"}
	newer.DatePublished = timePtr(time.Date(2024, 4, 21, 0, 0, 0, 0, time.UTC))
	serveProjectVersions(t, older, newer)

	var latest *modrinthApi.Version
	var err error
	out := cmdtest.CaptureStdout(t, func() { latest, err = getLatestVersion("p1", "Test Project", pack, "") })

	if err != nil {
		t.Fatalf("getLatestVersion() returned error: %v", err)
	}
	if *latest.ID != "v2" {
		t.Errorf("latest = %s, want the newest version, v2", *latest.ID)
	}
	if strings.Contains(out, "Warning") {
		t.Errorf("output = %q, want no warning", out)
	}
}

// setFlag sets one of the flags of 'mr add' for the duration of the test
func setFlag(t *testing.T, flag *string, value string) {
	t.Helper()
	old := *flag
	*flag = value
	t.Cleanup(func() { *flag = old })
}

// serveVersionByID answers Modrinth's lookups of version v1 of project p1, and of the project itself
func serveVersionByID(t *testing.T) {
	t.Helper()
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/version/v1",
		httpmock.NewStringResponder(200, `{"id":"v1","project_id":"p1","version_number":"1.0.0",
			"files":[{"url":"https://example.com/test.jar","filename":"test.jar","primary":true,"hashes":{"sha512":"deadbeef"}}]}`))
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/p1",
		httpmock.NewStringResponder(200, `{"id":"p1","slug":"test-project","title":"Test Project","project_type":"mod","client_side":"required","server_side":"required"}`))
}

// A version ID says which project it is for, so it is all that has to be given
func TestAddWithVersionIDAlone(t *testing.T) {
	setupPackFixture(t)
	serveVersionByID(t)
	setFlag(t, &versionIDFlag, "v1")

	out := cmdtest.CaptureStdout(t, func() { installCmd.Run(installCmd, nil) })

	if !strings.Contains(out, "successfully added") {
		t.Errorf("output = %q, want the project to be added", out)
	}
	if mod := loadTestMod(t); mod.Version != "1.0.0" {
		t.Errorf("Version = %q, want the version that was asked for", mod.Version)
	}
}

// Giving the project as well, which used to be needed, still works
func TestAddWithVersionIDAndProjectID(t *testing.T) {
	setupPackFixture(t)
	serveVersionByID(t)
	setFlag(t, &projectIDFlag, "p1")
	setFlag(t, &versionIDFlag, "v1")

	out := cmdtest.CaptureStdout(t, func() { installCmd.Run(installCmd, nil) })

	if !strings.Contains(out, "successfully added") {
		t.Errorf("output = %q, want the project to be added", out)
	}
}
