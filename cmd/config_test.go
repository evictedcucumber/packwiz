package cmd

import (
	"os"
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

func TestConfigListListsTrackedConfigFilesNotModsOwnFiles(t *testing.T) {
	setUpConfigFixture(t)

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"config/alpha.json", "config/alpha/sub.json", "config/orphan.json"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %q", len(lines), len(want), out)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
	if strings.Contains(out, "mods/alpha.jar") {
		t.Errorf("output = %q, shouldn't list the mod's own file", out)
	}
}

func TestConfigListInvalidOnlyListsUnclaimedFiles(t *testing.T) {
	setUpConfigFixture(t)
	setConfigListFlag(t, "invalid", "true")

	out := cmdtest.CaptureStdout(t, func() {
		configListCmd.Run(configListCmd, nil)
	})
	if strings.TrimSpace(out) != "config/orphan.json" {
		t.Errorf("output = %q, want only config/orphan.json", out)
	}
}
