package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// setUpListFixture builds a pack with three mods: a main client-side mod, a
// main universal-side mod, and a server-side mod added as a dependency.
func setUpListFixture(t *testing.T) {
	t.Helper()
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.20.1"},
	})

	if err := os.MkdirAll("mods", 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	mods := map[string]string{
		"alpha.pw.toml": `name = "Alpha Mod"
filename = "alpha.jar"
side = "client"

[download]
hash-format = "sha256"
hash = "a"
`,
		"gamma.pw.toml": `name = "Gamma Mod"
filename = "gamma.jar"

[download]
hash-format = "sha256"
hash = "g"
`,
		"beta.pw.toml": `name = "Beta Mod"
filename = "beta.jar"
side = "server"
added-as-dependency = true

[download]
hash-format = "sha256"
hash = "b"
`,
	}
	for name, content := range mods {
		if err := os.WriteFile("mods/"+name, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write mod fixture %s: %v", name, err)
		}
	}

	if err := os.WriteFile("index.toml", []byte(`hash-format = "sha256"

[[files]]
file = "mods/alpha.pw.toml"
hash = "irrelevant"
metafile = true

[[files]]
file = "mods/beta.pw.toml"
hash = "irrelevant"
metafile = true

[[files]]
file = "mods/gamma.pw.toml"
hash = "irrelevant"
metafile = true
`), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}
}

// setListFlag sets a listCmd flag as a real invocation would (marking it
// Changed, so viper.IsSet observes it), restoring the flag afterward.
func setListFlag(t *testing.T, name string, value string) {
	t.Helper()
	flag := listCmd.Flags().Lookup(name)
	if flag == nil {
		t.Fatalf("no such flag --%s on listCmd", name)
	}
	oldValue := flag.Value.String()
	oldChanged := flag.Changed
	if err := listCmd.Flags().Set(name, value); err != nil {
		t.Fatalf("failed to set --%s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = listCmd.Flags().Set(name, oldValue)
		flag.Changed = oldChanged
	})
}

func TestListDefaultOrderAndContent(t *testing.T) {
	setUpListFixture(t)

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"Alpha Mod", "Beta Mod", "Gamma Mod"} // case-insensitive alphabetical
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %q", len(lines), len(want), out)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestListOnlyMain(t *testing.T) {
	setUpListFixture(t)
	setListFlag(t, "only", "main")

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})
	if strings.Contains(out, "Beta Mod") {
		t.Errorf("output = %q, should exclude the dependency-only mod", out)
	}
	if !strings.Contains(out, "Alpha Mod") || !strings.Contains(out, "Gamma Mod") {
		t.Errorf("output = %q, should include both main mods", out)
	}
}

func TestListOnlyDependencies(t *testing.T) {
	setUpListFixture(t)
	setListFlag(t, "only", "dependencies")

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})
	if strings.TrimSpace(out) != "Beta Mod" {
		t.Errorf("output = %q, want only Beta Mod", out)
	}
}

func TestListSideClient(t *testing.T) {
	setUpListFixture(t)
	setListFlag(t, "side", core.ClientSide)

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})
	if strings.Contains(out, "Beta Mod") {
		t.Errorf("output = %q, should exclude the server-only mod", out)
	}
	if !strings.Contains(out, "Alpha Mod") || !strings.Contains(out, "Gamma Mod") {
		t.Errorf("output = %q, should include the client and universal mods", out)
	}
}

func TestListShowKind(t *testing.T) {
	setUpListFixture(t)
	setListFlag(t, "show-kind", "true")

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})
	if !strings.Contains(out, "Beta Mod [dependency]") {
		t.Errorf("output = %q, want Beta Mod tagged as [dependency]", out)
	}
	if !strings.Contains(out, "Alpha Mod [main]") {
		t.Errorf("output = %q, want Alpha Mod tagged as [main]", out)
	}
}

func TestListVersion(t *testing.T) {
	setUpListFixture(t)
	setListFlag(t, "version", "true")

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})
	if !strings.Contains(out, "Alpha Mod (alpha.jar)") {
		t.Errorf("output = %q, want Alpha Mod annotated with its filename", out)
	}
}
