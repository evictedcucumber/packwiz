package modrinth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/jarcoal/httpmock"
)

// mrProject is a project on the test's Modrinth, with the one version it has
type mrProject struct {
	id, slug, title, clientSide, serverSide string
	// number and file are the version's number and the name of its file
	number, file string
	// requires are the projects its version requires
	requires []string
	// noVersions leaves it without any version for the pack
	noVersions bool
}

func (p mrProject) versionID() string { return "v-" + p.id }

func (p mrProject) dependencies() []map[string]string {
	deps := []map[string]string{}
	for _, id := range p.requires {
		deps = append(deps, map[string]string{"project_id": id, "dependency_type": "required"})
	}
	return deps
}

// serveModrinth answers Modrinth for projects, each of which has one version, and for the versions the pack has
// already (installed, by version ID to number), which it is asked for in bulk. The pack is one from validatablePack,
// which turns httpmock on.
func serveModrinth(t *testing.T, projects []mrProject, installed map[string]string) {
	t.Helper()
	byID := make(map[string]mrProject, len(projects))
	for _, p := range projects {
		byID[p.id] = p
	}

	askedFor := func(req *http.Request) []string {
		var ids []string
		_ = json.Unmarshal([]byte(req.URL.Query().Get("ids")), &ids)
		return ids
	}

	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`, func(req *http.Request) (*http.Response, error) {
		found := []map[string]string{}
		for _, id := range askedFor(req) {
			if p, ok := byID[id]; ok {
				found = append(found, map[string]string{
					"id": p.id, "slug": p.slug, "title": p.title, "project_type": "mod",
					"client_side": p.clientSide, "server_side": p.serverSide,
				})
			}
		}
		return httpmock.NewJsonResponse(200, found)
	})

	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/versions`, func(req *http.Request) (*http.Response, error) {
		found := []map[string]any{}
		for _, id := range askedFor(req) {
			for _, p := range projects {
				if id == p.versionID() {
					found = append(found, map[string]any{"id": id, "project_id": p.id, "version_number": p.number, "dependencies": p.dependencies()})
				}
			}
			if number, ok := installed[id]; ok {
				found = append(found, map[string]any{"id": id, "version_number": number})
			}
		}
		return httpmock.NewJsonResponse(200, found)
	})

	for _, p := range projects {
		versions := []map[string]any{}
		if !p.noVersions {
			versions = append(versions, map[string]any{
				"id": p.versionID(), "project_id": p.id, "version_number": p.number, "version_type": "release",
				"date_published": "2024-01-01T00:00:00Z", "dependencies": p.dependencies(),
				"files": []map[string]any{{"url": "https://cdn.modrinth.com/data/" + p.id + "/" + p.file, "filename": p.file, "primary": true, "hashes": map[string]string{"sha512": "hash-" + p.id}}},
			})
		}
		httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/`+p.id+`/version`, httpmock.NewJsonResponderOrPanic(200, versions))
	}
}

// libraryOnModrinth is a project like Repurposed Structures: Modrinth says it isn't for the client
func libraryOnModrinth(id string, requires ...string) mrProject {
	return mrProject{
		id: id, slug: id + "-slug", title: "Title of " + id, clientSide: "unsupported", serverSide: "required",
		number: "1.0.0", file: id + ".jar", requires: requires,
	}
}

func assertIndexHasHashOf(t *testing.T, path string) {
	t.Helper()
	sum := sha256.Sum256([]byte(readFile(t, path)))
	if index := readFile(t, "index.toml"); !strings.Contains(index, hex.EncodeToString(sum[:])) {
		t.Errorf("index.toml doesn't have the hash of %s as it is now:\n%s", path, index)
	}
}

func loadModAt(t *testing.T, path string) core.Mod {
	t.Helper()
	mod, err := core.LoadMod(path)
	if err != nil {
		t.Fatalf("LoadMod(%s) returned error: %v", path, err)
	}
	return mod
}

// fixPackWithAServerModAModNeeds is a pack where alpha requires lib, which is only on the server
func fixPackWithAServerModAModNeeds(t *testing.T) (pack core.Pack, index core.Index, libPath, alphaPath string) {
	t.Helper()
	pack, index = validatablePack(t)
	lib := validMod("lib")
	lib.side = core.ServerSide
	lib.configFiles = []string{"config/lib.json"}
	libPath = addMod(t, &index, lib)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: projectOf("lib"), Type: "required"}}
	alpha.configFiles = []string{"config/alpha.json"}
	alphaPath = addMod(t, &index, alpha)
	return pack, index, libPath, alphaPath
}

func fix(t *testing.T, pack core.Pack, index core.Index) (*validation, string, error) {
	t.Helper()
	var after *validation
	var err error
	out := cmdtest.CaptureStdout(t, func() { after, err = runFix(pack, index) })
	return after, out, err
}

func TestFixPutsAServerModOnBothSidesAfterShowingAndAsking(t *testing.T) {
	pack, index, libPath, alphaPath := fixPackWithAServerModAModNeeds(t)
	alphaBefore := readFile(t, alphaPath)
	cmdtest.SetStdin(t, "y\n")

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	want := `Checking the pack...
lib (mods/lib.pw.toml):
  error: is only on the server, but "alpha" needs it on the client; its side should be "both"

Found 1 error and 0 warnings in 2 mods.
Changes to make:
lib (mods/lib.pw.toml):
  side: server -> both, as "alpha" needs it on the client

Would you like to make these changes? [Y/n]: Changed 1 file.

Checking the pack again...
All 2 mods are valid!
`
	if out != want {
		t.Errorf("output =\n%s\nwant\n%s", out, want)
	}
	if got := loadModAt(t, libPath).Side; got != core.UniversalSide {
		t.Errorf("library side = %q, want %q", got, core.UniversalSide)
	}
	if readFile(t, alphaPath) != alphaBefore {
		t.Error("alpha changed, want only the library to")
	}
	assertIndexHasHashOf(t, libPath)
	if after.errors() != 0 || len(after.subjects) != 0 {
		t.Errorf("problems after = %v, want none", after.subjects)
	}
}

func TestFixDeclinedChangesNothing(t *testing.T) {
	pack, index, libPath, _ := fixPackWithAServerModAModNeeds(t)
	before := readFile(t, libPath)
	cmdtest.SetStdin(t, "n\n")

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if !strings.HasSuffix(out, "Would you like to make these changes? [Y/n]: Cancelled!\n") {
		t.Errorf("output doesn't end with the question and Cancelled!:\n%s", out)
	}
	if readFile(t, libPath) != before {
		t.Error("the library changed, want it left as it was")
	}
	if after.errors() != 1 {
		t.Errorf("errors = %d, want the one there was, as nothing was fixed", after.errors())
	}
}

// --yes makes the changes without asking, and they are shown all the same
func TestFixWithYesMakesTheChangesWithoutAsking(t *testing.T) {
	pack, index, libPath, _ := fixPackWithAServerModAModNeeds(t)
	cmdtest.SetViperBool(t, "non-interactive", true)

	_, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	for _, want := range []string{"Changes to make:\n", "  side: server -> both, as \"alpha\" needs it on the client\n", "[Y/n]: Y (non-interactive mode)\n", "Changed 1 file.\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := loadModAt(t, libPath).Side; got != core.UniversalSide {
		t.Errorf("library side = %q, want %q", got, core.UniversalSide)
	}
}

func TestFixHasNothingToDoForAPackThatIsValid(t *testing.T) {
	pack, index := validatablePack(t)
	alpha := validMod("alpha")
	alpha.configFiles = []string{"config/alpha.json"}
	addMod(t, &index, alpha)
	beta := validMod("beta")
	beta.configFiles = []string{"config/beta.json"}
	addMod(t, &index, beta)

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if want := "Checking the pack...\nAll 2 mods are valid!\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if after.errors() != 0 {
		t.Errorf("errors = %d, want none", after.errors())
	}
}

func TestFixSaysWhenNothingThatWasFoundCanBeFixed(t *testing.T) {
	pack, index := validatablePack(t)
	bad := validMod("alpha")
	bad.side = "sideways"
	bad.configFiles = []string{"config/alpha.json"}
	path := addMod(t, &index, bad)
	before := readFile(t, path)

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if !strings.HasSuffix(out, "Found 1 error and 0 warnings in 1 mod.\nNothing that was found can be fixed automatically.\n") {
		t.Errorf("output = %s", out)
	}
	if strings.Contains(out, "Changes to make") || strings.Contains(out, "[Y/n]") {
		t.Errorf("output offers changes although there are none:\n%s", out)
	}
	if readFile(t, path) != before || after.errors() != 1 {
		t.Errorf("the mod changed, or errors = %d, want it left as it was with its error", after.errors())
	}
}

func TestFixAddsARequiredDependencyThePackLacks(t *testing.T) {
	pack, index := validatablePack(t)
	serveModrinth(t, []mrProject{libraryOnModrinth("lib1")}, nil)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: "lib1", Type: "required"}}
	alpha.configFiles = []string{"config/alpha.json"}
	alphaPath := addMod(t, &index, alpha)
	alphaBefore := readFile(t, alphaPath)
	cmdtest.SetStdin(t, "y\n")

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	// What it adds is what 'mr add' would, and Modrinth lists it as only for the server, but alpha needs it on the client
	wantPlan := `Finding dependencies to add...
Changes to make:
Title of lib1 (mods/lib1-slug.pw.toml):
  add: version 1.0.0 (lib1.jar), which "alpha" requires
  side: server -> both, as "alpha" needs it on the client

Would you like to make these changes? [Y/n]: Changed 1 file.
`
	if !strings.Contains(out, wantPlan) {
		t.Errorf("output missing\n%s\nin\n%s", wantPlan, out)
	}
	libPath := filepath.Join("mods", "lib1-slug"+core.MetaExtension)
	lib := loadModAt(t, libPath)
	if lib.Side != core.UniversalSide || !lib.AddedAsDependency || lib.Name != "Title of lib1" || lib.Version != "1.0.0" {
		t.Errorf("library = %+v, want it added as a dependency, on both sides", lib)
	}
	if readFile(t, alphaPath) != alphaBefore {
		t.Error("alpha changed, want only the dependency added")
	}
	assertIndexHasHashOf(t, libPath)
	if !strings.HasSuffix(out, "Checking the pack again...\nAll 2 mods are valid!\n") || after.errors() != 0 {
		t.Errorf("the pack isn't valid afterwards (errors = %d):\n%s", after.errors(), out)
	}
}

// What the dependency requires is added too, and is put on both sides if what needs it is
func TestFixAddsWhatADependencyRequiresToo(t *testing.T) {
	pack, index := validatablePack(t)
	serveModrinth(t, []mrProject{libraryOnModrinth("lib1", "lib2"), libraryOnModrinth("lib2")}, nil)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: "lib1", Type: "required"}}
	alpha.configFiles = []string{"config/alpha.json"}
	addMod(t, &index, alpha)
	cmdtest.SetStdin(t, "y\n")

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	wantPlan := `Changes to make:
Title of lib1 (mods/lib1-slug.pw.toml):
  add: version 1.0.0 (lib1.jar), which "alpha" requires
  side: server -> both, as "alpha" needs it on the client
Title of lib2 (mods/lib2-slug.pw.toml):
  add: version 1.0.0 (lib2.jar), which "Title of lib1" requires
  side: server -> both, as "Title of lib1" needs it on the client
`
	if !strings.Contains(out, wantPlan) {
		t.Errorf("output missing\n%s\nin\n%s", wantPlan, out)
	}
	for _, id := range []string{"lib1", "lib2"} {
		if got := loadModAt(t, filepath.Join("mods", id+"-slug"+core.MetaExtension)).Side; got != core.UniversalSide {
			t.Errorf("%s side = %q, want %q", id, got, core.UniversalSide)
		}
	}
	if after.errors() != 0 {
		t.Errorf("errors = %d after, want none:\n%s", after.errors(), out)
	}
}

// A dependency that is for both sides is added as it is, with nothing to say about its side
func TestFixAddsADependencyThatIsOnBothSidesAsItIs(t *testing.T) {
	pack, index := validatablePack(t)
	both := libraryOnModrinth("lib1")
	both.clientSide = "required"
	serveModrinth(t, []mrProject{both}, nil)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: "lib1", Type: "required"}}
	addMod(t, &index, alpha)
	cmdtest.SetViperBool(t, "non-interactive", true)

	_, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if strings.Contains(out, "side:") {
		t.Errorf("output says something about a side although the dependency needed no change:\n%s", out)
	}
}

// Nothing is offered for a dependency Modrinth has nothing for, which stays an error
func TestFixLeavesADependencyItCannotFindAVersionOf(t *testing.T) {
	pack, index := validatablePack(t)
	missing := libraryOnModrinth("lib1")
	missing.noVersions = true
	serveModrinth(t, []mrProject{missing}, nil)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: "lib1", Type: "required"}}
	alpha.configFiles = []string{"config/alpha.json"}
	addMod(t, &index, alpha)

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if !strings.HasSuffix(out, "Nothing that was found can be fixed automatically.\n") || strings.Contains(out, "[Y/n]") {
		t.Errorf("output = %s", out)
	}
	if after.errors() != 1 {
		t.Errorf("errors = %d, want the missing dependency", after.errors())
	}
}

// A file that is at the path of a dependency isn't replaced
func TestFixDoesNotReplaceAFileThatIsWhereADependencyWouldGo(t *testing.T) {
	pack, index := validatablePack(t)
	serveModrinth(t, []mrProject{libraryOnModrinth("lib1")}, nil)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: "lib1", Type: "required"}}
	alpha.configFiles = []string{"config/alpha.json"}
	addMod(t, &index, alpha)
	other := validMod("other")
	other.configFiles = []string{"config/other.json"}
	inTheWay := addModFile(t, &index, "lib1-slug", other.toml())
	before := readFile(t, inTheWay)
	cmdtest.SetStdin(t, "y\n") // Only asked if it were to go on, which it mustn't

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if want := "Won't add Title of lib1: " + filepath.Join("mods", "lib1-slug"+core.MetaExtension) + " is already there\n"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if readFile(t, inTheWay) != before {
		t.Error("the file that was there was replaced")
	}
	if after.errors() != 1 {
		t.Errorf("errors = %d, want the missing dependency", after.errors())
	}
}

func TestFixRecordsTheVersionOfAModThatDoesNotRecordIt(t *testing.T) {
	pack, index := validatablePack(t)
	serveModrinth(t, nil, map[string]string{"v-alpha": "2.0.0"})
	alpha := validMod("alpha")
	alpha.version = ""
	alpha.configFiles = []string{"config/alpha.json"}
	path := addMod(t, &index, alpha)
	cmdtest.SetStdin(t, "y\n")

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if want := "Changes to make:\nalpha (mods/alpha.pw.toml):\n  version: (not recorded) -> 2.0.0\n\n"; !strings.Contains(out, want) {
		t.Errorf("output missing\n%s\nin\n%s", want, out)
	}
	if got := loadModAt(t, path).Version; got != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", got)
	}
	if !strings.HasSuffix(out, "The mod is valid!\n") || len(after.subjects) != 0 {
		t.Errorf("the pack isn't valid afterwards: %v\n%s", after.subjects, out)
	}
}

func TestFixGivesAModWithNoConfigFilesAnEmptyOne(t *testing.T) {
	pack, index := validatablePack(t)
	alpha := validMod("alpha")
	path := addMod(t, &index, alpha)
	cmdtest.SetStdin(t, "y\n")

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if want := "Changes to make:\nalpha (mods/alpha.pw.toml):\n  config-files: added, empty\n\n"; !strings.Contains(out, want) {
		t.Errorf("output missing\n%s\nin\n%s", want, out)
	}
	if got := loadModAt(t, path).ConfigFiles; got == nil || len(*got) != 0 {
		t.Errorf("ConfigFiles = %v, want a non-nil empty slice", got)
	}
	if !strings.HasSuffix(out, "The mod is valid!\n") || len(after.subjects) != 0 {
		t.Errorf("the pack isn't valid afterwards: %v\n%s", after.subjects, out)
	}
}

// A mod that already has config-files, even an empty one, is left alone
func TestFixLeavesAModsExistingConfigFilesAlone(t *testing.T) {
	pack, index := validatablePack(t)
	alpha := validMod("alpha")
	alpha.configFiles = []string{"config/alpha.json"}
	addMod(t, &index, alpha)

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if want := "Checking the pack...\nThe mod is valid!\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if after.errors() != 0 {
		t.Errorf("errors = %d, want none", after.errors())
	}
}

func TestFixGivesAModThatHasNoSideOne(t *testing.T) {
	pack, index := validatablePack(t)
	alpha := validMod("alpha")
	alpha.side = ""
	path := addMod(t, &index, alpha)
	cmdtest.SetStdin(t, "y\n")

	after, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	if want := "  side: (none) -> both, which is what a mod with no side is treated as having\n"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if got := loadModAt(t, path).Side; got != core.UniversalSide {
		t.Errorf("side = %q, want %q", got, core.UniversalSide)
	}
	if len(after.subjects) != 0 {
		t.Errorf("problems after = %v, want none", after.subjects)
	}
}

// Everything done to a file is done in one go, and shown under it once
func TestFixMakesAllTheChangesToAFileTogether(t *testing.T) {
	pack, index, libPath, _ := fixPackWithAServerModAModNeeds(t)
	serveModrinth(t, nil, map[string]string{"v-lib": "3.0.0"})
	lib := validMod("lib")
	lib.side, lib.version = core.ServerSide, ""
	lib.configFiles = []string{"config/lib.json"}
	addMod(t, &index, lib)
	cmdtest.SetStdin(t, "y\n")

	_, out, err := fix(t, pack, index)
	if err != nil {
		t.Fatalf("runFix() returned error: %v", err)
	}

	want := "lib (mods/lib.pw.toml):\n  side: server -> both, as \"alpha\" needs it on the client\n  version: (not recorded) -> 3.0.0\n\n"
	if !strings.Contains(out, want) || strings.Count(out, "lib (mods/lib.pw.toml):\n  side") != 1 {
		t.Errorf("output missing %q, once:\n%s", want, out)
	}
	if lib := loadModAt(t, libPath); lib.Side != core.UniversalSide || lib.Version != "3.0.0" {
		t.Errorf("library = side %q, version %q, want both and 3.0.0", lib.Side, lib.Version)
	}
}

// Working out what to do is only looking: what was read, and what is on disk, are as they were
func TestPlanFixesChangesNothing(t *testing.T) {
	pack, index, libPath, alphaPath := fixPackWithAServerModAModNeeds(t)
	libBefore, alphaBefore := readFile(t, libPath), readFile(t, alphaPath)
	var v *validation
	cmdtest.CaptureStdout(t, func() { v = validatePack(pack, index) })

	plan := planFixes(pack, index, v)

	if len(plan.changes) != 1 || plan.changes[0].path != libPath || plan.changes[0].side == nil {
		t.Fatalf("plan = %+v, want a change of the library's side", plan.changes)
	}
	for _, e := range v.entries {
		if e.path == libPath && e.mod.Side != core.ServerSide {
			t.Errorf("the mod validation read now has side %q, want it left as it was", e.mod.Side)
		}
	}
	if readFile(t, libPath) != libBefore || readFile(t, alphaPath) != alphaBefore {
		t.Error("planning changed a file")
	}
}

// A file that can't be changed doesn't stop the others, which are saved and put in the index
func TestFixApplyGoesOnAfterAFileItCannotChange(t *testing.T) {
	pack, index, libPath, _ := fixPackWithAServerModAModNeeds(t)
	plan := &fixPlan{changes: []*fileChange{
		{path: "mods/ghost.pw.toml", name: "ghost", side: &sideChange{from: core.ServerSide, to: core.UniversalSide}},
		{path: libPath, name: "lib", side: &sideChange{from: core.ServerSide, to: core.UniversalSide, neededBy: "alpha"}},
	}}

	changed, err := plan.apply(&pack, &index)

	if changed != 1 {
		t.Errorf("changed = %d, want the one that could be", changed)
	}
	if err == nil || !strings.Contains(err.Error(), "mods/ghost.pw.toml") {
		t.Errorf("error = %v, want it to name the file that couldn't be changed", err)
	}
	if got := loadModAt(t, libPath).Side; got != core.UniversalSide {
		t.Errorf("library side = %q, want it changed anyway", got)
	}
	assertIndexHasHashOf(t, libPath)
}

func TestFixPlanColour(t *testing.T) {
	pack, index, _, _ := fixPackWithAServerModAModNeeds(t)
	var v *validation
	cmdtest.CaptureStdout(t, func() { v = validatePack(pack, index) })
	plan := planFixes(pack, index, v)

	plain, coloured := cmdtest.AssertColourOnlyAdds(t, plan.print)

	for name, want := range map[string]string{
		"the heading":               ui.Bold.Sprint("Changes to make:") + "\n",
		"the file and where it is":  ui.Bold.Sprint("lib") + " " + ui.Muted.Sprint("(mods/lib.pw.toml)") + ":\n",
		"what changes, old and new": "  side: " + ui.Transition("server", "both") + ", as \"alpha\" needs it on the client\n",
	} {
		if !strings.Contains(coloured, want) {
			t.Errorf("%s: output missing %q:\n%q", name, want, coloured)
		}
	}
	if !strings.Contains(plain, "  side: server -> both, as \"alpha\" needs it on the client\n") {
		t.Errorf("output without colour = %q", plain)
	}
}

func TestFixIsAModrinthCommand(t *testing.T) {
	if cmd, _, err := modrinthCmd.Find([]string{"fix"}); err != nil || cmd != fixCmd {
		t.Errorf("modrinth fix = %v (%v), want the fix command", cmd, err)
	}
}

// How the command ends decides whether a script goes on, so it is run again as a process to see: failing when errors
// are left after it, and not when it has fixed them
func TestFixCommandExitStatus(t *testing.T) {
	if scenario := os.Getenv("PACKWIZ_TEST_FIX"); scenario != "" {
		pack, index := setupPackFixture(t)
		pack.Versions["neoforge"] = "21.1.0"
		pack.Version = "1.0.0"
		httpmock.Activate(t) // Nothing here should be looked up
		cmdtest.SetViperBool(t, "non-interactive", true)
		switch scenario {
		case "unfixable":
			bad := validMod("alpha")
			bad.side = "sideways"
			bad.configFiles = []string{"config/alpha.json"}
			addMod(t, &index, bad)
		case "fixed":
			lib := validMod("lib")
			lib.side = core.ServerSide
			addMod(t, &index, lib)
			alpha := validMod("alpha")
			alpha.deps = []core.ModDependency{{ID: projectOf("lib"), Type: "required"}}
			addMod(t, &index, alpha)
		}
		if err := index.Write(); err != nil {
			t.Fatalf("Write() returned error: %v", err)
		}
		if err := pack.Write(); err != nil {
			t.Fatalf("Write() returned error: %v", err)
		}
		fixCmd.Run(fixCmd, nil)
		return // The command didn't end, so the process doesn't fail
	}

	for scenario, tt := range map[string]struct {
		exit int
		want string
	}{
		"unfixable": {1, "Nothing that was found can be fixed automatically.\n"},
		"fixed":     {0, "Checking the pack again...\nAll 2 mods are valid!\n"},
	} {
		t.Run(scenario, func(t *testing.T) {
			process := exec.Command(os.Args[0], "-test.run=^TestFixCommandExitStatus$")
			process.Env = append(os.Environ(), "PACKWIZ_TEST_FIX="+scenario)
			out, err := process.CombinedOutput()

			exit := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exit = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("the command couldn't be run: %v", err)
			}
			if exit != tt.exit {
				t.Errorf("the command ended with status %d, want %d\noutput: %s", exit, tt.exit, out)
			}
			if !strings.Contains(string(out), tt.want) {
				t.Errorf("output missing %q:\n%s", tt.want, out)
			}
		})
	}
}
