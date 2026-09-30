package tui

import (
	"slices"
	"strings"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

// Filters match fuzzily, the way fzf does, with fzf's own algorithm: what is typed only has to be found in order, not
// together, so "sdm" finds "Sodium"; a match at the start of a word, or of consecutive characters, scores higher than
// one scattered through it; and every word typed has to match, in any order ("sod ex" finds "Sodium Extra"). It is
// used as a library, so nothing has to be installed, and there is no process to start for each key that is typed.

func init() {
	// The scheme sets how much a match is worth by where in a string it is: the default one is for words, which is what
	// the names of mods are
	algo.Init("default")
}

// query is what has been typed into a filter: words that a candidate has to match all of. Case doesn't matter.
type query struct {
	terms [][]rune
}

func parseQuery(text string) query {
	var q query
	for _, term := range strings.Fields(strings.ToLower(text)) {
		q.terms = append(q.terms, []rune(term))
	}
	return q
}

// empty is whether there is nothing to match, which every candidate does.
func (q query) empty() bool {
	return len(q.terms) == 0
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

// mergePositions puts lists of positions together, in order and without repeats.
func mergePositions(lists ...[]int) []int {
	merged := slices.Concat(lists...)
	slices.Sort(merged)
	return slices.Compact(merged)
}
