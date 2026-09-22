package cmd

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
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

	out := cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"alpha", "config/new.json"})
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

	cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"alpha", "config/sub"})
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
	cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"alpha", "config/new.json"})
	})

	out := cmdtest.CaptureStdout(t, func() {
		configRelateCmd.Run(configRelateCmd, []string{"alpha", "config/new.json"})
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
		configRelateCmd.Run(configRelateCmd, []string{"alpha", "configureddefaults/config/new.json"})
	})

	mod, err := core.LoadMod("mods/alpha.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.ConfigFiles == nil || !slices.Contains(*mod.ConfigFiles, "config/new.json") {
		t.Errorf("ConfigFiles = %v, want config/new.json, with the configureddefaults/ folder taken back out", mod.ConfigFiles)
	}
}
