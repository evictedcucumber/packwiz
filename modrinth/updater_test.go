package modrinth

import (
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
)

func TestMrUpdaterDoUpdateRecordsVersionNumber(t *testing.T) {
	newVersion := &modrinthApi.Version{
		ID:            strPtr("v2"),
		VersionNumber: strPtr("2.0.0"),
		Files: []*modrinthApi.File{{
			URL: strPtr("https://example.com/test-2.0.0.jar"), Filename: strPtr("test-2.0.0.jar"), Primary: boolPtr(true),
			Hashes: map[string]string{"sha512": "cafe"},
		}},
	}
	mod := &core.Mod{
		Name: "Test", FileName: "test-1.0.0.jar", Version: "1.0.0",
		Update: map[string]map[string]interface{}{"modrinth": {"mod-id": "p1", "version": "v1"}},
	}

	err := mrUpdater{}.DoUpdate([]*core.Mod{mod}, []interface{}{cachedStateStore{ProjectID: "p1", Version: newVersion}})
	if err != nil {
		t.Fatalf("DoUpdate() returned error: %v", err)
	}
	if mod.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", mod.Version, "2.0.0")
	}
	if mod.FileName != "test-2.0.0.jar" {
		t.Errorf("FileName = %q, want %q", mod.FileName, "test-2.0.0.jar")
	}
}

func TestMrUpdaterDoUpdateClearsStaleVersionNumber(t *testing.T) {
	newVersion := &modrinthApi.Version{
		ID: strPtr("v2"), // no VersionNumber
		Files: []*modrinthApi.File{{
			URL: strPtr("https://example.com/test-2.jar"), Filename: strPtr("test-2.jar"), Primary: boolPtr(true),
			Hashes: map[string]string{"sha512": "cafe"},
		}},
	}
	mod := &core.Mod{
		Name: "Test", FileName: "test-1.jar", Version: "1.0.0",
		Update: map[string]map[string]interface{}{"modrinth": {"mod-id": "p1", "version": "v1"}},
	}

	err := mrUpdater{}.DoUpdate([]*core.Mod{mod}, []interface{}{cachedStateStore{ProjectID: "p1", Version: newVersion}})
	if err != nil {
		t.Fatalf("DoUpdate() returned error: %v", err)
	}
	if mod.Version != "" {
		t.Errorf("Version = %q, want it cleared rather than left as the previous release's number", mod.Version)
	}
}

func TestMrUpdateDataToMap(t *testing.T) {
	u := mrUpdateData{ProjectID: "abc123", InstalledVersion: "def456"}
	m, err := u.ToMap()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["mod-id"] != "abc123" {
		t.Errorf("expected mod-id %q, got %v", "abc123", m["mod-id"])
	}
	if m["version"] != "def456" {
		t.Errorf("expected version %q, got %v", "def456", m["version"])
	}
}

func TestMrUpdateDataToMapOmitsEmptyReleaseType(t *testing.T) {
	u := mrUpdateData{ProjectID: "abc123", InstalledVersion: "def456"}
	m, err := u.ToMap()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := m["release-type"]; ok {
		t.Errorf("expected release-type to be omitted when empty, got %v", m["release-type"])
	}
}

func TestMrUpdateDataToMapIncludesReleaseTypeWhenSet(t *testing.T) {
	u := mrUpdateData{ProjectID: "abc123", InstalledVersion: "def456", ReleaseType: "beta"}
	m, err := u.ToMap()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["release-type"] != "beta" {
		t.Errorf("expected release-type %q, got %v", "beta", m["release-type"])
	}
}

func TestMrUpdaterParseUpdate(t *testing.T) {
	input := map[string]interface{}{
		"mod-id":  "abc123",
		"version": "def456",
	}
	result, err := mrUpdater{}.ParseUpdate(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, ok := result.(mrUpdateData)
	if !ok {
		t.Fatalf("expected result to be mrUpdateData, got %T", result)
	}
	if data.ProjectID != "abc123" {
		t.Errorf("expected ProjectID %q, got %q", "abc123", data.ProjectID)
	}
	if data.InstalledVersion != "def456" {
		t.Errorf("expected InstalledVersion %q, got %q", "def456", data.InstalledVersion)
	}
}
