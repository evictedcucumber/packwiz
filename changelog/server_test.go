package changelog

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// serverReleases are a first release, one that changes the server, and one that only changes the client
var serverReleases = []Release{
	{Version: "1.0.0", Date: "2026-01-01", Bump: BumpNone, Changes: []Change{
		{Kind: ModAdded, Name: "Sodium", Side: core.ClientSide, To: "0.5.7"},
		{Kind: ModAdded, Name: "Lithium", Side: core.ServerSide, To: "0.12.0"},
		{Kind: ModAdded, Name: "Create", Side: core.UniversalSide, To: "6.0"},
	}},
	{Version: "2.0.0", Date: "2026-02-01", Bump: BumpMajor, Changes: []Change{
		{Kind: ModUpdated, Name: "Lithium", Side: core.ServerSide, From: "0.12.0", To: "0.12.1"},
		{Kind: ModUpdated, Name: "Sodium", Side: core.ClientSide, From: "0.5.7", To: "0.5.8"},
		{Kind: FileChanged, Path: "config/create.toml"},
		{Kind: Note, Type: "feat", Text: "add a splash screen"},
	}},
	{Version: "2.1.0", Date: "2026-03-01", Bump: BumpMinor, Changes: []Change{
		{Kind: ModAdded, Name: "Iris", Side: core.ClientSide, To: "1.0"},
	}},
}

func TestTheServersChangelogLeavesOutTheModsOnlyOnTheClient(t *testing.T) {
	text := RenderServerMarkdown(serverReleases, nil)

	if !strings.HasPrefix(text, "# Server Changelog\n") {
		t.Errorf("the server's changelog doesn't start with its title:\n%s", text)
	}
	for _, want := range []string{
		"Initial release with 2 mods.", "**Lithium** 0.12.0 → 0.12.1 (server)", "**Create** 6.0 (client + server)",
		"Changed `config/create.toml`", "add a splash screen",
		"## 2.1.0 - 2026-03-01\n\nNothing in this release changes the server.\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the server's changelog doesn't have %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Sodium", "Iris"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the server's changelog has %s, which is only on the client:\n%s", unwanted, text)
		}
	}
	// Every release is there, newest first
	if a, b, c := strings.Index(text, "## 2.1.0"), strings.Index(text, "## 2.0.0"), strings.Index(text, "## 1.0.0"); !(0 < a && a < b && b < c) {
		t.Errorf("the releases aren't all there newest first:\n%s", text)
	}
	// What is on the server is rendered as the pack's changelog renders it
	r := serverReleases[1]
	r.Changes = slices.DeleteFunc(slices.Clone(r.Changes), func(c Change) bool { return c.Name == "Sodium" })
	if !strings.Contains(text, RenderRelease(r)) {
		t.Errorf("the server's changelog doesn't render 2.0.0 as the pack's changelog does:\n%s\nwant it to have\n%s", text, RenderRelease(r))
	}
}

func TestTheServersChangelogListsOnlyWhatIsUnreleasedOnTheServer(t *testing.T) {
	client := []Change{{Kind: ModAdded, Name: "Zoom", Side: core.ClientSide, To: "1.0"}}
	if text := RenderServerMarkdown(serverReleases, client); strings.Contains(text, "Unreleased") {
		t.Errorf("the server's changelog lists unreleased changes that are only on the client:\n%s", text)
	}
	server := append(client, Change{Kind: ModAdded, Name: "Spark", Side: core.ServerSide, To: "1.0"})
	text := RenderServerMarkdown(serverReleases, server)
	if !strings.Contains(text, "## Unreleased") || !strings.Contains(text, "Spark") || strings.Contains(text, "Zoom") {
		t.Errorf("the server's changelog doesn't list only the server's unreleased changes:\n%s", text)
	}
	// With or without them, it is current, as what is unreleased isn't a release
	for _, text := range []string{text, RenderServerMarkdown(serverReleases, nil)} {
		if !ServerMarkdownIsCurrent(text, serverReleases) {
			t.Errorf("ServerMarkdownIsCurrent() = false for what was just rendered:\n%s", text)
		}
	}
	if ServerMarkdownIsCurrent(RenderServerMarkdown(serverReleases[:2], nil), serverReleases) {
		t.Error("ServerMarkdownIsCurrent() = true for a changelog without the last release")
	}
	if ServerMarkdownIsCurrent(RenderMarkdown(serverReleases), serverReleases[:1]) {
		t.Error("ServerMarkdownIsCurrent() = true for the pack's own changelog")
	}
}

func TestAReleaseWritesTheServersChangelogOnlyForAPackWithAServerPack(t *testing.T) {
	repo := releasedOnce(t)
	if _, err := os.Stat(ServerMarkdownFile); !os.IsNotExist(err) {
		t.Fatalf("a pack without %s/ got a server changelog (%v)", core.ServerConfigDir, err)
	}

	if err := os.Mkdir(core.ServerConfigDir, 0o755); err != nil {
		t.Fatalf("Mkdir() returned error: %v", err)
	}
	repo.commit("feat(mods)!: add Lithium 0.12.0 (server)", "", serverFooter)
	writeMod(t, "Lithium", core.ServerSide, "0.12.0")
	release(t, "")

	history := loadHistory(t)
	text := readFile(t, ServerMarkdownFile)
	if text != RenderServerMarkdown(history.Releases, nil) {
		t.Errorf("%s =\n%s\nwant it rendered from the releases", ServerMarkdownFile, text)
	}
	if !strings.Contains(text, "Lithium") || strings.Contains(text, "Sodium") {
		t.Errorf("%s doesn't have only the server's mods:\n%s", ServerMarkdownFile, text)
	}
}

func TestSavingTheChangelogWritesTheServersToo(t *testing.T) {
	repo := releasedOnce(t)
	repo.commit("feat(mods)!: add Lithium 0.12.0 (server)", "", serverFooter)
	writeMod(t, "Lithium", core.ServerSide, "0.12.0")
	if err := os.Mkdir(core.ServerConfigDir, 0o755); err != nil {
		t.Fatalf("Mkdir() returned error: %v", err)
	}

	var paths []string
	var err error
	out := cmdtest.CaptureStdout(t, func() { paths, _, err = SaveMarkdown() })
	if err != nil {
		t.Fatalf("SaveMarkdown() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("SaveMarkdown() wrote %q to the terminal", out)
	}
	if want := []string{MarkdownFile, filepath.FromSlash(ServerMarkdownFile)}; !slices.Equal(paths, want) {
		t.Errorf("SaveMarkdown() = %q, want %q", paths, want)
	}
	if text := readFile(t, ServerMarkdownFile); !strings.Contains(text, "## Unreleased") || !strings.Contains(text, "Lithium") {
		t.Errorf("%s doesn't list the server's unreleased change:\n%s", ServerMarkdownFile, text)
	}

	out = cmdtest.CaptureStdout(t, func() { err = runSave() })
	if err != nil {
		t.Fatalf("runSave() returned error: %v", err)
	}
	if want := "Wrote CHANGELOG.md.\nWrote " + ServerMarkdownFile + ".\n"; cmdtest.WithoutProgress(out) != want {
		t.Errorf("runSave() printed %q, want %q", out, want)
	}
}
