package changelog

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// colourfulRelease has one of every kind of change that is listed, so that every part of a release that can be coloured
// is there to be.
func colourfulRelease() Release {
	return Release{
		Version: "2.0.0", Date: "2026-02-01", Bump: BumpMajor,
		Changes: []Change{
			{Kind: FileChanged, Path: "config/sodium.json"},
			{Kind: ModRemoved, Path: "mods/old.pw.toml", Name: "Old", Side: core.ClientSide, From: "1.0"},
			{Kind: ModAdded, Path: "mods/lithium.pw.toml", Name: "Lithium", Side: core.ServerSide, To: "0.12.0"},
			{Kind: ModUpdated, Path: "mods/sodium.pw.toml", Name: "Sodium", Side: core.ClientSide, From: "0.5.7", To: "0.5.8"},
			{Kind: FileAdded, Path: "config/new.toml"},
			{Kind: FileRemoved, Path: "config/gone.toml"},
			{Kind: Note, Type: "feat", Scope: "world", Text: "reset the spawn point"},
			{Kind: Note, Type: "feat", Text: "update to Minecraft 1.21.4", Breaking: true},
		},
	}
}

// What a release says can't depend on whether it is coloured: the colour is added to the text, and nothing else.
func TestColouredReleaseIsTheSameTextWithColourAdded(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	r := colourfulRelease()

	got := renderRelease(r, style{colour: true})

	if got == RenderRelease(r) {
		t.Error("renderRelease() has no colour with colour on")
	}
	if stripped, want := ui.Strip(got), RenderRelease(r); stripped != want {
		t.Errorf("taking the colour out of the release doesn't give the plain one:\n%s\nwant\n%s", stripped, want)
	}
}

func TestColouredReleasePicksOutItsParts(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)

	got := renderRelease(colourfulRelease(), style{colour: true})

	for name, want := range map[string]string{
		"the heading":                    ui.Bold.Sprint(ui.Info.Sprint("## 2.0.0 - 2026-02-01")),
		"the server update callout":      ui.Warning.Sprint("> **Server update required.** This release changes mods that run on the server."),
		"a section heading":              ui.Bold.Sprint("### Updated"),
		"the name of a mod":              ui.Bold.Sprint("**Sodium**"),
		"the version that was replaced":  ui.Error.Sprint("0.5.7") + " → ",
		"the version that replaced it":   " → " + ui.Success.Sprint("0.5.8"),
		"a version that was added":       ui.Success.Sprint("0.12.0"),
		"a version that was removed":     ui.Error.Sprint("1.0"),
		"where a mod runs":               ui.Muted.Sprint("(client)"),
		"a file that changed":            "Changed " + ui.Info.Sprint("`config/sodium.json`"),
		"the scope of a note":            ui.Bold.Sprint("**world:**"),
		"a breaking note":                ui.Error.Sprint("**Breaking:**"),
		"a file that was added":          "Added " + ui.Info.Sprint("`config/new.toml`"),
		"a file that was removed":        "Removed " + ui.Info.Sprint("`config/gone.toml`"),
		"a name isn't given its version": ui.Bold.Sprint("**Lithium**") + " " + ui.Success.Sprint("0.12.0"),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%s: release missing %q:\n%q", name, want, got)
		}
	}
}

// The Markdown that is saved is read by people and by other programs, so it can't have colour in it however the
// command is asked to print.
func TestSavedMarkdownIsNeverColoured(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)

	for name, got := range map[string]string{
		"RenderRelease":  RenderRelease(colourfulRelease()),
		"RenderMarkdown": RenderMarkdown([]Release{colourfulRelease()}),
		"describe":       describe(colourfulRelease().Changes[3]),
	} {
		if strings.Contains(got, "\x1b") {
			t.Errorf("%s has escape sequences in it: %q", name, got)
		}
	}
}

func TestPreviewColourOnlyAdds(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("fix(config): change config/a.json")
	repo.pending = []Commit{commit("feat(mods)!: add Lithium 0.12.0 (server)", "", serverFooter)}

	_, coloured := cmdtest.AssertColourOnlyAdds(t, func() {
		if err := runPreview(""); err != nil {
			t.Fatalf("runPreview() returned error: %v", err)
		}
	})

	for name, want := range map[string]string{
		"the next version":   ui.Bold.Sprint("2.0.0"),
		"the size of a bump": ui.Muted.Sprint("(major bump)"),
		"the release":        ui.Bold.Sprint(ui.Info.Sprint("## 2.0.0 - 2026-09-18")),
		"a mod that's added": ui.Bold.Sprint("**Lithium**") + " " + ui.Success.Sprint("0.12.0"),
	} {
		if !strings.Contains(coloured, want) {
			t.Errorf("%s: preview missing %q:\n%q", name, want, coloured)
		}
	}
}

func TestReleaseAsksAndCancelsInColour(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("fix(config): change config/a.json")
	cmdtest.SetStdin(t, "n\n")
	cmdtest.SetColor(t, ui.Always)

	var err error
	out := cmdtest.CaptureStdout(t, func() { _, _, err = RunRelease("", "") })
	if err != nil {
		t.Fatalf("RunRelease() returned error: %v\noutput: %s", err, out)
	}

	if want := ui.Prompt("Release 1.0.1? [Y/n]: "); !strings.Contains(out, want) {
		t.Errorf("output missing the prompt %q:\n%q", want, out)
	}
	if want := ui.Warning.Sprint("Cancelled!"); !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%q", want, out)
	}
}

func TestNothingToReleaseIsAnInfoNotice(t *testing.T) {
	releasedOnce(t)
	cmdtest.SetColor(t, ui.Always)

	out := cmdtest.WithoutProgress(cmdtest.CaptureStdout(t, func() {
		if err := runPreview(""); err != nil {
			t.Fatalf("runPreview() returned error: %v", err)
		}
	}))

	if want := ui.Info.Sprint("No changes since the last release (1.0.0).") + "\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
