package changelog

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestRenderReleaseGroupsBySection(t *testing.T) {
	release := Release{
		Version: "2.0.0", Date: "2026-02-01", Bump: BumpMajor,
		Changes: []Change{
			{Kind: FileChanged, Path: "config/sodium.json"},
			{Kind: ModRemoved, Path: "mods/old.pw.toml", Name: "Old", Side: core.ClientSide, From: "1.0"},
			{Kind: ModAdded, Path: "mods/lithium.pw.toml", Name: "Lithium", Side: core.ServerSide, To: "0.12.0"},
			{Kind: ModUpdated, Path: "mods/sodium.pw.toml", Name: "Sodium", Side: core.ClientSide, From: "0.5.7", To: "0.5.8"},
			{Kind: FileAdded, Path: "config/new.toml"},
			{Kind: FileRemoved, Path: "config/gone.toml"},
		},
	}

	want := `## 2.0.0 - 2026-02-01

> **Server update required.** This release changes mods that run on the server.

### Added

- **Lithium** 0.12.0 (server)

### Updated

- **Sodium** 0.5.7 → 0.5.8 (client)

### Removed

- **Old** 1.0 (client)

### Config

- Removed ` + "`config/gone.toml`" + `
- Added ` + "`config/new.toml`" + `
- Changed ` + "`config/sodium.json`" + `
`
	if got := RenderRelease(release); got != want {
		t.Errorf("RenderRelease() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderReleaseListsServerModsFirstThenAlphabetically(t *testing.T) {
	got := RenderRelease(Release{
		Version: "2.0.0", Date: "2026-02-01", Bump: BumpMajor,
		Changes: []Change{
			{Kind: ModAdded, Path: "mods/zoom.pw.toml", Name: "Zoom", Side: core.ClientSide, To: "1"},
			{Kind: ModAdded, Path: "mods/sodium.pw.toml", Name: "Sodium", Side: core.ClientSide, To: "1"},
			{Kind: ModAdded, Path: "mods/lithium.pw.toml", Name: "lithium", Side: core.ServerSide, To: "1"},
			{Kind: ModAdded, Path: "mods/fabric-api.pw.toml", Name: "Fabric API", Side: core.UniversalSide, To: "1"},
			{Kind: ModAdded, Path: "mods/legacy.pw.toml", Name: "Another", To: "1"}, // sideless is both
		},
	})

	var names []string
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "- **") {
			names = append(names, strings.SplitN(strings.TrimPrefix(line, "- **"), "**", 2)[0])
		}
	}
	// What runs on the server first (case-insensitively alphabetical), then client-only mods
	want := "Another, Fabric API, lithium, Sodium, Zoom"
	if joined := strings.Join(names, ", "); joined != want {
		t.Errorf("mods listed as %q, want %q", joined, want)
	}
}

func TestRenderReleaseDoesNotReorderItsInput(t *testing.T) {
	// The changes are what gets stored in the history, in the order Diff gave them
	changes := []Change{
		{Kind: ModAdded, Path: "mods/z.pw.toml", Name: "Z", Side: core.ClientSide, To: "1"},
		{Kind: ModAdded, Path: "mods/a.pw.toml", Name: "A", Side: core.ClientSide, To: "1"},
	}

	RenderRelease(Release{Version: "1.1.0", Date: "2026-01-02", Bump: BumpMinor, Changes: changes})

	if changes[0].Name != "Z" || changes[1].Name != "A" {
		t.Errorf("RenderRelease() reordered the release's changes: %+v", changes)
	}
}

func TestRenderReleaseInitialRelease(t *testing.T) {
	got := RenderRelease(Release{
		Version: "1.0.0", Date: "2026-01-01", Bump: BumpNone,
		Changes: []Change{
			{Kind: ModAdded, Path: "mods/a.pw.toml", Name: "A", Side: core.ClientSide, To: "1"},
			{Kind: ModAdded, Path: "mods/b.pw.toml", Name: "B", Side: core.ServerSide, To: "1"},
			{Kind: FileAdded, Path: "config/a.json"},
		},
	})

	if !strings.Contains(got, "\nInitial release with 2 mods and 1 config file.\n") {
		t.Errorf("RenderRelease() should summarise an initial release:\n%s", got)
	}
	// It has server mods, but nothing to update a server from
	if strings.Contains(got, "Server update required") {
		t.Errorf("an initial release shouldn't ask for a server update:\n%s", got)
	}
}

func TestInitialSummary(t *testing.T) {
	mod := Change{Kind: ModAdded}
	file := Change{Kind: FileAdded}
	tests := []struct {
		name    string
		changes []Change
		want    string
	}{
		{"one mod", []Change{mod}, "1 mod"},
		{"mods", []Change{mod, mod, mod}, "3 mods"},
		{"one config file", []Change{file}, "1 config file"},
		{"both", []Change{mod, mod, file, file}, "2 mods and 2 config files"},
		{"nothing", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := initialSummary(tt.changes); got != tt.want {
				t.Errorf("initialSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderReleaseAsksForAServerUpdateOnlyForChangesToModsOnTheServer(t *testing.T) {
	change := func(kind Kind, side string) []Change { return []Change{{Kind: kind, Name: "M", Side: side, To: "1"}} }
	tests := []struct {
		name    string
		bump    Bump
		changes []Change
		want    bool
	}{
		{"a server mod", BumpMajor, change(ModAdded, core.ServerSide), true},
		{"a mod on both sides", BumpMajor, change(ModUpdated, core.UniversalSide), true},
		{"a client mod", BumpMinor, change(ModAdded, core.ClientSide), false},
		{"config", BumpPatch, []Change{{Kind: FileChanged, Path: "config/a.json"}}, false},
		// A change can be breaking without anything on the server changing, such as moving to a new Minecraft version
		{"a breaking note", BumpMajor, []Change{{Kind: Note, Type: "feat", Breaking: true, Text: "update"}}, false},
		// There is nothing to update a server from for the first release
		{"the first release", BumpNone, change(ModAdded, core.ServerSide), false},
		{"nothing", BumpMajor, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderRelease(Release{Version: "1.0.0", Date: "2026-01-01", Bump: tt.bump, Changes: tt.changes})
			if has := strings.Contains(got, "Server update required"); has != tt.want {
				t.Errorf("server note present = %v, want %v:\n%s", has, tt.want, got)
			}
		})
	}
}

func TestRenderReleaseNotes(t *testing.T) {
	got := RenderRelease(Release{
		Version: "2.0.0", Date: "2026-02-01", Bump: BumpMajor,
		Changes: []Change{
			{Kind: Note, Type: "feat", Scope: "world", Text: "reset the spawn point"},
			{Kind: Note, Type: "fix", Scope: "config", Text: "lower the particle count"},
			{Kind: FileChanged, Path: "config/a.json"},
			{Kind: Note, Type: "feat", Text: "update to Minecraft 1.21.4", Breaking: true},
			{Kind: Note, Type: "fix", Scope: "config", Text: "raise the fps cap"},
			{Kind: Note, Type: "feat", Text: "add a *splash* screen"},
		},
	})

	want := `## 2.0.0 - 2026-02-01

### Config

- Changed ` + "`config/a.json`" + `
- lower the particle count
- raise the fps cap

### Changes

- **world:** reset the spawn point
- **Breaking:** update to Minecraft 1.21.4
- add a \*splash\* screen
`
	if got != want {
		t.Errorf("RenderRelease() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderReleaseSkipsEmptySections(t *testing.T) {
	got := RenderRelease(Release{
		Version: "1.0.1", Date: "2026-01-02", Bump: BumpPatch,
		Changes: []Change{{Kind: FileChanged, Path: "config/a.json"}},
	})
	for _, heading := range []string{"### Added", "### Updated", "### Removed"} {
		if strings.Contains(got, heading) {
			t.Errorf("RenderRelease() has %q with nothing under it:\n%s", heading, got)
		}
	}
	if !strings.Contains(got, "### Config") {
		t.Errorf("RenderRelease() is missing the Config section:\n%s", got)
	}
}

func TestRenderReleaseSaysWhereModsRunInPlainWords(t *testing.T) {
	got := RenderRelease(Release{
		Version: "1.1.0", Date: "2026-01-02", Bump: BumpMajor,
		Changes: []Change{
			{Kind: ModAdded, Name: "Both", Side: core.UniversalSide, To: "1"},
			{Kind: ModAdded, Name: "Legacy", To: "1"},
			{Kind: ModAdded, Name: "Server", Side: core.ServerSide, To: "1"},
			{Kind: ModAdded, Name: "Client", Side: core.ClientSide, To: "1"},
		},
	})
	for _, want := range []string{"- **Both** 1 (client + server)", "- **Legacy** 1 (client + server)", "- **Server** 1 (server)", "- **Client** 1 (client)"} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderRelease() missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "(both)") {
		t.Errorf("RenderRelease() used packwiz's \"both\" where a reader wants words:\n%s", got)
	}
}

func TestDescribeToleratesMissingVersions(t *testing.T) {
	tests := []struct {
		name   string
		change Change
		want   string
	}{
		{"added without version", Change{Kind: ModAdded, Name: "A", Side: core.ClientSide}, "**A** (client)"},
		{"removed uses from", Change{Kind: ModRemoved, Name: "A", Side: core.ClientSide, From: "1"}, "**A** 1 (client)"},
		{"updated", Change{Kind: ModUpdated, Name: "A", Side: core.ClientSide, From: "1", To: "2"}, "**A** 1 → 2 (client)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describe(tt.change); got != tt.want {
				t.Errorf("describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDescribeEscapesWhatThePublisherWrote(t *testing.T) {
	got := describe(Change{Kind: ModUpdated, Name: "*Bold* <b>[x]</b> `y` a_b", Side: core.ClientSide, From: "1_beta", To: "2*"})
	want := `**\*Bold\* \<b\>\[x\]\</b\> \` + "`y\\`" + ` a\_b** 1\_beta → 2\* (client)`
	if got != want {
		t.Errorf("describe() = %q, want %q", got, want)
	}
}

func TestRenderMarkdownNewestFirst(t *testing.T) {
	got := RenderMarkdown(sampleHistory().Releases)

	if !strings.HasPrefix(got, "# Changelog\n") {
		t.Errorf("RenderMarkdown() should start with a title, got:\n%s", got)
	}
	newer, older := strings.Index(got, "## 2.0.0"), strings.Index(got, "## 1.0.0")
	if newer < 0 || older < 0 || newer > older {
		t.Errorf("RenderMarkdown() should list 2.0.0 before 1.0.0 (indexes %d, %d):\n%s", newer, older, got)
	}
}

func TestRenderMarkdownWithNoReleases(t *testing.T) {
	got := RenderMarkdown(nil)
	if !strings.HasPrefix(got, "# Changelog") || strings.Contains(got, "##") {
		t.Errorf("RenderMarkdown(nil) = %q, want only the title and introduction", got)
	}
}

func TestRenderMarkdownFullDocument(t *testing.T) {
	// The whole file, as a reader will see it
	want := "# Changelog\n\n" +
		"All notable changes to this modpack are documented here.\n\n" +
		"Generated by `packwiz changelog release`; edits made here are overwritten.\n\n" +
		"## 1.1.0 - 2026-02-01\n\n" +
		"### Updated\n\n" +
		"- **Iris** 1.7.0 → 1.7.1 (client)\n\n" +
		"## 1.0.0 - 2026-01-01\n\n" +
		"Initial release with 1 mod.\n\n" +
		"### Added\n\n" +
		"- **Iris** 1.7.0 (client)\n"

	got := RenderMarkdown([]Release{
		{Version: "1.0.0", Date: "2026-01-01", Bump: BumpNone, Changes: []Change{{Kind: ModAdded, Name: "Iris", Side: core.ClientSide, To: "1.7.0"}}},
		{Version: "1.1.0", Date: "2026-02-01", Bump: BumpPatch, Changes: []Change{{Kind: ModUpdated, Name: "Iris", Side: core.ClientSide, From: "1.7.0", To: "1.7.1"}}},
	})
	if got != want {
		t.Errorf("RenderMarkdown() =\n%s\nwant\n%s", got, want)
	}
}
