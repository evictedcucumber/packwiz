package migrate

import (
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestUpdatePackToVersionSameVersion(t *testing.T) {
	loader := core.ModLoaderComponent{Name: "fabric", FriendlyName: "Fabric loader"}
	pack := core.Pack{Versions: map[string]string{"fabric": "0.15.0"}}

	changed := updatePackToVersion("0.15.0", pack, loader)

	if changed {
		t.Error("expected updatePackToVersion to return false when the version is unchanged")
	}
	if pack.Versions["fabric"] != "0.15.0" {
		t.Errorf("expected version to remain 0.15.0, got %s", pack.Versions["fabric"])
	}
}

func TestUpdatePackToVersionNewVersion(t *testing.T) {
	loader := core.ModLoaderComponent{Name: "fabric", FriendlyName: "Fabric loader"}
	pack := core.Pack{Versions: map[string]string{"fabric": "0.15.0"}}

	changed := updatePackToVersion("0.16.0", pack, loader)

	if !changed {
		t.Error("expected updatePackToVersion to return true when the version changes")
	}
	if pack.Versions["fabric"] != "0.16.0" {
		t.Errorf("expected version to be updated to 0.16.0, got %s", pack.Versions["fabric"])
	}
}

func TestUpdatePackToVersionNewLoaderKey(t *testing.T) {
	loader := core.ModLoaderComponent{Name: "forge", FriendlyName: "Forge"}
	pack := core.Pack{Versions: map[string]string{"minecraft": "1.20.1"}}

	changed := updatePackToVersion("47.1.106", pack, loader)

	if !changed {
		t.Error("expected updatePackToVersion to return true for a new loader entry")
	}
	if pack.Versions["forge"] != "47.1.106" {
		t.Errorf("expected forge version to be set to 47.1.106, got %s", pack.Versions["forge"])
	}
}
