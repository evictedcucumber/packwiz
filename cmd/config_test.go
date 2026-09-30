package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/pflag"
)

// setUpConfigFixture builds a pack with one mod, whose config-files claims config/alpha.json and everything under
// config/alpha/, alongside an unclaimed file (config/orphan.json) and the mod's own jar.
func setUpConfigFixture(t *testing.T) {
	t.Helper()
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.20.1"},
	})

	if err := os.MkdirAll("mods", 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	alpha := `name = "Alpha Mod"
filename = "alpha.jar"
config-files = ["config/alpha.json", "config/alpha/"]

[download]
hash-format = "sha256"
hash = "a"
`
	if err := os.WriteFile("mods/alpha.pw.toml", []byte(alpha), 0644); err != nil {
		t.Fatalf("failed to write mod fixture: %v", err)
	}

	if err := os.WriteFile("index.toml", []byte(`hash-format = "sha256"

[[files]]
file = "mods/alpha.pw.toml"
hash = "irrelevant"
metafile = true

[[files]]
file = "mods/alpha.jar"
hash = "irrelevant"

[[files]]
file = "config/alpha.json"
hash = "irrelevant"

[[files]]
file = "config/alpha/sub.json"
hash = "irrelevant"

[[files]]
file = "config/orphan.json"
hash = "irrelevant"
`), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}
}

func setConfigListFlag(t *testing.T, name string, value string) {
	t.Helper()
	flag := configListCmd.Flags().Lookup(name)
	if flag == nil {
		t.Fatalf("no such flag --%s on configListCmd", name)
	}
	oldValue := flag.Value.String()
	oldChanged := flag.Changed
	if err := configListCmd.Flags().Set(name, value); err != nil {
		t.Fatalf("failed to set --%s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = configListCmd.Flags().Set(name, oldValue)
		flag.Changed = oldChanged
	})
}

// setConfigRelateModFlag gives configRelateCmd's --mod flag exactly these values for the duration of the test,
// restoring its previous value on cleanup. --mod is a StringArray flag, whose Set appends rather than replaces (so
// that "--mod a --mod b" builds up a list), so this goes through its SliceValue.Replace instead to set the whole
// list in one go, both here and when restoring it.
func setConfigRelateModFlag(t *testing.T, values ...string) {
	t.Helper()
	flag := configRelateCmd.Flags().Lookup("mod")
	if flag == nil {
		t.Fatalf("no such flag --mod on configRelateCmd")
	}
	sv, ok := flag.Value.(pflag.SliceValue)
	if !ok {
		t.Fatalf("--mod isn't a slice flag")
	}
	oldValues := sv.GetSlice()
	oldChanged := flag.Changed
	if err := sv.Replace(values); err != nil {
		t.Fatalf("failed to set --mod: %v", err)
	}
	flag.Changed = true
	t.Cleanup(func() {
		_ = sv.Replace(oldValues)
		flag.Changed = oldChanged
	})
}

func TestConfigListShowsATreeOfEachModsFilesThenInvalidOnes(t *testing.T) {
	setUpConfigFixture(t)

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Alpha Mod\n├── config/alpha.json\n└── config/alpha/sub.json\n\nInvalid\n└── config/orphan.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigListStateValidOnlyShowsClaimedFiles(t *testing.T) {
	setUpConfigFixture(t)
	setConfigListFlag(t, "state", "valid")

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Alpha Mod\n├── config/alpha.json\n└── config/alpha/sub.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigListStateInvalidOnlyShowsUnclaimedFiles(t *testing.T) {
	setUpConfigFixture(t)
	setConfigListFlag(t, "state", "invalid")

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Invalid\n└── config/orphan.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigListOmitsInvalidHeadingWhenNothingIsUnclaimed(t *testing.T) {
	setUpConfigFixture(t)
	// Claim the last file too, so nothing is left unclaimed
	if err := os.WriteFile("mods/alpha.pw.toml", []byte(`name = "Alpha Mod"
filename = "alpha.jar"
config-files = ["config/alpha.json", "config/alpha/", "config/orphan.json"]

[download]
hash-format = "sha256"
hash = "a"
`), 0644); err != nil {
		t.Fatalf("failed to rewrite mod fixture: %v", err)
	}

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Alpha Mod\n├── config/alpha.json\n├── config/alpha/sub.json\n└── config/orphan.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigListListsAFileClaimedByTwoModsUnderBoth(t *testing.T) {
	setUpConfigFixture(t)
	// Beta also claims config/alpha.json, alongside Alpha
	beta := `name = "Beta Mod"
filename = "beta.jar"
config-files = ["config/alpha.json"]

[download]
hash-format = "sha256"
hash = "b"
`
	if err := os.WriteFile("mods/beta.pw.toml", []byte(beta), 0644); err != nil {
		t.Fatalf("failed to write second mod fixture: %v", err)
	}
	index, err := os.ReadFile("index.toml")
	if err != nil {
		t.Fatalf("failed to read index.toml fixture: %v", err)
	}
	entry := "\n[[files]]\nfile = \"mods/beta.pw.toml\"\nhash = \"irrelevant\"\nmetafile = true\n"
	if err := os.WriteFile("index.toml", append(index, entry...), 0644); err != nil {
		t.Fatalf("failed to rewrite index.toml fixture: %v", err)
	}

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Alpha Mod\n├── config/alpha.json\n└── config/alpha/sub.json\n\n" +
		"Beta Mod\n└── config/alpha.json\n\n" +
		"Invalid\n└── config/orphan.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// claimMissingConfigFiles has the fixture's mod claim two more entries that nothing in the pack matches: a file that
// has been deleted, and a folder with nothing left in it
func claimMissingConfigFiles(t *testing.T) {
	t.Helper()
	if err := os.WriteFile("mods/alpha.pw.toml", []byte(`name = "Alpha Mod"
filename = "alpha.jar"
config-files = ["config/alpha.json", "config/alpha/", "config/gone.json", "config/gone/"]

[download]
hash-format = "sha256"
hash = "a"
`), 0644); err != nil {
		t.Fatalf("failed to rewrite mod fixture: %v", err)
	}
}

func TestConfigListShowsEntriesThatMatchNoFileAsMissingUnderTheirMod(t *testing.T) {
	setUpConfigFixture(t)
	claimMissingConfigFiles(t)

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Alpha Mod\n├── config/alpha.json\n├── config/alpha/sub.json\n├── config/gone.json (missing)\n└── config/gone/ (missing)\n\n" +
		"Invalid\n└── config/orphan.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigListStateMissingOnlyShowsEntriesThatMatchNoFile(t *testing.T) {
	setUpConfigFixture(t)
	claimMissingConfigFiles(t)
	setConfigListFlag(t, "state", "missing")

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Alpha Mod\n├── config/gone.json (missing)\n└── config/gone/ (missing)\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigListStateValidLeavesOutMissingEntries(t *testing.T) {
	setUpConfigFixture(t)
	claimMissingConfigFiles(t)
	setConfigListFlag(t, "state", "valid")

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Alpha Mod\n├── config/alpha.json\n└── config/alpha/sub.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigListStateInvalidLeavesOutMissingEntries(t *testing.T) {
	setUpConfigFixture(t)
	claimMissingConfigFiles(t)
	setConfigListFlag(t, "state", "invalid")

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Invalid\n└── config/orphan.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// A mod that has nothing left of what it claims is still listed, with only what is missing
func TestConfigListShowsAModWhoseConfigFilesAreAllMissing(t *testing.T) {
	setUpConfigFixture(t)
	if err := os.WriteFile("mods/alpha.pw.toml", []byte(`name = "Alpha Mod"
filename = "alpha.jar"
config-files = ["config/gone.json"]

[download]
hash-format = "sha256"
hash = "a"
`), 0644); err != nil {
		t.Fatalf("failed to rewrite mod fixture: %v", err)
	}
	setConfigListFlag(t, "state", "missing")

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	want := "Alpha Mod\n└── config/gone.json (missing)\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// setUpConfigRelateFixture builds a pack with one mod (Alpha Mod, with no config-files yet) and a real config/new.json
// file and config/sub/ folder on disk, for "packwiz config relate" to link it to.
func setUpConfigRelateFixture(t *testing.T) {
	t.Helper()
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.20.1"},
	})

	if err := os.MkdirAll("mods", 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	alpha := `name = "Alpha Mod"
filename = "alpha.jar"

[download]
hash-format = "sha256"
hash = "a"
`
	if err := os.WriteFile("mods/alpha.pw.toml", []byte(alpha), 0644); err != nil {
		t.Fatalf("failed to write mod fixture: %v", err)
	}
	if err := os.WriteFile("index.toml", []byte(`hash-format = "sha256"

[[files]]
file = "mods/alpha.pw.toml"
hash = "irrelevant"
metafile = true
`), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}

	if err := os.MkdirAll("config/sub", 0755); err != nil {
		t.Fatalf("failed to create config/sub dir: %v", err)
	}
	if err := os.WriteFile("config/new.json", []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write config/new.json: %v", err)
	}
}

func TestConfigRelateAddsAFileToAModsConfigFiles(t *testing.T) {
	setUpConfigRelateFixture(t)
	setConfigRelateModFlag(t, "alpha")

	out := cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"config/new.json"})
	})
	if !strings.Contains(out, "Alpha Mod now claims config/new.json") {
		t.Errorf("output = %q, want a success message", out)
	}

	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.ConfigFiles == nil || !slices.Contains(*mod.ConfigFiles, "config/new.json") {
		t.Errorf("ConfigFiles = %v, want it to contain config/new.json", mod.ConfigFiles)
	}
}

func TestConfigRelateAddsATrailingSlashForAFolder(t *testing.T) {
	setUpConfigRelateFixture(t)
	setConfigRelateModFlag(t, "alpha")

	cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"config/sub"})
	})

	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.ConfigFiles == nil || !slices.Contains(*mod.ConfigFiles, "config/sub/") {
		t.Errorf("ConfigFiles = %v, want it to contain config/sub/ (with a trailing slash)", mod.ConfigFiles)
	}
}

func TestConfigRelateDoesNothingWhenAlreadyClaimed(t *testing.T) {
	setUpConfigRelateFixture(t)
	setConfigRelateModFlag(t, "alpha")
	cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"config/new.json"})
	})

	out := cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"config/new.json"})
	})
	if !strings.Contains(out, "already claims") {
		t.Errorf("output = %q, want a notice that it already claims the file", out)
	}

	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if got := len(*mod.ConfigFiles); got != 1 {
		t.Errorf("ConfigFiles = %v, want just the one entry, not a duplicate", *mod.ConfigFiles)
	}
}

func TestConfigRelateResolvedAgainstTheConfigDir(t *testing.T) {
	setUpConfigRelateFixture(t)
	setConfigRelateModFlag(t, "alpha")
	cmdtest.RegisterConfigDirSource(t, "defaults", "configureddefaults")
	alpha := `name = "Alpha Mod"
filename = "alpha.jar"

[download]
hash-format = "sha256"
hash = "a"

[update.defaults]
version = "any"
`
	if err := os.WriteFile("mods/alpha.pw.toml", []byte(alpha), 0644); err != nil {
		t.Fatalf("failed to rewrite mod fixture: %v", err)
	}
	if err := os.MkdirAll("configureddefaults/config", 0755); err != nil {
		t.Fatalf("failed to create configureddefaults/config dir: %v", err)
	}
	if err := os.WriteFile("configureddefaults/config/new.json", []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write configureddefaults/config/new.json: %v", err)
	}

	cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"configureddefaults/config/new.json"})
	})

	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.ConfigFiles == nil || !slices.Contains(*mod.ConfigFiles, "config/new.json") {
		t.Errorf("ConfigFiles = %v, want config/new.json, with the configureddefaults/ folder taken back out", mod.ConfigFiles)
	}
}

func TestConfigRelateFailsWithoutAModFlag(t *testing.T) {
	if os.Getenv("PACKWIZ_TEST_CONFIG_RELATE_NO_MOD") == "1" {
		setUpConfigRelateFixture(t)
		configRelateCmd.Run(configRelateCmd, []string{"config/new.json"})
		return // The command didn't end, so the process doesn't fail, which is what the test looks for
	}

	process := exec.Command(os.Args[0], "-test.run=^TestConfigRelateFailsWithoutAModFlag$")
	process.Env = append(os.Environ(), "PACKWIZ_TEST_CONFIG_RELATE_NO_MOD=1")
	out, err := process.CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("the command ended with %v, want it to fail with exit status 1\noutput: %s", err, out)
	}
	if want := "--mod, --loader or --pack is required"; !strings.Contains(string(out), want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

// setUpConfigRelateSecondModFixture adds a second mod (Beta Mod, with no config-files yet) to the pack that
// setUpConfigRelateFixture built, for tests of relating config files to several mods at once.
func setUpConfigRelateSecondModFixture(t *testing.T) {
	t.Helper()
	beta := `name = "Beta Mod"
filename = "beta.jar"

[download]
hash-format = "sha256"
hash = "b"
`
	if err := os.WriteFile("mods/beta.pw.toml", []byte(beta), 0644); err != nil {
		t.Fatalf("failed to write second mod fixture: %v", err)
	}
	index, err := os.ReadFile("index.toml")
	if err != nil {
		t.Fatalf("failed to read index.toml fixture: %v", err)
	}
	entry := "\n[[files]]\nfile = \"mods/beta.pw.toml\"\nhash = \"irrelevant\"\nmetafile = true\n"
	if err := os.WriteFile("index.toml", append(index, entry...), 0644); err != nil {
		t.Fatalf("failed to rewrite index.toml fixture: %v", err)
	}
}

func TestConfigRelateAddsEveryConfigFileToEveryModGiven(t *testing.T) {
	setUpConfigRelateFixture(t)
	setUpConfigRelateSecondModFixture(t)
	setConfigRelateModFlag(t, "alpha", "beta")

	out := cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"config/new.json", "config/sub"})
	})
	for _, want := range []string{
		"Alpha Mod now claims config/new.json, config/sub/",
		"Beta Mod now claims config/new.json, config/sub/",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	}

	for _, fixture := range []struct{ file, name string }{
		{"mods/alpha.pw.toml", "Alpha"},
		{"mods/beta.pw.toml", "Beta"},
	} {
		mod, err := core.LoadMod(fixture.file)
		if err != nil {
			t.Fatalf("LoadMod(%q) returned error: %v", fixture.file, err)
		}
		for _, want := range []string{"config/new.json", "config/sub/"} {
			if mod.ConfigFiles == nil || !slices.Contains(*mod.ConfigFiles, want) {
				t.Errorf("%s's ConfigFiles = %v, want it to contain %s", fixture.name, mod.ConfigFiles, want)
			}
		}
	}
}

// setUpOwnersFixture builds a pack that has the mod loader neoforge, with one mod (Alpha Mod, which claims
// config/alpha.json), and the files that belong to nobody in particular: options.txt, which the pack as a whole owns,
// neoforge's own config/neoforge-common.toml and config/neoforge-client.toml, and config/orphan.json that nothing does.
// configFiles is what pack.toml says the pack and its loader own.
func setUpOwnersFixture(t *testing.T, configFiles map[string][]string) {
	t.Helper()
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions:    map[string]string{"minecraft": "1.21.1", "neoforge": "21.1.0"},
		ConfigFiles: configFiles,
	})
	if err := os.MkdirAll("mods", 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	alpha := `name = "Alpha Mod"
filename = "alpha.jar"
config-files = ["config/alpha.json"]

[download]
hash-format = "sha256"
hash = "a"
`
	if err := os.WriteFile("mods/alpha.pw.toml", []byte(alpha), 0644); err != nil {
		t.Fatalf("failed to write mod fixture: %v", err)
	}
	index := "hash-format = \"sha256\"\n\n[[files]]\nfile = \"mods/alpha.pw.toml\"\nhash = \"irrelevant\"\nmetafile = true\n"
	for _, f := range []string{"options.txt", "config/neoforge-common.toml", "config/neoforge-client.toml", "config/alpha.json", "config/orphan.json"} {
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			t.Fatalf("failed to create the folder of %s: %v", f, err)
		}
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", f, err)
		}
		index += "\n[[files]]\nfile = \"" + f + "\"\nhash = \"irrelevant\"\n"
	}
	if err := os.WriteFile("index.toml", []byte(index), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}
}

func setRelateBool(t *testing.T, name string) {
	t.Helper()
	cmdtest.SetViperBool(t, "config.relate."+name, true)
}

func packConfigFiles(t *testing.T) map[string][]string {
	t.Helper()
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	return pack.ConfigFiles
}

func TestConfigListShowsThePackAndItsLoaderBeforeTheMods(t *testing.T) {
	setUpOwnersFixture(t, map[string][]string{
		"neoforge": {"config/neoforge-common.toml", "config/neoforge-client.toml", "config/neoforge-gone.toml"},
		"pack":     {"options.txt"},
	})

	out := cmdtest.CaptureStdout(t, func() { configListCmd.Run(configListCmd, nil) })
	want := "Pack\n└── options.txt\n\n" +
		"NeoForge\n├── config/neoforge-client.toml\n├── config/neoforge-common.toml\n└── config/neoforge-gone.toml (missing)\n\n" +
		"Alpha Mod\n└── config/alpha.json\n\n" +
		"Invalid\n└── config/orphan.json\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestConfigListStatesAppliesToThePackAndItsLoader(t *testing.T) {
	setUpOwnersFixture(t, map[string][]string{
		"neoforge": {"config/neoforge-common.toml", "config/neoforge-gone.toml"},
		"pack":     {"options.txt"},
	})
	for state, want := range map[string]string{
		"valid":   "Pack\n└── options.txt\n\nNeoForge\n└── config/neoforge-common.toml\n\nAlpha Mod\n└── config/alpha.json\n",
		"missing": "NeoForge\n└── config/neoforge-gone.toml (missing)\n",
		// Only what nothing claims is invalid, which is less of it now that the pack claims options.txt
		"invalid": "Invalid\n├── config/neoforge-client.toml\n└── config/orphan.json\n",
	} {
		t.Run(state, func(t *testing.T) {
			setConfigListFlag(t, "state", state)
			if out := cmdtest.CaptureStdout(t, func() { configListCmd.Run(configListCmd, nil) }); out != want {
				t.Errorf("output = %q, want %q", out, want)
			}
		})
	}
}

func TestConfigListColoursThePackAndItsLoaderLikeTheMods(t *testing.T) {
	setUpOwnersFixture(t, map[string][]string{"pack": {"options.txt"}, "neoforge": {"config/neoforge-common.toml"}})
	plain, coloured := cmdtest.AssertColourOnlyAdds(t, func() { configListCmd.Run(configListCmd, nil) })
	if !strings.Contains(plain, "Pack\n") {
		t.Errorf("output = %q, want the pack", plain)
	}
	for _, want := range []string{ui.Bold.Sprint("Pack"), ui.Bold.Sprint("NeoForge"), ui.Success.Sprint("└── options.txt")} {
		if !strings.Contains(coloured, want) {
			t.Errorf("coloured output doesn't have %q: %q", want, coloured)
		}
	}
}

func TestConfigRelateTheFileToThePackAsAWhole(t *testing.T) {
	setUpOwnersFixture(t, nil)
	setRelateBool(t, "pack")
	indexBefore, _ := os.ReadFile("index.toml")

	out := cmdtest.CaptureStdout(t, func() { configRelateCmd.Run(configRelateCmd, []string{"options.txt"}) })
	if !strings.Contains(out, "Pack now claims options.txt") {
		t.Errorf("output = %q, want a success message", out)
	}
	if got := packConfigFiles(t); !reflect.DeepEqual(got, map[string][]string{"pack": {"options.txt"}}) {
		t.Errorf("pack.toml's config-files = %v, want options.txt under the pack", got)
	}
	// What the pack owns is only in pack.toml, which isn't in the index
	if indexAfter, _ := os.ReadFile("index.toml"); !slices.Equal(indexBefore, indexAfter) {
		t.Errorf("index.toml changed:\n%s", indexAfter)
	}

	listed := cmdtest.CaptureStdout(t, func() { configListCmd.Run(configListCmd, nil) })
	if !strings.HasPrefix(listed, "Pack\n└── options.txt\n") {
		t.Errorf("config list = %q, want options.txt under the pack", listed)
	}
}

func TestConfigRelateTheFilesToTheModLoader(t *testing.T) {
	setUpOwnersFixture(t, nil)
	setRelateBool(t, "loader")

	out := cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"config/neoforge-common.toml", "config/neoforge-client.toml"})
	})
	if !strings.Contains(out, "NeoForge now claims config/neoforge-common.toml, config/neoforge-client.toml") {
		t.Errorf("output = %q, want a success message that names the loader as it is written", out)
	}
	want := map[string][]string{"neoforge": {"config/neoforge-common.toml", "config/neoforge-client.toml"}}
	if got := packConfigFiles(t); !reflect.DeepEqual(got, want) {
		t.Errorf("pack.toml's config-files = %v, want %v", got, want)
	}
}

func TestConfigRelateToModsAndThePackAndItsLoaderTogether(t *testing.T) {
	setUpOwnersFixture(t, nil)
	setConfigRelateModFlag(t, "alpha")
	setRelateBool(t, "pack")
	setRelateBool(t, "loader")

	out := cmdtest.CaptureStdout(t, func() { configRelateCmd.Run(configRelateCmd, []string{"config/orphan.json"}) })
	for _, want := range []string{"Pack now claims config/orphan.json", "NeoForge now claims config/orphan.json", "Alpha Mod now claims config/orphan.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want %q", out, want)
		}
	}
	// The pack and its loader are said first, as config list has them
	if strings.Index(out, "Pack now") > strings.Index(out, "NeoForge now") || strings.Index(out, "NeoForge now") > strings.Index(out, "Alpha Mod now") {
		t.Errorf("output = %q, want the pack, then the loader, then the mod", out)
	}

	got := packConfigFiles(t)
	if !slices.Equal(got["pack"], []string{"config/orphan.json"}) || !slices.Equal(got["neoforge"], []string{"config/orphan.json"}) {
		t.Errorf("pack.toml's config-files = %v, want the file under both", got)
	}
	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if !slices.Contains(mod.ConfigEntries(), "config/orphan.json") {
		t.Errorf("Alpha Mod's config-files = %v, want the file in it", mod.ConfigEntries())
	}
	// Changing a mod saves the index with the pack, which has to be the pack that was changed
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.Index.Hash == "" {
		t.Error("pack.toml records no hash of the index after a mod was changed")
	}
}

func TestConfigRelateThePackAgainDoesNothingMore(t *testing.T) {
	setUpOwnersFixture(t, map[string][]string{"pack": {"options.txt"}})
	setRelateBool(t, "pack")
	before, _ := os.ReadFile("pack.toml")

	out := cmdtest.CaptureStdout(t, func() { configRelateCmd.Run(configRelateCmd, []string{"options.txt"}) })
	if !strings.Contains(out, "Pack already claims options.txt") || strings.Contains(out, "now claims") {
		t.Errorf("output = %q, want to be told it already does", out)
	}
	if after, _ := os.ReadFile("pack.toml"); !slices.Equal(before, after) {
		t.Errorf("pack.toml was rewritten:\n%s", after)
	}
}

func TestConfigRelateThePackIsWrittenWithoutTheConfigDir(t *testing.T) {
	setUpOwnersFixture(t, nil)
	cmdtest.RegisterConfigDirSource(t, "defaults", "configureddefaults")
	defaults := `name = "Defaults"
filename = "defaults.jar"

[download]
hash-format = "sha256"
hash = "a"

[update.defaults]
version = "any"
`
	if err := os.WriteFile("mods/defaults.pw.toml", []byte(defaults), 0644); err != nil {
		t.Fatalf("failed to write the mod: %v", err)
	}
	index, _ := os.ReadFile("index.toml")
	index = append(index, "\n[[files]]\nfile = \"mods/defaults.pw.toml\"\nhash = \"irrelevant\"\nmetafile = true\n"...)
	if err := os.WriteFile("index.toml", index, 0644); err != nil {
		t.Fatalf("failed to write index.toml: %v", err)
	}
	if err := os.MkdirAll("configureddefaults", 0755); err != nil {
		t.Fatalf("failed to create the config folder: %v", err)
	}
	if err := os.WriteFile("configureddefaults/options.txt", []byte("x"), 0644); err != nil {
		t.Fatalf("failed to write options.txt: %v", err)
	}
	setRelateBool(t, "pack")

	cmdtest.CaptureStdout(t, func() { configRelateCmd.Run(configRelateCmd, []string{"configureddefaults/options.txt"}) })
	if got := packConfigFiles(t); !reflect.DeepEqual(got, map[string][]string{"pack": {"options.txt"}}) {
		t.Errorf("pack.toml's config-files = %v, want the entry without the config folder", got)
	}
}

func TestConfigRelateTheLoaderFailsForAPackThatHasNone(t *testing.T) {
	if os.Getenv("PACKWIZ_TEST_CONFIG_RELATE_NO_LOADER") == "1" {
		setUpOwnersFixture(t, nil)
		cmdtest.WritePackFile(t, core.Pack{Name: "Test Pack", PackFormat: core.CurrentPackFormat, Versions: map[string]string{"minecraft": "1.21.1"}})
		setRelateBool(t, "loader")
		configRelateCmd.Run(configRelateCmd, []string{"config/orphan.json"})
		return
	}

	process := exec.Command(os.Args[0], "-test.run=^TestConfigRelateTheLoaderFailsForAPackThatHasNone$")
	process.Env = append(os.Environ(), "PACKWIZ_TEST_CONFIG_RELATE_NO_LOADER=1")
	out, err := process.CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("the command ended with %v, want it to fail with exit status 1\noutput: %s", err, out)
	}
	if want := "no mod loader"; !strings.Contains(string(out), want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}
