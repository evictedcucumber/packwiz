// Package ui colours the output of packwiz's commands.
//
// Colour only ever adds to text: with it off every function here returns its text unchanged, and with it on, taking the
// escape sequences out (see Strip) gives back that same text. So output that is piped to another program, written to a
// file or checked by a test is not changed by it, and a message is worded the same whether or not it is coloured.
//
// Styles are named for what they say rather than for their colour, so that how they look is decided in one place:
//
//	Error    something failed
//	Warning  something needs attention, or was declined
//	Success  something was done, or is as it should be
//	Info     a notice, or a value worth picking out such as a version
//	Muted    less important text: progress, file names, hints
//	Bold     emphasis, such as the name of a mod
//
// Whether to colour is decided each time something is styled, for the stream it is going to: os.Stdout, which is where
// commands print to, unless the style is for another (see Style.For), such as os.Stderr for what is printed there. See
// Mode for when that is.
package ui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
)

// Mode says when output is coloured.
type Mode int32

const (
	// Auto colours output that is going to a terminal, unless the environment asks for no colour (NO_COLOR is set, or
	// TERM is "dumb").
	Auto Mode = iota
	// Always colours output, wherever it is going and whatever the environment says.
	Always
	// Never doesn't colour output.
	Never
)

// ParseMode reads a Mode from its name, as given to --color: "auto" (or nothing), "always" or "never".
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return Auto, nil
	case "always":
		return Always, nil
	case "never":
		return Never, nil
	}
	return Auto, fmt.Errorf("invalid color mode %q; must be one of: auto, always, never", s)
}

var mode atomic.Int32

// SetMode sets when output is coloured, and returns what it was before, which is Auto until it is set.
func SetMode(m Mode) Mode {
	return Mode(mode.Swap(int32(m)))
}

// Enabled reports whether output printed to os.Stdout now would be coloured.
func Enabled() bool {
	return EnabledFor(os.Stdout)
}

// EnabledFor reports whether output printed to f now would be coloured. In the mode Auto that is if f is a terminal,
// whichever it is of the streams of a command: something that is printed to a terminal isn't coloured because of what
// the others are, and neither is something that is piped.
func EnabledFor(f *os.File) bool {
	switch Mode(mode.Load()) {
	case Always:
		// Best effort: someone who asked for colour gets it even if the console won't say it can show it
		prepare(f)
		return true
	case Never:
		return false
	}
	// https://no-color.org: any non-empty value asks for no colour
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal(f)
}

const reset = "\x1b[0m"

// Style is a way of showing text. Its methods work like their namesakes in fmt, apart from Printf and Println, which
// like fmt's write to os.Stdout, unless the style is For another stream.
type Style struct {
	open string
	// stream is where what is styled is going, or nil for os.Stdout, which is looked up each time as tests replace it
	stream *os.File
}

var (
	Bold    = Style{open: "\x1b[1m"}
	Muted   = Style{open: "\x1b[2m"}
	Error   = Style{open: "\x1b[31m"}
	Success = Style{open: "\x1b[32m"}
	Warning = Style{open: "\x1b[33m"}
	Info    = Style{open: "\x1b[36m"}
)

// For is the style for text that is going to f, rather than to os.Stdout: whether it is coloured depends on f, and
// Printf and Println write to it.
func (s Style) For(f *os.File) Style {
	s.stream = f
	return s
}

func (s Style) target() *os.File {
	if s.stream != nil {
		return s.stream
	}
	return os.Stdout
}

// Sprint is fmt.Sprint in the style.
func (s Style) Sprint(a ...any) string {
	return s.paint(fmt.Sprint(a...))
}

// Sprintf is fmt.Sprintf in the style.
func (s Style) Sprintf(format string, a ...any) string {
	return s.paint(fmt.Sprintf(format, a...))
}

// Printf is fmt.Printf in the style.
func (s Style) Printf(format string, a ...any) {
	_, _ = fmt.Fprint(s.target(), s.Sprintf(format, a...))
}

// Println is fmt.Println in the style.
func (s Style) Println(a ...any) {
	_, _ = fmt.Fprint(s.target(), s.paint(fmt.Sprintln(a...)))
}

// paint styles text, which may already contain styled text. Each line is styled on its own, ending before its line
// break, so that nothing is left styled across one if the output is cut off, paged or wrapped.
func (s Style) paint(text string) string {
	if text == "" || !EnabledFor(s.target()) {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		// Text styled inside this text ends with a reset, which ends this style too; start it again after each
		line = s.open + strings.ReplaceAll(line, reset, reset+s.open)
		if trimmed, ok := strings.CutSuffix(line, reset+s.open); ok {
			// Nothing follows the last reset, so there is nothing to start again for
			line = trimmed
		}
		lines[i] = line + reset
	}
	return strings.Join(lines, "\n")
}

// Transition shows a change from one value to another, e.g. "1.0 -> 1.1": what it was is red and what it is now is green.
func Transition(from, to string) string {
	return Error.Sprint(from) + " -> " + Success.Sprint(to)
}

var escapeSequence = regexp.MustCompile("\x1b\\[[0-9;]*m")

// Strip removes the escape sequences that style text, giving the text as it is without colour.
func Strip(s string) string {
	return escapeSequence.ReplaceAllString(s, "")
}

// promptGroup is a part of a prompt that isn't the question: its default in brackets, or its choices in parentheses.
var promptGroup = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)`)

// Prompt styles a prompt that asks for an answer, e.g. "Version [1.0.0]: ": the question is bold, the default in
// brackets stands out and the choices in parentheses fade back. What ends the prompt (a colon and space) is left as is.
func Prompt(prompt string) string {
	if !Enabled() {
		return prompt
	}
	question := strings.TrimRight(prompt, ": ")
	ending := prompt[len(question):]

	var b strings.Builder
	last := 0
	for _, group := range promptGroup.FindAllStringIndex(question, -1) {
		b.WriteString(Bold.Sprint(question[last:group[0]]))
		if question[group[0]] == '[' {
			b.WriteString(Info.Sprint(question[group[0]:group[1]]))
		} else {
			b.WriteString(Muted.Sprint(question[group[0]:group[1]]))
		}
		last = group[1]
	}
	b.WriteString(Bold.Sprint(question[last:]))
	return b.String() + ending
}
