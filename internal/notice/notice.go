// Package notice is what code that does the work of a command says as it goes: that a version was chosen although another
// has a higher number, that a mod runs through a compatibility layer, that a lookup failed and was carried on without.
//
// On the command line these are printed, in the style of what they say (see package ui), as they happen. An interface that
// has the terminal to itself, such as the TUI, can't have anything written over what it draws, so it asks for them
// instead: Collect runs some work and returns what it said, in plain text, to be shown where the interface chooses. Code
// that says its notices here can therefore be used by both, unchanged.
//
// Only what is said along the way goes through here. A command's own output, what it is for, is printed by the command;
// the work it calls returns that to it.
package notice

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/evictedcucumber/packwiz/internal/ui"
)

// Level is how a notice is to be taken, which is also how it is styled when it is printed.
type Level int

const (
	// Muted is chatter: what is being done now ("Finding dependencies..."), which says nothing that needs reading.
	Muted Level = iota
	// Info is something worth knowing that isn't wrong.
	Info
	// Warning is something that may not be what was wanted, but the work went on.
	Warning
	// Error is something that failed, which the work went on without.
	Error
	// Success is something that was done.
	Success
)

// style is how a level is printed.
func (l Level) style() ui.Style {
	switch l {
	case Info:
		return ui.Info
	case Warning:
		return ui.Warning
	case Error:
		return ui.Error
	case Success:
		return ui.Success
	}
	return ui.Muted
}

// Notice is something that was said.
type Notice struct {
	Level Level
	// Text is what was said, in plain text: no styling, whatever it was printed with.
	Text string
}

// collector is what collects the notices said while Collect runs work.
type collector struct {
	mu      sync.Mutex
	notices []Notice
}

// current is the collector that notices go to, or nil if they are printed.
var current atomic.Pointer[collector]

// Collect runs work, and returns what it said instead of printing it. It is for one piece of work at a time: a notice
// said by work that is running at the same time, elsewhere, goes to it as well.
func Collect(work func()) []Notice {
	c := &collector{}
	previous := current.Swap(c)
	defer current.Store(previous)
	work()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.notices
}

// Collecting is whether notices are being collected, which is to say that whatever is running has the terminal to itself:
// work that would show its progress there, with a bar or the like, asks it to do without.
func Collecting() bool { return current.Load() != nil }

// Say says something at a level: it is printed, as a line of its own, or collected if that is what is being asked for.
// The text may have styling in it, to put emphasis on a part of it.
func Say(level Level, format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	if c := current.Load(); c != nil {
		c.mu.Lock()
		c.notices = append(c.notices, Notice{Level: level, Text: ui.Strip(text)})
		c.mu.Unlock()
		return
	}
	level.style().Println(text)
}

func Mutedf(format string, args ...any)   { Say(Muted, format, args...) }
func Infof(format string, args ...any)    { Say(Info, format, args...) }
func Warnf(format string, args ...any)    { Say(Warning, format, args...) }
func Errorf(format string, args ...any)   { Say(Error, format, args...) }
func Successf(format string, args ...any) { Say(Success, format, args...) }
