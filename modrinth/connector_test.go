package modrinth

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/jarcoal/httpmock"
)

// loaderVersion is a version of a project that is for the given loaders
func loaderVersion(projectID, id, number string, loaders ...string) *modrinthApi.Version {
	v := testVersion(id, number)
	v.ProjectID = strPtr(projectID)
	v.Loaders = loaders
	return v
}

// serveLoaderVersions answers Modrinth's version listing of a project as Modrinth does, with only the versions for the
// loaders asked for, and records the loaders each request asked for
func serveLoaderVersions(t *testing.T, projectID string, versions ...*modrinthApi.Version) *[][]string {
	t.Helper()
	httpmock.Activate(t)
	var asked [][]string
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/`+projectID+`/version`, func(req *http.Request) (*http.Response, error) {
		var loaders []string
		if err := json.Unmarshal([]byte(req.URL.Query().Get("loaders")), &loaders); err != nil {
			return httpmock.NewStringResponse(400, "bad loaders"), nil
		}
		asked = append(asked, loaders)
		matching := []*modrinthApi.Version{}
		for _, v := range versions {
			if slices.ContainsFunc(v.Loaders, func(loader string) bool { return slices.Contains(loaders, loader) }) {
				matching = append(matching, v)
			}
		}
		return httpmock.NewJsonResponse(200, matching)
	})
	return &asked
}

// addTestProject adds a project to the pack, as 'mr add' would, at a version whose ID is "v-" and the project's
func addTestProject(t *testing.T, pack core.Pack, index *core.Index, id, slug, title string) {
	t.Helper()
	project := &modrinthApi.Project{
		ID: strPtr(id), Title: strPtr(title), Slug: strPtr(slug),
		ProjectType: strPtr("mod"), ClientSide: strPtr("required"), ServerSide: strPtr("required"),
	}
	version := &modrinthApi.Version{ID: strPtr("v-" + id), ProjectID: strPtr(id), VersionNumber: strPtr("1.0.0")}
	file := &modrinthApi.File{
		URL: strPtr("https://example.com/" + slug + ".jar"), Filename: strPtr(slug + ".jar"), Primary: boolPtr(true),
		Hashes: map[string]string{"sha512": "hash-" + id},
	}
	version.Files = []*modrinthApi.File{file}
	if err := createFileMeta(project, version, file, pack, index, "", false); err != nil {
		t.Fatalf("createFileMeta() returned error: %v", err)
	}
}

// neoForgePack is a NeoForge pack, saved to disk as well as returned
func neoForgePack(t *testing.T) (core.Pack, core.Index) {
	t.Helper()
	pack, index := setupPackFixture(t)
	pack.Versions["neoforge"] = "21.1.0"
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	return pack, index
}

// connectorPack is a NeoForge pack that has Sinytra Connector and Forgified Fabric API, so that it runs Fabric mods
func connectorPack(t *testing.T) (core.Pack, core.Index) {
	t.Helper()
	pack, index := neoForgePack(t)
	addTestProject(t, pack, &index, connectorProjectID, "connector", "Sinytra Connector")
	addTestProject(t, pack, &index, forgifiedFabricAPIProjectID, "forgified-fabric-api", "Forgified Fabric API")
	return pack, index
}

func TestRunsFabricMods(t *testing.T) {
	neoForge := core.Pack{Versions: map[string]string{"minecraft": "1.21.1", "neoforge": "21.1.0"}}
	noLoader := core.Pack{Versions: map[string]string{"minecraft": "1.21.1"}}
	tests := []struct {
		name      string
		pack      core.Pack
		installed []string
		want      bool
	}{
		{"both added", neoForge, []string{connectorProjectID, forgifiedFabricAPIProjectID}, true},
		{"both among other projects", neoForge, []string{"other", forgifiedFabricAPIProjectID, "another", connectorProjectID}, true},
		{"only Connector", neoForge, []string{connectorProjectID}, false},
		{"only Forgified Fabric API", neoForge, []string{forgifiedFabricAPIProjectID}, false},
		{"Fabric API is not Forgified Fabric API", neoForge, []string{connectorProjectID, fabricAPIProjectID}, false},
		{"neither", neoForge, nil, false},
		{"not a NeoForge pack", noLoader, []string{connectorProjectID, forgifiedFabricAPIProjectID}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runsFabricMods(tt.pack, tt.installed); got != tt.want {
				t.Errorf("runsFabricMods() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsConnector(t *testing.T) {
	pack := core.Pack{Versions: map[string]string{"neoforge": "21.1.0"}}
	tests := []struct {
		name    string
		loaders []string
		want    bool
	}{
		{"Fabric", []string{"fabric"}, true},
		{"Fabric and Quilt", []string{"fabric", "quilt"}, true},
		{"NeoForge", []string{"neoforge"}, false},
		{"NeoForge and Fabric, which runs by itself", []string{"fabric", "neoforge"}, false},
		{"a datapack", []string{"datapack"}, false},
		{"no loaders", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsConnector(&modrinthApi.Version{Loaders: tt.loaders}, pack); got != tt.want {
				t.Errorf("needsConnector(%v) = %v, want %v", tt.loaders, got, tt.want)
			}
		})
	}
}

func TestGetLatestVersionFallsBackToFabricWhenThePackRunsFabricMods(t *testing.T) {
	pack, _ := neoForgePack(t)
	asked := serveLoaderVersions(t, "p1", loaderVersion("p1", "v1", "1.0.0", "fabric"))

	var latest *modrinthApi.Version
	var err error
	cmdtest.CaptureStdout(t, func() { latest, err = getLatestVersion("p1", "Test Project", pack, "", true) })

	if err != nil {
		t.Fatalf("getLatestVersion() returned error: %v", err)
	}
	if *latest.ID != "v1" {
		t.Errorf("latest = %s, want the Fabric version, v1", *latest.ID)
	}
	if len(*asked) != 2 || !slices.Contains((*asked)[0], "neoforge") || !reflect.DeepEqual((*asked)[1], []string{"fabric"}) {
		t.Errorf("asked for loaders %v, want NeoForge's first and then only Fabric", *asked)
	}
}

// A NeoForge version is the one to use even when the Fabric one is newer: it needs nothing to run
func TestGetLatestVersionPrefersNeoForgeToFabric(t *testing.T) {
	pack, _ := neoForgePack(t)
	native := loaderVersion("p1", "v1", "1.0.0", "neoforge")
	fabric := loaderVersion("p1", "v2", "2.0.0", "fabric")
	fabric.DatePublished = timePtr(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	asked := serveLoaderVersions(t, "p1", native, fabric)

	var latest *modrinthApi.Version
	var err error
	cmdtest.CaptureStdout(t, func() { latest, err = getLatestVersion("p1", "Test Project", pack, "", true) })

	if err != nil {
		t.Fatalf("getLatestVersion() returned error: %v", err)
	}
	if *latest.ID != "v1" {
		t.Errorf("latest = %s, want the NeoForge version, v1", *latest.ID)
	}
	if len(*asked) != 1 {
		t.Errorf("asked for loaders %v, want one lookup, as Fabric isn't needed", *asked)
	}
}

// Nothing acceptable for NeoForge is as good as nothing at all: a stable Fabric version beats a beta one for NeoForge
// that the pack's release type rules out
func TestGetLatestVersionFallsBackToFabricWhenNeoForgeIsTooUnstable(t *testing.T) {
	pack, _ := neoForgePack(t)
	beta := loaderVersion("p1", "v1", "2.0.0-beta", "neoforge")
	beta.VersionType = strPtr("beta")
	serveLoaderVersions(t, "p1", beta, loaderVersion("p1", "v2", "1.0.0", "fabric"))

	var latest *modrinthApi.Version
	var err error
	cmdtest.CaptureStdout(t, func() { latest, err = getLatestVersion("p1", "Test Project", pack, "", true) })

	if err != nil {
		t.Fatalf("getLatestVersion() returned error: %v", err)
	}
	if *latest.ID != "v2" {
		t.Errorf("latest = %s, want the Fabric release, v2", *latest.ID)
	}
}

func TestGetLatestVersionIgnoresFabricUnlessThePackRunsFabricMods(t *testing.T) {
	pack, _ := neoForgePack(t)
	asked := serveLoaderVersions(t, "p1", loaderVersion("p1", "v1", "1.0.0", "fabric"))

	_, err := getLatestVersion("p1", "Test Project", pack, "", false)

	if err == nil || !strings.Contains(err.Error(), "no valid versions found") {
		t.Errorf("error = %v, want one saying there are no valid versions", err)
	}
	if len(*asked) != 1 {
		t.Errorf("asked for loaders %v, want one lookup, for NeoForge", *asked)
	}
}

// Having nothing for Fabric changes nothing about what is wrong, so the error is the one it always was
func TestGetLatestVersionKeepsTheErrorWhenFabricHasNothingEither(t *testing.T) {
	pack, _ := neoForgePack(t)
	asked := serveLoaderVersions(t, "p1")

	_, err := getLatestVersion("p1", "Test Project", pack, "", true)

	if err == nil || !strings.Contains(err.Error(), "no valid versions found") || !strings.Contains(err.Error(), "acceptable-versions") {
		t.Errorf("error = %v, want the one it gives without Fabric, with its advice", err)
	}
	if len(*asked) != 2 {
		t.Errorf("asked for loaders %v, want NeoForge and then Fabric", *asked)
	}
}

// A failed lookup for Fabric is not "nothing found", which would send the user off to change their game versions
func TestGetLatestVersionReportsFabricLookupFailing(t *testing.T) {
	pack, _ := neoForgePack(t)
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/p1/version`, func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Query().Get("loaders"), "fabric") {
			return httpmock.NewStringResponse(500, `{"error":"boom"}`), nil
		}
		return httpmock.NewJsonResponse(200, []*modrinthApi.Version{})
	})

	_, err := getLatestVersion("p1", "Test Project", pack, "", true)

	if err == nil || !strings.Contains(err.Error(), "failed to fetch latest version") || strings.Contains(err.Error(), "no valid versions") {
		t.Errorf("error = %v, want the lookup failing to be reported as that", err)
	}
}

// Forgified Fabric API takes the place of Fabric API, and having both is a crash
func TestGetLatestVersionDoesNotTakeFabricAPI(t *testing.T) {
	pack, _ := neoForgePack(t)
	asked := serveLoaderVersions(t, fabricAPIProjectID, loaderVersion(fabricAPIProjectID, "v1", "0.100.0", "fabric"))

	_, err := getLatestVersion(fabricAPIProjectID, "Fabric API", pack, "", true)

	if err == nil || !strings.Contains(err.Error(), "Forgified Fabric API") {
		t.Errorf("error = %v, want one saying Forgified Fabric API takes its place", err)
	}
	if len(*asked) != 1 {
		t.Errorf("asked for loaders %v, want Fabric not looked up", *asked)
	}
}

func TestInstallProjectAddsFabricModWithConnector(t *testing.T) {
	pack, index := connectorPack(t)
	serveLoaderVersions(t, "p1", loaderVersion("p1", "v1", "1.0.0", "fabric"))
	project, _, _ := testProjectAndFile()

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installProject(project, "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installProject() returned error: %v", err)
	}
	if want := "Notice: Test Project is a Fabric mod; it runs on NeoForge through Sinytra Connector"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
	// It goes with the mods, as a Fabric mod is loaded by NeoForge the same way
	if mod := loadTestMod(t); mod.Version != "1.0.0" {
		t.Errorf("Version = %q, want the Fabric version added", mod.Version)
	}
}

func TestInstallProjectPrintsNoNoticeForANeoForgeMod(t *testing.T) {
	pack, index := connectorPack(t)
	serveLoaderVersions(t, "p1", loaderVersion("p1", "v1", "1.0.0", "neoforge"))
	project, _, _ := testProjectAndFile()

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installProject(project, "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installProject() returned error: %v", err)
	}
	if strings.Contains(out, "Notice") {
		t.Errorf("output = %q, want no notice for a mod that runs by itself", out)
	}
}

// Both are needed: Connector runs the mod, and Forgified Fabric API is what it expects to find
func TestInstallProjectCannotAddFabricModWithoutBothConnectorAndForgifiedFabricAPI(t *testing.T) {
	tests := []struct {
		name     string
		projects map[string]string // project ID to slug
	}{
		{"neither", nil},
		{"only Connector", map[string]string{connectorProjectID: "connector"}},
		{"only Forgified Fabric API", map[string]string{forgifiedFabricAPIProjectID: "forgified-fabric-api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pack, index := neoForgePack(t)
			for id, slug := range tt.projects {
				addTestProject(t, pack, &index, id, slug, slug)
			}
			serveLoaderVersions(t, "p1", loaderVersion("p1", "v1", "1.0.0", "fabric"))
			project, _, _ := testProjectAndFile()

			var err error
			cmdtest.CaptureStdout(t, func() { err = installProject(project, "", pack, &index, "") })

			if err == nil || !strings.Contains(err.Error(), "no valid versions found") {
				t.Errorf("error = %v, want one saying there are no valid versions", err)
			}
			if _, statErr := os.Stat(testMetaPath); !os.IsNotExist(statErr) {
				t.Errorf("stat of the mod's metadata = %v, want it not to exist", statErr)
			}
		})
	}
}

// serveDependencyProject answers Modrinth's lookup of the projects a mod depends on, with the one project, dep1
func serveDependencyProject(t *testing.T, slug, title string) {
	t.Helper()
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewJsonResponderOrPanic(200, []map[string]string{
			{"id": "dep1", "slug": slug, "title": title, "project_type": "mod", "client_side": "required", "server_side": "required"},
		}))
}

// requiring makes a version depend on the project
func requiring(v *modrinthApi.Version, projectID string) *modrinthApi.Version {
	v.Dependencies = append(v.Dependencies, &modrinthApi.Dependency{ProjectID: strPtr(projectID), DependencyType: strPtr("required")})
	return v
}

// failOnRequests makes any request to Modrinth fail the test. (Counting the calls httpmock has seen wouldn't do: it only
// counts those that a responder answered.)
func failOnRequests(t *testing.T) {
	t.Helper()
	httpmock.Activate(t)
	httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		t.Errorf("unexpected request: %s", req.URL)
		return httpmock.NewStringResponse(500, ""), nil
	})
}

// This is what a Fabric mod asks for, and the reason for all this: Fabric API has no version for NeoForge to find, and
// its stand-in, Forgified Fabric API, is already in the pack
func TestInstallVersionDoesNotAddFabricAPIWhenForgifiedFabricAPIIsThere(t *testing.T) {
	pack, index := connectorPack(t)
	failOnRequests(t) // Fabric API mustn't even be looked up
	version := requiring(loaderVersion("p1", "v1", "1.0.0", "fabric"), fabricAPIProjectID)
	project, _, _ := testProjectAndFile()

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installVersion(project, version, "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
	if !strings.Contains(out, "All dependencies are already added!") || strings.Contains(out, "Failed") {
		t.Errorf("output = %q, want Fabric API to count as already added", out)
	}
	if _, statErr := os.Stat(filepath.Join("mods", "fabric-api"+core.MetaExtension)); !os.IsNotExist(statErr) {
		t.Errorf("stat of Fabric API's metadata = %v, want it not to exist", statErr)
	}
}

// Fabric API is only stood in for when there is something to stand in for it, and otherwise the user is told as before
func TestInstallVersionCannotFindFabricAPIWithoutForgifiedFabricAPI(t *testing.T) {
	pack, index := neoForgePack(t)
	addTestProject(t, pack, &index, connectorProjectID, "connector", "Sinytra Connector")
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewJsonResponderOrPanic(200, []map[string]string{
			{"id": fabricAPIProjectID, "slug": "fabric-api", "title": "Fabric API", "project_type": "mod", "client_side": "required", "server_side": "required"},
		}))
	serveLoaderVersions(t, fabricAPIProjectID, loaderVersion(fabricAPIProjectID, "fv1", "0.100.0", "fabric"))
	version := requiring(loaderVersion("p1", "v1", "1.0.0", "neoforge"), fabricAPIProjectID)
	project, _, _ := testProjectAndFile()

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installVersion(project, version, "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
	if want := "Failed to get latest version of dependency Fabric API: no valid versions found"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
}

func TestInstallVersionAddsFabricDependencyWithNotice(t *testing.T) {
	pack, index := connectorPack(t)
	serveDependencyProject(t, "dep-mod", "Dep Mod")
	serveLoaderVersions(t, "dep1", loaderVersion("dep1", "dv1", "1.0.0", "fabric"))
	version := requiring(loaderVersion("p1", "v1", "1.0.0", "neoforge"), "dep1")
	project, _, _ := testProjectAndFile()
	cmdtest.SetStdin(t, "y\n")

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = installVersion(project, version, "", pack, &index, "") })

	if err != nil {
		t.Fatalf("installVersion() returned error: %v", err)
	}
	if want := "Notice: Dep Mod is a Fabric mod; it runs on NeoForge through Sinytra Connector"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
	dep, err := core.LoadMod(depMetaPath)
	if err != nil {
		t.Fatalf("expected the dependency to be added: %v", err)
	}
	if !dep.AddedAsDependency {
		t.Error("AddedAsDependency = false, want the dependency marked as one")
	}
}

// A Fabric mod that is updated is one the pack has already been told about, and repeating it for every one of them
// each time updates are checked would bury what is being said about the updates
func TestCheckUpdateOfAFabricModWithConnector(t *testing.T) {
	pack, index := connectorPack(t)
	v1 := loaderVersion("p1", "v1", "1.0.0", "fabric")
	v2 := loaderVersion("p1", "v2", "2.0.0", "fabric")
	v2.DatePublished = timePtr(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	serveLoaderVersions(t, "p1", v1, v2)
	cmdtest.CaptureStdout(t, func() { addTestVersion(t, pack, &index, v1, "") })
	mod := loadTestMod(t)

	// Only the mod being updated is given, as when updating one mod: Connector isn't among them, but the pack has it
	var checks []core.UpdateCheck
	var err error
	out := cmdtest.CaptureStdout(t, func() { checks, err = mrUpdater{}.CheckUpdate([]*core.Mod{&mod}, pack) })

	if err != nil {
		t.Fatalf("CheckUpdate() returned error: %v", err)
	}
	if len(checks) != 1 || checks[0].Error != nil || !checks[0].UpdateAvailable {
		t.Fatalf("CheckUpdate() = %+v, want an update available", checks)
	}
	if want := "test-1.0.0.jar -> test-2.0.0.jar"; checks[0].UpdateString != want {
		t.Errorf("UpdateString = %q, want %q", checks[0].UpdateString, want)
	}
	if strings.Contains(out, "Notice") {
		t.Errorf("output = %q, want no notice when checking for updates", out)
	}
}

func TestCheckUpdateOfAFabricModWithoutConnector(t *testing.T) {
	pack, index := neoForgePack(t)
	v1 := loaderVersion("p1", "v1", "1.0.0", "fabric")
	serveLoaderVersions(t, "p1", v1)
	// Added as it might have been by hand, or before Connector was taken out of the pack
	cmdtest.CaptureStdout(t, func() { addTestVersion(t, pack, &index, v1, "") })
	mod := loadTestMod(t)

	checks, err := mrUpdater{}.CheckUpdate([]*core.Mod{&mod}, pack)

	if err != nil {
		t.Fatalf("CheckUpdate() returned error: %v", err)
	}
	if len(checks) != 1 || checks[0].Error == nil || !strings.Contains(checks[0].Error.Error(), "no valid versions found") {
		t.Errorf("CheckUpdate() = %+v, want an error saying there are no valid versions", checks)
	}
}

// runDeps runs 'mr deps', declining to save the dependency data it fetches for the mods that hadn't any, and returns
// what it printed
func runDeps(t *testing.T) string {
	t.Helper()
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/version/v-`, httpmock.NewStringResponder(200, `{"id":"v","dependencies":[]}`))
	cmdtest.SetStdin(t, "n\n")
	return cmdtest.CaptureStdout(t, func() { depsCmd.Run(depsCmd, nil) })
}

// addFabricModNeedingFabricAPI adds a mod that has Fabric API among its dependencies, saved in its metadata
func addFabricModNeedingFabricAPI(t *testing.T, pack core.Pack, index *core.Index) {
	t.Helper()
	project, _, file := testProjectAndFile()
	version := requiring(loaderVersion("p1", "v1", "1.0.0", "fabric"), fabricAPIProjectID)
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

func TestDepsCountsFabricAPIAsAddedWhenForgifiedFabricAPIIsThere(t *testing.T) {
	pack, index := connectorPack(t)
	addFabricModNeedingFabricAPI(t, pack, &index)

	out := runDeps(t)

	if want := "[required] Forgified Fabric API (already in pack)"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
	if !strings.Contains(out, "All required dependencies are already added to the pack!") {
		t.Errorf("output = %q, want nothing reported as missing", out)
	}
}

func TestDepsReportsFabricAPIMissingWithoutForgifiedFabricAPI(t *testing.T) {
	pack, index := neoForgePack(t)
	addTestProject(t, pack, &index, connectorProjectID, "connector", "Sinytra Connector")
	addFabricModNeedingFabricAPI(t, pack, &index)
	httpmock.Activate(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`,
		httpmock.NewJsonResponderOrPanic(200, []map[string]string{{"id": fabricAPIProjectID, "title": "Fabric API"}}))

	out := runDeps(t)

	if want := "[required] Fabric API (missing)"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
	if !strings.Contains(out, "1 required dependencies are missing") {
		t.Errorf("output = %q, want the missing dependency counted", out)
	}
}
