package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/internal/fuzzy"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// searchBox is the search of a list: text typed after "/", which leaves only what matches it fuzzily (see package fuzzy).
// While it is being typed keys go to it, and it is left on when that is done, so what matched can be acted on; esc takes
// it away.
type searchBox struct {
	input input
	// typing is whether keys are going to the text, rather than being commands
	typing bool
}

// query is what is being searched for.
func (b *searchBox) query() fuzzy.Query { return fuzzy.Parse(b.input.String()) }

// active is whether there is a search, being typed or left on.
func (b *searchBox) active() bool { return b.typing || !b.input.empty() }

// start begins typing a search.
func (b *searchBox) start() { b.typing = true }

// clear takes the search away.
func (b *searchBox) clear() {
	b.input.clear()
	b.typing = false
}

// searchResult is what a key did to a search that is being typed.
type searchResult int

const (
	// searchIgnored is a key that isn't for the search, such as an arrow key, which the screen uses to move
	searchIgnored searchResult = iota
	// searchChanged is the text having changed, so what matches it has
	searchChanged
	// searchLeft is the search being left as it is, or taken away
	searchLeft
)

// handleKey gives a key to the search while it is being typed.
func (b *searchBox) handleKey(msg tea.KeyPressMsg) searchResult {
	switch {
	case key.Matches(msg, keyFilterClear):
		b.clear()
		return searchLeft
	case key.Matches(msg, keyFilterDone):
		b.typing = false
		return searchLeft
	case key.Matches(msg, keyMoveUp, keyMoveDown, keyPageUp, keyPageDown) && msg.Key().Text == "":
		// Only the keys that aren't text move, as j and k are letters of a name here
		return searchIgnored
	}
	if b.input.handle(msg) {
		return searchChanged
	}
	return searchIgnored
}

// paste gives pasted text to the search, and says whether it changed.
func (b *searchBox) paste(msg tea.PasteMsg) bool {
	return b.typing && b.input.handle(msg)
}

// line is how the search is shown on the status line.
func (b *searchBox) line() string {
	switch {
	case b.typing:
		return ui.Bold.Sprint("/") + " " + b.input.String() + ui.Muted.Sprint("█")
	case !b.input.empty():
		return ui.Bold.Sprint("/") + " " + b.input.String() + ui.Muted.Sprint("  (esc clears)")
	}
	return ""
}
