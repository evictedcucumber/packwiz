package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typed(in *input, keys ...string) {
	for _, k := range keys {
		switch k {
		case "space":
			in.handle(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		case "backspace":
			in.handle(tea.KeyPressMsg{Code: tea.KeyBackspace})
		case "ctrl+u":
			in.handle(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		case "ctrl+w":
			in.handle(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
		default:
			in.handle(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
		}
	}
}

func TestInputTypesAndDeletes(t *testing.T) {
	var in input
	if !in.empty() {
		t.Error("a new input isn't empty")
	}
	typed(&in, "s", "o", "d", "space", "é", "日")
	if got := in.String(); got != "sod é日" {
		t.Errorf("the input is %q, want what was typed", got)
	}
	// A character is deleted whole, however many bytes it is
	typed(&in, "backspace", "backspace")
	if got := in.String(); got != "sod " {
		t.Errorf("after two backspaces the input is %q", got)
	}
	typed(&in, "ctrl+u")
	if !in.empty() {
		t.Errorf("ctrl+u left %q", in.String())
	}
	typed(&in, "backspace") // nothing to delete, and nothing goes wrong
	if !in.empty() {
		t.Errorf("backspace on an empty input left %q", in.String())
	}
}

func TestInputDeletesAWordWithCtrlW(t *testing.T) {
	var in input
	for _, r := range "sodium extra" {
		if r == ' ' {
			typed(&in, "space")
		} else {
			typed(&in, string(r))
		}
	}
	typed(&in, "ctrl+w")
	if got := in.String(); got != "sodium " {
		t.Errorf("after ctrl+w the input is %q, want the last word gone", got)
	}
	typed(&in, "ctrl+w")
	if got := in.String(); got != "" {
		t.Errorf("after ctrl+w again the input is %q, want the word before gone too, and the space with it", got)
	}
	typed(&in, "ctrl+w") // nothing to delete
}

func TestInputPasteTurnsLineBreaksIntoSpacesAndDropsControlCharacters(t *testing.T) {
	var in input
	if !in.handle(tea.PasteMsg{Content: "one\r\ntwo\nthree\tfour\x1b[31mfive\x00"}) {
		t.Error("a paste isn't an edit")
	}
	if got := in.String(); got != "one two three four[31mfive" {
		t.Errorf("the input is %q", got)
	}
}

func TestInputLeavesKeysThatAreNotTextForTheCaller(t *testing.T) {
	var in input
	for _, msg := range []tea.KeyPressMsg{
		{Code: tea.KeyUp},
		{Code: tea.KeyEnter},
		{Code: tea.KeyEscape},
		{Code: tea.KeyTab},
		{Code: 'c', Mod: tea.ModCtrl},
		{Code: 'x', Text: "x", Mod: tea.ModAlt},
	} {
		if in.handle(msg) {
			t.Errorf("%q was taken as an edit", msg.String())
		}
	}
	if !in.empty() {
		t.Errorf("the input is %q after keys that aren't text", in.String())
	}
	if in.handle(tea.WindowSizeMsg{Width: 1, Height: 1}) {
		t.Error("a message that isn't a key was taken as an edit")
	}
}
