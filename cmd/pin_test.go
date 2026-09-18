package cmd

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestPinCmdSetsPinFlag(t *testing.T) {
	setUpListFixture(t)

	out := cmdtest.CaptureStdout(t, func() {
		pinCmd.Run(pinCmd, []string{"alpha"})
	})
	if !strings.Contains(out, "alpha pinned successfully!") {
		t.Errorf("output = %q, want a success message", out)
	}

	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if !mod.Pin {
		t.Error("Pin = false, want true after pinCmd")
	}
}

func TestUnpinCmdClearsPinFlag(t *testing.T) {
	setUpListFixture(t)

	cmdtest.CaptureStdout(t, func() {
		pinCmd.Run(pinCmd, []string{"alpha"})
	})

	out := cmdtest.CaptureStdout(t, func() {
		unpinCmd.Run(unpinCmd, []string{"alpha"})
	})
	if !strings.Contains(out, "alpha unpinned successfully!") {
		t.Errorf("output = %q, want a success message", out)
	}

	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.Pin {
		t.Error("Pin = true, want false after unpinCmd")
	}
}
