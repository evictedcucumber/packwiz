package tui

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/modrinth"
	"github.com/jarcoal/httpmock"
)

// fakeAdd is an addBackend that says what a test tells it to, and records what it was asked.
type fakeAdd struct {
	results     *modrinth.SearchResults
	searchErr   error
	plan        *modrinth.AddPlan
	planErr     error
	result      *modrinth.AddResult
	applyErr    error
	searches    []string
	plans       []string
	applied     []bool
	lastApplied *modrinth.AddPlan
}

func (f *fakeAdd) search(query, kind string) (*modrinth.SearchResults, error) {
	f.searches = append(f.searches, kind+":"+query)
	return f.results, f.searchErr
}

func (f *fakeAdd) planAdd(ref, releaseType string) (*modrinth.AddPlan, error) {
	f.plans = append(f.plans, ref+"|"+releaseType)
	return f.plan, f.planErr
}

func (f *fakeAdd) applyAdd(plan *modrinth.AddPlan, dependencies bool) (*modrinth.AddResult, error) {
	f.applied = append(f.applied, dependencies)
	f.lastApplied = plan
	return f.result, f.applyErr
}

// sodiumPlan is what adding Sodium, which needs a library, would do.
func sodiumPlan() *modrinth.AddPlan {
	return &modrinth.AddPlan{
		Project: "Sodium", Slug: "sodium", Version: "0.6.0", ReleaseType: "release", File: "sodium-0.6.0.jar", Folder: "mods", Side: "client",
		Dependencies: []modrinth.AddDependency{{Name: "Lib", Version: "1.2", File: "lib.jar"}},
		Notices:      []string{"Notice: Sodium is a Fabric mod; it runs on NeoForge through Sinytra Connector"},
	}
}

func newFakeAdd() *fakeAdd {
	return &fakeAdd{
		results: &modrinth.SearchResults{Total: 2, Found: []modrinth.Found{
			{ID: "AANobbMI", Slug: "sodium", Title: "Sodium", Author: "jelly", Description: "A fast renderer", Downloads: 233_000_000},
			{ID: "PtjYWJkn", Slug: "sodium-extra", Title: "Sodium Extra", Author: "flashy", Description: "More options", Downloads: 4_500_000, InPack: true},
		}},
		plan:   sodiumPlan(),
		result: &modrinth.AddResult{Project: "Sodium", File: "sodium-0.6.0.jar", Dependencies: []string{"Lib"}},
	}
}

func addOn(t *testing.T, backend addBackend) *addScreen {
	t.Helper()
	setUpPack(t)
	s := newAddScreen(backend)
	s.setSize(100, 24)
	s.activate()
	// What is typed goes in the query once / is pressed, as in the other screens
	press(t, s, "/")
	return s
}

func TestAddTakesTheKeysOnlyOnceYouSayYouWillType(t *testing.T) {
	setUpPack(t)
	s := newAddScreen(newFakeAdd())
	s.setSize(100, 24)
	s.activate()
	if s.modal() {
		t.Error("the screen took the keys as soon as it was shown, which would keep tab and the numbers from going through the screens")
	}

	press(t, s, "/")
	if !s.modal() {
		t.Error("/ didn't start typing the query, so the keys that are typed would be commands")
	}
	typeText(t, s, "q 1")
	if got := s.query.String(); got != "q 1" {
		t.Errorf("the query is %q, want what was typed, including a q and a number", got)
	}
	if out := s.view(); !strings.Contains(out, "Search q 1█") {
		t.Errorf("the screen doesn't show what is typed:\n%s", out)
	}

	press(t, s, "esc")
	if s.modal() {
		t.Error("the screen is still modal after esc")
	}
	press(t, s, "i")
	if !s.modal() {
		t.Error("i didn't start typing the query again")
	}
}

func TestAddSearchesWhenYouPressEnterAndShowsWhatItFinds(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter")

	if len(f.searches) != 1 || f.searches[0] != "mod:sodium" {
		t.Errorf("the searches are %v, want one for mods of what was typed", f.searches)
	}
	rows := body(t, s)
	out := strings.Join(rows, "\n")
	for _, want := range []string{"Sodium", "by jelly", "233M", "A fast renderer", "Sodium Extra", "in the pack"} {
		if !strings.Contains(out, want) {
			t.Errorf("the results don't say %q:\n%s", want, out)
		}
	}
	if !strings.HasPrefix(rows[2], "> Sodium ") {
		t.Errorf("the first result is %q, want the cursor on it", rows[2])
	}
	if got := statusOf(s); got != "enter adds the one under the cursor" {
		t.Errorf("the status line is %q, want it to say what enter does now", got)
	}
}

func TestAddSaysHowManyMoreThereAre(t *testing.T) {
	f := newFakeAdd()
	f.results.Total = 40
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter")
	if got := statusOf(s); got != "Showing 2 of 40 matches: enter adds the one under the cursor" {
		t.Errorf("the status line is %q, want it to say there are more matches than are shown", got)
	}
}

func TestAddSaysWhenNothingMatches(t *testing.T) {
	f := newFakeAdd()
	f.results = &modrinth.SearchResults{}
	s := addOn(t, f)
	typeText(t, s, "zzz")
	press(t, s, "enter")
	if got := statusOf(s); got != `Nothing on Modrinth matches "zzz" for this pack` {
		t.Errorf("the status line is %q, want it to say nothing matches", got)
	}
}

func TestAddSaysWhenTheSearchFails(t *testing.T) {
	f := newFakeAdd()
	f.searchErr = errors.New("the search failed: no network")
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter")
	if got := statusOf(s); got != "the search failed: no network" {
		t.Errorf("the status line is %q, want the reason", got)
	}
}

func TestAddAsksWhatAddingWouldDoWhenYouPressEnterOnAResult(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter", "enter")

	if len(f.plans) != 1 || f.plans[0] != "AANobbMI|" {
		t.Errorf("the plans are %v, want one for the result under the cursor, by its ID", f.plans)
	}
	out := s.view()
	for _, want := range []string{"Add Sodium?", "Sodium 0.6.0 (release)", "sodium-0.6.0.jar", "mods/", "client", "It requires, which the pack doesn't have", "Lib 1.2 (lib.jar)", "Sinytra"} {
		if !strings.Contains(out, want) {
			t.Errorf("the question doesn't say %q:\n%s", want, out)
		}
	}
	if len(f.applied) != 0 {
		t.Fatalf("the project was added before it was asked: %v", f.applied)
	}

	press(t, s, "y")
	if len(f.applied) != 1 || !f.applied[0] {
		t.Errorf("the project was added with dependencies %v, want once, with its dependencies", f.applied)
	}
	if got := statusOf(s); got != "Added Sodium (sodium-0.6.0.jar) with Lib" {
		t.Errorf("the status line is %q, want it to say what was added", got)
	}
}

func TestAddCanAddAProjectWithoutItsDependencies(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter", "enter")
	if footer := strings.Join(keyHelps(s.keys()), " "); !strings.Contains(footer, "add without dependencies") {
		t.Errorf("the keys are %q, want d offered as there are dependencies", footer)
	}

	f.result = &modrinth.AddResult{Project: "Sodium", File: "sodium-0.6.0.jar"}
	press(t, s, "d")
	if len(f.applied) != 1 || f.applied[0] {
		t.Errorf("the project was added with dependencies %v, want once, without them", f.applied)
	}
	if got := statusOf(s); got != "Added Sodium (sodium-0.6.0.jar)" {
		t.Errorf("the status line is %q, want it to say what was added", got)
	}
}

func TestAddDoesNotOfferToLeaveOutDependenciesThereAreNoneOf(t *testing.T) {
	f := newFakeAdd()
	f.plan.Dependencies = nil
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter", "enter")
	if footer := strings.Join(keyHelps(s.keys()), " "); strings.Contains(footer, "without dependencies") {
		t.Errorf("the keys are %q, want no offer to leave out dependencies when there are none", footer)
	}
	press(t, s, "d")
	if len(f.applied) != 0 || !s.modal() {
		t.Error("d answered a question that didn't offer it")
	}
}

// keyHelps are what the bindings say they do.
func keyHelps(bindings []key.Binding) []string {
	var out []string
	for _, b := range bindings {
		out = append(out, b.Help().Desc)
	}
	return out
}

func TestAddDoesNothingWhenYouSayNo(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter", "enter", "n")
	if len(f.applied) != 0 {
		t.Errorf("the project was added though the answer was no: %v", f.applied)
	}
	press(t, s, "enter", "esc")
	if len(f.applied) != 0 {
		t.Errorf("the project was added though the question was dismissed: %v", f.applied)
	}
}

func TestAddAddsAProjectGivenByItsAddressWithoutSearching(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "https://modrinth.com/mod/sodium")
	press(t, s, "enter")

	if len(f.searches) != 0 {
		t.Errorf("the searches are %v, want none for an address", f.searches)
	}
	if len(f.plans) != 1 || f.plans[0] != "https://modrinth.com/mod/sodium|" {
		t.Errorf("the plans are %v, want one for the address", f.plans)
	}
	if !strings.Contains(s.view(), "Add Sodium?") {
		t.Errorf("the screen doesn't ask whether to add the project:\n%s", s.view())
	}
}

func TestAddSearchesAgainWhenWhatIsTypedChanges(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter")
	typeText(t, s, " extra")
	press(t, s, "enter")

	if len(f.searches) != 2 || f.searches[1] != "mod:sodium extra" {
		t.Errorf("the searches are %v, want a second for what is typed now", f.searches)
	}
	if len(f.plans) != 0 {
		t.Errorf("the plans are %v, want none, as enter searched", f.plans)
	}
}

func TestAddMovesThroughTheResultsWithTheArrowKeysWhileTyping(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter", "down")
	if rows := body(t, s); !strings.HasPrefix(rows[3], "> Sodium Extra") {
		t.Errorf("the rows are %q, want the cursor moved to the second result", rows)
	}
	if got := s.query.String(); got != "sodium" {
		t.Errorf("the query is %q, want it untouched by moving", got)
	}
	press(t, s, "enter")
	if len(f.plans) != 1 || f.plans[0] != "PtjYWJkn|" {
		t.Errorf("the plans are %v, want one for the second result", f.plans)
	}
}

func TestAddSaysWhenNothingIsPicked(t *testing.T) {
	s := addOn(t, newFakeAdd())
	press(t, s, "enter")
	if got := statusOf(s); !strings.HasPrefix(got, "Nothing is picked") {
		t.Errorf("the status line is %q, want it to say nothing is picked", got)
	}
}

func TestAddSaysWhatItCannotAdd(t *testing.T) {
	for name, tc := range map[string]struct {
		plan *modrinth.AddPlan
		err  error
		want string
	}{
		"it is up to date":  {plan: &modrinth.AddPlan{Project: "Sodium", Existing: &modrinth.AddExisting{UpToDate: true}}, want: "Sodium is already added and up to date"},
		"it is pinned":      {plan: &modrinth.AddPlan{Project: "Sodium", Existing: &modrinth.AddExisting{Pinned: true}}, want: "Sodium is pinned; unpin it to allow updating"},
		"it can't be found": {err: errors.New("not found"), want: "not found"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeAdd()
			f.plan, f.planErr = tc.plan, tc.err
			s := addOn(t, f)
			typeText(t, s, "https://modrinth.com/mod/sodium")
			press(t, s, "enter")
			if got := statusOf(s); got != tc.want {
				t.Errorf("the status line is %q, want %q", got, tc.want)
			}
			if s.overlay != nil {
				t.Error("a question is open, though there is nothing to add")
			}
		})
	}
}

func TestAddOffersAnUpdateOfAProjectThePackHas(t *testing.T) {
	f := newFakeAdd()
	f.plan.Existing = &modrinth.AddExisting{Name: "Sodium", Current: "0.5.0"}
	f.result = &modrinth.AddResult{Project: "Sodium", File: "sodium-0.6.0.jar", Updated: true}
	s := addOn(t, f)
	typeText(t, s, "https://modrinth.com/mod/sodium")
	press(t, s, "enter")

	out := s.view()
	for _, want := range []string{"Update Sodium?", "now at", "0.5.0 -> 0.6.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("the question doesn't say %q:\n%s", want, out)
		}
	}
	press(t, s, "y")
	if got := statusOf(s); !strings.HasPrefix(got, "Updated Sodium") {
		t.Errorf("the status line is %q, want it to say the project was updated", got)
	}
}

func TestAddSaysWhenAddingFails(t *testing.T) {
	f := newFakeAdd()
	f.applyErr = errors.New("disk full")
	s := addOn(t, f)
	typeText(t, s, "https://modrinth.com/mod/sodium")
	press(t, s, "enter", "y")
	if got := statusOf(s); got != "disk full" {
		t.Errorf("the status line is %q, want the reason", got)
	}
}

func TestAddShowsWhatApplyingSaidAsAWarning(t *testing.T) {
	f := newFakeAdd()
	f.result.Notices = []string{"Notice: Lib is now on both sides, as Sodium needs it on the client"}
	s := addOn(t, f)
	typeText(t, s, "https://modrinth.com/mod/sodium")
	press(t, s, "enter", "y")
	if got := statusOf(s); !strings.Contains(got, "Lib is now on both sides") {
		t.Errorf("the status line is %q, want what adding said", got)
	}
}

func TestAddMarksTheProjectAsInThePackOnceItIs(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter", "enter", "y")
	if rows := body(t, s); !strings.Contains(rows[2], "in the pack") {
		t.Errorf("the first result is %q, want it marked as in the pack now", rows[2])
	}
}

func TestAddSearchesForTheKindOfProjectThatWasPicked(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter", "tab")
	if out := s.view(); !strings.Contains(out, "resource packs") || strings.Contains(out, "Sodium Extra") {
		t.Errorf("the screen doesn't say it is for resource packs, or still shows what was found for mods:\n%s", out)
	}
	press(t, s, "enter")
	if len(f.searches) != 2 || f.searches[1] != "resourcepack:sodium" {
		t.Errorf("the searches are %v, want the second for resource packs", f.searches)
	}
	press(t, s, "tab", "tab")
	if out := s.view(); !strings.Contains(out, "Add · mods") {
		t.Errorf("after going through all three kinds the screen doesn't say it is for mods again:\n%s", out)
	}
}

func TestAddPassesTheReleaseTypeThatWasPicked(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "https://modrinth.com/mod/sodium")
	press(t, s, "ctrl+r")
	if out := s.view(); !strings.Contains(out, "release or more stable") {
		t.Errorf("the screen doesn't say which release type is picked:\n%s", out)
	}
	press(t, s, "ctrl+r", "enter", "n")
	if len(f.plans) != 1 || !strings.HasSuffix(f.plans[0], "|beta") {
		t.Errorf("the plans are %v, want the release type that was picked", f.plans)
	}
	press(t, s, "ctrl+r", "ctrl+r")
	if out := s.view(); strings.Contains(out, "or more stable") {
		t.Errorf("after going through them all the screen still says a release type is picked:\n%s", out)
	}
}

func TestAddKeysWorkOnTheResultsOnceTheBoxIsLeft(t *testing.T) {
	f := newFakeAdd()
	s := addOn(t, f)
	typeText(t, s, "sodium")
	press(t, s, "enter", "esc", "j")
	if rows := body(t, s); !strings.HasPrefix(rows[3], "> Sodium Extra") {
		t.Errorf("the rows are %q, want j to move the cursor once the box is left", rows)
	}
	press(t, s, "t")
	if out := s.view(); !strings.Contains(out, "resource packs") {
		t.Errorf("t didn't change the kind of project:\n%s", out)
	}
	press(t, s, "r")
	if out := s.view(); !strings.Contains(out, "release or more stable") {
		t.Errorf("r didn't change the release type:\n%s", out)
	}
	press(t, s, "enter")
	if got := statusOf(s); !strings.HasPrefix(got, "Nothing is picked") {
		t.Errorf("the status line is %q, want enter to have nothing to add, as changing the kind cleared the results", got)
	}
}

func TestAddPastesIntoTheQuery(t *testing.T) {
	s := addOn(t, newFakeAdd())
	feed(t, s, tea.PasteMsg{Content: "https://modrinth.com/mod/sodium\n"})
	if got := s.query.String(); got != "https://modrinth.com/mod/sodium " {
		t.Errorf("the query is %q, want what was pasted, with its line break as a space", got)
	}
}

// What the real backend does, against a Modrinth that answers for a project and its library.
func addableFixture(t *testing.T) {
	t.Helper()
	setUpPack(t)
	withLoader(t)
	httpmock.Activate(t)
	project := func(id, slug, title, client, server string) map[string]any {
		return map[string]any{"id": id, "slug": slug, "title": title, "project_type": "mod", "client_side": client, "server_side": server, "versions": []string{"v-" + id}}
	}
	version := func(id, file string, requires ...string) map[string]any {
		deps := []map[string]string{}
		for _, r := range requires {
			deps = append(deps, map[string]string{"project_id": r, "dependency_type": "required"})
		}
		return map[string]any{
			"id": "v-" + id, "project_id": id, "version_number": "1.0.0", "version_type": "release", "date_published": "2024-01-01T00:00:00Z",
			"dependencies": deps,
			"files":        []map[string]any{{"url": "https://cdn.modrinth.com/" + file, "filename": file, "primary": true, "hashes": map[string]string{"sha512": "h-" + id}}},
		}
	}
	main, lib := project("main1", "main-slug", "Main Mod", "required", "required"), project("lib1", "lib-slug", "Lib Mod", "unsupported", "required")
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/main-slug", httpmock.NewJsonResponderOrPanic(200, main))
	httpmock.RegisterResponder("GET", "https://api.modrinth.com/v2/project/main1", httpmock.NewJsonResponderOrPanic(200, main))
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/main1/version`, httpmock.NewJsonResponderOrPanic(200, []any{version("main1", "main.jar", "lib1")}))
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/project/lib1/version`, httpmock.NewJsonResponderOrPanic(200, []any{version("lib1", "lib.jar")}))
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/projects`, func(*http.Request) (*http.Response, error) {
		return httpmock.NewJsonResponse(200, []any{lib})
	})
	httpmock.RegisterResponder("GET", `=~^https://api\.modrinth\.com/v2/search`, httpmock.NewStringResponder(200,
		`{"hits":[{"project_id":"main1","slug":"main-slug","title":"Main Mod","description":"Does things","author":"someone","downloads":12}],"total_hits":1}`))
}

func TestAddAddsAProjectAndItsLibraryToThePack(t *testing.T) {
	addableFixture(t)
	s := newAddScreen(packBackend{})
	s.setSize(100, 24)
	press(t, s, "/")

	out := cmdtest.CaptureStdout(t, func() {
		typeText(t, s, "main")
		press(t, s, "enter")
		press(t, s, "enter")
		if view := s.view(); !strings.Contains(view, "Add Main Mod?") || !strings.Contains(view, "Lib Mod 1.0.0") {
			t.Errorf("the question doesn't say what would be added:\n%s", view)
		}
		press(t, s, "y")
	})
	if out != "" {
		t.Errorf("the add screen wrote %q to the terminal, which would be drawn over the screen", out)
	}

	if got := statusOf(s); !strings.HasPrefix(got, "Added Main Mod (main.jar) with Lib Mod") {
		t.Errorf("the status line is %q, want it to say what was added", got)
	}
	main, err := core.LoadMod("mods/main-slug.pw.toml")
	if err != nil || main.FileName != "main.jar" || main.AddedAsDependency {
		t.Fatalf("the project is %+v (%v), want it added as a main mod", main, err)
	}
	lib, err := core.LoadMod("mods/lib-slug.pw.toml")
	if err != nil || !lib.AddedAsDependency || lib.Side != core.UniversalSide {
		t.Fatalf("the library is %+v (%v), want it added as a dependency, on both sides as the project needs it on the client", lib, err)
	}
	assertIndexIsConsistent(t)
}

func TestAddSaysWhenAProjectIsAlreadyInThePack(t *testing.T) {
	addableFixture(t)
	s := newAddScreen(packBackend{})
	s.setSize(100, 24)
	press(t, s, "/")
	typeText(t, s, "https://modrinth.com/mod/main-slug")
	press(t, s, "enter", "d")
	if got := statusOf(s); !strings.HasPrefix(got, "Added Main Mod") {
		t.Fatalf("the status line is %q, want the project added", got)
	}

	press(t, s, "enter")
	if got := statusOf(s); got != "Main Mod is already added and up to date" {
		t.Errorf("the status line is %q, want it to say the project is already there", got)
	}
}

func TestAddSearchMarksWhatThePackHas(t *testing.T) {
	addableFixture(t)
	s := newAddScreen(packBackend{})
	s.setSize(100, 24)
	press(t, s, "/")
	typeText(t, s, "main")
	press(t, s, "enter")
	if rows := body(t, s); !strings.Contains(rows[2], "Main Mod") || strings.Contains(rows[2], "in the pack") {
		t.Errorf("the result is %q, want it without a mark as the pack hasn't got it", rows[2])
	}
}
