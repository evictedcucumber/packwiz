package curseforge

import (
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestFilterModsBySideIgnoresSideWhenRequested(t *testing.T) {
	mods := []*core.Mod{
		{Name: "Client Mod", Side: core.ClientSide},
		{Name: "Server Mod", Side: core.ServerSide},
		{Name: "Both Mod", Side: core.UniversalSide},
	}

	filtered := filterModsBySide(mods, core.ClientSide, true)
	if len(filtered) != len(mods) {
		t.Fatalf("expected all mods to be kept, got %d", len(filtered))
	}

	for i, mod := range mods {
		if filtered[i] != mod {
			t.Fatalf("expected mod %q at index %d to be preserved", mod.Name, i)
		}
	}
}
