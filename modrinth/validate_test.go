package modrinth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/jarcoal/httpmock"
)

// modFile is a mod's metadata as a test writes it: a field left empty is left out of the file
type modFile struct {
	name, fileName, version, side string
	url, mode, hashFormat, hash   string
	// modrinth says whether there is any Modrinth update data, of the project and version IDs given
	modrinth           bool
	project, versionID string
	deps               []core.ModDependency
	configFiles        []string
}

// validMod is a mod that has everything a metadata file needs. It records a dependency, as a mod that records none is
// looked up on Modrinth, which the tests that want that say so.
func validMod(name string) modFile {
	return modFile{
		name: name, fileName: name + ".jar", version: "1.0.0", side: core.UniversalSide,
		url: "https://cdn.modrinth.com/data/" + name + "/versions/v1/" + name + ".jar", hashFormat: "sha512", hash: "abc",
		modrinth: true, project: projectOf(name), versionID: "v-" + name,
		deps: []core.ModDependency{{ID: "elsewhere", Type: "optional"}},
	}
}

func (m modFile) toml() string {
	var b strings.Builder
	for _, field := range []struct{ key, value string }{{"name", m.name}, {"filename", m.fileName}, {"version", m.version}, {"side", m.side}} {
		if field.value != "" {
			fmt.Fprintf(&b, "%s = %q\n", field.key, field.value)
		}
	}
	if len(m.configFiles) > 0 {
		quoted := make([]string, len(m.configFiles))
		for i, cf := range m.configFiles {
			quoted[i] = fmt.Sprintf("%q", cf)
		}
		fmt.Fprintf(&b, "config-files = [%s]\n", strings.Join(quoted, ", "))
	}
	b.WriteString("\n[download]\n")
	for _, field := range []struct{ key, value string }{{"url", m.url}, {"mode", m.mode}, {"hash-format", m.hashFormat}, {"hash", m.hash}} {
		if field.value != "" {
			fmt.Fprintf(&b, "%s = %q\n", field.key, field.value)
		}
	}
	if m.modrinth {
		b.WriteString("\n[update.modrinth]\n")
		if m.project != "" {
			fmt.Fprintf(&b, "mod-id = %q\n", m.project)
		}
		if m.versionID != "" {
			fmt.Fprintf(&b, "version = %q\n", m.versionID)
		}
	}
	for _, dep := range m.deps {
		fmt.Fprintf(&b, "\n[[dependencies]]\nid = %q\ntype = %q\n", dep.ID, dep.Type)
	}
	return b.String()
}

// addModFile puts a metadata file in the pack and its index, as 'packwiz refresh' would, and returns where it is
func addModFile(t *testing.T, index *core.Index, name, content string) string {
	t.Helper()
	path := filepath.Join("mods", name+core.MetaExtension)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	if err := index.RefreshFileWithHash(path, "sha256", "unchecked", true); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}
	return filepath.ToSlash(path)
}

func addMod(t *testing.T, index *core.Index, m modFile) string {
	t.Helper()
	return addModFile(t, index, m.name, m.toml())
}

// validatablePack is a pack fixture that has what a pack needs, so that a test only gets the problems it makes
func validatablePack(t *testing.T) (core.Pack, core.Index) {
	t.Helper()
	pack, index := setupPackFixture(t)
	pack.Versions["neoforge"] = "21.1.0"
	pack.Version = "1.0.0"
	// Nothing is looked up unless a test says how Modrinth answers, and one that is looked up anyway is a problem
	httpmock.Activate(t)
	return pack, index
}

// validate checks the pack and prints what it found, as the command does, returning both
func validate(t *testing.T, pack core.Pack, index core.Index) (*validation, string) {
	t.Helper()
	var v *validation
	out := cmdtest.CaptureStdout(t, func() {
		v = validatePack(pack, index)
		v.print()
	})
	return v, out
}

// problemsIn lists what is wrong with the file at path, or with the pack for "", as "error: ..." and "warning: ..."
func problemsIn(v *validation, path string) []string {
	s := v.subjects[path]
	if s == nil {
		return nil
	}
	var problems []string
	for _, p := range s.problems {
		label := "warning"
		if p.severity == severityError {
			label = "error"
		}
		problems = append(problems, label+": "+p.message)
	}
	return problems
}

func assertProblems(t *testing.T, v *validation, path string, want ...string) {
	t.Helper()
	if got := problemsIn(v, path); !slices.Equal(got, want) {
		t.Errorf("problems of %q = %q, want %q", path, got, want)
	}
}

func TestValidateAValidPackHasNoProblems(t *testing.T) {
	pack, index := validatablePack(t)
	addMod(t, &index, validMod("alpha"))
	beta := validMod("beta")
	beta.deps = append(beta.deps, core.ModDependency{ID: projectOf("alpha"), Type: "required"})
	addMod(t, &index, beta)

	v, out := validate(t, pack, index)

	if len(v.subjects) != 0 {
		t.Errorf("problems = %v, want none", v.subjects)
	}
	if want := "All 2 mods are valid!\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestValidateChecksWhatAMetadataFileNeeds(t *testing.T) {
	tests := []struct {
		name string
		edit func(m *modFile)
		want []string
	}{
		{"no file name", func(m *modFile) { m.fileName = "" }, []string{"error: has no file name"}},
		{"a file name that leaves the folder", func(m *modFile) { m.fileName = "../alpha.jar" },
			[]string{`error: file name "../alpha.jar" would put the mod outside the folder its metadata is in`}},
		{"a file name in a folder is fine", func(m *modFile) { m.fileName = "sub/alpha.jar" }, nil},
		{"no download URL", func(m *modFile) { m.url = "" }, []string{"error: has no download URL"}},
		{"a download URL that isn't a web address", func(m *modFile) { m.url = "not a url" },
			[]string{`error: download URL "not a url" isn't a web address`}},
		{"a download URL that is a file", func(m *modFile) { m.url = "file:///etc/passwd" },
			[]string{`error: download URL "file:///etc/passwd" isn't a web address`}},
		{"a download mode that is another source's", func(m *modFile) { m.mode = "metadata:curseforge" },
			[]string{`error: download mode "metadata:curseforge" isn't supported, only "url" is`}},
		{"the url download mode", func(m *modFile) { m.mode = core.ModeURL }, nil},
		{"no hash format", func(m *modFile) { m.hashFormat = "" }, []string{"error: has no hash format"}},
		{"a hash format that isn't known", func(m *modFile) { m.hashFormat = "sha3" }, []string{`error: hash format "sha3" isn't known`}},
		{"no hash", func(m *modFile) { m.hash = "" }, []string{"error: has no hash"}},
		{"a side that isn't one", func(m *modFile) { m.side = "sideways" }, []string{`error: side "sideways" isn't one of client, server or both`}},
		{"no side", func(m *modFile) { m.side = "" }, []string{"warning: has no side, so it is on both"}},
		{"a client mod", func(m *modFile) { m.side = core.ClientSide }, nil},
		{"a server mod", func(m *modFile) { m.side = core.ServerSide }, nil},
		{"no name", func(m *modFile) { m.name = "" }, []string{"warning: has no name"}},
		{"no Modrinth update data", func(m *modFile) { m.modrinth = false },
			[]string{"warning: isn't from Modrinth, so it can't be updated and its dependencies can't be checked"}},
		{"no Modrinth project ID", func(m *modFile) { m.project = "" }, []string{"error: has no Modrinth project ID (mod-id)"}},
		{"no Modrinth version ID", func(m *modFile) { m.versionID = "" }, []string{"error: has no Modrinth version ID (version)"}},
		{"no version", func(m *modFile) { m.version = "" }, []string{"warning: doesn't record its version; 'packwiz modrinth fix' and 'packwiz git commit' save it"}},
		{"several problems, in the order they are found", func(m *modFile) { m.fileName, m.hash, m.version = "", "", "" },
			[]string{"error: has no file name", "error: has no hash", "warning: doesn't record its version; 'packwiz modrinth fix' and 'packwiz git commit' save it"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pack, index := validatablePack(t)
			m := validMod("alpha")
			tt.edit(&m)
			// Named by its file, as a mod with no name of its own is
			path := addModFile(t, &index, "alpha", m.toml())

			v := validatePack(pack, index)

			assertProblems(t, v, path, tt.want...)
			if len(v.subjects) > 1 || (len(v.subjects) == 1 && v.subjects[path] == nil) {
				t.Errorf("problems with something other than the mod: %v", v.subjects)
			}
		})
	}
}

func TestValidateReportsAFileItCannotReadAndGoesOn(t *testing.T) {
	pack, index := validatablePack(t)
	garbled := addModFile(t, &index, "garbled", "name = \n")
	unknown := addModFile(t, &index, "unknown", validMod("unknown").toml()+"\n[update.curseforge]\nid = 1\n")
	broken := validMod("broken")
	broken.hash = ""
	brokenPath := addMod(t, &index, broken)

	v := validatePack(pack, index)

	for _, path := range []string{garbled, unknown} {
		got := problemsIn(v, path)
		if len(got) != 1 || !strings.HasPrefix(got[0], "error: can't be read: ") {
			t.Errorf("problems of %q = %q, want one saying it can't be read", path, got)
		}
	}
	if got := problemsIn(v, unknown); len(got) == 1 && !strings.Contains(got[0], "Update plugin curseforge not found") {
		t.Errorf("problems of %q = %q, want it to say why", unknown, got)
	}
	assertProblems(t, v, brokenPath, "error: has no hash")
	if v.mods != 3 {
		t.Errorf("mods = %d, want the ones that can't be read counted too", v.mods)
	}
}

func TestValidateFindsTheSameProjectAndTheSameFileTwice(t *testing.T) {
	pack, index := validatablePack(t)
	first := addMod(t, &index, validMod("a"))
	sameProject := validMod("b")
	sameProject.project = projectOf("a")
	secondPath := addMod(t, &index, sameProject)
	sameFile := validMod("c")
	sameFile.fileName = "a.jar"
	thirdPath := addMod(t, &index, sameFile)

	v := validatePack(pack, index)

	assertProblems(t, v, first)
	assertProblems(t, v, secondPath, "error: is the same Modrinth project as "+first)
	assertProblems(t, v, thirdPath, "error: installs to the same file as "+first)
}

func TestValidateWarnsAboutConfigFilesNothingClaims(t *testing.T) {
	pack, index := validatablePack(t)
	addMod(t, &index, validMod("alpha"))
	if err := index.RefreshFileWithHash("config/orphan.json", "sha256", "unchecked", false); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}

	v := validatePack(pack, index)

	assertProblems(t, v, "", "warning: 1 file isn't claimed by any mod's config-files: config/orphan.json")
}

func TestValidateDoesNotWarnAboutClaimedConfigFiles(t *testing.T) {
	pack, index := validatablePack(t)
	alpha := validMod("alpha")
	alpha.configFiles = []string{"config/alpha.json", "config/alpha/"}
	addMod(t, &index, alpha)
	for _, p := range []string{"config/alpha.json", "config/alpha/sub.json"} {
		if err := index.RefreshFileWithHash(p, "sha256", "unchecked", false); err != nil {
			t.Fatalf("RefreshFileWithHash(%q) returned error: %v", p, err)
		}
	}

	v := validatePack(pack, index)

	assertProblems(t, v, "")
}

func TestValidateCombinesSeveralOrphanedConfigFilesIntoOneWarning(t *testing.T) {
	pack, index := validatablePack(t)
	addMod(t, &index, validMod("alpha"))
	for _, p := range []string{"config/a.json", "config/b.json"} {
		if err := index.RefreshFileWithHash(p, "sha256", "unchecked", false); err != nil {
			t.Fatalf("RefreshFileWithHash(%q) returned error: %v", p, err)
		}
	}

	v := validatePack(pack, index)

	assertProblems(t, v, "", "warning: 2 files aren't claimed by any mod's config-files: config/a.json, config/b.json")
}

// serveProjects answers Modrinth's project lookup with the titles given, by project ID
func serveProjects(titles map[string]string) {
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`, func(req *http.Request) (*http.Response, error) {
		var ids []string
		if err := json.Unmarshal([]byte(req.URL.Query().Get("ids")), &ids); err != nil {
			return httpmock.NewStringResponse(400, "bad ids"), nil
		}
		found := []map[string]string{}
		for _, id := range ids {
			if title, ok := titles[id]; ok {
				found = append(found, map[string]string{"id": id, "title": title})
			}
		}
		return httpmock.NewJsonResponse(200, found)
	})
}

func TestValidateFindsARequiredDependencyThatIsNotInThePack(t *testing.T) {
	pack, index := validatablePack(t)
	serveProjects(map[string]string{"lib1": "Lib Mod"})
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: "lib1", Type: "required"}, {ID: "opt1", Type: "optional"}, {ID: "emb1", Type: "embedded"}}
	path := addMod(t, &index, alpha)

	v := validatePack(pack, index)

	assertProblems(t, v, path, `error: requires "Lib Mod", which isn't in the pack`)
}

func TestValidateNamesAMissingDependencyByIDWhenModrinthCannotBeAsked(t *testing.T) {
	pack, index := validatablePack(t)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: "lib1", Type: "required"}}
	path := addMod(t, &index, alpha)

	v := validatePack(pack, index)

	assertProblems(t, v, path, `error: requires "lib1", which isn't in the pack`)
}

func TestValidateAcceptsARequiredDependencyThatIsInThePack(t *testing.T) {
	pack, index := validatablePack(t)
	lib := validMod("lib")
	addMod(t, &index, lib)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: projectOf("lib"), Type: "required"}}
	addMod(t, &index, alpha)

	v := validatePack(pack, index)

	if len(v.subjects) != 0 {
		t.Errorf("problems = %v, want none", v.subjects)
	}
}

func TestValidateFindsAModThatIsIncompatibleWithOneInThePack(t *testing.T) {
	pack, index := validatablePack(t)
	addMod(t, &index, validMod("beta"))
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: projectOf("beta"), Type: "incompatible"}, {ID: "absent", Type: "incompatible"}}
	path := addMod(t, &index, alpha)

	v := validatePack(pack, index)

	assertProblems(t, v, path, `error: is incompatible with "beta", which is in the pack`)
}

// A pack that runs Fabric mods has Forgified Fabric API where they expect Fabric API, which it mustn't have
func TestValidateTakesForgifiedFabricAPIForFabricAPI(t *testing.T) {
	pack, index := validatablePack(t)
	connector := validMod("connector")
	connector.project = connectorProjectID
	addMod(t, &index, connector)
	forgified := validMod("forgified")
	forgified.project = forgifiedFabricAPIProjectID
	addMod(t, &index, forgified)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: fabricAPIProjectID, Type: "required"}}
	addMod(t, &index, alpha)

	v := validatePack(pack, index)

	if len(v.subjects) != 0 {
		t.Errorf("problems = %v, want none as Forgified Fabric API stands in for Fabric API", v.subjects)
	}
}

func TestValidateFindsFabricAPIMissingWhenThePackCannotRunFabricMods(t *testing.T) {
	pack, index := validatablePack(t)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: fabricAPIProjectID, Type: "required"}}
	path := addMod(t, &index, alpha)

	v := validatePack(pack, index)

	assertProblems(t, v, path, `error: requires "`+fabricAPIProjectID+`", which isn't in the pack`)
}

func TestValidateFindsAServerModThatAModOnTheClientNeeds(t *testing.T) {
	pack, index := validatablePack(t)
	lib := validMod("lib")
	lib.side = core.ServerSide
	libPath := addMod(t, &index, lib)
	alpha := validMod("alpha")
	alpha.deps = []core.ModDependency{{ID: projectOf("lib"), Type: "required"}}
	addMod(t, &index, alpha)
	before := readFile(t, libPath)

	v := validatePack(pack, index)

	assertProblems(t, v, libPath, `error: is only on the server, but "alpha" needs it on the client; its side should be "both"`)
	if after := readFile(t, libPath); after != before {
		t.Errorf("validating changed the metadata:\n%s\nwas:\n%s", after, before)
	}
}

// serveDependencies answers Modrinth's version lookup with what each version depends on (version ID to the project IDs
// it requires; one that isn't there is left out of the answer, as Modrinth does), and records the IDs asked for in each
// request.
func serveDependencies(t *testing.T, requires map[string][]string) *[][]string {
	t.Helper()
	var batches [][]string
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/versions`, func(req *http.Request) (*http.Response, error) {
		var ids []string
		if err := json.Unmarshal([]byte(req.URL.Query().Get("ids")), &ids); err != nil {
			return httpmock.NewStringResponse(400, "bad ids"), nil
		}
		batches = append(batches, ids)
		found := []map[string]any{}
		for _, id := range ids {
			projects, ok := requires[id]
			if !ok {
				continue
			}
			deps := []map[string]string{}
			for _, project := range projects {
				deps = append(deps, map[string]string{"project_id": project, "dependency_type": "required"})
			}
			found = append(found, map[string]any{"id": id, "dependencies": deps})
		}
		return httpmock.NewJsonResponse(200, found)
	})
	return &batches
}

// A mod that records no dependencies is asked about, and only those: it may depend on nothing, or have been added
// before dependencies were recorded
func TestValidateLooksUpWhatModsThatRecordNoDependenciesNeed(t *testing.T) {
	pack, index := validatablePack(t)
	batches := serveDependencies(t, map[string][]string{"v-alpha": {"lib1"}, "v-gamma": nil})
	serveProjects(map[string]string{"lib1": "Lib Mod"})
	alpha := validMod("alpha")
	alpha.deps = nil
	alphaPath := addMod(t, &index, alpha)
	addMod(t, &index, validMod("beta")) // records a dependency
	gamma := validMod("gamma")
	gamma.deps = nil
	gammaPath := addMod(t, &index, gamma)
	before := readFile(t, alphaPath)

	v := validatePack(pack, index)

	assertProblems(t, v, alphaPath, `error: requires "Lib Mod", which isn't in the pack`)
	assertProblems(t, v, gammaPath)
	if len(*batches) != 1 || !slices.Equal((*batches)[0], []string{"v-alpha", "v-gamma"}) {
		t.Errorf("Modrinth was asked about %v, want one request for only the mods that record no dependencies", *batches)
	}
	if after := readFile(t, alphaPath); after != before {
		t.Errorf("validating saved what it looked up:\n%s\nwas:\n%s", after, before)
	}
}

func TestValidateLooksUpDependenciesInBatches(t *testing.T) {
	pack, index := validatablePack(t)
	requires := make(map[string][]string)
	for i := range versionLookupBatchSize + 1 {
		m := validMod(fmt.Sprintf("mod%03d", i))
		m.deps = nil
		addMod(t, &index, m)
		requires["v-"+m.name] = nil
	}
	batches := serveDependencies(t, requires)

	v := validatePack(pack, index)

	if len(v.subjects) != 0 {
		t.Errorf("problems = %v, want none", v.subjects)
	}
	if len(*batches) != 2 || len((*batches)[0]) != versionLookupBatchSize || len((*batches)[1]) != 1 {
		t.Errorf("requests for %d mods, want them in batches of %d", len(requires), versionLookupBatchSize)
	}
}

func TestValidateSaysWhenDependenciesCouldNotBeLookedUp(t *testing.T) {
	pack, index := validatablePack(t)
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/versions`, httpmock.NewStringResponder(500, "down"))
	for _, name := range []string{"alpha", "beta"} {
		m := validMod(name)
		m.deps = nil
		addMod(t, &index, m)
	}

	v := validatePack(pack, index)

	if v.errors() != 0 {
		t.Errorf("errors = %d, want none: what couldn't be looked up is a warning", v.errors())
	}
	got := problemsIn(v, "")
	if len(got) != 1 || !strings.HasPrefix(got[0], "warning: dependencies not checked for 2 mods, which couldn't be looked up on Modrinth (") ||
		!strings.HasSuffix(got[0], "): alpha, beta") {
		t.Errorf("problems of the pack = %q, want one saying the dependencies of alpha and beta weren't checked", got)
	}
}

func TestValidateSaysWhenModrinthDoesNotHaveTheVersion(t *testing.T) {
	pack, index := validatablePack(t)
	serveDependencies(t, map[string][]string{})
	alpha := validMod("alpha")
	alpha.deps = nil
	addMod(t, &index, alpha)

	v := validatePack(pack, index)

	assertProblems(t, v, "", "warning: dependencies not checked for 1 mod, which couldn't be looked up on Modrinth: alpha")
}

func TestListNamesStopsAtTheLimit(t *testing.T) {
	if got, want := listNames([]string{"a", "b", "c"}, 5), "a, b, c"; got != want {
		t.Errorf("listNames() = %q, want %q", got, want)
	}
	if got, want := listNames([]string{"a", "b", "c", "d", "e", "f", "g"}, 5), "a, b, c, d, e and 2 more"; got != want {
		t.Errorf("listNames() = %q, want %q", got, want)
	}
}

func TestValidateChecksThePackItself(t *testing.T) {
	_, index := setupPackFixture(t)
	httpmock.Activate(t)

	v := validatePack(core.Pack{}, index)

	assertProblems(t, v, "",
		"error: has no Minecraft version (versions.minecraft in pack.toml)",
		"error: has no NeoForge version (versions.neoforge in pack.toml), so an exported pack has no mod loader",
		"warning: has no version (version in pack.toml), which a Modrinth pack needs")
}

func TestValidateOutputSaysWhatIsWrongWithWhat(t *testing.T) {
	pack, index := validatablePack(t)
	pack.Version = ""
	broken := validMod("broken")
	broken.hash, broken.version = "", ""
	addMod(t, &index, broken)
	addMod(t, &index, validMod("fine"))
	noSide := validMod("noside")
	noSide.side = ""
	addMod(t, &index, noSide)

	_, out := validate(t, pack, index)

	want := `The pack:
  warning: has no version (version in pack.toml), which a Modrinth pack needs

broken (mods/broken.pw.toml):
  error: has no hash
  warning: doesn't record its version; 'packwiz modrinth fix' and 'packwiz git commit' save it

noside (mods/noside.pw.toml):
  warning: has no side, so it is on both

Found 1 error and 3 warnings in 3 mods.
`
	if out != want {
		t.Errorf("output =\n%s\nwant\n%s", out, want)
	}
}

func TestValidateSummary(t *testing.T) {
	tests := []struct {
		name string
		mods int
		edit func(m *modFile)
		want string
	}{
		{"no mods", 0, nil, "The pack has no mods to check.\n"},
		{"one mod", 1, nil, "The mod is valid!\n"},
		{"several mods", 3, nil, "All 3 mods are valid!\n"},
		{"only warnings", 1, func(m *modFile) { m.side = "" }, "No errors, but 1 warning.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pack, index := validatablePack(t)
			for i := range tt.mods {
				m := validMod(fmt.Sprintf("mod%d", i))
				if tt.edit != nil {
					tt.edit(&m)
				}
				addMod(t, &index, m)
			}

			_, out := validate(t, pack, index)

			if !strings.HasSuffix(out, tt.want) {
				t.Errorf("output = %q, want it to end with %q", out, tt.want)
			}
		})
	}
}

func TestValidateOutputColour(t *testing.T) {
	pack, index := validatablePack(t)
	pack.Version = ""
	broken := validMod("broken")
	broken.hash = ""
	addMod(t, &index, broken)

	plain, coloured := cmdtest.AssertColourOnlyAdds(t, func() {
		v := validatePack(pack, index)
		v.print()
	})

	for name, want := range map[string]string{
		"who the pack's problems are about": ui.Bold.Sprint("The pack") + ":\n",
		"a mod and where it is":             ui.Bold.Sprint("broken") + " " + ui.Muted.Sprint("(mods/broken.pw.toml)") + ":\n",
		"an error":                          ui.Error.Sprint("  error: has no hash") + "\n",
		"a warning":                         ui.Warning.Sprint("  warning: has no version (version in pack.toml), which a Modrinth pack needs") + "\n",
		"the summary":                       ui.Error.Sprint("Found 1 error and 1 warning in 1 mod.") + "\n",
	} {
		if !strings.Contains(coloured, want) {
			t.Errorf("%s: output missing %q:\n%q", name, want, coloured)
		}
	}
	if !strings.Contains(plain, "Found 1 error and 1 warning in 1 mod.") {
		t.Errorf("output without colour missing the summary:\n%q", plain)
	}
}

func TestValidateSuccessColour(t *testing.T) {
	pack, index := validatablePack(t)
	addMod(t, &index, validMod("alpha"))
	addMod(t, &index, validMod("beta"))

	_, coloured := cmdtest.AssertColourOnlyAdds(t, func() {
		v := validatePack(pack, index)
		v.print()
	})

	if want := ui.Success.Sprint("All 2 mods are valid!") + "\n"; coloured != want {
		t.Errorf("output = %q, want %q", coloured, want)
	}
}

func TestValidateIsAModrinthCommand(t *testing.T) {
	if cmd, _, err := modrinthCmd.Find([]string{"validate"}); err != nil || cmd != validateCmd {
		t.Errorf("modrinth validate = %v (%v), want the validate command", cmd, err)
	}
}

// How the command ends decides whether a script goes on, so it is run again as a process to see: failing when the pack
// has errors, and not when it has only warnings
func TestValidateCommandExitStatus(t *testing.T) {
	if scenario := os.Getenv("PACKWIZ_TEST_VALIDATE"); scenario != "" {
		pack, index := setupPackFixture(t)
		pack.Versions["neoforge"] = "21.1.0"
		pack.Version = "1.0.0"
		m := validMod("alpha")
		switch scenario {
		case "errors":
			m.hash = ""
		case "warnings":
			m.side = ""
		}
		addMod(t, &index, m)
		if err := index.Write(); err != nil {
			t.Fatalf("Write() returned error: %v", err)
		}
		if err := pack.Write(); err != nil {
			t.Fatalf("Write() returned error: %v", err)
		}
		validateCmd.Run(validateCmd, nil)
		return // The command didn't end, so the process doesn't fail
	}

	for scenario, tt := range map[string]struct {
		exit int
		want string
	}{
		"errors":   {1, "  error: has no hash\n"},
		"warnings": {0, "No errors, but 1 warning.\n"},
	} {
		t.Run(scenario, func(t *testing.T) {
			process := exec.Command(os.Args[0], "-test.run=^TestValidateCommandExitStatus$")
			process.Env = append(os.Environ(), "PACKWIZ_TEST_VALIDATE="+scenario)
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
