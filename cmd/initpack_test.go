package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestCreatePackWritesThePackAndItsIndexWithoutSayingAnything(t *testing.T) {
	cmdtest.Chdir(t)
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	if err := os.WriteFile("config/options.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	var err error
	out := cmdtest.CaptureStdout(t, func() {
		err = CreatePack(NewPack{Name: "My Pack", Author: "Me", Version: "1.0.0", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1.100"})
	})

	if err != nil {
		t.Fatalf("CreatePack() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("CreatePack() wrote %q to the terminal", out)
	}
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.Name != "My Pack" || pack.Author != "Me" || pack.Version != "1.0.0" || pack.PackFormat != core.CurrentPackFormat {
		t.Errorf("the pack is %+v, want what it was created with", pack)
	}
	if pack.Versions["minecraft"] != "1.21.1" || pack.Versions["neoforge"] != "21.1.100" || pack.Index.File != "index.toml" {
		t.Errorf("the pack's versions are %v and its index %q, want Minecraft and NeoForge, and index.toml", pack.Versions, pack.Index.File)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	if _, ok := index.Files["config/options.json"]; !ok {
		t.Errorf("the index has %v, want it to track the files that were there", index.Files)
	}
	if pack.Index.Hash == "" {
		t.Error("the pack doesn't record the hash of its index")
	}
}

func TestCreatePackWritesTheDefaultFileCategoriesAndKeepsOnesThatAreThere(t *testing.T) {
	cmdtest.Chdir(t)
	if err := CreatePack(NewPack{Name: "P", Version: "1.0.0", MCVersion: "1.21.1"}); err != nil {
		t.Fatalf("CreatePack() returned error: %v", err)
	}
	data, err := os.ReadFile(core.FileCategoriesFile)
	if err != nil || string(data) != core.DefaultFileCategories {
		t.Fatalf("%s = %q, %v, want the default", core.FileCategoriesFile, data, err)
	}
	pack, _ := core.LoadPack()
	index, _ := pack.LoadIndex()
	if _, tracked := index.Files[core.FileCategoriesFile]; tracked {
		t.Errorf("the index tracks %s, which would distribute it with the pack", core.FileCategoriesFile)
	}

	// A pack made where there already is one keeps it
	_ = os.Remove("pack.toml")
	mine := "[categories]\ndev = [\"mine.nix\"]\n"
	if err := os.WriteFile(core.FileCategoriesFile, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CreatePack(NewPack{Name: "P", Version: "1.0.0", MCVersion: "1.21.1"}); err != nil {
		t.Fatalf("CreatePack() returned error: %v", err)
	}
	if data, _ := os.ReadFile(core.FileCategoriesFile); string(data) != mine {
		t.Errorf("%s is now %q, want it left as it was", core.FileCategoriesFile, data)
	}
}

func TestCreatePackWithoutAModLoaderHasNoLoaderVersion(t *testing.T) {
	cmdtest.Chdir(t)
	for _, loader := range []string{"", "none"} {
		_ = os.Remove("pack.toml")
		if err := CreatePack(NewPack{Name: "P", Version: "1.0.0", MCVersion: "1.21.1", Loader: loader, LoaderVersion: "ignored"}); err != nil {
			t.Fatalf("CreatePack(%q) returned error: %v", loader, err)
		}
		pack, _ := core.LoadPack()
		if len(pack.Versions) != 1 || pack.Versions["minecraft"] != "1.21.1" {
			t.Errorf("loader %q: the versions are %v, want only Minecraft's", loader, pack.Versions)
		}
	}
}

func TestCreatePackWillNotReplaceAPack(t *testing.T) {
	cmdtest.Chdir(t)
	if err := os.WriteFile("pack.toml", []byte("name = \"mine\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	err := CreatePack(NewPack{Name: "Other", MCVersion: "1.21.1"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("CreatePack() returned %v, want an error that a pack is there already", err)
	}
	if data, _ := os.ReadFile("pack.toml"); string(data) != "name = \"mine\"\n" {
		t.Errorf("pack.toml is now %q, want it left as it was", data)
	}
}

func TestCreatePackKeepsAnIndexThatIsThere(t *testing.T) {
	cmdtest.Chdir(t)
	if err := os.WriteFile("index.toml", []byte("hash-format = \"sha256\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	var told []string
	created, err := createPack(NewPack{Name: "P", Version: "1.0.0", MCVersion: "1.21.1"}, true, func(f string) { told = append(told, f) })
	if err != nil || created || len(told) != 0 {
		t.Errorf("createPack() = %v, %v and told of %v, want no index made, as there was one", created, err, told)
	}
}

func TestCreatePackSaysWhenItMakesTheIndex(t *testing.T) {
	cmdtest.Chdir(t)
	var told []string
	created, err := createPack(NewPack{Name: "P", Version: "1.0.0", MCVersion: "1.21.1", IndexFile: "files.toml"}, true, func(f string) { told = append(told, f) })
	if err != nil || !created || len(told) != 1 || told[0] != "files.toml" {
		t.Errorf("createPack() = %v, %v and told of %v, want the index it made to be said", created, err, told)
	}
	if pack, _ := core.LoadPack(); pack.Index.File != "files.toml" {
		t.Errorf("the pack's index is %q, want the file that was asked for", pack.Index.File)
	}
}

func TestLoaderVersionStripsTheMinecraftVersionFromNeoForgeForOneTwentyOnly(t *testing.T) {
	neoforge := core.ModLoaders["neoforge"]
	for _, tc := range []struct{ mc, chosen, want string }{
		{"1.20.1", "1.20.1-47.1.106", "47.1.106"},
		{"1.20.1", "47.1.106", "47.1.106"},
		{"1.21.1", "21.1.100", "21.1.100"},
		{"1.21.1", "a-b", "a-b"},
	} {
		if got := LoaderVersion(neoforge, tc.mc, tc.chosen); got != tc.want {
			t.Errorf("LoaderVersion(neoforge, %q, %q) = %q, want %q", tc.mc, tc.chosen, got, tc.want)
		}
	}
}

func TestLoaderVersionsOfAnUnknownLoader(t *testing.T) {
	if _, err := LoaderVersions("forge-ish", "1.21.1"); err == nil {
		t.Error("LoaderVersions() returned no error for a mod loader that isn't supported")
	}
}

func TestPackNameFromDirectoryMakesWordsOfIt(t *testing.T) {
	for directory, want := range map[string]string{
		"my-cool_pack": "My Cool Pack",
		"demoPack":     "Demo Pack",
		"survival":     "Survival",
	} {
		if got := packNameFromDirectory(directory); got != want {
			t.Errorf("packNameFromDirectory(%q) = %q, want %q", directory, got, want)
		}
	}
}

func TestDefaultPackNameIsWhatTheCurrentFolderIsCalled(t *testing.T) {
	dir := cmdtest.Chdir(t)
	if got, want := DefaultPackName(), packNameFromDirectory(filepath.Base(dir)); got != want || got == "" {
		t.Errorf("DefaultPackName() = %q, want %q", got, want)
	}
}
