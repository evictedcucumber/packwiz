package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func makeCheckedPack(t *testing.T) {
	t.Helper()
	cmdtest.Chdir(t)
	if err := CreatePack(NewPack{Name: "P", Author: "Me", Version: "1.0.0", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1.100"}); err != nil {
		t.Fatalf("CreatePack() returned error: %v", err)
	}
}

func TestCheckExistingPackLeavesAValidPackAlone(t *testing.T) {
	makeCheckedPack(t)
	packBefore, _ := os.ReadFile("pack.toml")
	indexBefore, _ := os.ReadFile("index.toml")

	result, err := CheckExistingPack(true)

	if err != nil || len(result.Created)+len(result.Errors)+len(result.Warnings) != 0 {
		t.Errorf("CheckExistingPack() = %+v, %v, want nothing to report", result, err)
	}
	if packAfter, _ := os.ReadFile("pack.toml"); string(packAfter) != string(packBefore) {
		t.Errorf("pack.toml is now %q, want it as it was", packAfter)
	}
	if indexAfter, _ := os.ReadFile("index.toml"); string(indexAfter) != string(indexBefore) {
		t.Errorf("index.toml is now %q, want it as it was", indexAfter)
	}
}

func TestCheckExistingPackCreatesAMissingIndexAndRecordsItsHash(t *testing.T) {
	makeCheckedPack(t)
	if err := os.WriteFile("options.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("index.toml"); err != nil {
		t.Fatal(err)
	}

	result, err := CheckExistingPack(true)

	if err != nil || len(result.Created) != 1 || len(result.Errors) != 0 {
		t.Fatalf("CheckExistingPack() = %+v, %v, want the index made", result, err)
	}
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.Name != "P" || pack.Versions["neoforge"] != "21.1.100" {
		t.Errorf("the pack is %+v, want what it was", pack)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	if _, ok := index.Files["options.txt"]; !ok {
		t.Errorf("the index has %v, want it to track the files that are there", index.Files)
	}
	if again, _ := CheckExistingPack(true); len(again.Warnings)+len(again.Errors)+len(again.Created) != 0 {
		t.Errorf("a second check found %+v, want the hash to match the index that was made", again)
	}
}

func TestCheckExistingPackReportsWhatIsWrongWithThePack(t *testing.T) {
	cmdtest.Chdir(t)
	if err := os.WriteFile("pack.toml", []byte("name = \"\"\npack-format = \"packwiz:1.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckExistingPack(true)

	if err != nil || len(result.Errors) < 3 {
		t.Fatalf("CheckExistingPack() = %+v, %v, want errors for the format, the version and the index", result, err)
	}
	if !strings.Contains(result.Errors[0], "pack-format") || !strings.Contains(result.Errors[1], "Minecraft") {
		t.Errorf("the errors are %q, want the format and Minecraft's version", result.Errors)
	}
	if _, err := os.Stat("index.toml"); err == nil {
		t.Error("index.toml was made for a pack that can't be used")
	}
}

func TestCheckExistingPackReportsAPackThatCannotBeRead(t *testing.T) {
	cmdtest.Chdir(t)
	if err := os.WriteFile("pack.toml", []byte("not toml ["), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := CheckExistingPack(true)
	if err != nil || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "-r") {
		t.Errorf("CheckExistingPack() = %+v, %v, want one error that suggests -r", result, err)
	}
}

func TestCheckExistingPackReportsAnIndexThatCannotBeRead(t *testing.T) {
	makeCheckedPack(t)
	if err := os.WriteFile("index.toml", []byte("files = ["), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := CheckExistingPack(true)
	if err != nil || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "index.toml") {
		t.Errorf("CheckExistingPack() = %+v, %v, want an error for the index", result, err)
	}
	if data, _ := os.ReadFile("index.toml"); string(data) != "files = [" {
		t.Errorf("index.toml is now %q, want it left as it was", data)
	}
}

func TestCheckExistingPackWarnsOfAnIndexThatDoesNotMatchItsHash(t *testing.T) {
	makeCheckedPack(t)
	f, err := os.OpenFile("index.toml", os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("\n# edited\n")
	_ = f.Close()

	result, err := CheckExistingPack(true)

	if err != nil || len(result.Errors) != 0 || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "refresh") {
		t.Errorf("CheckExistingPack() = %+v, %v, want a warning to refresh", result, err)
	}
}

func TestCheckExistingPackWarnsOfConfigFilesForAnOwnerThePackLacks(t *testing.T) {
	makeCheckedPack(t)
	pack, _ := core.LoadPack()
	pack.ClaimConfigFile("fabric", "config/x.toml")
	if err := pack.Write(); err != nil {
		t.Fatal(err)
	}
	result, _ := CheckExistingPack(true)
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "fabric") {
		t.Errorf("the warnings are %q, want one about fabric", result.Warnings)
	}
}

func TestInitWhereAPackIsDoesNotAskAnything(t *testing.T) {
	makeCheckedPack(t)
	if err := os.Remove("index.toml"); err != nil {
		t.Fatal(err)
	}
	cmdtest.SetStdin(t, "")
	var ok bool
	out := cmdtest.CaptureStdout(t, func() { ok = reportExistingPack() })
	if !ok || !strings.Contains(out, "index.toml created!") {
		t.Errorf("reportExistingPack() = %v and %q, want the index made", ok, out)
	}
}
