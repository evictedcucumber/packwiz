package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// withPackFile creates a pack.toml with the given content in a temp dir,
// points viper's "pack-file" at it, and restores the previous value afterward.
func withPackFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	packFile := filepath.Join(dir, "pack.toml")
	if content != "" {
		if err := os.WriteFile(packFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write pack.toml fixture: %v", err)
		}
	}
	old := viper.GetString("pack-file")
	viper.Set("pack-file", packFile)
	t.Cleanup(func() { viper.Set("pack-file", old) })
	return packFile
}

func TestLoadPackMissingFormatDefaults(t *testing.T) {
	withPackFile(t, `name = "Test Pack"

[index]
file = "index.toml"
`)
	pack, err := LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.PackFormat != "packwiz:1.1.0" {
		t.Errorf("PackFormat = %q, want %q", pack.PackFormat, "packwiz:1.1.0")
	}
}

func TestLoadPackAutoMigrate100To110(t *testing.T) {
	withPackFile(t, `name = "Test Pack"
pack-format = "packwiz:1.0.0"

[index]
file = "index.toml"
`)
	pack, err := LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.PackFormat != "packwiz:1.1.0" {
		t.Errorf("PackFormat = %q, want %q", pack.PackFormat, "packwiz:1.1.0")
	}
}

func TestLoadPackIncompatibleFormat(t *testing.T) {
	withPackFile(t, `name = "Test Pack"
pack-format = "packwiz:2.0.0"

[index]
file = "index.toml"
`)
	_, err := LoadPack()
	if err == nil {
		t.Error("expected error for incompatible pack-format, got nil")
	}
}

func TestLoadPackInvalidSemver(t *testing.T) {
	withPackFile(t, `name = "Test Pack"
pack-format = "packwiz:not-a-version"

[index]
file = "index.toml"
`)
	_, err := LoadPack()
	if err == nil {
		t.Error("expected error for invalid semver pack-format, got nil")
	}
}

func TestLoadPackInvalidPrefix(t *testing.T) {
	withPackFile(t, `name = "Test Pack"
pack-format = "notpackwiz:1.1.0"

[index]
file = "index.toml"
`)
	_, err := LoadPack()
	if err == nil {
		t.Error("expected error for non-packwiz pack-format prefix, got nil")
	}
}

func TestPackWriteLoadRoundTrip(t *testing.T) {
	withPackFile(t, "")

	pack := Pack{
		Name:       "My Pack",
		Author:     "Someone",
		Version:    "1.0.0",
		PackFormat: CurrentPackFormat,
		Versions:   map[string]string{"minecraft": "1.20.1", "fabric": "0.15.0"},
	}
	pack.Index.File = "index.toml"
	pack.Index.HashFormat = "sha256"
	pack.Index.Hash = "abc123"

	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	loaded, err := LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}

	if loaded.Name != pack.Name {
		t.Errorf("loaded.Name = %q, want %q", loaded.Name, pack.Name)
	}
	if loaded.Author != pack.Author {
		t.Errorf("loaded.Author = %q, want %q", loaded.Author, pack.Author)
	}
	if loaded.Version != pack.Version {
		t.Errorf("loaded.Version = %q, want %q", loaded.Version, pack.Version)
	}
	if loaded.Index.File != pack.Index.File {
		t.Errorf("loaded.Index.File = %q, want %q", loaded.Index.File, pack.Index.File)
	}
	if loaded.Index.Hash != pack.Index.Hash {
		t.Errorf("loaded.Index.Hash = %q, want %q", loaded.Index.Hash, pack.Index.Hash)
	}
	if loaded.Versions["minecraft"] != "1.20.1" {
		t.Errorf("loaded.Versions[minecraft] = %q, want %q", loaded.Versions["minecraft"], "1.20.1")
	}
}

func TestGetMCVersion(t *testing.T) {
	pack := Pack{Versions: map[string]string{"minecraft": "1.20.1"}}
	got, err := pack.GetMCVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.20.1" {
		t.Errorf("GetMCVersion() = %q, want %q", got, "1.20.1")
	}
}

func TestGetMCVersionMissing(t *testing.T) {
	pack := Pack{Versions: map[string]string{}}
	_, err := pack.GetMCVersion()
	if err == nil {
		t.Error("expected error when minecraft version is missing, got nil")
	}
}

func TestGetSupportedMCVersionsDedupAndOrdering(t *testing.T) {
	oldVal := viper.Get("acceptable-game-versions")
	viper.Set("acceptable-game-versions", []string{"1.19", "1.20", "1.20.1"})
	t.Cleanup(func() { viper.Set("acceptable-game-versions", oldVal) })

	pack := Pack{Versions: map[string]string{"minecraft": "1.20"}}
	got, err := pack.GetSupportedMCVersions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"1.19", "1.20.1", "1.20"}
	if len(got) != len(want) {
		t.Fatalf("GetSupportedMCVersions() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetSupportedMCVersions()[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
	// The main pack version should always be last
	if got[len(got)-1] != "1.20" {
		t.Errorf("last element = %q, want main version %q", got[len(got)-1], "1.20")
	}
}

func TestGetSupportedMCVersionsMissing(t *testing.T) {
	pack := Pack{Versions: map[string]string{}}
	_, err := pack.GetSupportedMCVersions()
	if err == nil {
		t.Error("expected error when minecraft version is missing, got nil")
	}
}

func TestGetPackNameNoName(t *testing.T) {
	pack := Pack{}
	if got := pack.GetPackName(); got != "export" {
		t.Errorf("GetPackName() = %q, want %q", got, "export")
	}
}

func TestGetPackNameOnly(t *testing.T) {
	pack := Pack{Name: "My Pack"}
	if got := pack.GetPackName(); got != "My Pack" {
		t.Errorf("GetPackName() = %q, want %q", got, "My Pack")
	}
}

func TestGetPackNameWithVersion(t *testing.T) {
	pack := Pack{Name: "My Pack", Version: "1.2.3"}
	want := "My Pack-1.2.3"
	if got := pack.GetPackName(); got != want {
		t.Errorf("GetPackName() = %q, want %q", got, want)
	}
}

func TestGetCompatibleLoadersQuilt(t *testing.T) {
	pack := Pack{Versions: map[string]string{"quilt": "0.20.0"}}
	got := pack.GetCompatibleLoaders()
	want := []string{"quilt", "fabric"}
	if !slicesEqual(got, want) {
		t.Errorf("GetCompatibleLoaders() = %v, want %v", got, want)
	}
}

func TestGetCompatibleLoadersFabricOnly(t *testing.T) {
	pack := Pack{Versions: map[string]string{"fabric": "0.15.0"}}
	got := pack.GetCompatibleLoaders()
	want := []string{"fabric"}
	if !slicesEqual(got, want) {
		t.Errorf("GetCompatibleLoaders() = %v, want %v", got, want)
	}
}

func TestGetCompatibleLoadersNeoForge(t *testing.T) {
	pack := Pack{Versions: map[string]string{"neoforge": "20.4.0"}}
	got := pack.GetCompatibleLoaders()
	want := []string{"neoforge", "forge"}
	if !slicesEqual(got, want) {
		t.Errorf("GetCompatibleLoaders() = %v, want %v", got, want)
	}
}

func TestGetCompatibleLoadersForgeOnly(t *testing.T) {
	pack := Pack{Versions: map[string]string{"forge": "47.0.0"}}
	got := pack.GetCompatibleLoaders()
	want := []string{"forge"}
	if !slicesEqual(got, want) {
		t.Errorf("GetCompatibleLoaders() = %v, want %v", got, want)
	}
}

func TestGetCompatibleLoadersQuiltAndNeoForge(t *testing.T) {
	pack := Pack{Versions: map[string]string{"quilt": "0.20.0", "neoforge": "20.4.0"}}
	got := pack.GetCompatibleLoaders()
	want := []string{"quilt", "fabric", "neoforge", "forge"}
	if !slicesEqual(got, want) {
		t.Errorf("GetCompatibleLoaders() = %v, want %v", got, want)
	}
}

// GetLoaders lists only loaders explicitly present in pack.Versions - unlike
// GetCompatibleLoaders, it does not imply fabric from quilt or forge from neoforge.
func TestGetLoadersQuiltOnlyDoesNotImplyFabric(t *testing.T) {
	pack := Pack{Versions: map[string]string{"quilt": "0.20.0"}}
	got := pack.GetLoaders()
	want := []string{"quilt"}
	if !slicesEqual(got, want) {
		t.Errorf("GetLoaders() = %v, want %v", got, want)
	}
}

func TestGetLoadersNeoForgeOnlyDoesNotImplyForge(t *testing.T) {
	pack := Pack{Versions: map[string]string{"neoforge": "20.4.0"}}
	got := pack.GetLoaders()
	want := []string{"neoforge"}
	if !slicesEqual(got, want) {
		t.Errorf("GetLoaders() = %v, want %v", got, want)
	}
}

func TestGetLoadersAll(t *testing.T) {
	pack := Pack{Versions: map[string]string{
		"quilt": "0.20.0", "fabric": "0.15.0", "neoforge": "20.4.0", "forge": "47.0.0",
	}}
	got := pack.GetLoaders()
	want := []string{"quilt", "fabric", "neoforge", "forge"}
	if !slicesEqual(got, want) {
		t.Errorf("GetLoaders() = %v, want %v", got, want)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
