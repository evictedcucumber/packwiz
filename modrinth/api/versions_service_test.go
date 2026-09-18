package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestVersionsServiceListVersionsNoOptions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project/sodium/version" {
			t.Errorf("Path = %q, want /project/sodium/version", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("RawQuery = %q, want empty when no options are given", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"v1"}]`))
	})

	versions, err := c.Versions.ListVersions("sodium", ListVersionsOptions{})
	if err != nil {
		t.Fatalf("ListVersions() returned error: %v", err)
	}
	if len(versions) != 1 || *versions[0].ID != "v1" {
		t.Errorf("got %+v, want a single version with ID v1", versions)
	}
}

func TestVersionsServiceListVersionsWithOptions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var loaders []string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("loaders")), &loaders); err != nil {
			t.Fatalf("failed to unmarshal loaders query param: %v", err)
		}
		if len(loaders) != 1 || loaders[0] != "fabric" {
			t.Errorf("loaders = %v, want [fabric]", loaders)
		}

		var gameVersions []string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("game_versions")), &gameVersions); err != nil {
			t.Fatalf("failed to unmarshal game_versions query param: %v", err)
		}
		if len(gameVersions) != 1 || gameVersions[0] != "1.20.1" {
			t.Errorf("game_versions = %v, want [1.20.1]", gameVersions)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})

	_, err := c.Versions.ListVersions("sodium", ListVersionsOptions{
		Loaders:      []string{"fabric"},
		GameVersions: []string{"1.20.1"},
	})
	if err != nil {
		t.Fatalf("ListVersions() returned error: %v", err)
	}
}

func TestVersionsServiceGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version/abc123" {
			t.Errorf("Path = %q, want /version/abc123", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"abc123","version_number":"1.0.0","game_versions":["1.20.1"],"loaders":["fabric"]}`))
	})

	version, err := c.Versions.Get("abc123")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if version.ID == nil || *version.ID != "abc123" {
		t.Errorf("ID = %v, want abc123", version.ID)
	}
	if version.VersionNumber == nil || *version.VersionNumber != "1.0.0" {
		t.Errorf("VersionNumber = %v, want 1.0.0", version.VersionNumber)
	}
	if len(version.GameVersions) != 1 || version.GameVersions[0] != "1.20.1" {
		t.Errorf("GameVersions = %v, want [1.20.1]", version.GameVersions)
	}
}

func TestVersionsServiceGetMultiple(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/versions" {
			t.Errorf("Path = %q, want /versions", r.URL.Path)
		}
		var ids []string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("ids")), &ids); err != nil {
			t.Fatalf("failed to unmarshal ids query param: %v", err)
		}
		if len(ids) != 2 {
			t.Errorf("ids = %v, want 2 entries", ids)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"v1"},{"id":"v2"}]`))
	})

	versions, err := c.Versions.GetMultiple([]string{"v1", "v2"})
	if err != nil {
		t.Fatalf("GetMultiple() returned error: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
}

func TestVersionsServiceGetServerError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.Versions.Get("abc123")
	if err == nil {
		t.Fatal("expected an error for a 500 response, got nil")
	}
}

// TestVersionFileHashesRoundTrip exercises the File/Dependency JSON shapes
// used by the modrinth package when reading install metadata.
func TestVersionFileHashesRoundTrip(t *testing.T) {
	data := []byte(`{
		"id": "abc123",
		"files": [{"hashes": {"sha1": "deadbeef"}, "url": "https://example.com/f.jar", "filename": "f.jar", "primary": true}],
		"dependencies": [{"project_id": "P7dR8mSH", "dependency_type": "required"}]
	}`)
	var version Version
	if err := json.Unmarshal(data, &version); err != nil {
		t.Fatalf("Unmarshal() returned error: %v", err)
	}
	if len(version.Files) != 1 || version.Files[0].Hashes["sha1"] != "deadbeef" {
		t.Errorf("Files = %+v, want a single file with sha1 hash deadbeef", version.Files)
	}
	if !*version.Files[0].Primary {
		t.Error("Primary = false, want true")
	}
	if len(version.Dependencies) != 1 || *version.Dependencies[0].DependencyType != "required" {
		t.Errorf("Dependencies = %+v, want a single required dependency", version.Dependencies)
	}
}
