package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// setUpModRefFixture builds a pack of mods that have these names, by slug, and returns its index.
func setUpModRefFixture(t *testing.T, mods map[string]string) core.Index {
	t.Helper()
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.21.1"},
	})
	if err := os.MkdirAll("mods", 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	indexFile := "hash-format = \"sha256\"\n"
	for slug, name := range mods {
		mod := fmt.Sprintf("name = %q\nfilename = %q\n\n[download]\nhash-format = \"sha256\"\nhash = \"a\"\n", name, slug+".jar")
		if err := os.WriteFile("mods/"+slug+".pw.toml", []byte(mod), 0644); err != nil {
			t.Fatalf("failed to write mod fixture: %v", err)
		}
		indexFile += fmt.Sprintf("\n[[files]]\nfile = \"mods/%s.pw.toml\"\nhash = \"irrelevant\"\nmetafile = true\n", slug)
	}
	if err := os.WriteFile("index.toml", []byte(indexFile), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	return index
}

var modRefMods = map[string]string{
	"sodium": "Sodium", "sodium-extra": "Sodium Extra", "iris": "Iris Shaders", "lithium": "Lithium",
}

func TestResolveModTakesAnExactReferenceAsItAlwaysDid(t *testing.T) {
	index := setUpModRefFixture(t, modRefMods)
	for _, ref := range []string{"sodium", "sodium.pw.toml", "mods/sodium.pw.toml"} {
		path, matched, err := resolveMod(index, ref)
		if err != nil || matched != nil || !strings.HasSuffix(path, "sodium.pw.toml") {
			t.Errorf("resolveMod(%q) = %q, %v, %v, want the mod, found by what it is", ref, path, matched, err)
		}
	}
}

// What is exactly a mod's is that mod, though it is also part of another's, so a mod can always be got at
func TestResolveModPrefersAnExactMatchToAFuzzyOne(t *testing.T) {
	index := setUpModRefFixture(t, map[string]string{"sod": "Sod", "sodium": "Sodium"})
	path, matched, err := resolveMod(index, "sod")
	if err != nil || matched != nil || !strings.HasSuffix(path, "mods/sod.pw.toml") {
		t.Errorf("resolveMod() = %q, %v, %v, want sod, which is the name of a mod, though it fuzzily matches sodium too", path, matched, err)
	}
}

func TestResolveModTakesTheOneModThatAPartOfANameMatches(t *testing.T) {
	index := setUpModRefFixture(t, modRefMods)
	for ref, slug := range map[string]string{
		"sodex":         "sodium-extra", // characters in order
		"sod ex":        "sodium-extra", // several words
		"Extra Sodium":  "sodium-extra",
		"iris shaders":  "iris", // its name, which isn't its slug
		"irsh":          "iris",
		"lith":          "lithium",
		"sodex.pw.toml": "sodium-extra", // a file name that is shortened
		"sodex.toml":    "sodium-extra",
	} {
		path, matched, err := resolveMod(index, ref)
		if err != nil {
			t.Errorf("resolveMod(%q) returned error: %v", ref, err)
			continue
		}
		if !strings.HasSuffix(path, "mods/"+slug+".pw.toml") || matched == nil || matched.Slug != slug {
			t.Errorf("resolveMod(%q) = %q, %+v, want %s, and to say it was a match", ref, path, matched, slug)
		}
	}
}

// It would be acting on the wrong mod as likely as the right one, so none is chosen and the candidates are listed
func TestResolveModWontChooseBetweenSeveralMatches(t *testing.T) {
	index := setUpModRefFixture(t, modRefMods)
	path, matched, err := resolveMod(index, "sdm")
	if err == nil || path != "" || matched != nil {
		t.Fatalf("resolveMod() = %q, %v, %v, want an error and no mod", path, matched, err)
	}
	for _, want := range []string{`"sdm" matches several mods`, "Sodium (sodium), Sodium Extra (sodium-extra)", "specify its slug, its .pw.toml file name, or a path"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q doesn't say %q", err, want)
		}
	}
	// The best is first, as it is in every list, and there is no colour in an error
	if strings.Contains(err.Error(), "\x1b") {
		t.Errorf("the error has styling in it: %q", err)
	}
}

func TestResolveModSaysHowManyMoreMatchesThereAreThanItLists(t *testing.T) {
	mods := map[string]string{}
	for i := 1; i <= 8; i++ {
		mods[fmt.Sprintf("mod-%d", i)] = fmt.Sprintf("Mod %d", i)
	}
	index := setUpModRefFixture(t, mods)
	_, _, err := resolveMod(index, "mod")
	if err == nil || !strings.Contains(err.Error(), "and 3 more") {
		t.Fatalf("resolveMod() returned %v, want an error that says there are three more than the five it lists", err)
	}
	if got := strings.Count(err.Error(), "Mod "); got != maxCandidates {
		t.Errorf("the error lists %d mods, want %d", got, maxCandidates)
	}
}

func TestResolveModSaysWhenThereIsNoSuchMod(t *testing.T) {
	index := setUpModRefFixture(t, modRefMods)
	for _, ref := range []string{"nonexistent", "zzz", "", "   "} {
		_, _, err := resolveMod(index, ref)
		if err == nil || !strings.Contains(err.Error(), "Can't find") || !strings.Contains(err.Error(), "packwiz refresh") {
			t.Errorf("resolveMod(%q) returned %v, want the error that says to refresh and what can be given", ref, err)
		}
	}
}

// A reference with a folder in it is a path, and one that isn't there is a mistake, not a name to look for
func TestResolveModNeverSearchesForAPath(t *testing.T) {
	index := setUpModRefFixture(t, map[string]string{"weird": "Weird mods/x Mod", "other": "Other"})
	if _, _, err := resolveMod(index, "mods/x"); err == nil {
		t.Error("a path that isn't there was searched for, and found a mod with that in its name")
	}
	if _, matched, err := resolveMod(index, "weird"); err != nil || matched != nil {
		t.Errorf("resolveMod() = %v, %v, want the mod by its slug", matched, err)
	}
	if _, _, err := resolveMod(index, `mods\x`); err == nil {
		t.Error("a path with backslashes was searched for")
	}
}

func TestResolveModFailsForAModThatCantBeRead(t *testing.T) {
	index := setUpModRefFixture(t, modRefMods)
	if err := os.WriteFile("mods/iris.pw.toml", []byte("this is not toml ="), 0644); err != nil {
		t.Fatalf("failed to break the mod: %v", err)
	}
	// An exact reference doesn't read anything, so the mod that can't be read isn't in the way
	if _, _, err := resolveMod(index, "sodium"); err != nil {
		t.Errorf("an exact reference returned %v, want it not to read other mods", err)
	}
	if _, _, err := resolveMod(index, "lith"); err == nil || !strings.Contains(err.Error(), "iris.pw.toml") {
		t.Errorf("a search returned %v, want an error that says which file couldn't be read", err)
	}
}

func TestPinAModByAPartOfItsNameSaysWhichItTook(t *testing.T) {
	setUpListFixture(t)
	out := cmdtest.CaptureStdout(t, func() { PinCmd.Run(PinCmd, []string{"gamm"}) })
	for _, want := range []string{"gamm isn't the name of a mod, so using Gamma Mod (gamma), which it matches", "gamm pinned successfully!"} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want %q", out, want)
		}
	}
	mod, err := core.LoadMod("mods/gamma.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if !mod.Pin {
		t.Error("Gamma Mod isn't pinned")
	}
	if other, _ := core.LoadMod("mods/alpha.pw.toml"); other.Pin {
		t.Error("Alpha Mod is pinned too")
	}
}

// Exactly as before when the name is exact: nothing is said about a match
func TestPinAModByItsSlugSaysNothingAboutAMatch(t *testing.T) {
	setUpListFixture(t)
	out := cmdtest.CaptureStdout(t, func() { PinCmd.Run(PinCmd, []string{"alpha"}) })
	if out != "Loading modpack...\nalpha pinned successfully!\n" {
		t.Errorf("output = %q, want only that it was loaded and pinned, with nothing about a match", out)
	}
}

func TestMarkAModAsADependencyByAPartOfItsName(t *testing.T) {
	setUpListFixture(t)
	out := cmdtest.CaptureStdout(t, func() { MarkDependencyCmd.Run(MarkDependencyCmd, []string{"gamm"}) })
	if !strings.Contains(out, "using Gamma Mod (gamma)") {
		t.Errorf("output = %q, want to be told which mod was taken", out)
	}
	if mod, err := core.LoadMod("mods/gamma.pw.toml"); err != nil || !mod.AddedAsDependency {
		t.Errorf("Gamma Mod = %+v, %v, want it marked as a dependency", mod, err)
	}
}

func TestConfigRelateTakesAPartOfAModsNameToo(t *testing.T) {
	setUpConfigRelateFixture(t)
	setConfigRelateModFlag(t, "alp")
	out := cmdtest.CaptureStdout(t, func() { configRelateCmd.Run(configRelateCmd, []string{"config/new.json"}) })
	if !strings.Contains(out, "using Alpha Mod (alpha)") || !strings.Contains(out, "Alpha Mod now claims config/new.json") {
		t.Errorf("output = %q, want it to say which mod it took and that it claims the file", out)
	}
}

// A reference that could be several mods ends the command, without changing any of them
func TestPinWithAnAmbiguousNameFailsWithoutChangingAnything(t *testing.T) {
	if os.Getenv("PACKWIZ_TEST_PIN_AMBIGUOUS") == "1" {
		setUpListFixture(t)
		PinCmd.Run(PinCmd, []string{"mod"}) // every mod in the fixture is called ... Mod
		return
	}

	process := exec.Command(os.Args[0], "-test.run=^TestPinWithAnAmbiguousNameFailsWithoutChangingAnything$")
	process.Env = append(os.Environ(), "PACKWIZ_TEST_PIN_AMBIGUOUS=1")
	out, err := process.CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("the command ended with %v, want it to fail with exit status 1\noutput: %s", err, out)
	}
	if want := `"mod" matches several mods`; !strings.Contains(string(out), want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if strings.Contains(string(out), "pinned successfully") {
		t.Errorf("output says something was pinned:\n%s", out)
	}
}
