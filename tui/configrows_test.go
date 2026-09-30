package tui

import (
	"slices"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
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
	got := describe(buildRows(rowTree(), allStates, nil))
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
			if got := describe(buildRows(rowTree(), tt.filter, nil)); !slices.Equal(got, tt.want) {
				t.Errorf("rows = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildRowsOfNothingIsNothing(t *testing.T) {
	if rows := buildRows(core.ConfigFileTree{}, allStates, nil); len(rows) != 0 {
		t.Errorf("rows = %v, want none", describe(rows))
	}
}

func TestBuildRowsCountsAndMarksTheLastRowOfAGroup(t *testing.T) {
	rows := buildRows(rowTree(), allStates, nil)
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
	rows := buildRows(rowTree(), allStates, map[string]bool{"mods/alpha.pw.toml": true, invalidGroup: true})
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
	rows := buildRows(rowTree(), allStates, nil)
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
	for _, r := range buildRows(tree, allStates, nil) {
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
