package modrinth

import (
	"testing"
)

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
