package tui

import (
	"slices"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/fuzzy"
)

// rowTree is a tree of the kind core.Index.ConfigFileTree makes: two mods and a file that no mod claims. Alpha has two
// files and an entry that matches none, Beta has only an entry that matches none.
func rowTree() core.ConfigFileTree {
	return core.ConfigFileTree{
		Mods: []core.ModConfigFiles{
			{Mod: fakeMod("Alpha", "alpha"), Files: []string{"config/a.json", "config/b.json"}, Missing: []string{"config/gone.json"}},
			{Mod: fakeMod("Beta", "beta"), Missing: []string{"config/old.json"}},
		},
		Unclaimed: []string{"config/orphan.json"},
	}
}

// describe writes rows the way a test can compare them: a group as "> title (count)" and the rest by their path,
// "missing:" first if it is an entry that matches no file.
func describe(rows []row) []string {
	var out []string
	for _, r := range rows {
		switch r.kind {
		case groupRow:
			s := "> " + r.title
			if r.folded {
				s += " (folded)"
			}
			out = append(out, s)
		case fileRow:
			out = append(out, r.path)
		case missingRow:
			out = append(out, "missing:"+r.path)
		}
	}
	return out
}

func TestBuildRowsShowsTheTreeThatConfigListPrints(t *testing.T) {
	got := describe(buildRows(rowTree(), allStates, nil, fuzzy.Query{}))
	want := []string{
		"> Alpha", "config/a.json", "config/b.json", "missing:config/gone.json",
		"> Beta", "missing:config/old.json",
		"> Invalid", "config/orphan.json",
	}
	if !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestBuildRowsShowsOnlyWhatTheFilterAllows(t *testing.T) {
	tests := []struct {
		filter stateFilter
		want   []string
	}{
		{validState, []string{"> Alpha", "config/a.json", "config/b.json"}},
		{invalidState, []string{"> Invalid", "config/orphan.json"}},
		// A mod with nothing to show isn't a group, as it isn't in the command's output
		{missingState, []string{"> Alpha", "missing:config/gone.json", "> Beta", "missing:config/old.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.filter.String(), func(t *testing.T) {
			if got := describe(buildRows(rowTree(), tt.filter, nil, fuzzy.Query{})); !slices.Equal(got, tt.want) {
				t.Errorf("rows = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildRowsOfNothingIsNothing(t *testing.T) {
	if rows := buildRows(core.ConfigFileTree{}, allStates, nil, fuzzy.Query{}); len(rows) != 0 {
		t.Errorf("rows = %v, want none", describe(rows))
	}
}

func TestBuildRowsCountsAndMarksTheLastRowOfAGroup(t *testing.T) {
	rows := buildRows(rowTree(), allStates, nil, fuzzy.Query{})
	alpha := rows[0]
	if alpha.count != 3 || alpha.folded {
		t.Errorf("Alpha's group has count %d and folded %v, want 3 and false", alpha.count, alpha.folded)
	}
	var last []string
	for _, r := range rows {
		if r.last {
			last = append(last, r.path)
		}
	}
	if want := []string{"config/gone.json", "config/old.json", "config/orphan.json"}; !slices.Equal(last, want) {
		t.Errorf("the last rows of the groups are %v, want %v", last, want)
	}
}

// A group that is folded away keeps its files, as they are still its own: relating the group relates them
func TestBuildRowsFoldedGroupHasOnlyItsHeadingButKeepsItsFiles(t *testing.T) {
	rows := buildRows(rowTree(), allStates, map[string]bool{"mods/alpha.pw.toml": true, invalidGroup: true}, fuzzy.Query{})
	want := []string{"> Alpha (folded)", "> Beta", "missing:config/old.json", "> Invalid (folded)"}
	if got := describe(rows); !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if !slices.Equal(rows[0].files, []string{"config/a.json", "config/b.json"}) || rows[0].count != 3 {
		t.Errorf("the folded group has files %v and count %d, want its two files and 3", rows[0].files, rows[0].count)
	}
	if !slices.Equal(rows[len(rows)-1].files, []string{"config/orphan.json"}) {
		t.Errorf("the folded Invalid group has files %v, want its one file", rows[len(rows)-1].files)
	}
}

func TestBuildRowsGroupFilesAreOnlyFilesNotEntriesThatMatchNone(t *testing.T) {
	rows := buildRows(rowTree(), allStates, nil, fuzzy.Query{})
	if beta := rows[4]; beta.title != "Beta" || len(beta.files) != 0 {
		t.Errorf("Beta's group is %+v, want one without files (its only entry matches none)", beta)
	}
}

func TestBuildRowsKeysAreUniqueEvenForAFileThatTwoModsClaim(t *testing.T) {
	tree := core.ConfigFileTree{Mods: []core.ModConfigFiles{
		{Mod: fakeMod("Alpha", "alpha"), Files: []string{"config/shared.json"}},
		{Mod: fakeMod("Beta", "beta"), Files: []string{"config/shared.json"}},
	}}
	seen := map[rowKey]bool{}
	for _, r := range buildRows(tree, allStates, nil, fuzzy.Query{}) {
		if seen[r.key()] {
			t.Errorf("two rows have the key %+v", r.key())
		}
		seen[r.key()] = true
	}
}

func TestStateFilterCyclesThroughEveryStateAndBack(t *testing.T) {
	var got []string
	f := allStates
	for range 5 {
		got = append(got, f.String())
		f = f.next()
	}
	if want := []string{"all", "valid", "invalid", "missing", "all"}; !slices.Equal(got, want) {
		t.Errorf("cycling gives %v, want %v", got, want)
	}
}

func TestCountStates(t *testing.T) {
	got := countStates(rowTree())
	if want := (stateCounts{valid: 2, invalid: 1, missing: 2}); got != want {
		t.Errorf("countStates() = %+v, want %+v", got, want)
	}
}

// Searching keeps the rows whose path, or whose group's name, has every word, fuzzily
func TestBuildRowsWithASearchKeepsOnlyWhatMatches(t *testing.T) {
	for _, tt := range []struct {
		name, search string
		want         []string
	}{
		{"a word in a path", "orph", []string{"> Invalid", "config/orphan.json"}},
		// Characters in order, not together: a substring search wouldn't find it
		{"fuzzily", "cfgorph", []string{"> Invalid", "config/orphan.json"}},
		{"a word in a group's name finds everything in the group", "alpha", []string{"> Alpha", "config/a.json", "config/b.json", "missing:config/gone.json"}},
		{"a word of each", "alpha gone", []string{"> Alpha", "missing:config/gone.json"}},
		{"in either order", "gone alpha", []string{"> Alpha", "missing:config/gone.json"}},
		{"a word of a path in a group", "alpha b.j", []string{"> Alpha", "config/b.json"}},
		{"whatever the case", "ORPH", []string{"> Invalid", "config/orphan.json"}},
		{"every word has to match", "orph alpha", nil},
		{"nothing matches", "zzz", nil},
		{"entries that match no file are searched too", "old", []string{"> Beta", "missing:config/old.json"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := describe(buildRows(rowTree(), allStates, nil, fuzzy.Parse(tt.search)))
			if !slices.Equal(got, tt.want) {
				t.Errorf("rows = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildRowsWithASearchCountsAndListsOnlyWhatMatches(t *testing.T) {
	rows := buildRows(rowTree(), allStates, nil, fuzzy.Parse("alpha b.j"))
	if len(rows) != 2 || rows[0].count != 1 || !slices.Equal(rows[0].files, []string{"config/b.json"}) {
		t.Errorf("rows = %+v, want a group of the one file that matches, which is what relating the group would relate", rows)
	}
	if !rows[1].last {
		t.Error("the one row that matches isn't the last of its group")
	}
}

func TestBuildRowsWithASearchSaysWhereItMatched(t *testing.T) {
	rows := buildRows(rowTree(), allStates, nil, fuzzy.Parse("orph"))
	file := rows[1]
	if !slices.Equal(file.positions, []int{7, 8, 9, 10}) || file.score <= 0 {
		t.Errorf("the file's positions are %v and its score %d, want orph in config/orphan.json to be shown", file.positions, file.score)
	}
	if len(rows[0].positions) != 0 {
		t.Errorf("the group's positions are %v, want none: nothing matched its name", rows[0].positions)
	}

	// A word found in the name of the group is shown in the name, and not in the files that it finds
	rows = buildRows(rowTree(), allStates, nil, fuzzy.Parse("alpha"))
	if !slices.Equal(rows[0].positions, []int{0, 1, 2, 3, 4}) {
		t.Errorf("the group's positions are %v, want its whole name", rows[0].positions)
	}
	if !slices.Equal(rows[3].positions, nil) || rows[3].path != "config/gone.json" && rows[3].path != "config/b.json" {
		t.Errorf("row %+v: want a file that matched by its group's name to have nothing shown in its path", rows[3])
	}
}

// A group that is folded hides what a search found, so a search shows it all, and it is folded again once it is gone
func TestBuildRowsWithASearchIgnoresFolding(t *testing.T) {
	folded := map[string]bool{"mods/alpha.pw.toml": true}
	if got := describe(buildRows(rowTree(), allStates, folded, fuzzy.Query{})); !slices.Contains(got, "> Alpha (folded)") || slices.Contains(got, "config/a.json") {
		t.Fatalf("rows = %v, want Alpha folded without a search", got)
	}
	got := describe(buildRows(rowTree(), allStates, folded, fuzzy.Parse("b.json")))
	if want := []string{"> Alpha", "config/b.json"}; !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v: the group isn't folded while there is a search", got, want)
	}
}

func TestBuildRowsWithASearchAppliesToWhatTheStateFilterShows(t *testing.T) {
	got := describe(buildRows(rowTree(), missingState, nil, fuzzy.Parse("alpha")))
	if want := []string{"> Alpha", "missing:config/gone.json"}; !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v: only what is missing, of what matches", got, want)
	}
	if got := describe(buildRows(rowTree(), validState, nil, fuzzy.Parse("orph"))); len(got) != 0 {
		t.Errorf("rows = %v, want none: the file that matches isn't valid", got)
	}
}
