package settings

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/spf13/viper"
)

// newTestPack creates a fresh pack.toml in a temp dir and points viper at
// it. viper.Reset() first, since these commands round-trip values through
// pack.toml's [options] table, and core.LoadPack's viper.MergeConfigMap call
// has no clean way to be undone between tests otherwise (see WritePackFile).
// Safe here: settings commands don't rely on any viper.BindPFlag bindings
// established elsewhere, and "pack-file" is always re-set as an explicit
// override immediately after by WritePackFile.
func newTestPack(t *testing.T) {
	t.Helper()
	viper.Reset()
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.20.1"},
	})
}

// resetAcceptableVersionsFlags clears the package-level add/remove flags
// between subtests, since the cobra command's Run closure reads them
// directly rather than through parsed per-invocation flags.
func resetAcceptableVersionsFlags(t *testing.T) {
	t.Helper()
	oldAdd, oldRemove := flagAdd, flagRemove
	flagAdd, flagRemove = false, false
	t.Cleanup(func() { flagAdd, flagRemove = oldAdd, oldRemove })
}

func TestAcceptableVersionsOverwrite(t *testing.T) {
	newTestPack(t)
	resetAcceptableVersionsFlags(t)

	out := cmdtest.CaptureStdout(t, func() {
		acceptableVersionsCommand.Run(acceptableVersionsCommand, []string{"1.19.4,1.20"})
	})
	if !strings.Contains(out, "Set acceptable versions to 1.19.4, 1.20") {
		t.Errorf("output = %q, want confirmation of the new list", out)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	got := pack.Options["acceptable-game-versions"].([]interface{})
	if len(got) != 2 || got[0] != "1.19.4" || got[1] != "1.20" {
		t.Errorf("acceptable-game-versions = %v, want [1.19.4 1.20]", got)
	}
}

func TestAcceptableVersionsOverwriteDedupsAndWarnsOutOfOrder(t *testing.T) {
	newTestPack(t)
	resetAcceptableVersionsFlags(t)
	// non-interactive mode auto-accepts the "fix the order" prompt.
	cmdtest.SetViperBool(t, "non-interactive", true)

	out := cmdtest.CaptureStdout(t, func() {
		acceptableVersionsCommand.Run(acceptableVersionsCommand, []string{"1.20,1.19.4,1.19.4"})
	})
	if !strings.Contains(out, "out of order") {
		t.Errorf("output = %q, want a warning about the list being out of order", out)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	got := pack.Options["acceptable-game-versions"].([]interface{})
	if len(got) != 2 {
		t.Fatalf("acceptable-game-versions = %v, want 2 deduped entries", got)
	}
	if got[0] != "1.19.4" || got[1] != "1.20" {
		t.Errorf("acceptable-game-versions = %v, want sorted [1.19.4 1.20] after auto-fix", got)
	}
}

func TestAcceptableVersionsAdd(t *testing.T) {
	newTestPack(t)
	resetAcceptableVersionsFlags(t)
	flagAdd = true

	out := cmdtest.CaptureStdout(t, func() {
		acceptableVersionsCommand.Run(acceptableVersionsCommand, []string{"1.20"})
	})
	if !strings.Contains(out, "Added 1.20") {
		t.Errorf("output = %q, want confirmation the version was added", out)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	got := pack.Options["acceptable-game-versions"].([]interface{})
	if len(got) != 1 || got[0] != "1.20" {
		t.Errorf("acceptable-game-versions = %v, want [1.20]", got)
	}
}

func TestAcceptableVersionsRemove(t *testing.T) {
	newTestPack(t)
	resetAcceptableVersionsFlags(t)

	// Seed an existing list to remove from.
	cmdtest.CaptureStdout(t, func() {
		acceptableVersionsCommand.Run(acceptableVersionsCommand, []string{"1.19.4,1.20"})
	})

	flagRemove = true
	out := cmdtest.CaptureStdout(t, func() {
		acceptableVersionsCommand.Run(acceptableVersionsCommand, []string{"1.19.4"})
	})
	if !strings.Contains(out, "Removed 1.19.4") {
		t.Errorf("output = %q, want confirmation the version was removed", out)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	got := pack.Options["acceptable-game-versions"].([]interface{})
	if len(got) != 1 || got[0] != "1.20" {
		t.Errorf("acceptable-game-versions = %v, want [1.20]", got)
	}
}
