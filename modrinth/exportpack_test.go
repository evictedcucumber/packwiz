package modrinth

import (
	"archive/zip"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
	"github.com/spf13/viper"
)

// exportablePack is a pack with mods that can be downloaded from a Modrinth that serves them, on disk as a pack that is
// used is, with a cache of downloads of its own.
func exportablePack(t *testing.T, mods ...modFile) (core.Pack, core.Index) {
	t.Helper()
	pack, index := validatablePack(t)
	old := viper.GetString("cache.directory")
	viper.Set("cache.directory", t.TempDir())
	t.Cleanup(func() { viper.Set("cache.directory", old) })

	for _, m := range mods {
		content := "bytes of " + m.name
		sum := sha512.Sum512([]byte(content))
		m.hashFormat, m.hash = "sha512", hex.EncodeToString(sum[:])
		if m.url == "" {
			m.url = "https://cdn.modrinth.com/data/" + m.name + "/versions/v1/" + m.name + ".jar"
		}
		httpmock.RegisterResponder("GET", m.url, httpmock.NewStringResponder(200, content))
		addMod(t, &index, m)
	}
	saveFixture(t, pack, index)
	return pack, index
}

// readMrpack reads an exported pack: the names of what is in it, and its manifest.
func readMrpack(t *testing.T, path string) (names []string, manifest Pack) {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("failed to open %s: %v", path, err)
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		names = append(names, f.Name)
		if f.Name != "modrinth.index.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open the manifest: %v", err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatalf("failed to read the manifest: %v", err)
		}
	}
	return names, manifest
}

func TestExportWritesThePackAndSaysWhatWentIntoItWithoutSayingAnythingOnTheTerminal(t *testing.T) {
	exportablePack(t, validMod("alpha"), validMod("beta"))
	output := t.TempDir() + "/pack.mrpack"

	var result *ExportResult
	var err error
	var progress []int
	out := cmdtest.CaptureStdout(t, func() {
		result, err = Export(ExportOptions{Output: output, RestrictDomains: true}, func(done, total int) {
			if total != 2 {
				t.Errorf("the total is %d, want 2", total)
			}
			progress = append(progress, done)
		})
	})

	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("Export() wrote %q to the terminal", out)
	}
	if result.Path != output {
		t.Errorf("Path = %q, want %q", result.Path, output)
	}
	if !slices.Equal(progress, []int{1, 2}) {
		t.Errorf("the progress is %v, want each file counted as it was done", progress)
	}
	if len(result.Files) != 2 || result.Files[0].Name != "alpha" && result.Files[1].Name != "alpha" {
		t.Fatalf("the files are %+v, want both mods", result.Files)
	}
	for _, f := range result.Files {
		if f.Client != "required" || f.Server != "required" || f.Bundled || !strings.HasPrefix(f.Path, "mods/") || f.Size == 0 {
			t.Errorf("the file is %+v, want a mod that the launcher downloads, needed on both sides", f)
		}
	}

	names, manifest := readMrpack(t, output)
	if !slices.Contains(names, "modrinth.index.json") || !slices.Contains(names, "overrides/") {
		t.Errorf("the pack has %v, want a manifest and an overrides folder", names)
	}
	if len(manifest.Files) != 2 || manifest.Name != "Test Pack" || manifest.VersionID != "1.0.0" || manifest.Dependencies["neoforge"] != "21.1.0" {
		t.Errorf("the manifest is %+v, want what the pack says", manifest)
	}
	if table := result.Breakdown(); !strings.Contains(table, "Exported files:") || !strings.Contains(table, "mods/alpha.jar") {
		t.Errorf("the breakdown is %q, want a table with the mods", table)
	}
}

func TestExportStoresFilesThatAreNotOnAllowedDomainsInThePackItself(t *testing.T) {
	elsewhere := validMod("alpha")
	elsewhere.url = "https://example.com/alpha.jar"
	elsewhere.side = core.ClientSide
	exportablePack(t, elsewhere)
	output := t.TempDir() + "/pack.mrpack"

	result, err := Export(ExportOptions{Output: output, RestrictDomains: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}

	if len(result.Files) != 1 || !result.Files[0].Bundled || result.Files[0].Path != "client-overrides/mods/alpha.jar" {
		t.Errorf("the files are %+v, want the mod stored in the client overrides of the pack", result.Files)
	}
	names, manifest := readMrpack(t, output)
	if !slices.Contains(names, "client-overrides/mods/alpha.jar") || len(manifest.Files) != 0 {
		t.Errorf("the pack has %v and a manifest of %d files, want the mod in the archive and not in the manifest", names, len(manifest.Files))
	}
}

func TestExportGoesOnWithoutAFileThatFailsToDownloadAndSaysSo(t *testing.T) {
	pack, index := exportablePack(t, validMod("alpha"), validMod("beta"))
	_, _ = pack, index
	httpmock.RegisterResponder("GET", "https://cdn.modrinth.com/data/beta/versions/v1/beta.jar", httpmock.NewStringResponder(500, ""))
	output := t.TempDir() + "/pack.mrpack"

	result, err := Export(ExportOptions{Output: output, RestrictDomains: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0].Name != "alpha" {
		t.Errorf("the files are %+v, want only the one that downloaded", result.Files)
	}
	if !strings.Contains(strings.Join(result.Notices, "\n"), "Download of beta (beta.jar) failed") {
		t.Errorf("the notices are %v, want one that says the download failed", result.Notices)
	}
}

func TestExportPutsWhatTheIndexTracksThatIsNotAModInTheOverrides(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	if err := os.WriteFile("config/alpha.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	output := t.TempDir() + "/pack.mrpack"

	if _, err := Export(ExportOptions{Output: output, RestrictDomains: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	// It wasn't in the index, but exporting refreshes that first, as the command does
	if names, _ := readMrpack(t, output); !slices.Contains(names, "overrides/config/alpha.json") {
		t.Errorf("the pack has %v, want the config file that was added in its overrides", names)
	}
}

func TestExportDoesNotLeaveAHalfWrittenPackBehind(t *testing.T) {
	pack, index := exportablePack(t, validMod("alpha"))
	delete(pack.Versions, "minecraft")
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	_ = index
	output := t.TempDir() + "/pack.mrpack"

	if _, err := Export(ExportOptions{Output: output, RestrictDomains: true}, nil); err == nil || !strings.Contains(err.Error(), "Error creating manifest") {
		t.Fatalf("Export() returned %v, want an error about the manifest, as the pack has no Minecraft version", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("the half written pack is still there (%v), want it removed", err)
	}
}

func TestExportSaysWhichModsAreOnlyForTheServerThatAModOnTheClientNeeds(t *testing.T) {
	lib := validMod("lib")
	lib.side = core.ServerSide
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: projectOf("lib"), Type: "required"}}
	exportablePack(t, lib, alpha)

	result, err := Export(ExportOptions{Output: t.TempDir() + "/pack.mrpack", RestrictDomains: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if want := []ExportPromotion{{Mod: "lib", NeededBy: "alpha"}}; !slices.Equal(result.Promotions, want) {
		t.Errorf("the promotions are %v, want %v", result.Promotions, want)
	}
}

func TestExportFailsWhereThePackCannotBeWritten(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	if _, err := Export(ExportOptions{Output: t.TempDir() + "/no-such-folder/pack.mrpack"}, nil); err == nil || !strings.Contains(err.Error(), "Failed to create zip") {
		t.Errorf("Export() returned %v, want an error that the pack couldn't be created", err)
	}
}

func TestExportNamesThePackAfterItByDefault(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	result, err := Export(ExportOptions{RestrictDomains: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if result.Path != "Test Pack-1.0.0.mrpack" {
		t.Errorf("Path = %q, want the pack's name and version", result.Path)
	}
}

func TestExportPutsTheReadmeLicenseAndChangelogInTheOverridesWithoutTheIndexTrackingThem(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	docs := map[string]string{"README.md": "about the pack", "LICENSE": "the terms", "CHANGELOG.md": "what changed"}
	for name, content := range docs {
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() returned error: %v", err)
		}
	}
	// The list of mods is made from the pack, not part of it
	if err := os.WriteFile("MODS.md", []byte("- alpha"), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	output := t.TempDir() + "/pack.mrpack"

	if _, err := Export(ExportOptions{Output: output, RestrictDomains: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}

	names, _ := readMrpack(t, output)
	for name := range docs {
		if !slices.Contains(names, "overrides/"+name) {
			t.Errorf("the pack has %v, want %s in its overrides", names, name)
		}
	}
	if slices.Contains(names, "overrides/MODS.md") {
		t.Errorf("the pack has %v, want no list of mods", names)
	}
	r, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("failed to open %s: %v", output, err)
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		name, ok := strings.CutPrefix(f.Name, "overrides/")
		if !ok || docs[name] == "" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if string(data) != docs[name] {
			t.Errorf("%s holds %q, want %q", f.Name, data, docs[name])
		}
	}

	// They went in as themselves: the index, which a launcher installs from, doesn't list them
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	for name := range docs {
		if _, ok := index.Files[name]; ok {
			t.Errorf("the index lists %s", name)
		}
	}
}

func TestExportOfAPackWithoutAReadmeOrLicenseIsNotAnError(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	output := t.TempDir() + "/pack.mrpack"

	result, err := Export(ExportOptions{Output: output, RestrictDomains: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if len(result.Notices) != 0 {
		t.Errorf("the notices are %v, want none for files the pack doesn't have", result.Notices)
	}
	names, _ := readMrpack(t, output)
	for _, name := range core.DocFiles {
		if slices.Contains(names, "overrides/"+name) {
			t.Errorf("the pack has %v, want no %s as there wasn't one", names, name)
		}
	}
}

func TestDevExportPutsEveryModOnBothSidesWhateverItsSide(t *testing.T) {
	client, server := validMod("clientonly"), validMod("serveronly")
	client.side, server.side = core.ClientSide, core.ServerSide
	exportablePack(t, client, server)
	output := t.TempDir() + "/pack-dev.mrpack"

	if _, err := Export(ExportOptions{Output: output, RestrictDomains: true, Dev: true}, nil); err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	_, manifest := readMrpack(t, output)
	if len(manifest.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(manifest.Files))
	}
	for _, f := range manifest.Files {
		if f.Env.Client != "required" || f.Env.Server != "required" {
			t.Errorf("%s is %s on the client and %s on the server, want required on both", f.Path, f.Env.Client, f.Env.Server)
		}
	}
}

func TestDevExportCannotBeAServerPack(t *testing.T) {
	if _, err := exportWith(core.Pack{}, &core.Index{}, nil, ExportOptions{Dev: true, Server: true}, exportHooks{}); err == nil {
		t.Error("a dev server pack was exported")
	}
}
