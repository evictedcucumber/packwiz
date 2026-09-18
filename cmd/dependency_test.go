package cmd

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestMarkDependencyCmd(t *testing.T) {
	setUpListFixture(t)

	// gamma starts as a main mod in the fixture.
	out := cmdtest.CaptureStdout(t, func() {
		markDependencyCmd.Run(markDependencyCmd, []string{"gamma"})
	})
	if !strings.Contains(out, "gamma marked as a dependency successfully!") {
		t.Errorf("output = %q, want a success message", out)
	}

	mod, err := core.LoadMod("mods/gamma.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if !mod.AddedAsDependency {
		t.Error("AddedAsDependency = false, want true after markDependencyCmd")
	}
}

func TestUnmarkDependencyCmd(t *testing.T) {
	setUpListFixture(t)

	// beta starts as a dependency in the fixture.
	out := cmdtest.CaptureStdout(t, func() {
		unmarkDependencyCmd.Run(unmarkDependencyCmd, []string{"beta"})
	})
	if !strings.Contains(out, "beta marked as a main mod successfully!") {
		t.Errorf("output = %q, want a success message", out)
	}

	mod, err := core.LoadMod("mods/beta.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.AddedAsDependency {
		t.Error("AddedAsDependency = true, want false after unmarkDependencyCmd")
	}
}
