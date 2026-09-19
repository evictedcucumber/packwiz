package modrinth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/jarcoal/httpmock"
)

// projectOf is the Modrinth project of the mod whose metadata file is at path, in these tests
func projectOf(path string) string { return "project-" + path }

// sideMod is a mod, whose metadata file is at path, that is on side and depends on deps
func sideMod(t *testing.T, path, side string, deps ...core.ModDependency) *core.Mod {
	t.Helper()
	mod, err := core.DecodeMod([]byte(fmt.Sprintf("name = %q\nside = %q\n[update.modrinth]\nmod-id = %q\nversion = \"v\"\n", path, side, projectOf(path))))
	if err != nil {
		t.Fatalf("DecodeMod() returned error: %v", err)
	}
	mod.SetMetaPath(path)
	mod.Dependencies = deps
	return &mod
}

// requires is a required dependency on the mod whose metadata file is at path
func requires(path string) core.ModDependency {
	return core.ModDependency{ID: projectOf(path), Type: "required"}
}

func sidesOf(mods []*core.Mod) map[string]string {
	sides := make(map[string]string, len(mods))
	for _, mod := range mods {
		sides[mod.GetFilePath()] = mod.Side
	}
	return sides
}

func TestWidenSides(t *testing.T) {
	tests := []struct {
		name string
		mods func(t *testing.T) []*core.Mod
		want map[string]string
		// promoted is the mods that were widened, in order
		promoted []string
	}{
		{
			name: "a mod on both sides needs what it requires on the client",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{sideMod(t, "lib", core.ServerSide), sideMod(t, "user", core.UniversalSide, requires("lib"))}
			},
			want:     map[string]string{"lib": core.UniversalSide, "user": core.UniversalSide},
			promoted: []string{"lib"},
		},
		{
			name: "so does a mod that is only on the client",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{sideMod(t, "lib", core.ServerSide), sideMod(t, "user", core.ClientSide, requires("lib"))}
			},
			want:     map[string]string{"lib": core.UniversalSide, "user": core.ClientSide},
			promoted: []string{"lib"},
		},
		{
			name: "and a mod with no side, which is on both",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{sideMod(t, "lib", core.ServerSide), sideMod(t, "user", core.EmptySide, requires("lib"))}
			},
			want:     map[string]string{"lib": core.UniversalSide, "user": core.EmptySide},
			promoted: []string{"lib"},
		},
		{
			name: "a mod that is only on the server doesn't",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{sideMod(t, "lib", core.ServerSide), sideMod(t, "user", core.ServerSide, requires("lib"))}
			},
			want: map[string]string{"lib": core.ServerSide, "user": core.ServerSide},
		},
		{
			name: "only a required dependency is needed",
			mods: func(t *testing.T) []*core.Mod {
				var deps []core.ModDependency
				for _, depType := range []string{"optional", "embedded", "incompatible"} {
					deps = append(deps, core.ModDependency{ID: projectOf("lib"), Type: depType})
				}
				return []*core.Mod{sideMod(t, "lib", core.ServerSide), sideMod(t, "user", core.UniversalSide, deps...)}
			},
			want: map[string]string{"lib": core.ServerSide, "user": core.UniversalSide},
		},
		{
			name: "a dependency that isn't in the pack is left for 'mr deps' to report",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{sideMod(t, "user", core.UniversalSide, requires("missing"))}
			},
			want: map[string]string{"user": core.UniversalSide},
		},
		{
			name: "a mod that is on both sides already is left alone",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{sideMod(t, "lib", core.UniversalSide), sideMod(t, "user", core.UniversalSide, requires("lib"))}
			},
			want: map[string]string{"lib": core.UniversalSide, "user": core.UniversalSide},
		},
		{
			name: "a client-only dependency is left alone",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{sideMod(t, "lib", core.ClientSide), sideMod(t, "user", core.UniversalSide, requires("lib"))}
			},
			want: map[string]string{"lib": core.ClientSide, "user": core.UniversalSide},
		},
		{
			name: "what a mod that is put on the client requires is needed there too",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{
					sideMod(t, "c", core.ServerSide),
					sideMod(t, "b", core.ServerSide, requires("c")),
					sideMod(t, "a", core.ServerSide, requires("b")),
					sideMod(t, "user", core.UniversalSide, requires("a")),
				}
			},
			want:     map[string]string{"a": core.UniversalSide, "b": core.UniversalSide, "c": core.UniversalSide, "user": core.UniversalSide},
			promoted: []string{"a", "b", "c"},
		},
		{
			name: "however the mods are ordered",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{
					sideMod(t, "user", core.UniversalSide, requires("a")),
					sideMod(t, "a", core.ServerSide, requires("b")),
					sideMod(t, "b", core.ServerSide, requires("c")),
					sideMod(t, "c", core.ServerSide),
				}
			},
			want:     map[string]string{"a": core.UniversalSide, "b": core.UniversalSide, "c": core.UniversalSide, "user": core.UniversalSide},
			promoted: []string{"a", "b", "c"},
		},
		{
			name: "a chain of server mods that nothing on the client requires is left alone",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{
					sideMod(t, "b", core.ServerSide),
					sideMod(t, "a", core.ServerSide, requires("b")),
					sideMod(t, "user", core.UniversalSide),
				}
			},
			want: map[string]string{"a": core.ServerSide, "b": core.ServerSide, "user": core.UniversalSide},
		},
		{
			name: "mods that require each other don't go on for ever",
			mods: func(t *testing.T) []*core.Mod {
				return []*core.Mod{
					sideMod(t, "user", core.UniversalSide, requires("lib")),
					sideMod(t, "lib", core.ServerSide, requires("user"), requires("lib")),
				}
			},
			want:     map[string]string{"lib": core.UniversalSide, "user": core.UniversalSide},
			promoted: []string{"lib"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mods := tt.mods(t)
			promotions := widenSides(mods)

			if got := sidesOf(mods); !maps.Equal(got, tt.want) {
				t.Errorf("sides = %v, want %v", got, tt.want)
			}
			var promoted []string
			for _, p := range promotions {
				promoted = append(promoted, p.mod.GetFilePath())
			}
			if strings.Join(promoted, ",") != strings.Join(tt.promoted, ",") {
				t.Errorf("promoted = %v, want %v", promoted, tt.promoted)
			}
		})
	}
}

func TestWidenSidesNamesTheFirstModThatNeedsIt(t *testing.T) {
	lib := sideMod(t, "lib", core.ServerSide)
	mods := []*core.Mod{sideMod(t, "mods/b", core.UniversalSide, requires("lib")), lib, sideMod(t, "mods/a", core.UniversalSide, requires("lib"))}

	promotions := widenSides(mods)

	if len(promotions) != 1 || promotions[0].mod != lib {
		t.Fatalf("promotions = %v, want just the library", promotions)
	}
	if got := promotions[0].neededBy.GetFilePath(); got != "mods/a" {
		t.Errorf("needed by %q, want the first of the mods that need it by path", got)
	}
}

// libProject is a project that Modrinth lists as unsupported on the client, as it does for Repurposed Structures
func libProject() *modrinthApi.Project {
	return &modrinthApi.Project{
		ID: strPtr("lib1"), Title: strPtr("Lib Mod"), Slug: strPtr("lib-mod"), ProjectType: strPtr("mod"),
		ClientSide: strPtr("unsupported"), ServerSide: strPtr("required"),
	}
}

func libVersion() *modrinthApi.Version {
	return &modrinthApi.Version{
		ID: strPtr("lv1"), ProjectID: strPtr("lib1"), VersionNumber: strPtr("1.0.0"), VersionType: strPtr("release"),
		Files: []*modrinthApi.File{{
			URL: strPtr("https://example.com/lib.jar"), Filename: strPtr("lib.jar"),
			Primary: boolPtr(true), Hashes: map[string]string{"sha512": "libdead"},
		}},
	}
}

var libMetaPath = filepath.Join("mods", "lib-mod"+core.MetaExtension)

// addLib adds the library to the pack as 'mr add' does, which puts it on the server only
func addLib(t *testing.T, pack core.Pack, index *core.Index) {
	t.Helper()
	if err := installVersion(libProject(), libVersion(), "", pack, index, ""); err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
	if mod, err := core.LoadMod(libMetaPath); err != nil || mod.Side != core.ServerSide {
		t.Fatalf("library = %+v (%v), want one on the server only to start with", mod, err)
	}
}

// testVersionNeeding is version 1 of the test project, which needs the library in the way given
func testVersionNeeding(depType string) *modrinthApi.Version {
	version := testVersion("v1", "1.0.0")
	version.Dependencies = []*modrinthApi.Dependency{{ProjectID: strPtr("lib1"), DependencyType: strPtr(depType)}}
	return version
}

func loadLib(t *testing.T) core.Mod {
	t.Helper()
	mod, err := core.LoadMod(libMetaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	return mod
}

// assertIndexHasSavedLib checks that the index records the library's metadata file as it is on disk now, so that a
// side that was changed is not taken for a change made behind the index's back
func assertIndexHasSavedLib(t *testing.T) {
	t.Helper()
	sum := sha256.Sum256([]byte(readFile(t, libMetaPath)))
	if index := readFile(t, "index.toml"); !strings.Contains(index, hex.EncodeToString(sum[:])) {
		t.Errorf("index.toml doesn't have the hash of the library's metadata as it is now:\n%s", index)
	}
}

// Adding a mod that requires a library the pack already has, which is only on the server, puts the library on the client
// too
func TestInstallVersionPutsARequiredServerModAlreadyAddedOnBothSides(t *testing.T) {
	pack, index := setupPackFixture(t)
	addLib(t, pack, &index)

	out, err := addAgain(t, pack, &index, testVersionNeeding("required"), "")
	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	if got := loadLib(t).Side; got != core.UniversalSide {
		t.Errorf("library side = %q, want %q as a mod on the client requires it", got, core.UniversalSide)
	}
	if want := "Notice: Lib Mod is now on both sides, as Test Project needs it on the client\n"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if got := loadTestMod(t).Side; got != core.UniversalSide {
		t.Errorf("test project side = %q, want it left as it was", got)
	}
	assertIndexHasSavedLib(t)
}

// It doesn't matter which of the two is added first: adding the library once the mod that needs it is in the pack
// puts it on both sides too
func TestInstallVersionPutsARequiredServerModAddedAfterItsUserOnBothSides(t *testing.T) {
	pack, index := setupPackFixture(t)
	addModWithDependencies(t, pack, &index, map[string]string{"lib1": "required"})

	out := cmdtest.CaptureStdout(t, func() {
		if err := installVersion(libProject(), libVersion(), "", pack, &index, ""); err != nil {
			t.Errorf("installVersion() returned error: %v", err)
		}
	})

	if got := loadLib(t).Side; got != core.UniversalSide {
		t.Errorf("library side = %q, want %q as a mod on the client requires it", got, core.UniversalSide)
	}
	if want := "Notice: Lib Mod is now on both sides, as Test Project needs it on the client\n"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	assertIndexHasSavedLib(t)
}

// A library that 'mr add' brings in as a dependency is added on both sides in the first place
func TestInstallVersionAddsARequiredServerModDependencyOnBothSides(t *testing.T) {
	pack, index := setupPackFixture(t)
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewStringResponder(200, `[{"id":"lib1","slug":"lib-mod","title":"Lib Mod","project_type":"mod","client_side":"unsupported","server_side":"required"}]`))
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/lib1/version`,
		httpmock.NewStringResponder(200, `[{"id":"lv1","project_id":"lib1","version_number":"1.0.0","version_type":"release","date_published":"2024-01-01T00:00:00Z",
			"files":[{"url":"https://example.com/lib.jar","filename":"lib.jar","primary":true,"hashes":{"sha512":"libdead"}}]}]`))
	cmdtest.SetStdin(t, "y\n")

	out, err := addAgain(t, pack, &index, testVersionNeeding("required"), "")
	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	lib := loadLib(t)
	if lib.Side != core.UniversalSide {
		t.Errorf("library side = %q, want %q as a mod on the client requires it", lib.Side, core.UniversalSide)
	}
	if !lib.AddedAsDependency {
		t.Error("AddedAsDependency = false, want the library marked as one")
	}
	if want := "Notice: Lib Mod is now on both sides, as Test Project needs it on the client\n"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	assertIndexHasSavedLib(t)
}

func TestInstallVersionLeavesAServerModThatIsOnlyOptionalOnTheServer(t *testing.T) {
	pack, index := setupPackFixture(t)
	addLib(t, pack, &index)

	out, err := addAgain(t, pack, &index, testVersionNeeding("optional"), "")
	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	if got := loadLib(t).Side; got != core.ServerSide {
		t.Errorf("library side = %q, want %q as nothing requires it", got, core.ServerSide)
	}
	if strings.Contains(out, "both sides") {
		t.Errorf("output mentions changing a side that wasn't changed:\n%s", out)
	}
}

// Promoting a mod is saying so and nothing else, so it reads the same with or without colour
func TestPromoteSidesNoticeIsColoured(t *testing.T) {
	pack, index := setupPackFixture(t)
	cmdtest.SetColor(t, ui.Never)
	cmdtest.CaptureStdout(t, func() {
		addLib(t, pack, &index)
		addModWithDependencies(t, pack, &index, map[string]string{"lib1": "required"})
	})
	cmdtest.SetColor(t, ui.Always)

	out := cmdtest.CaptureStdout(t, func() {
		if err := promoteSides(&index); err != nil {
			t.Errorf("promoteSides() returned error: %v", err)
		}
	})

	want := ui.Info.Sprintf("Notice: %s is now on both sides, as %s needs it on the client\n", ui.Bold.Sprint("Lib Mod"), ui.Bold.Sprint("Test Project"))
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if got, plain := ui.Strip(out), "Notice: Lib Mod is now on both sides, as Test Project needs it on the client\n"; got != plain {
		t.Errorf("output without colour = %q, want %q", got, plain)
	}
}

// With nothing to promote it says nothing and changes nothing
func TestPromoteSidesWithNothingToDoIsQuiet(t *testing.T) {
	pack, index := setupPackFixture(t)
	addModWithDependencies(t, pack, &index, map[string]string{"lib1": "required"})
	before := readFile(t, testMetaPath)

	out := cmdtest.CaptureStdout(t, func() {
		if err := promoteSides(&index); err != nil {
			t.Errorf("promoteSides() returned error: %v", err)
		}
	})

	if out != "" {
		t.Errorf("output = %q, want none", out)
	}
	if after := readFile(t, testMetaPath); after != before {
		t.Errorf("metadata changed:\n%s\nwas:\n%s", after, before)
	}
}

// Export and validate want to know what would be promoted, without it happening: what export writes is what the pack says
func TestSidePromotionsFindsWhatWidenSidesWouldChangeWithoutChangingIt(t *testing.T) {
	lib := sideMod(t, "lib", core.ServerSide)
	user := sideMod(t, "user", core.UniversalSide, requires("lib"))
	mods := []*core.Mod{lib, user}

	promotions := sidePromotions(mods)

	if len(promotions) != 1 || promotions[0].mod != lib || promotions[0].neededBy != user {
		t.Fatalf("promotions = %v, want the library needed by the user, as the mods that were given", promotions)
	}
	if lib.Side != core.ServerSide || user.Side != core.UniversalSide {
		t.Errorf("sides = %q and %q, want them left as they were", lib.Side, user.Side)
	}
	if got := len(widenSides(mods)); got != 1 || lib.Side != core.UniversalSide {
		t.Errorf("widenSides() after it changed %d mods and the library's side is %q, want it still to have the one to do", got, lib.Side)
	}
}
