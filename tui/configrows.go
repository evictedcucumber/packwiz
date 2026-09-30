package tui

import "github.com/evictedcucumber/packwiz/core"

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

// invalidGroup is the id of the group of files that no mod claims. The id of a mod's group is the path of its metadata
// file, which always has an extension, so it is never this.
const invalidGroup = "(invalid)"

// row is a line of the config screen's tree.
type row struct {
	kind rowKind
	// group is the id of the group the row is in, or is the heading of
	group string
	// mod is the mod that claims the row's file, or the group is for: nil in the group of files nothing claims
	mod *core.Mod
	// path is the file of a fileRow, or the config-files entry of a missingRow (written as it is in the mod's file)
	path string

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

// buildRows makes the rows of the tree that "packwiz config list" prints: a group for each mod with what it claims
// (files that are tracked, then entries that match none), then the files that nothing claims. Only what filter shows is
// in it, and a group with nothing to show is left out, as it is in the command. The rows of a group that is folded are
// left out and the group is marked, as its files are still its own.
func buildRows(tree core.ConfigFileTree, filter stateFilter, folded map[string]bool) []row {
	var rows []row
	for _, m := range tree.Mods {
		id := m.Mod.GetFilePath()
		var children []row
		if filter.showsValid() {
			for _, f := range m.Files {
				children = append(children, row{kind: fileRow, group: id, mod: m.Mod, path: f})
			}
		}
		if filter.showsMissing() {
			for _, e := range m.Missing {
				children = append(children, row{kind: missingRow, group: id, mod: m.Mod, path: e})
			}
		}
		rows = appendGroup(rows, row{kind: groupRow, group: id, mod: m.Mod, title: m.Mod.Name}, children, folded[id])
	}
	if filter.showsInvalid() {
		var children []row
		for _, f := range tree.Unclaimed {
			children = append(children, row{kind: fileRow, group: invalidGroup, path: f})
		}
		rows = appendGroup(rows, row{kind: groupRow, group: invalidGroup, title: "Invalid"}, children, folded[invalidGroup])
	}
	return rows
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
	// valid is how many files mods claim, counting a file that more than one mod claims once for each
	valid, invalid, missing int
}

func countStates(tree core.ConfigFileTree) stateCounts {
	counts := stateCounts{invalid: len(tree.Unclaimed)}
	for _, m := range tree.Mods {
		counts.valid += len(m.Files)
		counts.missing += len(m.Missing)
	}
	return counts
}
