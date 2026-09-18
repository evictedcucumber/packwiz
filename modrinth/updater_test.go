package modrinth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/jarcoal/httpmock"
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

// modrinthMod is a mod that has the given Modrinth version installed and no version number recorded, as one added
// before Mod.Version existed
func modrinthMod(t *testing.T, name, installedVersionID string) *core.Mod {
	t.Helper()
	mod, err := core.DecodeMod([]byte(fmt.Sprintf(`name = %q
filename = "%s.jar"

[download]
hash-format = "sha256"
hash = "h"

[update.modrinth]
mod-id = "p-%s"
version = %q
`, name, name, name, installedVersionID)))
	if err != nil {
		t.Fatalf("DecodeMod() returned error: %v", err)
	}
	return &mod
}

// serveVersions answers Modrinth's bulk version lookup from numbers (version ID to version number; IDs it doesn't
// have are left out of the answer, as Modrinth does), and records the IDs asked for in each request.
func serveVersions(t *testing.T, numbers map[string]string) *[][]string {
	t.Helper()
	httpmock.Activate(t)
	var batches [][]string
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/versions`, func(req *http.Request) (*http.Response, error) {
		var ids []string
		if err := json.Unmarshal([]byte(req.URL.Query().Get("ids")), &ids); err != nil {
			return httpmock.NewStringResponse(400, "bad ids"), nil
		}
		batches = append(batches, ids)
		found := []map[string]string{}
		for _, id := range ids {
			if n, ok := numbers[id]; ok {
				found = append(found, map[string]string{"id": id, "version_number": n})
			}
		}
		return httpmock.NewJsonResponse(200, found)
	})
	return &batches
}

func TestMrUpdaterResolveVersions(t *testing.T) {
	batches := serveVersions(t, map[string]string{"v1": "1.0.0", "v2": "2.0.0"})

	got, err := mrUpdater{}.ResolveVersions([]*core.Mod{modrinthMod(t, "a", "v1"), modrinthMod(t, "b", "v2"), modrinthMod(t, "c", "gone")})
	if err != nil {
		t.Fatalf("ResolveVersions() returned error: %v", err)
	}

	// In the order of the mods, with "" for the one Modrinth doesn't have
	if want := []string{"1.0.0", "2.0.0", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveVersions() = %q, want %q", got, want)
	}
	if want := [][]string{{"v1", "v2", "gone"}}; !reflect.DeepEqual(*batches, want) {
		t.Errorf("requested %v, want one request for all three", *batches)
	}
}

func TestMrUpdaterResolveVersionsAsksForEachVersionOnce(t *testing.T) {
	batches := serveVersions(t, map[string]string{"v1": "1.0.0"})

	got, err := mrUpdater{}.ResolveVersions([]*core.Mod{modrinthMod(t, "a", "v1"), modrinthMod(t, "b", "v1")})
	if err != nil {
		t.Fatalf("ResolveVersions() returned error: %v", err)
	}

	if want := []string{"1.0.0", "1.0.0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveVersions() = %q, want %q", got, want)
	}
	if want := [][]string{{"v1"}}; !reflect.DeepEqual(*batches, want) {
		t.Errorf("requested %v, want the shared version asked for once", *batches)
	}
}

func TestMrUpdaterResolveVersionsBatchesLargePacks(t *testing.T) {
	numbers := make(map[string]string)
	var mods []*core.Mod
	for i := 0; i < 250; i++ {
		id := fmt.Sprintf("v%03d", i)
		numbers[id] = fmt.Sprintf("1.0.%d", i)
		mods = append(mods, modrinthMod(t, fmt.Sprintf("mod%03d", i), id))
	}
	batches := serveVersions(t, numbers)

	got, err := mrUpdater{}.ResolveVersions(mods)
	if err != nil {
		t.Fatalf("ResolveVersions() returned error: %v", err)
	}

	var sizes []int
	for _, b := range *batches {
		sizes = append(sizes, len(b))
	}
	if want := []int{100, 100, 50}; !reflect.DeepEqual(sizes, want) {
		t.Errorf("request sizes = %v, want %v so no URL grows with the size of the pack", sizes, want)
	}
	for i, v := range got {
		if want := fmt.Sprintf("1.0.%d", i); v != want {
			t.Fatalf("version %d = %q, want %q; results must stay matched to their mods across batches", i, v, want)
		}
	}
}

func TestMrUpdaterResolveVersionsAPIFailure(t *testing.T) {
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/versions`,
		httpmock.NewStringResponder(500, `{"error":"boom"}`))

	got, err := mrUpdater{}.ResolveVersions([]*core.Mod{modrinthMod(t, "a", "v1")})

	if err == nil || !strings.Contains(err.Error(), "look up versions") {
		t.Errorf("error = %v, want one about looking up versions", err)
	}
	if got != nil {
		t.Errorf("ResolveVersions() = %q, want nothing when the lookup failed", got)
	}
}

func TestMrUpdaterResolveVersionsWithoutVersionNumber(t *testing.T) {
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/versions`,
		httpmock.NewStringResponder(200, `[{"id":"v1"}]`))

	got, err := mrUpdater{}.ResolveVersions([]*core.Mod{modrinthMod(t, "a", "v1")})
	if err != nil {
		t.Fatalf("ResolveVersions() returned error: %v", err)
	}
	if want := []string{""}; !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveVersions() = %q, want %q for a version that has no number", got, want)
	}
}

func TestMrUpdaterResolveVersionsSkipsModsItDoesNotManage(t *testing.T) {
	httpmock.Activate(t)
	unmanaged, err := core.DecodeMod([]byte("name = \"x\"\nfilename = \"x.jar\"\n"))
	if err != nil {
		t.Fatalf("DecodeMod() returned error: %v", err)
	}

	got, err := mrUpdater{}.ResolveVersions([]*core.Mod{&unmanaged})
	if err != nil {
		t.Fatalf("ResolveVersions() returned error: %v", err)
	}
	if want := []string{""}; !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveVersions() = %q, want %q", got, want)
	}
	if calls := httpmock.GetTotalCallCount(); calls != 0 {
		t.Errorf("made %d requests, want none when there is nothing to look up", calls)
	}
}

// The whole path a pack takes: a mod added before versions were recorded, found by the index, looked up through the
// registered Modrinth updater, and saved
func TestMissingVersionIsLookedUpAndRecordedFromModrinth(t *testing.T) {
	pack, index := setupPackFixture(t)
	project, version, file := testProjectAndFile()
	version.VersionNumber = nil // as if added before the version number was recorded
	if err := createFileMeta(project, version, file, pack, &index, "", false); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}
	serveVersions(t, map[string]string{"v1": "1.2.3"})

	found, err := index.ResolveMissingVersions()
	if err != nil {
		t.Fatalf("ResolveMissingVersions() returned error: %v", err)
	}
	if want := map[string]string{"mods/test-project.pw.toml": "1.2.3"}; !reflect.DeepEqual(found, want) {
		t.Fatalf("ResolveMissingVersions() = %v, want %v", found, want)
	}
	if err := index.RecordVersions(found); err != nil {
		t.Fatalf("RecordVersions() returned error: %v", err)
	}

	mod, err := core.LoadMod(filepath.Join("mods", "test-project"+core.MetaExtension))
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.Version != "1.2.3" {
		t.Errorf("Version = %q, want the looked-up 1.2.3 saved in the file", mod.Version)
	}
}
