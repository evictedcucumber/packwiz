package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
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

// A mod that has the pack keep its files in the folder testdefaults names (see setUpConfigDirPack)
const configDirModTOML = `name = "Defaults"
filename = "defaults.jar"

[download]
hash-format = "sha256"
hash = "d"

[update.testdefaults]
version = "any"
`

// setUpConfigDirPack is the list fixture with config files inside and outside configureddefaults/, and a source of
// mods that lets a pack keep its files there. Nothing in it does yet, so the pack tracks all of them.
func setUpConfigDirPack(t *testing.T, outside ...string) {
	t.Helper()
	setUpListFixture(t)
	cmdtest.RegisterConfigDirSource(t, "testdefaults", "configureddefaults")
	for _, f := range append([]string{"configureddefaults/config/sodium.json"}, outside...) {
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			t.Fatalf("failed to create the folder of %s: %v", f, err)
		}
		if err := os.WriteFile(f, []byte("{}"), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", f, err)
		}
	}
}

func refreshOutput(t *testing.T) string {
	t.Helper()
	return cmdtest.WithoutProgress(cmdtest.CaptureStdout(t, func() { refreshCmd.Run(refreshCmd, nil) }))
}

func TestRefreshSaysWhenFilesLeaveTheIndex(t *testing.T) {
	setUpConfigDirPack(t, "config/sodium.json", "config/iris.properties")

	if out := refreshOutput(t); strings.Contains(out, "Notice") {
		t.Errorf("output = %q, want no notice while the pack has no mod that keeps its files in a folder", out)
	}

	if err := os.WriteFile("mods/defaults.pw.toml", []byte(configDirModTOML), 0644); err != nil {
		t.Fatalf("failed to write the mod: %v", err)
	}
	const notice = "Notice: this pack keeps its files in configureddefaults/, so 2 files outside it are no longer tracked\n"
	if out := refreshOutput(t); !strings.Contains(out, notice) {
		t.Errorf("output = %q, want it to contain %q", out, notice)
	}

	// The files were left out of the index, so the next refresh has nothing to say
	if out := refreshOutput(t); strings.Contains(out, "Notice") {
		t.Errorf("output of a second refresh = %q, want no notice", out)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	for p, want := range map[string]bool{
		"configureddefaults/config/sodium.json": true,
		"mods/defaults.pw.toml":                 true,
		"config/sodium.json":                    false,
		"config/iris.properties":                false,
	} {
		if _, tracked := index.Files[p]; tracked != want {
			t.Errorf("tracked(%s) = %v, want %v", p, tracked, want)
		}
	}
}

func TestRefreshSaysWhenOneFileLeavesTheIndex(t *testing.T) {
	setUpConfigDirPack(t, "config/sodium.json")
	refreshOutput(t)
	if err := os.WriteFile("mods/defaults.pw.toml", []byte(configDirModTOML), 0644); err != nil {
		t.Fatalf("failed to write the mod: %v", err)
	}

	const notice = "Notice: this pack keeps its files in configureddefaults/, so 1 file outside it is no longer tracked\n"
	if out := refreshOutput(t); !strings.Contains(out, notice) {
		t.Errorf("output = %q, want it to contain %q", out, notice)
	}
}

func TestRefreshNoticeIsColouredWithoutChangingItsText(t *testing.T) {
	setUpConfigDirPack(t, "config/sodium.json")
	refreshOutput(t)
	if err := os.WriteFile("mods/defaults.pw.toml", []byte(configDirModTOML), 0644); err != nil {
		t.Fatalf("failed to write the mod: %v", err)
	}
	cmdtest.SetColor(t, ui.Always)

	out := refreshOutput(t)

	want := ui.Info.Sprintf("Notice: this pack keeps its files in %s, so 1 file outside it is no longer tracked",
		ui.Bold.Sprint("configureddefaults/"))
	if !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
	if plain := "Notice: this pack keeps its files in configureddefaults/, so 1 file outside it is no longer tracked"; !strings.Contains(ui.Strip(out), plain) {
		t.Errorf("output without colour = %q, want it to contain %q", ui.Strip(out), plain)
	}
}
