package cmdshared

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestPromptYesNoStylesThePromptAndTheAnswerItGives(t *testing.T) {
	cmdtest.SetViperBool(t, "non-interactive", true)
	const prompt = "Would you like to add them? [Y/n]: "

	cmdtest.SetColor(t, ui.Never)
	plain := cmdtest.CaptureStdout(t, func() { PromptYesNo(prompt) })
	if want := prompt + "Y (non-interactive mode)\n"; plain != want {
		t.Errorf("output without colour = %q, want %q", plain, want)
	}

	cmdtest.SetColor(t, ui.Always)
	coloured := cmdtest.CaptureStdout(t, func() { PromptYesNo(prompt) })
	if want := ui.Prompt(prompt) + ui.Info.Sprint("Y (non-interactive mode)") + "\n"; coloured != want {
		t.Errorf("output with colour = %q, want %q", coloured, want)
	}
	if ui.Strip(coloured) != plain {
		t.Errorf("taking the colour out gives %q, want %q", ui.Strip(coloured), plain)
	}
}

func TestPromptYesNoStillReadsTheAnswerWhenColoured(t *testing.T) {
	cmdtest.SetStdin(t, "n\ny\n")
	cmdtest.SetColor(t, ui.Always)

	var first, second bool
	cmdtest.CaptureStdout(t, func() {
		first = PromptYesNo("First? [Y/n]: ")
		second = PromptYesNo("Second? [Y/n]: ")
	})
	if first || !second {
		t.Errorf("answers = %v, %v, want false, true: colour can't change what is read", first, second)
	}
}

func TestAddToZipSaysWhatWentWrongInRed(t *testing.T) {
	idx, _ := newTestIndex(t)
	exp := zip.NewWriter(&bytes.Buffer{})
	dl := core.CompletedDownload{
		Mod:   &core.Mod{Name: "Broken Mod", FileName: "broken.jar"},
		Error: errors.New("download failed"),
	}

	_, coloured := cmdtest.AssertColourOnlyAdds(t, func() { AddToZip(dl, exp, "overrides", &idx) })

	if want := ui.Error.Sprint("Download of Broken Mod (broken.jar) failed: download failed") + "\n"; coloured != want {
		t.Errorf("output = %q, want %q", coloured, want)
	}
}

// Export lists what went into the pack once it is done, so adding a file to it says nothing
func TestAddToZipSaysNothingWhenItAdds(t *testing.T) {
	idx, dir := newTestIndex(t)
	if err := os.MkdirAll(filepath.Join(dir, "mods"), 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	mod := &core.Mod{Name: "Test Mod", FileName: "test.jar"}
	mod.SetMetaPath(filepath.Join(dir, "mods", "test.pw.toml"))
	exp := zip.NewWriter(&bytes.Buffer{})
	file, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		t.Fatalf("failed to create temp download file: %v", err)
	}

	var added bool
	out := cmdtest.CaptureStdout(t, func() { added = AddToZip(core.CompletedDownload{Mod: mod, File: file}, exp, "overrides", &idx) })

	if !added {
		t.Error("AddToZip() = false, want true")
	}
	if out != "" {
		t.Errorf("output = %q, want none", out)
	}
}

func TestPrintDisclaimerIsAWarning(t *testing.T) {
	plain, coloured := cmdtest.AssertColourOnlyAdds(t, PrintDisclaimer)

	lines := strings.Split(strings.TrimSuffix(plain, "\n\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "Disclaimer: ") {
		t.Fatalf("plain output = %q, want the disclaimer in two lines", plain)
	}
	for _, line := range lines {
		if want := ui.Warning.Sprint(line); !strings.Contains(coloured, want) {
			t.Errorf("output missing the warning %q:\n%q", want, coloured)
		}
	}
}
