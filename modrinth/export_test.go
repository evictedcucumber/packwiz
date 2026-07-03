package modrinth

import (
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestModrinthExportEnvRespectsIgnoreSide(t *testing.T) {
	mod := &core.Mod{Side: core.ClientSide}
	clientEnv, serverEnv := modrinthExportEnv(mod, false)
	if clientEnv != "required" || serverEnv != "unsupported" {
		t.Fatalf("expected client-only env mapping, got %q/%q", clientEnv, serverEnv)
	}

	clientEnv, serverEnv = modrinthExportEnv(mod, true)
	if clientEnv != "required" || serverEnv != "required" {
		t.Fatalf("expected ignore-side env mapping, got %q/%q", clientEnv, serverEnv)
	}
}

func TestModrinthOverrideFolderRespectsIgnoreSide(t *testing.T) {
	if got := modrinthOverrideFolder(&core.Mod{Side: core.ServerSide}, false); got != "server-overrides" {
		t.Fatalf("expected server-overrides, got %q", got)
	}

	if got := modrinthOverrideFolder(&core.Mod{Side: core.ServerSide}, true); got != "overrides" {
		t.Fatalf("expected overrides when ignoring side, got %q", got)
	}
}
