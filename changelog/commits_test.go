package changelog

import (
	"reflect"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

// commit is a commit with the given message: the subject, then the lines of the body
func commit(subject string, body ...string) Commit {
	return Commit{Subject: subject, Body: strings.Join(body, "\n")}
}

func TestChangesFromCommits(t *testing.T) {
	const breaking = "BREAKING CHANGE: changes mods that run on the server; servers must be updated to match"

	tests := []struct {
		name    string
		commits []Commit
		want    []Change
	}{
		{
			"a mod added",
			[]Commit{commit("feat(mods): add Sodium 0.5.7 (client)")},
			[]Change{{Kind: ModAdded, Name: "Sodium", Side: core.ClientSide, To: "0.5.7"}},
		},
		{
			"a mod removed",
			[]Commit{commit("feat(mods): remove Sodium 0.5.7 (client)")},
			[]Change{{Kind: ModRemoved, Name: "Sodium", Side: core.ClientSide, From: "0.5.7"}},
		},
		{
			"a mod updated",
			[]Commit{commit("fix(mods): update Iris 1.7.0 -> 1.7.1 (client)")},
			[]Change{{Kind: ModUpdated, Name: "Iris", Side: core.ClientSide, From: "1.7.0", To: "1.7.1"}},
		},
		{
			"a server mod, whose commit says it is breaking",
			[]Commit{commit("feat(mods)!: add Lithium 0.12.0 (server)", "", breaking)},
			[]Change{{Kind: ModAdded, Name: "Lithium", Side: core.ServerSide, To: "0.12.0"}},
		},
		{
			"a mod on both sides",
			[]Commit{commit("feat(mods)!: add Fabric API 0.100 (both)", "", breaking)},
			[]Change{{Kind: ModAdded, Name: "Fabric API", Side: core.UniversalSide, To: "0.100"}},
		},
		{
			"a name with spaces and a version with symbols",
			[]Commit{commit("feat(mods): add Iris Shaders 1.8.12+1.21.1-neoforge (client)")},
			[]Change{{Kind: ModAdded, Name: "Iris Shaders", Side: core.ClientSide, To: "1.8.12+1.21.1-neoforge"}},
		},
		{
			"an update of a name with spaces",
			[]Commit{commit("fix(mods): update Iris Shaders 1.8.11 -> 1.8.12+1.21.1 (client)")},
			[]Change{{Kind: ModUpdated, Name: "Iris Shaders", Side: core.ClientSide, From: "1.8.11", To: "1.8.12+1.21.1"}},
		},
		{
			"a file name as the version, for a mod that never recorded one",
			[]Commit{commit("fix(mods): update Iris iris-1.7.0.jar -> iris-1.7.1.jar (client)")},
			[]Change{{Kind: ModUpdated, Name: "Iris", Side: core.ClientSide, From: "iris-1.7.0.jar", To: "iris-1.7.1.jar"}},
		},
		{
			"a mod with no version",
			[]Commit{commit("feat(mods): add Sodium (client)")},
			[]Change{{Kind: ModAdded, Name: "Sodium", Side: core.ClientSide}},
		},
		{
			"a config file changed",
			[]Commit{commit("fix(config): change config/sodium.json")},
			[]Change{{Kind: FileChanged, Path: "config/sodium.json"}},
		},
		{
			"a config file added and one removed",
			[]Commit{commit("fix(config): add config/a.toml"), commit("fix(config): remove config/b.toml")},
			[]Change{{Kind: FileAdded, Path: "config/a.toml"}, {Kind: FileRemoved, Path: "config/b.toml"}},
		},
		{
			"several config files in one commit, listed in its body",
			[]Commit{commit("fix(config): update 2 config files", "", "- add config/a.json", "- remove config/b.json")},
			[]Change{{Kind: FileAdded, Path: "config/a.json"}, {Kind: FileRemoved, Path: "config/b.json"}},
		},
		{
			// What packwiz git commit wrote before it made a commit for each mod
			"a commit with mods and config together, listed in its body",
			[]Commit{commit("feat(pack)!: add 1 mod, update 1 config file", "",
				"- change config/iris.json", "- add Lithium 0.12.0 (server)", "", breaking)},
			[]Change{
				{Kind: FileChanged, Path: "config/iris.json"},
				{Kind: ModAdded, Name: "Lithium", Side: core.ServerSide, To: "0.12.0"},
			},
		},

		{
			"a note about config written by hand",
			[]Commit{commit("fix(config): lower the particle count")},
			[]Change{{Kind: Note, Type: "fix", Scope: "config", Text: "lower the particle count"}},
		},
		{
			"a note that is a single word is not mistaken for a file",
			[]Commit{commit("fix(config): change render-distance")},
			[]Change{{Kind: Note, Type: "fix", Scope: "config", Text: "change render-distance"}},
		},
		{
			"a feature with no scope",
			[]Commit{commit("feat: add a splash screen")},
			[]Change{{Kind: Note, Type: "feat", Text: "add a splash screen"}},
		},
		{
			"a breaking change with no scope",
			[]Commit{commit("feat!: update to Minecraft 1.21.4")},
			[]Change{{Kind: Note, Type: "feat", Text: "update to Minecraft 1.21.4", Breaking: true}},
		},
		{
			"a breaking change in a footer",
			[]Commit{commit("fix(world): reset the spawn point", "", "Players will lose their beds.", "", "BREAKING CHANGE: worlds must be reset")},
			[]Change{{Kind: Note, Type: "fix", Scope: "world", Text: "reset the spawn point", Breaking: true}},
		},
		{
			"perf counts as a fix",
			[]Commit{commit("perf: cut the load time")},
			[]Change{{Kind: Note, Type: "perf", Text: "cut the load time"}},
		},
		{
			// packwiz's format is only structure in the scope packwiz writes it in, so a note that happens to end in
			// "(client)" isn't taken for a mod called "a shader"
			"a note that happens to look like a mod's line",
			[]Commit{commit("feat: add a shader pack (client)")},
			[]Change{{Kind: Note, Type: "feat", Text: "add a shader pack (client)"}},
		},
		{
			"a body with bullets that aren't packwiz's leaves the note as the subject",
			[]Commit{commit("feat(shaders): add three packs", "", "- Complementary", "- BSL", "- Photon")},
			[]Change{{Kind: Note, Type: "feat", Scope: "shaders", Text: "add three packs"}},
		},

		{
			"commits that don't make a release",
			[]Commit{
				commit("chore(pack): update pack files"),
				commit("chore(pack): initial commit"),
				commit("chore(release): 1.2.0"),
				commit("docs: explain the config"),
				commit("style: tidy"),
				commit("refactor: reorganise"),
				commit("test: try it"),
				commit("ci: build"),
				commit("build: build"),
			},
			nil,
		},
		{
			"a chore that says it is breaking does",
			[]Commit{commit("chore!: drop the old launcher")},
			[]Change{{Kind: Note, Type: "chore", Text: "drop the old launcher", Breaking: true}},
		},
		{
			"commits that aren't conventional",
			[]Commit{commit("Fix a typo"), commit("Merge branch 'main'"), commit(""), commit("feat add a thing"), commit("feat(: broken")},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChangesFromCommits(tt.commits); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChangesFromCommits() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestChangesFromCommitsBumps(t *testing.T) {
	tests := []struct {
		subject string
		want    Bump
	}{
		{"feat(mods)!: add Lithium 0.12.0 (server)", BumpMajor},
		{"feat(mods)!: update Fabric API 0.100 -> 0.101 (both)", BumpMajor},
		{"feat(mods): add Sodium 0.5.7 (client)", BumpMinor},
		{"fix(mods): update Iris 1.7.0 -> 1.7.1 (client)", BumpPatch},
		{"fix(config): change config/sodium.json", BumpPatch},
		{"feat: something new", BumpMinor},
		{"fix: something wrong", BumpPatch},
		{"feat!: something breaking", BumpMajor},
		{"chore: nothing", BumpNone},
	}
	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			if got := HighestBump(ChangesFromCommits([]Commit{commit(tt.subject)})); got != tt.want {
				t.Errorf("bump = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSquash(t *testing.T) {
	add := func(v string) Commit { return commit("feat(mods): add Sodium " + v + " (client)") }
	remove := func(v string) Commit { return commit("feat(mods): remove Sodium " + v + " (client)") }
	update := func(from, to string) Commit {
		return commit("fix(mods): update Sodium " + from + " -> " + to + " (client)")
	}
	sodium := func(kind Kind, from, to string) Change {
		return Change{Kind: kind, Name: "Sodium", Side: core.ClientSide, From: from, To: to}
	}

	tests := []struct {
		name    string
		commits []Commit
		want    []Change
	}{
		{"added, then updated: added as it ended up", []Commit{add("1"), update("1", "2")}, []Change{sodium(ModAdded, "", "2")}},
		{"added, then updated twice", []Commit{add("1"), update("1", "2"), update("2", "3")}, []Change{sodium(ModAdded, "", "3")}},
		{"added, then removed: nothing", []Commit{add("1"), remove("1")}, nil},
		{"added, updated, then removed: nothing", []Commit{add("1"), update("1", "2"), remove("2")}, nil},
		{"updated twice: one update", []Commit{update("1", "2"), update("2", "3")}, []Change{sodium(ModUpdated, "1", "3")}},
		{"updated and put back: nothing", []Commit{update("1", "2"), update("2", "1")}, nil},
		{"updated, then removed: removed from where it began", []Commit{update("1", "2"), remove("2")}, []Change{sodium(ModRemoved, "1", "")}},
		{"removed, then added again: an update", []Commit{remove("1"), add("2")}, []Change{sodium(ModUpdated, "1", "2")}},
		{"removed, then added again as it was: nothing", []Commit{remove("1"), add("1")}, nil},
		{"added, removed, then added again", []Commit{add("1"), remove("1"), add("2")}, []Change{sodium(ModAdded, "", "2")}},
		{"removed, added, then removed again", []Commit{remove("1"), add("2"), remove("2")}, []Change{sodium(ModRemoved, "1", "")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChangesFromCommits(tt.commits); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChangesFromCommits() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestSquashFiles(t *testing.T) {
	add := commit("fix(config): add config/a.json")
	change := commit("fix(config): change config/a.json")
	remove := commit("fix(config): remove config/a.json")
	file := func(kind Kind) []Change { return []Change{{Kind: kind, Path: "config/a.json"}} }

	tests := []struct {
		name    string
		commits []Commit
		want    []Change
	}{
		{"added, then changed: added", []Commit{add, change}, file(FileAdded)},
		{"added, then removed: nothing", []Commit{add, remove}, nil},
		{"changed, then removed: removed", []Commit{change, remove}, file(FileRemoved)},
		{"removed, then added again: changed", []Commit{remove, add}, file(FileChanged)},
		{"changed twice: once", []Commit{change, change}, file(FileChanged)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChangesFromCommits(tt.commits); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChangesFromCommits() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestSquashKeepsModsApartAndNotesAsTheyWere(t *testing.T) {
	got := ChangesFromCommits([]Commit{
		commit("fix: first note"),
		commit("feat(mods): add Sodium 1 (client)"),
		commit("feat(mods): add Iris 2 (client)"),
		commit("fix: second note"),
		commit("fix(mods): update Sodium 1 -> 3 (client)"),
		commit("fix: first note"), // the same again is still said twice
	})

	want := []Change{
		// Mods in the order they first appeared, then the notes in the order they were made
		{Kind: ModAdded, Name: "Sodium", Side: core.ClientSide, To: "3"},
		{Kind: ModAdded, Name: "Iris", Side: core.ClientSide, To: "2"},
		{Kind: Note, Type: "fix", Text: "first note"},
		{Kind: Note, Type: "fix", Text: "second note"},
		{Kind: Note, Type: "fix", Text: "first note"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChangesFromCommits() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSquashedBumpIsTheNetEffect(t *testing.T) {
	// A server mod that was added and removed again before the release changed nothing for anyone's server
	got := ChangesFromCommits([]Commit{
		commit("feat(mods)!: add Lithium 0.12.0 (server)", "", "BREAKING CHANGE: servers must be updated"),
		commit("feat(mods)!: remove Lithium 0.12.0 (server)", "", "BREAKING CHANGE: servers must be updated"),
		commit("fix(config): change config/a.json"),
	})
	if bump := HighestBump(got); bump != BumpPatch {
		t.Errorf("bump = %v, want patch: the server mod came and went", bump)
	}
}

func TestNoteBump(t *testing.T) {
	tests := []struct {
		name string
		note Change
		want Bump
	}{
		{"breaking", Change{Kind: Note, Type: "chore", Breaking: true}, BumpMajor},
		{"feature", Change{Kind: Note, Type: "feat"}, BumpMinor},
		{"fix", Change{Kind: Note, Type: "fix"}, BumpPatch},
		{"perf", Change{Kind: Note, Type: "perf"}, BumpPatch},
		{"chore", Change{Kind: Note, Type: "chore"}, BumpNone},
		{"docs", Change{Kind: Note, Type: "docs"}, BumpNone},
		{"unknown type", Change{Kind: Note, Type: "whatever"}, BumpNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.note.Bump(); got != tt.want {
				t.Errorf("Bump() = %v, want %v", got, tt.want)
			}
		})
	}
}
