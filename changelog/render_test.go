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

> This release changes mods that run on the server; servers must be updated to match.

### Added

- **Lithium** 0.12.0 (server)

### Removed

- **Old** 1.0 (client)

### Updated

- **Sodium** 0.5.7 → 0.5.8 (client)

### Config

- Changed ` + "`config/sodium.json`" + `
- Added ` + "`config/new.toml`" + `
- Removed ` + "`config/gone.toml`" + `
`
	if got := RenderRelease(release); got != want {
		t.Errorf("RenderRelease() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderReleaseOnlyMajorGetsServerNote(t *testing.T) {
	for bump, wantNote := range map[Bump]bool{BumpNone: false, BumpPatch: false, BumpMinor: false, BumpMajor: true} {
		got := RenderRelease(Release{Version: "1.0.0", Date: "2026-01-01", Bump: bump})
		if has := strings.Contains(got, "servers must be updated"); has != wantNote {
			t.Errorf("bump %v: server note present = %v, want %v", bump, has, wantNote)
		}
	}
}

func TestRenderReleaseSkipsEmptySections(t *testing.T) {
	got := RenderRelease(Release{
		Version: "1.0.1", Date: "2026-01-02", Bump: BumpPatch,
		Changes: []Change{{Kind: FileChanged, Path: "config/a.json"}},
	})
	for _, heading := range []string{"### Added", "### Removed", "### Updated"} {
		if strings.Contains(got, heading) {
			t.Errorf("RenderRelease() has %q with nothing under it:\n%s", heading, got)
		}
	}
	if !strings.Contains(got, "### Config") {
		t.Errorf("RenderRelease() is missing the Config section:\n%s", got)
	}
}

func TestRenderReleaseSidelessModShowsBoth(t *testing.T) {
	got := RenderRelease(Release{
		Version: "1.1.0", Date: "2026-01-02",
		Changes: []Change{{Kind: ModAdded, Name: "Legacy", To: "1.0"}},
	})
	if !strings.Contains(got, "- **Legacy** 1.0 (both)") {
		t.Errorf("RenderRelease() = %s, want a sideless mod listed as (both)", got)
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
		t.Errorf("RenderMarkdown(nil) = %q, want only the title", got)
	}
}
