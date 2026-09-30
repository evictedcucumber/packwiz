// Package fuzzy matches text the way fzf does, with fzf's own algorithm: what is typed only has to be found in order, not
// together, so "sdm" finds "Sodium"; a match at the start of a word, or of consecutive characters, scores higher than one
// scattered through it; and every word typed has to match, in any order ("sod ex" finds "Sodium Extra"). Case doesn't
// matter, and accents are ignored.
//
// It uses fzf as a library rather than running the binary, so nothing has to be installed, no process starts for each key
// that is typed, and it works wherever packwiz does. It is a leaf package, so anything can search with it: the TUI's
// lists and the lookup of a mod by what is typed for it on the command line.
package fuzzy

import (
	"slices"
	"strings"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

func init() {
	// The scheme sets how much a match is worth by where in a string it is: the default one is for words, which is what
	// the names of mods are
	algo.Init("default")
}

// Query is what has been typed to search for: words that a candidate has to match all of.
type Query struct {
	terms [][]rune
}

// Parse reads a query from what was typed, which is words separated by spaces.
func Parse(text string) Query {
	var q Query
	for _, term := range strings.Fields(strings.ToLower(text)) {
		q.terms = append(q.terms, []rune(term))
	}
	return q
}

// Empty is whether there is nothing to search for, which everything matches.
func (q Query) Empty() bool {
	return len(q.terms) == 0
}

// Match is how a candidate matched a query.
type Match struct {
	// Score is how well it matched, which is higher the better. It is only for comparing matches of the same query.
	Score int
	// Positions says which characters were matched, for showing them: Positions[i] is for the i-th of the texts that were
	// matched against, as indexes of its runes, in order and without repeats.
	Positions [][]int
}

// Match matches the query against a candidate that has more than one text, such as a name and what else it is called: each
// word of the query has to be found in one of them, the first that has it, which can be a different one for each word. It
// is how well they matched together, and ok is false if some word is in none. An empty query matches everything, with
// nothing to show.
func (q Query) Match(texts ...string) (m Match, ok bool) {
	m.Positions = make([][]int, len(texts))
	for _, term := range q.terms {
		found := false
		for i, text := range texts {
			if score, positions, ok := matchTerm(term, text); ok {
				m.Score += score
				m.Positions[i] = append(m.Positions[i], positions...)
				found = true
				break
			}
		}
		if !found {
			return Match{}, false
		}
	}
	for i := range m.Positions {
		slices.Sort(m.Positions[i])
		m.Positions[i] = slices.Compact(m.Positions[i])
	}
	return m, true
}

// matchTerm matches one word of a query, which has to be in lowercase, against text. It says how well it matches, where
// the score is higher the better, and which characters of text it found, as indexes of its runes in order, so that they
// can be shown. ok is false if term isn't in text.
func matchTerm(term []rune, text string) (score int, positions []int, ok bool) {
	chars := util.ToChars([]byte(text))
	result, found := algo.FuzzyMatchV2(false, true, true, &chars, term, true, nil)
	if result.Start < 0 {
		return 0, nil, false
	}
	if found != nil {
		positions = slices.Clone(*found)
		slices.Sort(positions)
	}
	return result.Score, positions, true
}
