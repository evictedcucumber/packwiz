package modrinth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/jarcoal/httpmock"
)

func TestGetInstalledProjectIDs(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, file := testProjectAndFile()

	if err := createFileMeta(project, version, file, pack, &index, "", false); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}

	ids := getInstalledProjectIDs(&index)
	if len(ids) != 1 || ids[0] != "p1" {
		t.Errorf("getInstalledProjectIDs() = %v, want [p1]", ids)
	}
}

func TestGetInstalledProjectIDsEmptyIndex(t *testing.T) {
	_, index := setupPackFixture(t)
	ids := getInstalledProjectIDs(&index)
	if len(ids) != 0 {
		t.Errorf("getInstalledProjectIDs() = %v, want empty", ids)
	}
}

func TestFindInstalledMod(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, file := testProjectAndFile()
	if err := createFileMeta(project, version, file, pack, &index, "", false); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}

	mod, err := findInstalledMod(&index, "p1")
	if err != nil {
		t.Fatalf("findInstalledMod() returned error: %v", err)
	}
	if mod == nil || mod.Name != "Test Project" {
		t.Errorf("findInstalledMod() = %+v, want the Test Project mod", mod)
	}

	other, err := findInstalledMod(&index, "someone-else")
	if err != nil {
		t.Fatalf("findInstalledMod() returned error: %v", err)
	}
	if other != nil {
		t.Errorf("findInstalledMod() = %+v, want nil for a project that isn't added", other)
	}
}

func TestFindInstalledModEmptyIndex(t *testing.T) {
	_, index := setupPackFixture(t)

	mod, err := findInstalledMod(&index, "p1")
	if err != nil || mod != nil {
		t.Errorf("findInstalledMod() = %+v, %v; want nil, nil", mod, err)
	}
}

// Not knowing what is added could mean adding a mod twice, so an unreadable metadata file is an error
func TestFindInstalledModUnreadableMod(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, file := testProjectAndFile()
	if err := createFileMeta(project, version, file, pack, &index, "", false); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join("mods", "test-project"+core.MetaExtension), []byte("not = [toml"), 0644); err != nil {
		t.Fatalf("failed to corrupt the metadata: %v", err)
	}

	if _, err := findInstalledMod(&index, "p1"); err == nil {
		t.Error("expected an error when a metadata file can't be read, got nil")
	}
}

func TestResolveVersionByKnownID(t *testing.T) {
	httpmock.Activate(t)
	project := &modrinthApi.Project{ID: strPtr("p1"), Versions: []string{"v1", "v2"}}
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/version/v1",
		httpmock.NewStringResponder(200, `{"id":"v1","version_number":"1.0.0"}`))

	v, err := resolveVersion(project, "v1")
	if err != nil {
		t.Fatalf("resolveVersion() returned error: %v", err)
	}
	if v.ID == nil || *v.ID != "v1" {
		t.Errorf("resolveVersion() ID = %v, want v1", v.ID)
	}
}

func TestResolveVersionByVersionNumber(t *testing.T) {
	httpmock.Activate(t)
	project := &modrinthApi.Project{ID: strPtr("p1"), Versions: []string{"v1"}}
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/p1/version",
		httpmock.NewStringResponder(200, `[{"id":"v1","version_number":"1.0.0"},{"id":"v2","version_number":"2.0.0"}]`))

	v, err := resolveVersion(project, "2.0.0")
	if err != nil {
		t.Fatalf("resolveVersion() returned error: %v", err)
	}
	if v.ID == nil || *v.ID != "v2" {
		t.Errorf("resolveVersion() ID = %v, want v2", v.ID)
	}
}

func TestResolveVersionNumberNotFound(t *testing.T) {
	httpmock.Activate(t)
	project := &modrinthApi.Project{ID: strPtr("p1"), Versions: []string{"v1"}}
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/p1/version",
		httpmock.NewStringResponder(200, `[{"id":"v1","version_number":"1.0.0"}]`))

	if _, err := resolveVersion(project, "9.9.9"); err == nil {
		t.Error("expected an error for a version number that doesn't exist, got nil")
	}
}

func TestInstallVersionById(t *testing.T) {
	httpmock.Activate(t)
	pack, index := setupPackFixture(t)

	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/version/v1",
		httpmock.NewStringResponder(200, `{"id":"v1","project_id":"p1","version_number":"1.0.0",
			"files":[{"url":"https://example.com/test.jar","filename":"test.jar","primary":true,"hashes":{"sha512":"deadbeef"}}]}`))
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/p1",
		httpmock.NewStringResponder(200, `{"id":"p1","slug":"test-project","title":"Test Project","project_type":"mod","client_side":"required","server_side":"required"}`))

	if err := installVersionById("v1", "", pack, &index, ""); err != nil {
		t.Fatalf("installVersionById() returned error: %v", err)
	}

	metaPath := filepath.Join("mods", "test-project"+core.MetaExtension)
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("expected mod metadata file to exist at %s: %v", metaPath, err)
	}
}

func TestInstallVersionByIdUnknownVersion(t *testing.T) {
	httpmock.Activate(t)
	pack, index := setupPackFixture(t)

	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/version/missing",
		httpmock.NewStringResponder(404, `{"error":"not_found","description":"missing"}`))

	if err := installVersionById("missing", "", pack, &index, ""); err == nil {
		t.Error("expected an error for an unknown version ID, got nil")
	}
}

func TestInstallProject(t *testing.T) {
	httpmock.Activate(t)
	pack, index := setupPackFixture(t)

	project := &modrinthApi.Project{
		ID: strPtr("p1"), Title: strPtr("Test Project"), Slug: strPtr("test-project"),
		ProjectType: strPtr("mod"), ClientSide: strPtr("required"), ServerSide: strPtr("required"),
	}
	// getLatestVersion's ListVersions call includes an encoded query string
	// (game versions/loaders), so match on path only via a regex responder.
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/p1/version`,
		httpmock.NewStringResponder(200, `[{"id":"v1","project_id":"p1","version_number":"1.0.0","version_type":"release","date_published":"2024-01-01T00:00:00Z",
			"files":[{"url":"https://example.com/test.jar","filename":"test.jar","primary":true,"hashes":{"sha512":"deadbeef"}}]}]`))

	if err := installProject(project, "", pack, &index, ""); err != nil {
		t.Fatalf("installProject() returned error: %v", err)
	}

	metaPath := filepath.Join("mods", "test-project"+core.MetaExtension)
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("expected mod metadata file to exist at %s: %v", metaPath, err)
	}
}

func TestInstallProjectNoValidVersions(t *testing.T) {
	httpmock.Activate(t)
	pack, index := setupPackFixture(t)

	project := &modrinthApi.Project{ID: strPtr("p1"), Title: strPtr("Test Project")}
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/p1/version`,
		httpmock.NewStringResponder(200, `[]`))

	if err := installProject(project, "", pack, &index, ""); err == nil {
		t.Error("expected an error when no versions are available, got nil")
	}
}
