package tui

import (
	"slices"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/fuzzy"
)

// stateFilter is which config files the config screen shows, by their state, as "packwiz config list --state" does.
type stateFilter int

const (
	allStates stateFilter = iota
	validState
	invalidState
	missingState
)

func (f stateFilter) String() string {
	switch f {
	case validState:
		return "valid"
	case invalidState:
		return "invalid"
	case missingState:
		return "missing"
	}
	return "all"
}

// next is the filter that follows f when it is cycled through: all, valid, invalid, missing, and back to all.
func (f stateFilter) next() stateFilter {
	return (f + 1) % 4
}

func (f stateFilter) showsValid() bool   { return f == allStates || f == validState }
func (f stateFilter) showsInvalid() bool { return f == allStates || f == invalidState }
func (f stateFilter) showsMissing() bool { return f == allStates || f == missingState }

// rowKind is what a row of the config screen's tree is.
type rowKind int

const (
	// groupRow is the heading of a mod, or of the files nothing claims
	groupRow rowKind = iota
	// fileRow is a tracked file
	fileRow
	// missingRow is an entry of a mod's config-files that matches no tracked file
	missingRow
)

// invalidGroup is the id of the group of files that nothing claims. The id of any other group is the key of its owner (see
// owner.key), which is never this.
const invalidGroup = "(invalid)"

// row is a line of the config screen's tree.
type row struct {
	kind rowKind
	// group is the id of the group the row is in, or is the heading of
	group string
	// owner is who claims the row's file, or the group is for: of kind ownerNone in the group of files nothing claims
	owner owner
	// path is the file of a fileRow, or the config-files entry of a missingRow (written as it is in the mod's file)
	path string

	// While there is a search, score is how well a file's row matched it, and positions are the characters of its path
	// that matched, as indexes of its runes, or for a group's row the characters of its title (the words that matched a
	// file by the name of the group it is in), to show them.
	score     int
	positions []int

	// The rest is for a groupRow
	title string
	// files are the files in the group that the state filter shows, whether or not the group is folded
	files []string
	// count is how many rows the group has when it isn't folded
	count  int
	folded bool

	// last is whether the row is the last of its group, which decides how it is drawn
	last bool
}

// rowKey identifies a row, so that the cursor can stay on it when the rows are built again.
type rowKey struct {
	kind  rowKind
	group string
	path  string
}

func (r row) key() rowKey {
	return rowKey{r.kind, r.group, r.path}
}

// buildRows makes the rows of the tree that "packwiz config list" prints: a group for the pack and for its mod loader, if
// they claim anything, then one for each mod, with what it claims (files that are tracked, then entries that match none),
// then the files that nothing claims. Only what filter shows is in it, and a group with nothing to show is left out, as it
// is in the command. The rows of a group that is folded are left out and the group is marked, as its files are still its
// own.
//
// With something to search for in search, what is in it is also only what matches, fuzzily (see package fuzzy): each word
// of it has to be in the path of a file or entry, or in the name of the group it is in, so "sodium json" finds the json
// files of Sodium, and "sodium" alone all of its files. A group is there if a file in it is, and the files it stands for
// are those. No group is folded while there is a search, as a file that matches would be hidden in it, and the tree comes
// back as it was when the search is gone.
func buildRows(tree core.ConfigFileTree, filter stateFilter, folded map[string]bool, search fuzzy.Query) []row {
	var rows []row
	searching := !search.Empty()
	add := func(o owner, files, missing []string) {
		var children []row
		if filter.showsValid() {
			for _, f := range files {
				children = append(children, row{kind: fileRow, group: o.key(), owner: o, path: f})
			}
		}
		if filter.showsMissing() {
			for _, e := range missing {
				children = append(children, row{kind: missingRow, group: o.key(), owner: o, path: e})
			}
		}
		children, titlePositions := searchRows(children, o.name, search)
		heading := row{kind: groupRow, group: o.key(), owner: o, title: o.name, positions: titlePositions}
		rows = appendGroup(rows, heading, children, folded[o.key()] && !searching)
	}
	for _, o := range tree.Owners {
		kind := ownerLoader
		if o.Owner == core.ConfigOwnerPack {
			kind = ownerPack
		}
		add(owner{kind: kind, id: o.Owner, name: o.Name, slug: o.Owner, entries: o.Entries}, o.Files, o.Missing)
	}
	for _, m := range tree.Mods {
		add(modOwner(m.Mod), m.Files, m.Missing)
	}
	if filter.showsInvalid() {
		var children []row
		for _, f := range tree.Unclaimed {
			children = append(children, row{kind: fileRow, group: invalidGroup, path: f})
		}
		children, titlePositions := searchRows(children, "Invalid", search)
		heading := row{kind: groupRow, group: invalidGroup, title: "Invalid", positions: titlePositions}
		rows = appendGroup(rows, heading, children, folded[invalidGroup] && !searching)
	}
	return rows
}

// searchRows keeps the rows of a group, called title, that search matches: each word of it has to be in the row's path or
// in the title, the path first. It gives each a score and the characters of its path that matched, and says which
// characters of the title were found, for the group's own row. Everything matches nothing being searched for.
func searchRows(children []row, title string, search fuzzy.Query) (matched []row, titlePositions []int) {
	if search.Empty() {
		return children, nil
	}
	for _, c := range children {
		m, ok := search.Match(c.path, title)
		if !ok {
			continue
		}
		c.score, c.positions = m.Score, m.Positions[0]
		titlePositions = append(titlePositions, m.Positions[1]...)
		matched = append(matched, c)
	}
	slices.Sort(titlePositions)
	return matched, slices.Compact(titlePositions)
}

// appendGroup adds a group's heading and its children to rows, unless it has no children.
func appendGroup(rows []row, heading row, children []row, folded bool) []row {
	if len(children) == 0 {
		return rows
	}
	for _, c := range children {
		if c.kind == fileRow {
			heading.files = append(heading.files, c.path)
		}
	}
	heading.count = len(children)
	heading.folded = folded
	rows = append(rows, heading)
	if folded {
		return rows
	}
	children[len(children)-1].last = true
	return append(rows, children...)
}

// stateCounts is how many config files are in each state.
type stateCounts struct {
	// valid is how many files are claimed, counting a file that more than one owner claims once for each
	valid, invalid, missing int
}

func countStates(tree core.ConfigFileTree) stateCounts {
	counts := stateCounts{invalid: len(tree.Unclaimed)}
	for _, o := range tree.Owners {
		counts.valid += len(o.Files)
		counts.missing += len(o.Missing)
	}
	for _, m := range tree.Mods {
		counts.valid += len(m.Files)
		counts.missing += len(m.Missing)
	}
	return counts
}
