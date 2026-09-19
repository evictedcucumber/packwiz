package modrinth

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/jarcoal/httpmock"
)

func TestStyleDependencyType(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	tests := map[string]string{
		"required":     "[required]",
		"optional":     ui.Muted.Sprint("[optional]"),
		"embedded":     ui.Muted.Sprint("[embedded]"),
		"incompatible": ui.Error.Sprint("[incompatible]"),
	}
	for depType, want := range tests {
		t.Run(depType, func(t *testing.T) {
			got := styleDependencyType(depType)
			if got != want {
				t.Errorf("styleDependencyType(%q) = %q, want %q", depType, got, want)
			}
			if ui.Strip(got) != "["+depType+"]" {
				t.Errorf("styleDependencyType(%q) changed its text to %q", depType, ui.Strip(got))
			}
		})
	}
}

func TestInstallVersionShowsWhatAnUpdateChangesInColour(t *testing.T) {
	pack, index := setupPackFixture(t)
	cmdtest.SetColor(t, ui.Never)
	cmdtest.CaptureStdout(t, func() { addTestVersion(t, pack, &index, testVersion("v1", "1.0.0"), "") })
	cmdtest.SetStdin(t, "y\n")
	cmdtest.SetColor(t, ui.Always)

	out, err := addAgain(t, pack, &index, testVersion("v2", "2.0.0"), "")
	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	for name, want := range map[string]string{
		"what the update is":       ui.Info.Sprintf("\"%s\" is already added. Update available: %s\n", ui.Bold.Sprint("Test Project"), ui.Transition("1.0.0", "2.0.0")),
		"the question":             ui.Prompt("Would you like to update it? [Y/n]: "),
		"the mod that was updated": ui.Success.Sprintf("Project \"%s\" successfully updated! %s\n", ui.Bold.Sprint("Test Project"), ui.Muted.Sprint("(test-2.0.0.jar)")),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%s: output missing %q:\n%q", name, want, out)
		}
	}
	// Only colour is added, so without it the output says what it always has
	if want := "Update available: 1.0.0 -> 2.0.0"; !strings.Contains(ui.Strip(out), want) {
		t.Errorf("output without colour missing %q:\n%q", want, ui.Strip(out))
	}
}

func TestInstallVersionAlreadyUpToDateIsGreen(t *testing.T) {
	pack, index := setupPackFixture(t)
	v1 := testVersion("v1", "1.0.0")
	cmdtest.SetColor(t, ui.Never)
	cmdtest.CaptureStdout(t, func() { addTestVersion(t, pack, &index, v1, "") })
	cmdtest.SetColor(t, ui.Always)

	out, err := addAgain(t, pack, &index, v1, "")
	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}

	want := ui.Success.Sprintf("\"%s\" is already added and up to date! %s\n", ui.Bold.Sprint("Test Project"), ui.Muted.Sprint("(test-1.0.0.jar)"))
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// addModWithDependencies adds a mod that depends on the projects, with the types of dependency given, saved in its
// metadata. None of them are added to the pack.
func addModWithDependencies(t *testing.T, pack core.Pack, index *core.Index, deps map[string]string) {
	t.Helper()
	project, _, file := testProjectAndFile()
	version := loaderVersion("p1", "v1", "1.0.0", "neoforge")
	for id, depType := range deps {
		version.Dependencies = append(version.Dependencies, &modrinthApi.Dependency{ProjectID: strPtr(id), DependencyType: strPtr(depType)})
	}
	if err := createFileMeta(project, version, file, pack, index, "", false); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}
	if err := index.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if err := pack.UpdateIndexHash(); err != nil {
		t.Fatalf("UpdateIndexHash() returned error: %v", err)
	}
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
}

// Only a required dependency is a problem when it is missing, which colour shows: a missing one that is needed is red,
// and one that isn't is only a warning
func TestDepsShowsHowMuchWhatIsMissingMatters(t *testing.T) {
	pack, index := neoForgePack(t)
	addModWithDependencies(t, pack, &index, map[string]string{"req1": "required", "opt1": "optional", "bad1": "incompatible"})
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewJsonResponderOrPanic(200, []map[string]string{
			{"id": "req1", "title": "Req Mod"}, {"id": "opt1", "title": "Opt Mod"}, {"id": "bad1", "title": "Bad Mod"},
		}))

	// The dependencies are saved in the mod, so there is nothing to fetch for it and nothing to ask about
	_, coloured := cmdtest.AssertColourOnlyAdds(t, func() { depsCmd.Run(depsCmd, nil) })

	for name, want := range map[string]string{
		"the mod":                         ui.Bold.Sprint("Test Project:"),
		"a required dependency":           "  [required] Req Mod (" + ui.Error.Sprint("missing") + ")",
		"an optional dependency":          "  " + ui.Muted.Sprint("[optional]") + " Opt Mod (" + ui.Warning.Sprint("missing") + ")",
		"an incompatible mod":             "  " + ui.Error.Sprint("[incompatible]") + " Bad Mod (" + ui.Warning.Sprint("missing") + ")",
		"the mods that are missing count": ui.Warning.Sprint("1 required dependencies are missing from the pack. Use 'packwiz mr add' to install them."),
	} {
		if !strings.Contains(coloured, want) {
			t.Errorf("%s: report missing %q:\n%q", name, want, coloured)
		}
	}
	if strings.Contains(ui.Strip(coloured), "2 required") {
		t.Errorf("an optional or incompatible mod was counted as a required one:\n%s", ui.Strip(coloured))
	}
}

func TestDepsSaysWhenEverythingIsThereInGreen(t *testing.T) {
	pack, index := connectorPack(t)
	addFabricModNeedingFabricAPI(t, pack, &index)
	cmdtest.SetColor(t, ui.Always)

	out := runDeps(t)

	for name, want := range map[string]string{
		"what is there": "  [required] Forgified Fabric API (" + ui.Success.Sprint("already in pack") + ")",
		"the summary":   ui.Success.Sprint("All required dependencies are already added to the pack!"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%s: report missing %q:\n%q", name, want, out)
		}
	}
}
