package settings

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestReleaseTypeGetDefault(t *testing.T) {
	newTestPack(t)

	out := cmdtest.CaptureStdout(t, func() {
		releaseTypeCommand.Run(releaseTypeCommand, nil)
	})
	if !strings.Contains(out, "Current release type: release") {
		t.Errorf("output = %q, want it to mention the default release type", out)
	}
}

func TestReleaseTypeSetValid(t *testing.T) {
	newTestPack(t)

	out := cmdtest.CaptureStdout(t, func() {
		releaseTypeCommand.Run(releaseTypeCommand, []string{"beta"})
	})
	if !strings.Contains(out, "Set default release type to beta") {
		t.Errorf("output = %q, want confirmation of the new release type", out)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.GetReleaseType() != "beta" {
		t.Errorf("GetReleaseType() = %q, want %q", pack.GetReleaseType(), "beta")
	}
}
