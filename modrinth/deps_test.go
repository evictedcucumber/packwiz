package modrinth

import (
	"errors"
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
)

func TestDependenciesReportsWhatTheModsNeedWithoutSayingAnything(t *testing.T) {
	pack, index := neoForgePack(t)
	addModWithDependencies(t, pack, &index, map[string]string{"req1": "required", "opt1": "optional"})
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewJsonResponderOrPanic(200, []map[string]string{{"id": "req1", "title": "Req Mod"}, {"id": "opt1", "title": "Opt Mod"}}))

	var report *DependencyReport
	var err error
	out := cmdtest.CaptureStdout(t, func() { report, err = Dependencies(pack, index, false) })

	if err != nil {
		t.Fatalf("Dependencies() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("Dependencies() wrote %q to the terminal", out)
	}
	if report.Mods != 1 || len(report.Groups) != 1 || report.Groups[0].Mod != "Test Project" {
		t.Fatalf("the report is %+v, want the one mod that has dependencies", report)
	}
	got := map[string]Dependency{}
	for _, d := range report.Groups[0].Dependencies {
		got[d.Name] = d
	}
	if want := (Dependency{Name: "Req Mod", Kind: "required"}); got["Req Mod"] != want {
		t.Errorf("the required dependency is %+v, want %+v", got["Req Mod"], want)
	}
	if want := (Dependency{Name: "Opt Mod", Kind: "optional"}); got["Opt Mod"] != want {
		t.Errorf("the optional dependency is %+v, want %+v", got["Opt Mod"], want)
	}
	if report.MissingRequired != 1 {
		t.Errorf("MissingRequired = %d, want only the required dependency counted", report.MissingRequired)
	}
	if report.Fetched != 0 {
		t.Errorf("Fetched = %d, want nothing fetched for a mod that records its dependencies", report.Fetched)
	}
}

func TestDependenciesHasNothingToSayForAPackWithNoModrinthMods(t *testing.T) {
	pack, index := neoForgePack(t)
	failOnRequests(t)
	report, err := Dependencies(pack, index, false)
	if err != nil {
		t.Fatalf("Dependencies() returned error: %v", err)
	}
	if report.Mods != 0 || len(report.Groups) != 0 {
		t.Errorf("the report is %+v, want nothing in it", report)
	}
}

func TestDependenciesFetchesWhatAModRecordsNoneOfAndSavesItWhenAsked(t *testing.T) {
	pack, index := neoForgePack(t)
	addModWithDependencies(t, pack, &index, nil)
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/v1`,
		httpmock.NewStringResponder(200, `{"id":"v1","dependencies":[{"project_id":"lib1","dependency_type":"required"}]}`))
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewJsonResponderOrPanic(200, []map[string]string{{"id": "lib1", "title": "Lib"}}))

	report, err := Dependencies(pack, index, false)
	if err != nil {
		t.Fatalf("Dependencies() returned error: %v", err)
	}
	if report.Fetched != 1 || report.MissingRequired != 1 {
		t.Fatalf("the report is %+v, want one mod fetched and its dependency missing", report)
	}
	if mod := loadTestMod(t); len(mod.Dependencies) != 0 {
		t.Fatalf("the mod has dependencies %v before they were saved, want it left alone", mod.Dependencies)
	}

	failures, err := report.Save()
	if err != nil || len(failures) != 0 {
		t.Fatalf("Save() = %v, %v, want no problems", failures, err)
	}
	if report.Fetched != 0 {
		t.Errorf("Fetched = %d after saving, want none left to save", report.Fetched)
	}
	mod := loadTestMod(t)
	if len(mod.Dependencies) != 1 || mod.Dependencies[0].ID != "lib1" || mod.Dependencies[0].Type != "required" {
		t.Errorf("the mod's dependencies are %v, want the one that was fetched", mod.Dependencies)
	}
	assertIndexHasHashOf(t, "mods/test-project.pw.toml")
}

func TestDependenciesSaveKeepsWhatWasChangedInThePackSinceTheyWereFetched(t *testing.T) {
	pack, index := neoForgePack(t)
	addModWithDependencies(t, pack, &index, nil)
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/v1`,
		httpmock.NewStringResponder(200, `{"id":"v1","dependencies":[{"project_id":"lib1","dependency_type":"required"}]}`))
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`, httpmock.NewJsonResponderOrPanic(200, []map[string]string{}))

	report, err := Dependencies(pack, index, false)
	if err != nil {
		t.Fatalf("Dependencies() returned error: %v", err)
	}
	// Something else pins the mod in the meantime
	changed := loadTestMod(t)
	changed.Pin = true
	if _, _, err := changed.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	if _, err := report.Save(); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}
	mod := loadTestMod(t)
	if !mod.Pin || len(mod.Dependencies) != 1 {
		t.Errorf("the mod is pinned %v with dependencies %v, want the pin kept and the dependencies saved", mod.Pin, mod.Dependencies)
	}
}

func TestDependenciesSaysWhichModsCouldNotBeLookedUp(t *testing.T) {
	pack, index := neoForgePack(t)
	addModWithDependencies(t, pack, &index, nil)
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/v1`, httpmock.NewErrorResponder(errors.New("no network")))

	report, err := Dependencies(pack, index, false)
	if err != nil {
		t.Fatalf("Dependencies() returned error: %v", err)
	}
	if len(report.FetchFailed) != 1 || report.FetchFailed[0] != "Test Project" {
		t.Errorf("FetchFailed = %v, want the mod that couldn't be looked up", report.FetchFailed)
	}
	if report.Fetched != 0 {
		t.Errorf("Fetched = %d, want none, as it failed", report.Fetched)
	}
}

func TestDependenciesSaveDoesNothingWhenNothingWasFetched(t *testing.T) {
	pack, index := neoForgePack(t)
	addModWithDependencies(t, pack, &index, map[string]string{"req1": "required"})
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`, httpmock.NewJsonResponderOrPanic(200, []map[string]string{}))

	report, err := Dependencies(pack, index, false)
	if err != nil {
		t.Fatalf("Dependencies() returned error: %v", err)
	}
	before := readFile(t, "mods/test-project.pw.toml")
	if failures, err := report.Save(); err != nil || len(failures) != 0 {
		t.Fatalf("Save() = %v, %v, want no problems", failures, err)
	}
	if readFile(t, "mods/test-project.pw.toml") != before {
		t.Error("Save() changed the mod though nothing was fetched")
	}
}
