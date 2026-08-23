package migrate

import (
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestUpdatePackToVersionSameVersion(t *testing.T) {
	loader := core.ModLoaderComponent{Name: "neoforge", FriendlyName: "NeoForge"}
	pack := core.Pack{Versions: map[string]string{"neoforge": "21.1.213"}}

	changed := updatePackToVersion("21.1.213", pack, loader)

	if changed {
		t.Error("expected updatePackToVersion to return false when the version is unchanged")
	}
	if pack.Versions["neoforge"] != "21.1.213" {
		t.Errorf("expected version to remain 21.1.213, got %s", pack.Versions["neoforge"])
	}
}

func TestUpdatePackToVersionNewVersion(t *testing.T) {
	loader := core.ModLoaderComponent{Name: "neoforge", FriendlyName: "NeoForge"}
	pack := core.Pack{Versions: map[string]string{"neoforge": "21.1.213"}}

	changed := updatePackToVersion("21.1.214", pack, loader)

	if !changed {
		t.Error("expected updatePackToVersion to return true when the version changes")
	}
	if pack.Versions["neoforge"] != "21.1.214" {
		t.Errorf("expected version to be updated to 21.1.214, got %s", pack.Versions["neoforge"])
	}
}

func TestUpdatePackToVersionNewLoaderKey(t *testing.T) {
	loader := core.ModLoaderComponent{Name: "neoforge", FriendlyName: "NeoForge"}
	pack := core.Pack{Versions: map[string]string{"minecraft": "1.20.1"}}

	changed := updatePackToVersion("47.1.106", pack, loader)

	if !changed {
		t.Error("expected updatePackToVersion to return true for a new loader entry")
	}
	if pack.Versions["neoforge"] != "47.1.106" {
		t.Errorf("expected neoforge version to be set to 47.1.106, got %s", pack.Versions["neoforge"])
	}
}
