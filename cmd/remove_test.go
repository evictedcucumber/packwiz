package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestRemoveDeletesFileAndUpdatesIndex(t *testing.T) {
	setUpListFixture(t)

	out := cmdtest.CaptureStdout(t, func() {
		removeCmd.Run(removeCmd, []string{"alpha"})
	})
	if !strings.Contains(out, "alpha removed successfully!") {
		t.Errorf("output = %q, want a success message", out)
	}

	if _, err := os.Stat("mods/alpha.pw.toml"); !os.IsNotExist(err) {
		t.Error("mods/alpha.pw.toml should have been deleted from disk")
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	if _, ok := index.Files["mods/alpha.pw.toml"]; ok {
		t.Error("mods/alpha.pw.toml should have been removed from the index")
	}
	if _, ok := index.Files["mods/beta.pw.toml"]; !ok {
		t.Error("mods/beta.pw.toml should still be tracked in the index")
	}
}
