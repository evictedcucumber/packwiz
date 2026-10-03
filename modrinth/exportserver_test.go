package modrinth

import (
	"archive/zip"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
)

// readZip reads a zip: what is in it, by name.
func readZip(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("failed to open %s: %v", path, err)
	}
	defer func() { _ = r.Close() }()
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if _, ok := files[f.Name]; ok {
			t.Errorf("%s is in the zip twice", f.Name)
		}
		files[f.Name] = string(data)
	}
	return files
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
}

// serverModList writes the server's list of mods, as "packwiz list --save --side server" does
func serverModList(t *testing.T) {
	t.Helper()
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		t.Fatalf("LoadAllMods() returned error: %v", err)
	}
	if _, err := cmd.SaveServerModList(pack, index, mods); err != nil {
		t.Fatalf("SaveServerModList() returned error: %v", err)
	}
}

func TestServerExportHasTheServersModsAndFilesLaidOutAsOnTheServer(t *testing.T) {
	client := validMod("shaders")
	client.side = core.ClientSide
	server := validMod("worldgen")
	server.side = core.ServerSide
	pack, index := exportablePack(t, validMod("alpha"), client, server)
	// Optional mods are on the server as they are on a client: only if they are on by default
	for name, on := range map[string]bool{"extra": true, "spare": false} {
		m := validMod(name)
		sum := sha512.Sum512([]byte("bytes of " + name))
		m.hash = hex.EncodeToString(sum[:])
		content := m.toml() + "\n[option]\noptional = true\ndefault = " + map[bool]string{true: "true", false: "false"}[on] + "\n"
		addModFile(t, &index, name, content)
		httpmock.RegisterResponder("GET", "https://cdn.modrinth.com/data/"+name+"/versions/v1/"+name+".jar", httpmock.NewStringResponder(200, "bytes of "+name))
	}
	saveFixture(t, pack, index)

	writeTestFile(t, "config/alpha.toml", "client = true")
	writeTestFile(t, "config/beta.toml", "shared = true")
	writeTestFile(t, "README.md", "about the pack")
	writeTestFile(t, "LICENSE", "the terms")
	writeTestFile(t, "CHANGELOG.md", "what changed")
	writeTestFile(t, "MODS.md", "every mod")
	writeTestFile(t, "serverconfig/server.properties", "motd=hi")
	writeTestFile(t, "serverconfig/config/alpha.toml", "client = false")
	serverModList(t)
	output := t.TempDir() + "/server.zip"

	var result *ExportResult
	var err error
	out := cmdtest.CaptureStdout(t, func() {
		result, err = Export(ExportOptions{Output: output, Server: true}, nil)
	})
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("Export() wrote %q to the terminal", out)
	}
	if !result.Server || result.Path != output {
		t.Errorf("the result is %+v, want the server pack at %s", result, output)
	}
	if len(result.Notices) != 0 {
		t.Errorf("the notices are %v, want none", result.Notices)
	}

	files := readZip(t, output)
	want := map[string]string{
		"mods/alpha.jar":    "bytes of alpha",
		"mods/worldgen.jar": "bytes of worldgen",
		"mods/extra.jar":    "bytes of extra",
		"config/alpha.toml": "client = false",
		"config/beta.toml":  "shared = true",
		"server.properties": "motd=hi",
		"README.md":         "about the pack",
		"LICENSE":           "the terms",
		"CHANGELOG.md":      "what changed",
	}
	for name, content := range want {
		if got, ok := files[name]; !ok || got != content {
			t.Errorf("%s holds %q (there: %v), want %q", name, got, ok, content)
		}
	}
	for _, name := range []string{"mods/shaders.jar", "mods/spare.jar", "modrinth.index.json", "overrides/"} {
		if _, ok := files[name]; ok {
			t.Errorf("the server pack has %s", name)
		}
	}
	for name := range files {
		if strings.HasSuffix(name, core.MetaExtension) || strings.HasPrefix(name, core.ServerConfigDir) {
			t.Errorf("the server pack has %s", name)
		}
	}
	// The server's own list of mods, not the pack's
	list := files["MODS.md"]
	if !strings.Contains(list, "worldgen") || strings.Contains(list, "shaders") || list == "every mod" {
		t.Errorf("MODS.md is %q, want the list of the server's mods", list)
	}

	names := make([]string, len(result.Files))
	for i, f := range result.Files {
		names[i] = f.Name
		if f.Bundled || f.Size == 0 {
			t.Errorf("the file is %+v, want one in the pack with its size", f)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"alpha", "extra", "worldgen"}) {
		t.Errorf("the files are %v, want the server's mods", names)
	}

	// The server's files are nothing a client installs
	pack, err = core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if index, err = pack.LoadIndex(); err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	for p := range index.Files {
		if strings.HasPrefix(p, core.ServerConfigDir) {
			t.Errorf("the index lists %s", p)
		}
	}
}

func TestServerExportSaysWhenTheServersListOfModsIsMissingOrOutOfDate(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	output := t.TempDir() + "/server.zip"

	result, err := Export(ExportOptions{Output: output, Server: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if notices := strings.Join(result.Notices, "\n"); !strings.Contains(notices, "there is no serverconfig/MODS.md") || !strings.Contains(notices, "packwiz list --save --side server") {
		t.Errorf("the notices are %v, want one that the list is missing and how to write it", result.Notices)
	}
	if _, ok := readZip(t, output)["MODS.md"]; ok {
		t.Errorf("the server pack has a MODS.md, though there was none to put in it")
	}

	serverModList(t)
	if result, err = Export(ExportOptions{Output: output, Server: true}, nil); err != nil || len(result.Notices) != 0 {
		t.Fatalf("Export() = %v, %v, want no notices with the list written", result, err)
	}

	writeTestFile(t, core.ServerModListFile, "# Old\n")
	if result, err = Export(ExportOptions{Output: output, Server: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if notices := strings.Join(result.Notices, "\n"); !strings.Contains(notices, "doesn't list the server's mods as they are now") {
		t.Errorf("the notices are %v, want one that the list is out of date", result.Notices)
	}
}

func TestServerExportIsNamedAfterThePackByDefault(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	result, err := Export(ExportOptions{Server: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if result.Path != "Test Pack-1.0.0-server.zip" {
		t.Errorf("Path = %q, want the pack's name and version, as a server pack", result.Path)
	}
	// And exporting again doesn't put the last one in the pack
	if _, err := Export(ExportOptions{Server: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if _, ok := readZip(t, result.Path)[result.Path]; ok {
		t.Errorf("the server pack has the server pack exported before it")
	}
}

func TestServerExportFailsWhereThePackCannotBeWritten(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	if _, err := Export(ExportOptions{Output: t.TempDir() + "/no-such-folder/server.zip", Server: true}, nil); err == nil || !strings.Contains(err.Error(), "Failed to create zip") {
		t.Errorf("Export() returned %v, want an error that the pack couldn't be created", err)
	}
}

func TestServerExportTakesAFileNamedServerconfigForAnyOtherFile(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	writeTestFile(t, core.ServerConfigDir, "not a folder")
	output := t.TempDir() + "/server.zip"
	if _, err := Export(ExportOptions{Output: output, Server: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	// It is a file of the pack like any other, not a folder of the server's files
	for name := range readZip(t, output) {
		if name != "mods/alpha.jar" && name != core.ServerConfigDir {
			t.Errorf("the server pack has %s", name)
		}
	}
}

func TestServerExportHasTheServersChangelogAndSaysWhenItIsMissingOrOutOfDate(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	history := changelog.History{Releases: []changelog.Release{
		{Version: "1.0.0", Date: "2026-01-01", Changes: []changelog.Change{{Kind: changelog.ModAdded, Name: "alpha", Side: core.UniversalSide, To: "1.0.0"}}},
	}}
	if err := history.Write(changelog.HistoryFile); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	writeTestFile(t, changelog.MarkdownFile, changelog.RenderMarkdown(history.Releases))
	serverModList(t)
	output := t.TempDir() + "/server.zip"
	export := func() (*ExportResult, map[string]string) {
		t.Helper()
		result, err := Export(ExportOptions{Output: output, Server: true}, nil)
		if err != nil {
			t.Fatalf("Export() returned error: %v", err)
		}
		return result, readZip(t, output)
	}

	// Without it, the server pack has the pack's own, and says how to write the server's
	result, files := export()
	if notices := strings.Join(result.Notices, "\n"); !strings.Contains(notices, "there is no serverconfig/CHANGELOG.md") || !strings.Contains(notices, "packwiz changelog --save") {
		t.Errorf("the notices are %v, want one that the server's changelog is missing and how to write it", result.Notices)
	}
	if files[changelog.MarkdownFile] != changelog.RenderMarkdown(history.Releases) {
		t.Errorf("CHANGELOG.md is %q, want the pack's own", files[changelog.MarkdownFile])
	}

	server := changelog.RenderServerMarkdown(history.Releases, nil)
	writeTestFile(t, changelog.ServerMarkdownFile, server)
	if result, files = export(); len(result.Notices) != 0 || files[changelog.MarkdownFile] != server {
		t.Errorf("the notices are %v and CHANGELOG.md is %q, want none and the server's", result.Notices, files[changelog.MarkdownFile])
	}

	// A release since makes it out of date
	history.Releases = append(history.Releases, changelog.Release{Version: "1.1.0", Date: "2026-02-01", Bump: changelog.BumpMinor})
	if err := history.Write(changelog.HistoryFile); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if result, _ = export(); !strings.Contains(strings.Join(result.Notices, "\n"), "doesn't have the pack's releases as they are now") {
		t.Errorf("the notices are %v, want one that the server's changelog is out of date", result.Notices)
	}
}

func TestServerExportSaysNothingOfAChangelogForAPackWithNoReleases(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	serverModList(t)
	result, err := Export(ExportOptions{Output: t.TempDir() + "/server.zip", Server: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if len(result.Notices) != 0 {
		t.Errorf("the notices are %v, want none", result.Notices)
	}
}
