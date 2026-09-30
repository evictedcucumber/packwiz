package tui

import "strconv"

// scroller is a cursor in a list that is longer than the room it has, and how far down the list is shown from. The list
// is the caller's: a scroller only knows how many lines it has and how many it can show.
type scroller struct {
	cursor, offset int
}

// clamp puts the cursor in a list of n lines, and scrolls so that it is in the h lines that are shown, going no further
// than the list does.
func (s *scroller) clamp(n, h int) {
	h = max(h, 1)
	s.cursor = max(min(s.cursor, n-1), 0)
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+h {
		s.offset = s.cursor - h + 1
	}
	s.offset = max(min(s.offset, n-h), 0)
}

// move goes delta lines down the list (up, if it is negative), as far as it goes.
func (s *scroller) move(delta, n, h int) {
	s.cursor += delta
	s.clamp(n, h)
}

// visible is the range of lines that are shown, which ends where the list does.
func (s *scroller) visible(n, h int) (from, to int) {
	s.clamp(n, h)
	return s.offset, min(s.offset+max(h, 1), n)
}

// position says where the cursor is in a list of n lines, as "3/40", or "" for a list that has none.
func (s *scroller) position(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(s.cursor+1) + "/" + strconv.Itoa(n)
}
