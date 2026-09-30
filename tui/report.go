package tui

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// report is text that is read more than it is used: what a check found, the dependencies of the mods, the release the
// pack's changes would make. It is made for the width of the screen it is shown in, as it wraps to it, and it is scrolled
// with the keys that move through a list.
type report struct {
	// source makes the lines for a width
	source func(width int) []string
	lines  []string
	width  int
	// top is the first line that is shown
	top int
}

// set replaces what is shown, scrolled back to its start.
func (r *report) set(source func(width int) []string) {
	r.source = source
	r.top = 0
	r.lines = nil
	r.width = 0
}

// clear empties the report.
func (r *report) clear() {
	r.set(nil)
}

// layout makes the lines for a width, again if it isn't the width they were made for.
func (r *report) layout(width int) {
	if r.source == nil || (r.lines != nil && r.width == width) {
		return
	}
	r.lines, r.width = r.source(width), width
	if r.lines == nil {
		r.lines = []string{}
	}
}

// empty is whether there is nothing to show.
func (r *report) empty() bool { return r.source == nil }

// scroll moves the window of lines that is shown by delta, as far as the text goes.
func (r *report) scroll(delta, height int) {
	r.top = max(min(r.top+delta, len(r.lines)-height), 0)
}

// window is the lines that are shown in height lines of room.
func (r *report) window(width, height int) []string {
	r.layout(width)
	r.scroll(0, height)
	return r.lines[r.top:min(r.top+height, len(r.lines))]
}

// position says how far through the text the window is: "1-20/57", or "" if all of it is shown.
func (r *report) position(height int) string {
	if len(r.lines) <= height {
		return ""
	}
	return strconv.Itoa(r.top+1) + "-" + strconv.Itoa(min(r.top+height, len(r.lines))) + "/" + strconv.Itoa(len(r.lines))
}

// handleKey scrolls for a key that moves, and says whether it was one.
func (r *report) handleKey(msg tea.KeyPressMsg, height int) bool {
	switch {
	case key.Matches(msg, keyMoveUp):
		r.scroll(-1, height)
	case key.Matches(msg, keyMoveDown):
		r.scroll(1, height)
	case key.Matches(msg, keyPageUp):
		r.scroll(-height, height)
	case key.Matches(msg, keyPageDown):
		r.scroll(height, height)
	case key.Matches(msg, keyTop):
		r.scroll(-len(r.lines), height)
	case key.Matches(msg, keyBottom):
		r.scroll(len(r.lines), height)
	default:
		return false
	}
	return true
}
