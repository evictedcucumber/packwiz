package modrinth

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestExportEnv(t *testing.T) {
	tests := []struct {
		side           string
		optional       bool
		client, server string
		description    string
	}{
		{core.UniversalSide, false, "required", "required", "a mod on both sides is required on both"},
		{core.EmptySide, false, "required", "required", "a mod with no side is on both"},
		{core.ClientSide, false, "required", "unsupported", "a client mod is unsupported on the server"},
		{core.ServerSide, false, "unsupported", "required", "a server mod is unsupported on the client"},
		{core.UniversalSide, true, "optional", "optional", "an optional mod is optional wherever it is"},
		{core.ClientSide, true, "optional", "unsupported", "an optional client mod is only optional on the client"},
		{core.ServerSide, true, "unsupported", "optional", "an optional server mod is only optional on the server"},
		{"sideways", false, "required", "required", "a side that isn't known is on both, rather than left empty in the manifest"},
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			client, server := exportEnv(tt.side, tt.optional)
			if client != tt.client || server != tt.server {
				t.Errorf("exportEnv(%q, %v) = %q, %q, want %q, %q", tt.side, tt.optional, client, server, tt.client, tt.server)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := map[uint64]string{
		0:                             "0 B",
		512:                           "512 B",
		1023:                          "1023 B",
		1024:                          "1.0 KiB",
		1536:                          "1.5 KiB",
		1024 * 1024:                   "1.0 MiB",
		5 * 1024 * 1024 / 2:           "2.5 MiB",
		3 * 1024 * 1024 * 1024:        "3.0 GiB",
		2 * 1024 * 1024 * 1024 * 1024: "2.0 TiB",
	}
	for bytes, want := range tests {
		if got := formatSize(bytes); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", bytes, got, want)
		}
	}
}

func breakdownFiles() []exportedFile {
	return []exportedFile{
		{name: "Lib Mod", path: "mods/lib.jar", client: "unsupported", server: "required", size: 2048},
		{name: "Farmer's Delight", path: "mods/farmers.jar", client: "required", server: "required", size: 1024 * 1024},
		{name: "Shaders", path: "shaderpacks/s.zip", client: "optional", server: "unsupported", size: 512},
		{name: "Bundled", path: "overrides/mods/b.jar", client: "required", server: "required", size: 100, bundled: true},
	}
}

// Files are in the order of their paths, whatever order they were added in, and the columns line up
func TestBreakdownListsEachFileWithItsSides(t *testing.T) {
	cmdtest.SetColor(t, ui.Never)

	got := breakdown(breakdownFiles())

	want := `Exported files:
Mod               Client       Server          Size  File
Farmer's Delight  required     required     1.0 MiB  mods/farmers.jar
Lib Mod           unsupported  required     2.0 KiB  mods/lib.jar
Bundled           required     required       100 B  overrides/mods/b.jar
Shaders           optional     unsupported    512 B  shaderpacks/s.zip
4 files, 1.0 MiB: 2 on both sides, 1 client only, 1 server only; 1 optional; 1 stored in the pack itself, not downloaded by the launcher
`
	if got != want {
		t.Errorf("breakdown() =\n%s\nwant\n%s", got, want)
	}
}

func TestBreakdownDoesNotReorderWhatItIsGiven(t *testing.T) {
	files := breakdownFiles()
	first := files[0].name

	breakdown(files)

	if files[0].name != first {
		t.Errorf("the first file is now %q, want the slice left as it was", files[0].name)
	}
}

func TestBreakdownOfNothingIsNothing(t *testing.T) {
	if got := breakdown(nil); got != "" {
		t.Errorf("breakdown(nil) = %q, want none", got)
	}
}

func TestSummariseLeavesOutWhatThereIsNoneOf(t *testing.T) {
	got := summarise([]exportedFile{{name: "a", path: "mods/a.jar", client: "required", server: "required", size: 10}})

	if want := "1 file, 10 B: 1 on both sides"; got != want {
		t.Errorf("summarise() = %q, want %q", got, want)
	}
}

// The columns are padded before they are styled, so colour only adds to what is printed
func TestBreakdownColourOnlyAdds(t *testing.T) {
	plain, coloured := cmdtest.AssertColourOnlyAdds(t, func() { fmt.Print(breakdown(breakdownFiles())) })

	if !strings.Contains(plain, "Farmer's Delight  required     required") {
		t.Errorf("output isn't laid out as expected:\n%s", plain)
	}
	// Where a mod isn't wanted fades back, and its column is as wide as the others
	if want := ui.Muted.Sprint("unsupported") + "  "; !strings.Contains(coloured, want) {
		t.Errorf("output missing %q:\n%q", want, coloured)
	}
	if want := ui.Bold.Sprint(padRight("Lib Mod", 16)); !strings.Contains(coloured, want) {
		t.Errorf("output missing the padded name %q:\n%q", want, coloured)
	}
}

// A mod stored in the zip is described by where it is in there, and isn't optional whatever it is set to be
func TestBundledFile(t *testing.T) {
	_, index := setupPackFixture(t)
	mod := core.Mod{Name: "Lib", FileName: "lib.jar", Side: core.ServerSide, Option: &core.ModOption{Optional: true}}
	mod.SetMetaPath(testMetaPath)
	dl := core.CompletedDownload{Mod: &mod, Hashes: map[string]string{"length-bytes": "2048"}}

	got := bundledFile(dl, "server-overrides", &index)

	want := exportedFile{name: "Lib", path: "server-overrides/mods/lib.jar", client: "unsupported", server: "required", size: 2048, bundled: true}
	if got != want {
		t.Errorf("bundledFile() = %+v, want %+v", got, want)
	}
}

func TestBundledFileWithoutASizeIsZero(t *testing.T) {
	_, index := setupPackFixture(t)
	mod := core.Mod{Name: "Lib", FileName: "lib.jar"}
	mod.SetMetaPath(testMetaPath)

	got := bundledFile(core.CompletedDownload{Mod: &mod}, "overrides", &index)

	if got.size != 0 || got.path != "overrides/mods/lib.jar" {
		t.Errorf("bundledFile() = %+v, want no size and the path in the zip", got)
	}
}
