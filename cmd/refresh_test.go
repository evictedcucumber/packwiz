package cmd

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestRefreshCmdUpdatesIndexAndPack(t *testing.T) {
	setUpListFixture(t)

	out := cmdtest.CaptureStdout(t, func() {
		refreshCmd.Run(refreshCmd, nil)
	})
	if !strings.Contains(out, "Index refreshed!") {
		t.Errorf("output = %q, want a completion message", out)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.Index.Hash == "" {
		t.Error("Index.Hash is empty; expected pack.UpdateIndexHash() to have run")
	}

	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	for _, p := range []string{"mods/alpha.pw.toml", "mods/beta.pw.toml", "mods/gamma.pw.toml"} {
		if _, ok := index.Files[p]; !ok {
			t.Errorf("expected %q to still be tracked in the index after refresh", p)
		}
	}
}
