package git

import (
	"regexp"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/core"
)

const wantBreakingFooter = "BREAKING CHANGE: changes mods that run on the server; servers must be updated to match"

func TestMessage(t *testing.T) {
	tests := []struct {
		name    string
		changes []changelog.Change
		want    string
	}{
		{"no changes", nil, "chore(pack): update pack files"},

		{
			"client mod added",
			[]changelog.Change{{Kind: changelog.ModAdded, Name: "Sodium", Side: core.ClientSide, To: "0.5.7"}},
			"feat(mods): add Sodium 0.5.7 (client)",
		},
		{
			"client mod removed",
			[]changelog.Change{{Kind: changelog.ModRemoved, Name: "Sodium", Side: core.ClientSide, From: "0.5.7"}},
			"feat(mods): remove Sodium 0.5.7 (client)",
		},
		{
			"client mod updated",
			[]changelog.Change{{Kind: changelog.ModUpdated, Name: "Iris", Side: core.ClientSide, From: "1.7.0", To: "1.7.1"}},
			"fix(mods): update Iris 1.7.0 -> 1.7.1 (client)",
		},
		{
			"server mod added",
			[]changelog.Change{{Kind: changelog.ModAdded, Name: "Lithium", Side: core.ServerSide, To: "0.12.0"}},
			"feat(mods)!: add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter,
		},
		{
			"both mod updated",
			[]changelog.Change{{Kind: changelog.ModUpdated, Name: "Fabric API", Side: core.UniversalSide, From: "0.100", To: "0.101"}},
			"feat(mods)!: update Fabric API 0.100 -> 0.101 (both)\n\n" + wantBreakingFooter,
		},
		{
			"sideless mod is both",
			[]changelog.Change{{Kind: changelog.ModAdded, Name: "Legacy", To: "1.0"}},
			"feat(mods)!: add Legacy 1.0 (both)\n\n" + wantBreakingFooter,
		},
		{
			"mod without a version",
			[]changelog.Change{{Kind: changelog.ModAdded, Name: "Sodium", Side: core.ClientSide}},
			"feat(mods): add Sodium (client)",
		},

		{"config changed", []changelog.Change{{Kind: changelog.FileChanged, Path: "config/sodium.json"}}, "fix(config): change config/sodium.json"},
		{"config added", []changelog.Change{{Kind: changelog.FileAdded, Path: "config/sodium.json"}}, "fix(config): add config/sodium.json"},
		{"config removed", []changelog.Change{{Kind: changelog.FileRemoved, Path: "config/sodium.json"}}, "fix(config): remove config/sodium.json"},

		{
			"several mods",
			[]changelog.Change{
				{Kind: changelog.ModAdded, Name: "Iris", Side: core.ClientSide, To: "1.7.0"},
				{Kind: changelog.ModAdded, Name: "Sodium", Side: core.ClientSide, To: "0.5.7"},
				{Kind: changelog.ModRemoved, Name: "Old", Side: core.ClientSide, From: "1.0"},
				{Kind: changelog.ModUpdated, Name: "Zoom", Side: core.ClientSide, From: "1", To: "2"},
			},
			"feat(mods): add 2 mods, remove 1 mod, update 1 mod\n\n" +
				"- add Iris 1.7.0 (client)\n- add Sodium 0.5.7 (client)\n- remove Old 1.0 (client)\n- update Zoom 1 -> 2 (client)",
		},
		{
			"several config files",
			[]changelog.Change{
				{Kind: changelog.FileAdded, Path: "config/a.json"},
				{Kind: changelog.FileRemoved, Path: "config/b.json"},
			},
			"fix(config): update 2 config files\n\n- add config/a.json\n- remove config/b.json",
		},
		{
			"mods and config together",
			[]changelog.Change{
				{Kind: changelog.ModAdded, Name: "Iris", Side: core.ClientSide, To: "1.7.0"},
				{Kind: changelog.FileChanged, Path: "config/iris.json"},
			},
			"feat(pack): add 1 mod, update 1 config file\n\n- add Iris 1.7.0 (client)\n- change config/iris.json",
		},
		{
			"the most significant change decides the type and the footer follows the body",
			[]changelog.Change{
				{Kind: changelog.FileChanged, Path: "config/a.json"},
				{Kind: changelog.ModAdded, Name: "Lithium", Side: core.ServerSide, To: "0.12.0"},
			},
			"feat(pack)!: add 1 mod, update 1 config file\n\n- change config/a.json\n- add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Message(tt.changes); got != tt.want {
				t.Errorf("Message() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// The commit type is the version bump written another way, so the two must never disagree
func TestMessageTypeAgreesWithBump(t *testing.T) {
	kinds := []changelog.Change{
		{Kind: changelog.ModAdded, Name: "A", Side: core.ClientSide, To: "1"},
		{Kind: changelog.ModRemoved, Name: "B", Side: core.ClientSide, From: "1"},
		{Kind: changelog.ModUpdated, Name: "C", Side: core.ClientSide, From: "1", To: "2"},
		{Kind: changelog.ModAdded, Name: "D", Side: core.ServerSide, To: "1"},
		{Kind: changelog.ModUpdated, Name: "E", Side: core.UniversalSide, From: "1", To: "2"},
		{Kind: changelog.FileChanged, Path: "config/f.json"},
		{Kind: changelog.FileAdded, Path: "config/g.json"},
	}
	header := regexp.MustCompile(`^(feat|fix)\((mods|config|pack)\)(!?): \S.*$`)

	// Every non-empty combination of the changes above
	for mask := 1; mask < 1<<len(kinds); mask++ {
		var changes []changelog.Change
		for i, c := range kinds {
			if mask&(1<<i) != 0 {
				changes = append(changes, c)
			}
		}
		message := Message(changes)
		first, _, _ := strings.Cut(message, "\n")

		m := header.FindStringSubmatch(first)
		if m == nil {
			t.Fatalf("header %q is not a conventional commit header (changes %+v)", first, changes)
		}
		commitType, marker := m[1], m[3]

		bump := changelog.HighestBump(changes)
		wantType := map[changelog.Bump]string{changelog.BumpPatch: "fix", changelog.BumpMinor: "feat", changelog.BumpMajor: "feat"}[bump]
		if commitType != wantType {
			t.Errorf("bump %v gave commit type %q, want %q (changes %+v)", bump, commitType, wantType, changes)
		}
		if (marker == "!") != (bump == changelog.BumpMajor) {
			t.Errorf("bump %v gave marker %q; only major changes are breaking (changes %+v)", bump, marker, changes)
		}
		if strings.Contains(message, "BREAKING CHANGE") != (bump == changelog.BumpMajor) {
			t.Errorf("bump %v: BREAKING CHANGE footer presence is wrong in:\n%s", bump, message)
		}
	}
}

func TestReleaseMessageAndTag(t *testing.T) {
	if got := ReleaseMessage("1.2.3"); got != "chore(release): 1.2.3" {
		t.Errorf("ReleaseMessage() = %q", got)
	}
	if got := TagName("1.2.3"); got != "v1.2.3" {
		t.Errorf("TagName() = %q", got)
	}
	// Tags must be readable back as versions, since that is how a release is found again
	v, err := changelog.ParseVersion(TagName("1.2.3"))
	if err != nil || v.String() != "1.2.3" {
		t.Errorf("ParseVersion(TagName()) = %v, %v", v, err)
	}
}

func TestIndexFileRejectsAbsolutePath(t *testing.T) {
	var pack core.Pack
	pack.Index.File = "/somewhere/index.toml"
	if _, err := indexFile(pack); err == nil {
		t.Error("indexFile() accepted an absolute path, which git can't look up")
	}

	pack.Index.File = "sub/../index.toml"
	if got, err := indexFile(pack); err != nil || got != "index.toml" {
		t.Errorf("indexFile() = %q, %v, want the cleaned path index.toml", got, err)
	}
}
