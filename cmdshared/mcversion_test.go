package cmdshared

import (
	"encoding/json"
	"testing"

	"github.com/jarcoal/httpmock"
)

func TestGetValidMCVersionsSortedNewestFirst(t *testing.T) {
	// Regression test: GetValidMCVersions claimed to sort "newest to oldest"
	// but the comparator was inverted and actually sorted oldest-to-newest.
	httpmock.Activate(t)

	body := `{
		"latest": {"release": "1.21.1", "snapshot": "24w40a"},
		"versions": [
			{"id": "1.20.1", "type": "release", "url": "", "time": "2023-06-07T00:00:00+00:00", "releaseTime": "2023-06-07T00:00:00+00:00"},
			{"id": "1.21.1", "type": "release", "url": "", "time": "2024-08-08T00:00:00+00:00", "releaseTime": "2024-08-08T00:00:00+00:00"},
			{"id": "1.20.4", "type": "release", "url": "", "time": "2023-12-07T00:00:00+00:00", "releaseTime": "2023-12-07T00:00:00+00:00"}
		]
	}`
	httpmock.RegisterResponder("GET", "https://launchermeta.mojang.com/mc/game/version_manifest.json",
		httpmock.NewStringResponder(200, body))

	manifest, err := GetValidMCVersions()
	if err != nil {
		t.Fatalf("GetValidMCVersions() returned error: %v", err)
	}

	if len(manifest.Versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(manifest.Versions))
	}
	want := []string{"1.21.1", "1.20.4", "1.20.1"}
	for i, w := range want {
		if manifest.Versions[i].ID != w {
			t.Errorf("Versions[%d].ID = %q, want %q (expected newest-to-oldest order)", i, manifest.Versions[i].ID, w)
		}
	}
}

func testManifest(t *testing.T) McVersionManifest {
	t.Helper()
	body := `{
		"latest": {"release": "1.21.1", "snapshot": "24w40a"},
		"versions": [
			{"id": "1.20.1", "type": "release", "url": "", "time": "2023-06-07T00:00:00+00:00", "releaseTime": "2023-06-07T00:00:00+00:00"},
			{"id": "1.21.1", "type": "release", "url": "", "time": "2024-08-08T00:00:00+00:00", "releaseTime": "2024-08-08T00:00:00+00:00"}
		]
	}`
	var m McVersionManifest
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("failed to unmarshal test manifest fixture: %v", err)
	}
	return m
}

func TestMcVersionManifestIsValid(t *testing.T) {
	m := testManifest(t)
	if !m.IsValid("1.20.1") {
		t.Error("IsValid(\"1.20.1\") = false, want true")
	}
	if !m.IsValid("1.21.1") {
		t.Error("IsValid(\"1.21.1\") = false, want true")
	}
	if m.IsValid("1.99.9") {
		t.Error("IsValid(\"1.99.9\") = true, want false")
	}
	if m.IsValid("") {
		t.Error("IsValid(\"\") = true, want false")
	}
}

func TestMcVersionManifestCheckValidValidVersion(t *testing.T) {
	m := testManifest(t)
	// Only the valid branch is safe to exercise here: the invalid branch
	// calls os.Exit(1), which would kill the test process.
	m.CheckValid("1.20.1")
}
