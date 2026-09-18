package changelog

import (
	"regexp"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
)

// Commit is a commit in the pack's history, as far as a changelog is concerned.
type Commit struct {
	// Subject is the first line of the commit message
	Subject string
	// Body is the rest of it, after the blank line that follows the subject
	Body string
}

// ChangesFromCommits reads what a list of commits, oldest first, changed in the pack, as conventional commits.
//
// What "packwiz git commit" writes is read exactly: a commit for a mod that was added, updated or removed, and one for
// config files. Any other commit that is a feature, a fix or breaking is a note, which says what the commit did in its
// own words. Everything else, such as chores and documentation and commits that aren't conventional, has no part in a
// release.
//
// The changes are then reduced to their net effect, so a mod that was added and then updated in the same release is
// just added, and one that was added and then removed isn't mentioned at all.
func ChangesFromCommits(commits []Commit) []Change {
	var changes []Change
	for _, c := range commits {
		changes = append(changes, changesFromCommit(c)...)
	}
	return squash(changes)
}

// headerRegex is a conventional commit's subject: a type, an optional scope in brackets, an optional "!" for a
// breaking change, then what was done.
var headerRegex = regexp.MustCompile(`^(\w+)(?:\(([^)]*)\))?(!)?: (.+)$`)

func changesFromCommit(c Commit) []Change {
	m := headerRegex.FindStringSubmatch(c.Subject)
	if m == nil {
		return nil
	}
	commitType, scope, breaking, description := strings.ToLower(m[1]), m[2], m[3] == "!", m[4]

	// What packwiz writes says exactly what changed. Commits that list several changes give them as bullets in the
	// body, as packwiz git commit once did for all of a pack's changes at once.
	var changes []Change
	for _, line := range bulletLines(c.Body) {
		if change, ok := parseLine(line); ok {
			changes = append(changes, change)
		}
	}
	if len(changes) > 0 {
		return changes
	}
	// Only a subject in the scope packwiz writes it in is read, so that a note that happens to end in "(client)"
	// isn't taken for a mod
	if change, ok := parseLine(description); ok && scopeFits(change, scope) {
		return []Change{change}
	}

	note := Change{
		Kind: Note, Type: commitType, Scope: scope, Text: description,
		Breaking: breaking || hasBreakingFooter(c.Body),
	}
	if note.Bump() == BumpNone {
		return nil
	}
	return []Change{note}
}

// bulletLines are the lines of a commit body that are items in a list.
func bulletLines(body string) []string {
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
			lines = append(lines, item)
		}
	}
	return lines
}

func hasBreakingFooter(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "BREAKING CHANGE:") || strings.HasPrefix(line, "BREAKING-CHANGE:") {
			return true
		}
	}
	return false
}

var (
	// modLineRegex is a change to a mod as packwiz git commit writes it, e.g. "update Iris 1.7.0 -> 1.7.1 (client)"
	modLineRegex = regexp.MustCompile(`^(add|remove|update) (.+) \((client|server|both)\)$`)
	// fileLineRegex is a change to a file, e.g. "change config/sodium.json". A path has a directory or an extension,
	// which keeps a note like "change render-distance" from being taken for a file.
	fileLineRegex = regexp.MustCompile(`^(add|change|remove) (\S*[/.]\S*)$`)
)

// parseLine reads one change in the form packwiz writes it.
func parseLine(line string) (Change, bool) {
	line = strings.TrimSpace(line)
	if m := modLineRegex.FindStringSubmatch(line); m != nil {
		return parseModLine(m[1], m[2], m[3])
	}
	if m := fileLineRegex.FindStringSubmatch(line); m != nil {
		kinds := map[string]Kind{"add": FileAdded, "change": FileChanged, "remove": FileRemoved}
		return Change{Kind: kinds[m[1]], Path: m[2]}, true
	}
	return Change{}, false
}

// scopeFits reports whether a commit's scope is the one packwiz writes a change like this in: mods for a mod, and
// config for a file.
func scopeFits(c Change, scope string) bool {
	if c.IsMod() {
		return scope == "mods"
	}
	return scope == "config"
}

func parseModLine(verb, rest, sideName string) (Change, bool) {
	side := map[string]string{"client": core.ClientSide, "server": core.ServerSide, "both": core.UniversalSide}[sideName]

	switch verb {
	case "update":
		i := strings.LastIndex(rest, " -> ")
		if i < 0 {
			return Change{}, false
		}
		name, from := splitVersion(rest[:i])
		to := rest[i+len(" -> "):]
		if name == "" || from == "" || to == "" {
			return Change{}, false
		}
		return Change{Kind: ModUpdated, Name: name, Side: side, From: from, To: to}, true
	case "add":
		name, version := splitVersion(rest)
		return Change{Kind: ModAdded, Name: name, Side: side, To: version}, true
	default:
		name, version := splitVersion(rest)
		return Change{Kind: ModRemoved, Name: name, Side: side, From: version}, true
	}
}

// splitVersion splits "Iris Shaders 1.8.12" into a name and the version at the end of it. A mod's name can have
// spaces in it, but its version can't.
func splitVersion(s string) (name, version string) {
	i := strings.LastIndex(s, " ")
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

// span is the first and last of the changes made to one mod or file.
type span struct {
	first, last Change
}

// squash reduces changes, oldest first, to what they add up to: one change for each mod or file that ended up
// different, in the order they were first mentioned, and then any notes as they were.
func squash(changes []Change) []Change {
	var order []string
	spans := make(map[string]*span)
	var notes []Change
	for _, c := range changes {
		var key string
		switch {
		case c.Kind == Note:
			notes = append(notes, c)
			continue
		case c.IsMod():
			key = "mod:" + c.Name
		default:
			key = "file:" + c.Path
		}
		if s, ok := spans[key]; ok {
			s.last = c
		} else {
			spans[key] = &span{first: c, last: c}
			order = append(order, key)
		}
	}

	var net []Change
	for _, key := range order {
		if c, ok := spans[key].net(); ok {
			net = append(net, c)
		}
	}
	return append(net, notes...)
}

// net is what the changes from first to last add up to, or false if they cancel out.
func (s span) net() (Change, bool) {
	if s.first.IsMod() {
		// Whether it was there before is what the first change to it says, and whether it is there now is what the last does
		existedBefore, existsAfter := s.first.Kind != ModAdded, s.last.Kind != ModRemoved
		switch {
		case !existedBefore && !existsAfter:
			return Change{}, false
		case !existedBefore:
			return Change{Kind: ModAdded, Name: s.last.Name, Side: s.last.Side, To: s.last.To}, true
		case !existsAfter:
			return Change{Kind: ModRemoved, Name: s.last.Name, Side: s.last.Side, From: s.first.From}, true
		case s.first.From == s.last.To:
			return Change{}, false
		default:
			return Change{Kind: ModUpdated, Name: s.last.Name, Side: s.last.Side, From: s.first.From, To: s.last.To}, true
		}
	}

	existedBefore, existsAfter := s.first.Kind != FileAdded, s.last.Kind != FileRemoved
	switch {
	case !existedBefore && !existsAfter:
		return Change{}, false
	case !existedBefore:
		return Change{Kind: FileAdded, Path: s.last.Path}, true
	case !existsAfter:
		return Change{Kind: FileRemoved, Path: s.last.Path}, true
	default:
		return Change{Kind: FileChanged, Path: s.last.Path}, true
	}
}
