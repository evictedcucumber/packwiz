package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestModVersionRoundTrip(t *testing.T) {
	metaPath := filepath.Join(t.TempDir(), "test-mod.pw.toml")

	mod := Mod{Name: "Test Mod", FileName: "test-mod-1.2.3.jar", Version: "1.2.3"}
	mod.SetMetaPath(metaPath)
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	loaded, err := LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if loaded.Version != "1.2.3" {
		t.Errorf("loaded.Version = %q, want %q", loaded.Version, "1.2.3")
	}
}

func TestModVersionOmittedWhenEmpty(t *testing.T) {
	metaPath := filepath.Join(t.TempDir(), "test-mod.pw.toml")

	mod := Mod{Name: "Test Mod", FileName: "test-mod.jar"}
	mod.SetMetaPath(metaPath)
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read mod file: %v", err)
	}
	// Packs written before Version existed must stay byte-identical
	if strings.Contains(string(data), "version") {
		t.Errorf("expected no version key when Version is empty, got:\n%s", data)
	}
}

func TestModConfigFilesOmittedWhenNil(t *testing.T) {
	metaPath := filepath.Join(t.TempDir(), "test-mod.pw.toml")

	mod := Mod{Name: "Test Mod", FileName: "test-mod.jar"}
	mod.SetMetaPath(metaPath)
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read mod file: %v", err)
	}
	if strings.Contains(string(data), "config-files") {
		t.Errorf("expected no config-files key for a mod that has never had one, got:\n%s", data)
	}

	loaded, err := LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if loaded.ConfigFiles != nil {
		t.Errorf("loaded.ConfigFiles = %v, want nil", loaded.ConfigFiles)
	}
}

func TestModConfigFilesWrittenAsEmptyArrayWhenSetButEmpty(t *testing.T) {
	metaPath := filepath.Join(t.TempDir(), "test-mod.pw.toml")

	mod := Mod{Name: "Test Mod", FileName: "test-mod.jar", ConfigFiles: &[]string{}}
	mod.SetMetaPath(metaPath)
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read mod file: %v", err)
	}
	if !strings.Contains(string(data), "config-files = []") {
		t.Errorf("expected an explicit empty config-files key, got:\n%s", data)
	}

	loaded, err := LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if loaded.ConfigFiles == nil || len(*loaded.ConfigFiles) != 0 {
		t.Errorf("loaded.ConfigFiles = %v, want a non-nil empty slice", loaded.ConfigFiles)
	}
}

func TestModConfigFilesRoundTrip(t *testing.T) {
	metaPath := filepath.Join(t.TempDir(), "test-mod.pw.toml")

	want := []string{"config/a.json", "config/b/"}
	mod := Mod{Name: "Test Mod", FileName: "test-mod.jar", ConfigFiles: &want}
	mod.SetMetaPath(metaPath)
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	loaded, err := LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if loaded.ConfigFiles == nil || !slices.Equal(*loaded.ConfigFiles, want) {
		t.Errorf("loaded.ConfigFiles = %v, want %v", loaded.ConfigFiles, want)
	}
}

func TestModEnsureConfigFilesAllocatesOnlyWhenNil(t *testing.T) {
	mod := Mod{}
	mod.EnsureConfigFiles()
	if mod.ConfigFiles == nil || len(*mod.ConfigFiles) != 0 {
		t.Fatalf("ConfigFiles = %v, want a non-nil empty slice", mod.ConfigFiles)
	}

	*mod.ConfigFiles = append(*mod.ConfigFiles, "config/a.json")
	mod.EnsureConfigFiles()
	if want := []string{"config/a.json"}; !slices.Equal(*mod.ConfigFiles, want) {
		t.Errorf("ConfigFiles = %v, want %v (an existing list must be left alone)", *mod.ConfigFiles, want)
	}
}

func TestModClaimConfigFileAddsAndDedupes(t *testing.T) {
	mod := Mod{}
	if !mod.ClaimConfigFile("config/a.json") {
		t.Error("ClaimConfigFile() = false, want true for a new entry")
	}
	if mod.ClaimConfigFile("config/a.json") {
		t.Error("ClaimConfigFile() = true, want false for an entry already claimed")
	}
	if !mod.ClaimConfigFile("config/b.json") {
		t.Error("ClaimConfigFile() = false, want true for a second new entry")
	}
	if want := []string{"config/a.json", "config/b.json"}; mod.ConfigFiles == nil || !slices.Equal(*mod.ConfigFiles, want) {
		t.Errorf("ConfigFiles = %v, want %v", mod.ConfigFiles, want)
	}
}

func TestModDisplayVersion(t *testing.T) {
	tests := []struct {
		name string
		mod  Mod
		want string
	}{
		{"prefers Version", Mod{FileName: "sodium-0.5.8.jar", Version: "0.5.8"}, "0.5.8"},
		{"falls back to FileName", Mod{FileName: "sodium-0.5.8.jar"}, "sodium-0.5.8.jar"},
		{"both empty", Mod{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mod.DisplayVersion(); got != tt.want {
				t.Errorf("DisplayVersion() = %q, want %q", got, tt.want)
			}
		})
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

func TestDecodeMod(t *testing.T) {
	Updaters["stub"] = stubUpdater{}
	t.Cleanup(func() { delete(Updaters, "stub") })

	mod, err := DecodeMod([]byte(`name = "Test Mod"
filename = "test-mod-1.2.3.jar"
version = "1.2.3"
side = "client"

[download]
hash-format = "sha256"
hash = "deadbeef"

[update.stub]
version = "abc"
`))
	if err != nil {
		t.Fatalf("DecodeMod() returned error: %v", err)
	}
	if mod.Name != "Test Mod" || mod.Version != "1.2.3" || mod.Side != ClientSide || mod.FileName != "test-mod-1.2.3.jar" {
		t.Errorf("DecodeMod() = %+v, want the fields from the file", mod)
	}
	// Updaters are still resolved, as they are when loading from disk
	if data, ok := mod.GetParsedUpdateData("stub"); !ok || data != "abc" {
		t.Errorf("GetParsedUpdateData(\"stub\") = %v, %v, want abc, true", data, ok)
	}
	// ...but there is no file behind it
	if got := mod.GetFilePath(); got != "" {
		t.Errorf("GetFilePath() = %q, want empty for a mod that didn't come from a file", got)
	}
}

func TestDecodeModRejectsBadInput(t *testing.T) {
	if _, err := DecodeMod([]byte("this is [not toml")); err == nil {
		t.Error("DecodeMod() accepted invalid TOML")
	}
	if _, err := DecodeMod([]byte("name = \"x\"\n\n[update.doesnotexist]\nversion = \"1\"\n")); err == nil {
		t.Error("DecodeMod() accepted an unregistered update plugin")
	}
}

func TestLoadModSetsMetaPathButDecodeModDoesNot(t *testing.T) {
	metaPath := filepath.Join(t.TempDir(), "test-mod.pw.toml")
	if err := os.WriteFile(metaPath, []byte("name = \"Test Mod\"\nfilename = \"test-mod.jar\"\n"), 0644); err != nil {
		t.Fatalf("failed to write mod fixture: %v", err)
	}

	loaded, err := LoadMod(metaPath)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if loaded.GetFilePath() != metaPath {
		t.Errorf("LoadMod().GetFilePath() = %q, want %q", loaded.GetFilePath(), metaPath)
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
