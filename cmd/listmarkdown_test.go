package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestSectionOf(t *testing.T) {
	for file, want := range map[string]string{
		"mods/sodium.pw.toml":                   "Mods",
		"mods/client/sodium.pw.toml":            "Mods",
		"src/mods/sodium.pw.toml":               "Mods",
		"Mods/sodium.pw.toml":                   "Mods",
		"resourcepacks/faithful.pw.toml":        "Resource Packs",
		"shaderpacks/extra/complementary.toml":  "Shader Packs",
		"config/paxi/datapacks/terra.pw.toml":   "Data Packs",
		"plugins/worldedit.pw.toml":             "Plugins",
		"my_extras/thing.pw.toml":               "My_extras",
		"épices/thing.pw.toml":                  "Épices",
		"loose.pw.toml":                         "Other",
		"mods/plugins/a-plugin-as-a-mod.toml":   "Mods",
		"plugins/mods/a-mod-as-a-plugin.toml":   "Mods",
		"unknown/resourcepacks/faithful.toml":   "Resource Packs",
		"unknown/datapacks/deeper/terra.toml":   "Data Packs",
		"unknown/another/deeper/something.toml": "Deeper",
	} {
		if got := sectionOf(file); got != want {
			t.Errorf("sectionOf(%q) = %q, want %q", file, got, want)
		}
	}
}

func TestEscapeMarkdown(t *testing.T) {
	for text, want := range map[string]string{
		"":                             "",
		"Sodium":                       "Sodium",
		"Sodium-Extra":                 "Sodium-Extra",
		"Create: Steam 'n' Rails":      "Create: Steam 'n' Rails",
		"Mod 1.5 (Lite)":               "Mod 1.5 (Lite)",
		"3D Models":                    "3D Models",
		"Salt & Pepper":                "Salt & Pepper",
		"Zeta_Mod":                     `Zeta\_Mod`,
		"*Star*":                       `\*Star\*`,
		"[Fabric] Mod":                 `\[Fabric\] Mod`,
		"<b>bold</b>":                  `\<b\>bold\</b\>`,
		"`code`":                       "\\`code\\`",
		`a\b`:                          `a\\b`,
		"~~strike~~":                   `\~\~strike\~\~`,
		"Fast&amp;Furious":             `Fast\&amp;Furious`,
		"Fast&#38;Furious":             `Fast\&#38;Furious`,
		"# Heading":                    `\# Heading`,
		"- item":                       `\- item`,
		"+ item":                       `\+ item`,
		"> quote":                      `\> quote`,
		"1. First":                     `1\. First`,
		"12) Twelfth":                  `12\) Twelfth`,
		"  many   spaces\nand\tlines ": "many spaces and lines",
	} {
		if got := escapeMarkdown(text); got != want {
			t.Errorf("escapeMarkdown(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestRenderMarkdownList(t *testing.T) {
	pack := core.Pack{Name: "My *Pack*", Description: "Cozy  building\nand farming"}
	entries := []markdownEntry{
		{name: "Sodium", path: "mods/sodium.pw.toml", url: "https://modrinth.com/project/AANobbMI", version: "0.5.8+mc1.21"},
		{name: "iris", path: "mods/iris.pw.toml", url: "https://modrinth.com/project/YL57xq9U"},
		{name: "Lithium", path: "src/mods/sub/lithium.pw.toml"},
		{name: "", path: "mods/nameless.pw.toml"},
		{name: "Faithful", path: "resourcepacks/faithful.pw.toml"},
		{name: "Rethinking Voxels", path: "shaderpacks/rv.pw.toml"},
		{name: "Terralith", path: "config/paxi/datapacks/terralith.pw.toml"},
		{name: "WorldEdit", path: "plugins/worldedit.pw.toml"},
		{name: "Zed", path: "extras/zed.pw.toml"},
		{name: "Loose", path: "loose.pw.toml"},
	}
	want := `# My \*Pack\*

Cozy building and farming

## Mods

- [iris](https://modrinth.com/project/YL57xq9U)
- Lithium
- nameless
- [Sodium](https://modrinth.com/project/AANobbMI) — 0.5.8+mc1.21

## Resource Packs

- Faithful

## Shader Packs

- Rethinking Voxels

## Data Packs

- Terralith

## Extras

- Zed

## Plugins

- WorldEdit

## Other

- Loose
`
	if got := renderMarkdownList(pack, entries); got != want {
		t.Errorf("renderMarkdownList() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderMarkdownListWithNothingInIt(t *testing.T) {
	if got, want := renderMarkdownList(core.Pack{Name: "Test"}, nil), "# Test\n\n_Nothing to list._\n"; got != want {
		t.Errorf("renderMarkdownList() = %q, want %q", got, want)
	}
	if got, want := renderMarkdownList(core.Pack{}, nil), "# Modpack\n\n_Nothing to list._\n"; got != want {
		t.Errorf("renderMarkdownList() of a pack with no name = %q, want %q", got, want)
	}
}

// The order things were read in is a map's, so the file must not depend on it, even for two files with one name
func TestRenderMarkdownListIsTheSameInAnyOrder(t *testing.T) {
	entries := []markdownEntry{
		{name: "Same", path: "mods/b.pw.toml"},
		{name: "Same", path: "mods/a.pw.toml"},
		{name: "Other", path: "mods/c.pw.toml"},
	}
	reversed := []markdownEntry{entries[2], entries[1], entries[0]}

	pack := core.Pack{Name: "Test"}
	if a, b := renderMarkdownList(pack, entries), renderMarkdownList(pack, reversed); a != b {
		t.Errorf("the list depends on the order of what is in it:\n%s\nand\n%s", a, b)
	}
}

// setUpMarkdownFixture is setUpListFixture's pack with a resource pack, a shader pack and a mod in a subfolder added,
// one of them with a version, and a name that needs escaping
func setUpMarkdownFixture(t *testing.T) {
	t.Helper()
	setUpListFixture(t)
	// A mod with update data needs its source to be known to be read
	cmdtest.RegisterVersionSource(t, "modrinth", nil)

	files := map[string]string{
		"resourcepacks/faithful.pw.toml": `name = "Faithful"
filename = "faithful.zip"
version = "9.9.9"

[download]
hash-format = "sha256"
hash = "f"
`,
		"shaderpacks/complementary.pw.toml": `name = "Complementary Shaders"
filename = "complementary.zip"

[download]
hash-format = "sha256"
hash = "c"
`,
		"mods/extras/zeta.pw.toml": `name = "Zeta_Mod [Lite]"
filename = "zeta.jar"
version = "3.2.1"
added-as-dependency = true

[download]
hash-format = "sha256"
hash = "z"

[update.modrinth]
mod-id = "Zz9Zz9Zz"
version = "zv"
`,
	}
	index, err := os.OpenFile("index.toml", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("failed to open index.toml fixture: %v", err)
	}
	defer func() { _ = index.Close() }()
	for file, content := range files {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatalf("failed to create the folder of %s: %v", file, err)
		}
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s fixture: %v", file, err)
		}
		if _, err := index.WriteString("\n[[files]]\nfile = \"" + file + "\"\nhash = \"irrelevant\"\nmetafile = true\n"); err != nil {
			t.Fatalf("failed to add %s to the index fixture: %v", file, err)
		}
	}
}

const markdownFixtureList = `# Test Pack

## Mods

- Alpha Mod — alpha.jar
- Beta Mod — beta.jar
- Gamma Mod — gamma.jar
- [Zeta\_Mod \[Lite\]](https://modrinth.com/project/Zz9Zz9Zz) — 3.2.1

## Resource Packs

- Faithful — 9.9.9

## Shader Packs

- Complementary Shaders — complementary.zip
`

func readMarkdownFile(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("expected %s to have been written: %v", file, err)
	}
	return string(data)
}

func TestListMarkdownWritesTheModListToThePackFolder(t *testing.T) {
	setUpMarkdownFixture(t)
	setListFlag(t, "save", "true")

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})

	if got := readMarkdownFile(t, core.ModListFile); got != markdownFixtureList {
		t.Errorf("%s =\n%s\nwant\n%s", core.ModListFile, got, markdownFixtureList)
	}
	// The list goes in the file, and only what was done is printed
	if want := "Wrote " + core.ModListFile + "\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// What was added as a dependency is left out, however it is recorded, and the dependency is listed with the rest
func TestListMarkdownHasNoDependencyMarks(t *testing.T) {
	setUpMarkdownFixture(t)
	setListFlag(t, "save", "true")
	cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})

	got := readMarkdownFile(t, core.ModListFile)
	for _, unwanted := range []string{"dependency", "[main]", "added"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%s = %q, should not have %q in it", core.ModListFile, got, unwanted)
		}
	}
	if !strings.Contains(got, "- Beta Mod \u2014 beta.jar\n") {
		t.Errorf("%s = %q, want the mod that was added as a dependency listed like the others", core.ModListFile, got)
	}
}

func TestListOutputChoosesTheFileAndImpliesMarkdown(t *testing.T) {
	setUpMarkdownFixture(t)
	setListFlag(t, "output", "mods-list.md")

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})

	if got := readMarkdownFile(t, "mods-list.md"); got != markdownFixtureList {
		t.Errorf("mods-list.md =\n%s\nwant\n%s", got, markdownFixtureList)
	}
	if _, err := os.Stat(core.ModListFile); err == nil {
		t.Errorf("%s was written as well as the file --output named", core.ModListFile)
	}
	if want := "Wrote mods-list.md\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// What is printed rather than written isn't coloured either, however colour is set
func TestListOutputDashPrintsTheMarkdown(t *testing.T) {
	setUpMarkdownFixture(t)
	setListFlag(t, "output", "-")
	cmdtest.SetColor(t, ui.Always)

	out := cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})

	if out != markdownFixtureList {
		t.Errorf("output =\n%q\nwant\n%q", out, markdownFixtureList)
	}
	if _, err := os.Stat(core.ModListFile); err == nil {
		t.Errorf("%s was written when the list was to be printed", core.ModListFile)
	}
}

func TestListMarkdownFileHasNoColour(t *testing.T) {
	setUpMarkdownFixture(t)
	setListFlag(t, "save", "true")

	plain, coloured := cmdtest.AssertColourOnlyAdds(t, func() {
		listCmd.Run(listCmd, nil)
	})

	if want := "Wrote " + core.ModListFile + "\n"; plain != want {
		t.Errorf("output = %q, want %q", plain, want)
	}
	// What was done is green, and the file's name is in it
	if want := ui.Success.Sprint("Wrote "+core.ModListFile) + "\n"; coloured != want {
		t.Errorf("coloured output = %q, want %q", coloured, want)
	}
	if got := readMarkdownFile(t, core.ModListFile); got != markdownFixtureList {
		t.Errorf("%s was written with colour in it: %q", core.ModListFile, got)
	}
}

func TestListMarkdownIsFilteredLikeTheList(t *testing.T) {
	setUpMarkdownFixture(t)
	setListFlag(t, "save", "true")
	setListFlag(t, "only", "main")
	cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})

	// Beta Mod and Zeta_Mod were added as dependencies
	want := `# Test Pack

## Mods

- Alpha Mod — alpha.jar
- Gamma Mod — gamma.jar

## Resource Packs

- Faithful — 9.9.9

## Shader Packs

- Complementary Shaders — complementary.zip
`
	if got := readMarkdownFile(t, core.ModListFile); got != want {
		t.Errorf("%s =\n%s\nwant\n%s", core.ModListFile, got, want)
	}
}

func TestListMarkdownWhenTheFiltersLeaveNothing(t *testing.T) {
	// In the plain fixture the only dependency is on the server, so on the client there are none
	setUpListFixture(t)
	setListFlag(t, "save", "true")
	setListFlag(t, "only", "dependencies")
	setListFlag(t, "side", core.ClientSide)
	cmdtest.CaptureStdout(t, func() {
		listCmd.Run(listCmd, nil)
	})

	want := "# Test Pack\n\n_Nothing to list._\n"
	if got := readMarkdownFile(t, core.ModListFile); got != want {
		t.Errorf("%s = %q, want %q", core.ModListFile, got, want)
	}
}

// --save is for the names alone, so the flags that put more on each line of the plain list aren't allowed with it
func TestListMarkdownCannotBeCombinedWithVersionOrShowKind(t *testing.T) {
	// Each combination is a test of its own, as flags are put back when a test ends and not before
	for _, markdown := range []string{"save", "output"} {
		t.Run("--"+markdown+" alone", func(t *testing.T) {
			if err := setAndValidate(t, markdown); err != nil {
				t.Errorf("--%s on its own is an error: %v", markdown, err)
			}
		})
		for _, plain := range []string{"version", "show-kind"} {
			t.Run("--"+markdown+" with --"+plain, func(t *testing.T) {
				err := setAndValidate(t, markdown, plain)
				if err == nil {
					t.Fatalf("--%s with --%s is allowed", markdown, plain)
				}
				if !strings.Contains(err.Error(), markdown) || !strings.Contains(err.Error(), plain) {
					t.Errorf("error = %q, want it to name both flags", err)
				}
			})
		}
	}
	// The plain list still has them
	t.Run("--version with --show-kind", func(t *testing.T) {
		if err := setAndValidate(t, "version", "show-kind"); err != nil {
			t.Errorf("--version with --show-kind is an error: %v", err)
		}
	})
}

// setAndValidate sets the list flags as given, and returns what cobra makes of the combination
func setAndValidate(t *testing.T, flags ...string) error {
	t.Helper()
	for _, name := range flags {
		value := "true"
		if name == "output" {
			value = "list.md"
		}
		setListFlag(t, name, value)
	}
	return listCmd.ValidateFlagGroups()
}

func TestMarkdownListPath(t *testing.T) {
	cmdtest.SetViper(t, "pack-file", filepath.Join("some", "pack", "pack.toml"))
	if got, want := markdownListPath(), filepath.Join("some", "pack", core.ModListFile); got != want {
		t.Errorf("markdownListPath() = %q, want %q (the pack's folder)", got, want)
	}

	setListFlag(t, "output", filepath.Join("docs", "list.md"))
	if got, want := markdownListPath(), filepath.Join("docs", "list.md"); got != want {
		t.Errorf("markdownListPath() = %q, want %q (where --output says)", got, want)
	}
}

func TestWriteMarkdownListFailsWhenTheFileCannotBeWritten(t *testing.T) {
	setUpMarkdownFixture(t)
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		t.Fatalf("LoadAllMods() returned error: %v", err)
	}

	dest := filepath.Join("missing", core.ModListFile)
	out := cmdtest.CaptureStdout(t, func() {
		err = writeMarkdownList(dest, pack, index, mods)
	})
	if err == nil {
		t.Fatal("writeMarkdownList() returned no error for a folder that doesn't exist")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %q, want it to say where it couldn't write", err)
	}
	if out != "" {
		t.Errorf("output = %q, want nothing said of a file that wasn't written", out)
	}
}
