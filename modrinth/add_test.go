package modrinth

import (
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
)

// mainMod is a project that requires a library, which Modrinth says is only for the server
func mainMod() mrProject {
	return mrProject{
		id: "main1", slug: "main-slug", title: "Main Mod", clientSide: "required", serverSide: "required",
		number: "2.0.0", file: "main.jar", requires: []string{"lib1"},
	}
}

// serveProjectPages answers Modrinth for a project by its ID and by its slug, as the pages of projects are, on top of what
// serveModrinth answers for the versions and the lists of projects.
func serveProjectPages(t *testing.T, projects ...mrProject) {
	t.Helper()
	for _, p := range projects {
		page := map[string]any{
			"id": p.id, "slug": p.slug, "title": p.title, "project_type": "mod",
			"client_side": p.clientSide, "server_side": p.serverSide, "versions": []string{p.versionID()},
		}
		httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/"+p.id, httpmock.NewJsonResponderOrPanic(200, page))
		httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/"+p.slug, httpmock.NewJsonResponderOrPanic(200, page))
	}
}

// addablePack is a pack that can have main-slug added to it, with what Modrinth says of it and its library.
func addablePack(t *testing.T) (core.Pack, core.Index) {
	t.Helper()
	pack, index := validatablePack(t)
	lib := libraryOnModrinth("lib1")
	projects := []mrProject{mainMod(), lib}
	serveModrinth(t, projects, nil)
	serveProjectPages(t, projects...)
	saveFixture(t, pack, index)
	return pack, index
}

func TestPlanAddSaysWhatAddingAProjectWouldDoAndDoesNotDoIt(t *testing.T) {
	pack, index := addablePack(t)

	var plan *AddPlan
	var err error
	out := cmdtest.CaptureStdout(t, func() { plan, err = PlanAdd(pack, index, "main-slug", AddOptions{}) })

	if err != nil {
		t.Fatalf("PlanAdd() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("PlanAdd() wrote %q to the terminal", out)
	}
	if plan.Project != "Main Mod" || plan.Slug != "main-slug" || plan.Version != "2.0.0" || plan.ReleaseType != "release" ||
		plan.File != "main.jar" || plan.Folder != "mods" || plan.Side != core.UniversalSide || plan.Existing != nil {
		t.Errorf("the plan is %+v, want the project, its latest version and where it goes", plan)
	}
	want := []AddDependency{{Name: "Title of lib1", Version: "1.0.0", File: "lib1.jar"}}
	if !slices.Equal(plan.Dependencies, want) {
		t.Errorf("the dependencies are %v, want %v", plan.Dependencies, want)
	}
	if _, err := os.Stat("mods"); !os.IsNotExist(err) {
		t.Errorf("planning made the mods folder (%v), want nothing written", err)
	}
}

func TestPlanAddAcceptsTheAddressOfAProjectsPageAndAVersionOfIt(t *testing.T) {
	pack, index := addablePack(t)
	version := func(id, number, file, date string) map[string]any {
		return map[string]any{
			"id": id, "project_id": "main1", "version_number": number, "version_type": "release", "date_published": date,
			"files": []map[string]any{{"url": "https://cdn.modrinth.com/" + file, "filename": file, "primary": true, "hashes": map[string]string{"sha512": "h"}}},
		}
	}
	older, newer := version("old", "1.0.0", "main-old.jar", "2023-01-01T00:00:00Z"), version("v-main1", "2.0.0", "main.jar", "2024-01-01T00:00:00Z")
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/main1/version`, httpmock.NewJsonResponderOrPanic(200, []map[string]any{older, newer}))
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/version/v-main1", httpmock.NewJsonResponderOrPanic(200, newer))

	for ref, number := range map[string]string{
		"https://modrinth.com/mod/main-slug":                 "2.0.0",
		"https://modrinth.com/mod/main-slug/version/1.0.0":   "1.0.0",
		"https://modrinth.com/mod/main-slug/version/v-main1": "2.0.0",
	} {
		plan, err := PlanAdd(pack, index, ref, AddOptions{})
		if err != nil {
			t.Errorf("PlanAdd(%q) returned error: %v", ref, err)
			continue
		}
		if plan.Project != "Main Mod" || plan.Version != number {
			t.Errorf("PlanAdd(%q) is for %s at %s, want Main Mod at %s", ref, plan.Project, plan.Version, number)
		}
	}
}

func TestPlanAddRefusesWhatIsNotAProject(t *testing.T) {
	pack, index := addablePack(t)
	for _, ref := range []string{"", "a b", "https://example.com/mod/x", "x!"} {
		if _, err := PlanAdd(pack, index, ref, AddOptions{}); err == nil {
			t.Errorf("PlanAdd(%q) returned no error, want one for something that isn't a Modrinth address, slug or ID", ref)
		}
	}
	if _, err := PlanAdd(pack, index, "main-slug", AddOptions{ReleaseType: "nightly"}); err == nil || !strings.Contains(err.Error(), "release type") {
		t.Errorf("PlanAdd() with a release type that isn't one returned %v, want an error that says so", err)
	}
}

func TestPlanAddSaysWhenTheProjectCannotBeFound(t *testing.T) {
	pack, index := addablePack(t)
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/nothing-here", httpmock.NewStringResponder(404, `{"error":"not_found","description":"no"}`))
	if _, err := PlanAdd(pack, index, "nothing-here", AddOptions{}); err == nil {
		t.Error("PlanAdd() returned no error for a project that isn't there")
	}
}

func TestAddPlanApplyAddsTheProjectAndItsDependencies(t *testing.T) {
	pack, index := addablePack(t)
	plan, err := PlanAdd(pack, index, "main-slug", AddOptions{})
	if err != nil {
		t.Fatalf("PlanAdd() returned error: %v", err)
	}

	var result *AddResult
	out := cmdtest.CaptureStdout(t, func() { result, err = plan.Apply(true) })

	if err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("Apply() wrote %q to the terminal", out)
	}
	if result.Project != "Main Mod" || result.File != "main.jar" || result.Updated || !slices.Equal(result.Dependencies, []string{"Title of lib1"}) {
		t.Errorf("the result is %+v, want the project added with its library", result)
	}
	main := loadModAt(t, "mods/main-slug.pw.toml")
	lib := loadModAt(t, "mods/lib1-slug.pw.toml")
	if main.FileName != "main.jar" || main.AddedAsDependency || !lib.AddedAsDependency {
		t.Errorf("the project is %+v and the library %+v, want the project a main mod and the library a dependency", main, lib)
	}
	// The library is only for the server, but the project runs on the client and requires it
	if lib.Side != core.UniversalSide {
		t.Errorf("the library's side is %q, want it put on both as the project needs it on the client", lib.Side)
	}
	if !strings.Contains(strings.Join(result.Notices, "\n"), "is now on both sides") {
		t.Errorf("the notices are %v, want one that says the library was put on both sides", result.Notices)
	}
	assertIndexHasHashOf(t, "mods/main-slug.pw.toml")
	assertIndexHasHashOf(t, "mods/lib1-slug.pw.toml")
}

func TestAddPlanApplyCanLeaveTheDependenciesOut(t *testing.T) {
	pack, index := addablePack(t)
	plan, err := PlanAdd(pack, index, "main-slug", AddOptions{})
	if err != nil {
		t.Fatalf("PlanAdd() returned error: %v", err)
	}
	result, err := plan.Apply(false)
	if err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}
	if len(result.Dependencies) != 0 {
		t.Errorf("the dependencies added are %v, want none", result.Dependencies)
	}
	if _, err := os.Stat("mods/lib1-slug.pw.toml"); !os.IsNotExist(err) {
		t.Errorf("the library was added though it wasn't asked for (%v)", err)
	}
}

// addedOlder puts the project in the pack at an older version, as it would be after being added before, with something of
// the user's in it
func addedOlder(t *testing.T, pinned bool) {
	t.Helper()
	pack, index := addablePack(t)
	plan, err := PlanAdd(pack, index, "main-slug", AddOptions{})
	if err != nil {
		t.Fatalf("PlanAdd() returned error: %v", err)
	}
	if _, err := plan.Apply(false); err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}
	mod := loadModAt(t, "mods/main-slug.pw.toml")
	mod.Update["modrinth"]["version"] = "older"
	mod.Version = "1.0.0"
	mod.Pin = pinned
	mod.ClaimConfigFile("config/main.json")
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	// The index is what the files are, as a refresh makes it
	index, err = mustPack(t).LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	if err := index.RefreshFileWithHash("mods/main-slug.pw.toml", "sha256", "x", true); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}
	if err := index.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
}

func mustPack(t *testing.T) core.Pack {
	t.Helper()
	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	return pack
}

func planAgain(t *testing.T) *AddPlan {
	t.Helper()
	pack := mustPack(t)
	index, err := pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	plan, err := PlanAdd(pack, index, "main-slug", AddOptions{})
	if err != nil {
		t.Fatalf("PlanAdd() returned error: %v", err)
	}
	return plan
}

func TestPlanAddSaysWhenTheProjectIsAlreadyInThePack(t *testing.T) {
	pack, index := addablePack(t)
	plan, _ := PlanAdd(pack, index, "main-slug", AddOptions{})
	if _, err := plan.Apply(false); err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}

	before := httpmock.GetCallCountInfo()["GET =~^https://api\\.modrinth\\.com/v2/projects"]
	again := planAgain(t)
	if again.Existing == nil || !again.Existing.UpToDate || again.Existing.Name != "Main Mod" {
		t.Fatalf("the existing is %+v, want the project up to date", again.Existing)
	}
	if len(again.Dependencies) != 0 {
		t.Errorf("the dependencies are %v, want none looked up for something that is up to date", again.Dependencies)
	}
	if after := httpmock.GetCallCountInfo()["GET =~^https://api\\.modrinth\\.com/v2/projects"]; after != before {
		t.Error("the dependencies were looked up for a project that is up to date")
	}
	if _, err := again.Apply(false); err == nil {
		t.Error("Apply() returned no error for a plan with nothing to do")
	}
}

func TestPlanAddOffersAnUpdateAndApplyKeepsWhatTheUserSetOnTheMod(t *testing.T) {
	addedOlder(t, false)
	plan := planAgain(t)

	if plan.Existing == nil || plan.Existing.UpToDate || plan.Existing.Pinned || plan.Existing.Current != "1.0.0" {
		t.Fatalf("the existing is %+v, want the project at 1.0.0 with an update to 2.0.0", plan.Existing)
	}
	result, err := plan.Apply(false)
	if err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}
	if !result.Updated {
		t.Error("the result says the project was added, want it updated")
	}
	mod := loadModAt(t, "mods/main-slug.pw.toml")
	if mod.Version != "2.0.0" || mod.FileName != "main.jar" {
		t.Errorf("the mod is at %s in %s, want it updated", mod.Version, mod.FileName)
	}
	if entries := mod.ConfigEntries(); len(entries) != 1 || entries[0] != "config/main.json" {
		t.Errorf("the mod's config-files are %v, want what was in them kept", entries)
	}
}

func TestPlanAddSaysWhenTheProjectIsPinnedAndApplyWillNotUpdateIt(t *testing.T) {
	addedOlder(t, true)
	plan := planAgain(t)
	if plan.Existing == nil || !plan.Existing.Pinned {
		t.Fatalf("the existing is %+v, want the project pinned", plan.Existing)
	}
	if _, err := plan.Apply(false); err == nil {
		t.Error("Apply() returned no error for a pinned project")
	}
	if mod := loadModAt(t, "mods/main-slug.pw.toml"); mod.Version != "1.0.0" {
		t.Errorf("the pinned mod is at %s, want it left alone", mod.Version)
	}
}

func TestAddPlanApplyNoticesThatTheProjectWasPinnedSinceItWasPlanned(t *testing.T) {
	addedOlder(t, false)
	plan := planAgain(t)

	mod := loadModAt(t, "mods/main-slug.pw.toml")
	mod.Pin = true
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if _, err := plan.Apply(false); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Errorf("Apply() returned %v, want an error that says the project is pinned", err)
	}
	if got := loadModAt(t, "mods/main-slug.pw.toml"); got.Version != "1.0.0" {
		t.Errorf("the mod is at %s, want it left alone", got.Version)
	}
}

func TestAddPlanApplyNoticesThatTheProjectWasAddedSinceItWasPlanned(t *testing.T) {
	pack, index := addablePack(t)
	first, _ := PlanAdd(pack, index, "main-slug", AddOptions{})
	second, _ := PlanAdd(pack, index, "main-slug", AddOptions{})

	if _, err := first.Apply(false); err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}
	if _, err := second.Apply(false); err == nil || !strings.Contains(err.Error(), "has changed") {
		t.Errorf("Apply() returned %v, want an error that says the pack has changed", err)
	}
}

func TestSearchAsksForProjectsThatSuitThePack(t *testing.T) {
	pack, index := validatablePack(t)
	var facets [][]string
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/search`, func(req *http.Request) (*http.Response, error) {
		if got := req.URL.Query().Get("query"); got != "sodium" {
			t.Errorf("the query is %q, want what was searched for", got)
		}
		_ = json.Unmarshal([]byte(req.URL.Query().Get("facets")), &facets)
		return httpmock.NewStringResponse(200, `{"hits":[
			{"project_id":"AANobbMI","slug":"sodium","title":"Sodium","description":"Fast","author":"jelly","downloads":99,"client_side":"required","server_side":"unsupported"},
			{"slug":"no-id","title":"Broken"}],"total_hits":7}`), nil
	})

	out := cmdtest.CaptureStdout(t, func() {
		results, err := Search(pack, index, "sodium", KindMod)
		if err != nil {
			t.Fatalf("Search() returned error: %v", err)
		}
		if results.Total != 7 || len(results.Found) != 1 {
			t.Fatalf("the results are %+v, want the one complete hit, of 7", results)
		}
		want := Found{ID: "AANobbMI", Slug: "sodium", Title: "Sodium", Description: "Fast", Author: "jelly", Kind: KindMod, Downloads: 99, ClientSide: "required", ServerSide: "unsupported"}
		if results.Found[0] != want {
			t.Errorf("the hit is %+v, want %+v", results.Found[0], want)
		}
	})
	if out != "" {
		t.Errorf("Search() wrote %q to the terminal", out)
	}

	want := [][]string{{"project_type:mod"}, {"versions:1.20.1"}, {"categories:neoforge"}}
	if !slices.EqualFunc(facets, want, slices.Equal) {
		t.Errorf("the facets are %v, want %v: mods for the pack's game version and loader", facets, want)
	}
}

func TestSearchSaysWhichProjectsThePackHasAlready(t *testing.T) {
	pack, index := neoForgePack(t)
	addTestProject(t, pack, &index, "AANobbMI", "sodium", "Sodium")
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/search`, httpmock.NewStringResponder(200, `{"hits":[
		{"project_id":"AANobbMI","slug":"sodium","title":"Sodium"},{"project_id":"other","slug":"other","title":"Other"}],"total_hits":2}`))

	results, err := Search(pack, index, "x", KindMod)
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	if len(results.Found) != 2 || !results.Found[0].InPack || results.Found[1].InPack {
		t.Errorf("the results are %+v, want only Sodium marked as in the pack", results.Found)
	}
}

func TestSearchOfOtherKindsDoesNotAskForALoader(t *testing.T) {
	pack, index := validatablePack(t)
	var facets [][]string
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/search`, func(req *http.Request) (*http.Response, error) {
		_ = json.Unmarshal([]byte(req.URL.Query().Get("facets")), &facets)
		return httpmock.NewStringResponse(200, `{"hits":[],"total_hits":0}`), nil
	})
	if _, err := Search(pack, index, "faithful", KindResourcePack); err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	want := [][]string{{"project_type:resourcepack"}, {"versions:1.20.1"}}
	if !slices.EqualFunc(facets, want, slices.Equal) {
		t.Errorf("the facets are %v, want %v: resource packs don't have a mod loader", facets, want)
	}
}

func TestSearchAlsoAsksForFabricModsWhenThePackRunsThem(t *testing.T) {
	pack, index := connectorPack(t)
	httpmock.Activate(t)
	var facets [][]string
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/search`, func(req *http.Request) (*http.Response, error) {
		_ = json.Unmarshal([]byte(req.URL.Query().Get("facets")), &facets)
		return httpmock.NewStringResponse(200, `{"hits":[],"total_hits":0}`), nil
	})
	if _, err := Search(pack, index, "x", KindMod); err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	if len(facets) != 3 || !slices.Equal(facets[2], []string{"categories:neoforge", "categories:fabric"}) {
		t.Errorf("the facets are %v, want mods for NeoForge or Fabric as the pack has Sinytra Connector", facets)
	}
}

func TestSearchSaysWhenItFails(t *testing.T) {
	pack, index := validatablePack(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/search`, httpmock.NewStringResponder(500, ``))
	if _, err := Search(pack, index, "x", KindMod); err == nil || !strings.Contains(err.Error(), "the search failed") {
		t.Errorf("Search() returned %v, want an error that says the search failed", err)
	}
}
