package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSlugifyNameLowercasing(t *testing.T) {
	got := SlugifyName("Hello World")
	want := "hello-world"
	if got != want {
		t.Errorf("SlugifyName(%q) = %q, want %q", "Hello World", got, want)
	}
}

func TestSlugifyNameStripsBrackets(t *testing.T) {
	got := SlugifyName("JEI (Just Enough Items)")
	want := "jei"
	if got != want {
		t.Errorf("SlugifyName() = %q, want %q", got, want)
	}
}

func TestSlugifyNameStripsDashSuffix(t *testing.T) {
	got := SlugifyName("Mod Name - Extra Info")
	want := "mod-name"
	if got != want {
		t.Errorf("SlugifyName() = %q, want %q", got, want)
	}
}

func TestSlugifyNameNonAlphanumericToDash(t *testing.T) {
	got := SlugifyName("A!!!B")
	want := "a-b"
	if got != want {
		t.Errorf("SlugifyName() = %q, want %q", got, want)
	}
}

func TestSlugifyNameCollapsesDashes(t *testing.T) {
	got := SlugifyName("A -- B")
	want := "a-b"
	if got != want {
		t.Errorf("SlugifyName() = %q, want %q", got, want)
	}
}

func TestSlugifyNameTrimsLeadingTrailingDashes(t *testing.T) {
	got := SlugifyName("!Test!")
	want := "test"
	if got != want {
		t.Errorf("SlugifyName() = %q, want %q", got, want)
	}
}

func TestSlugifyNameCombined(t *testing.T) {
	got := SlugifyName("Some Mod (Fabric) - Extra Stuff!!")
	want := "some-mod"
	if got != want {
		t.Errorf("SlugifyName() = %q, want %q", got, want)
	}
}

func TestModWriteLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "test-mod.pw.toml")

	mod := Mod{
		Name:     "Test Mod",
		FileName: "test-mod.jar",
		Side:     ClientSide,
		Download: ModDownload{
			URL:        "https://example.com/test-mod.jar",
			HashFormat: "sha256",
			Hash:       "deadbeef",
			Mode:       ModeURL,
		},
	}
	mod.SetMetaPath(metaPath)

	hashFormat, hashValue, err := mod.Write()
	if err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if hashFormat != "sha256" {
		t.Errorf("Write() hashFormat = %q, want %q", hashFormat, "sha256")
	}
	if hashValue == "" {
		t.Error("Write() returned empty hash value")
	}

	loaded, err := LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}

	if loaded.Name != mod.Name {
		t.Errorf("loaded.Name = %q, want %q", loaded.Name, mod.Name)
	}
	if loaded.FileName != mod.FileName {
		t.Errorf("loaded.FileName = %q, want %q", loaded.FileName, mod.FileName)
	}
	if loaded.Side != mod.Side {
		t.Errorf("loaded.Side = %q, want %q", loaded.Side, mod.Side)
	}
	if loaded.Download != mod.Download {
		t.Errorf("loaded.Download = %+v, want %+v", loaded.Download, mod.Download)
	}
	if loaded.GetFilePath() != metaPath {
		t.Errorf("loaded.GetFilePath() = %q, want %q", loaded.GetFilePath(), metaPath)
	}
}

func TestModGetDestFilePath(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "mods", "test-mod.pw.toml")

	mod := Mod{FileName: "test-mod.jar"}
	mod.SetMetaPath(metaPath)

	got := mod.GetDestFilePath()
	want := filepath.Join(dir, "mods", "test-mod.jar")
	if got != want {
		t.Errorf("GetDestFilePath() = %q, want %q", got, want)
	}
}

// stubUpdater is a minimal Updater for exercising LoadMod's update-plugin
// parsing and GetParsedUpdateData.
type stubUpdater struct{}

func (stubUpdater) ParseUpdate(raw map[string]interface{}) (interface{}, error) {
	return raw["version"], nil
}
func (stubUpdater) CheckUpdate([]*Mod, Pack) ([]UpdateCheck, error) { return nil, nil }
func (stubUpdater) DoUpdate([]*Mod, []interface{}) error            { return nil }

func TestModGetParsedUpdateDataKnownUpdater(t *testing.T) {
	Updaters["stub"] = stubUpdater{}
	t.Cleanup(func() { delete(Updaters, "stub") })

	dir := t.TempDir()
	metaPath := filepath.Join(dir, "test-mod.pw.toml")
	if err := os.WriteFile(metaPath, []byte(`name = "Test Mod"
filename = "test-mod.jar"

[download]
hash-format = "sha256"
hash = "deadbeef"

[update.stub]
version = "1.2.3"
`), 0644); err != nil {
		t.Fatalf("failed to write mod fixture: %v", err)
	}

	mod, err := LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}

	data, ok := mod.GetParsedUpdateData("stub")
	if !ok {
		t.Fatal("GetParsedUpdateData(\"stub\") ok = false, want true")
	}
	if data != "1.2.3" {
		t.Errorf("GetParsedUpdateData(\"stub\") = %v, want %q", data, "1.2.3")
	}
}

func TestModGetParsedUpdateDataUnknownUpdater(t *testing.T) {
	mod := Mod{Name: "Test Mod"}
	// LoadMod normally populates updateData; a zero-value Mod has a nil map,
	// so this also exercises the not-found path when nothing has been parsed.
	_, ok := mod.GetParsedUpdateData("nonexistent")
	if ok {
		t.Error("GetParsedUpdateData(\"nonexistent\") ok = true, want false")
	}
}

func TestLoadModUnknownUpdatePlugin(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "test-mod.pw.toml")
	if err := os.WriteFile(metaPath, []byte(`name = "Test Mod"
filename = "test-mod.jar"

[download]
hash-format = "sha256"
hash = "deadbeef"

[update.doesnotexist]
version = "1.2.3"
`), 0644); err != nil {
		t.Fatalf("failed to write mod fixture: %v", err)
	}

	if _, err := LoadMod(metaPath); err == nil {
		t.Error("expected an error for an unregistered update plugin, got nil")
	}
}

func TestModSetMetaPath(t *testing.T) {
	var mod Mod
	path := filepath.Join(t.TempDir(), "foo.pw.toml")
	got := mod.SetMetaPath(path)
	if got != path {
		t.Errorf("SetMetaPath() returned %q, want %q", got, path)
	}
	if mod.GetFilePath() != path {
		t.Errorf("GetFilePath() = %q, want %q", mod.GetFilePath(), path)
	}
}
