package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// input is a line of text being typed: the filter of a list. It has no cursor to move about in it, as a filter is only
// ever added to or cut back at its end, which keeps it simple enough to have nothing to get wrong.
type input struct {
	text []rune
}

func (in *input) String() string { return string(in.text) }

func (in *input) empty() bool { return len(in.text) == 0 }

func (in *input) clear() { in.text = nil }

// insert adds text at the end. A line break, as in pasted text, is a space, and other control characters are left out.
func (in *input) insert(text string) {
	text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(text)
	for _, r := range text {
		if !unicode.IsControl(r) {
			in.text = append(in.text, r)
		}
	}
}

// handle edits the text as msg says, and reports whether msg was an edit (so the caller knows what it does with the
// text may have changed). A message that isn't one, such as an arrow key, is left for the caller.
func (in *input) handle(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		in.insert(msg.Content)
		return true
	case tea.KeyPressMsg:
		switch msg.String() {
		case "backspace":
			if len(in.text) > 0 {
				in.text = in.text[:len(in.text)-1]
			}
			return true
		case "ctrl+u":
			in.clear()
			return true
		case "ctrl+w":
			end := len(in.text)
			for end > 0 && in.text[end-1] == ' ' {
				end--
			}
			for end > 0 && in.text[end-1] != ' ' {
				end--
			}
			in.text = in.text[:end]
			return true
		}
		k := msg.Key()
		if k.Text != "" && !k.Mod.Contains(tea.ModCtrl) && !k.Mod.Contains(tea.ModAlt) {
			in.insert(k.Text)
			return true
		}
	}
	return false
}
