//go:build linux

package ui

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// openTerminal opens a pseudo-terminal, and returns its terminal end (the one a program would print to). The test is
// skipped where there isn't one to be had, as in some sandboxes.
func openTerminal(t *testing.T) *os.File {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })

	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Skipf("can't unlock the pseudo-terminal: %v", err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Skipf("can't find the pseudo-terminal: %v", err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("can't open the pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return slave
}

// stdoutIsTerminal makes os.Stdout a terminal for the duration of the test.
func stdoutIsTerminal(t *testing.T) {
	t.Helper()
	slave := openTerminal(t)
	old := os.Stdout
	os.Stdout = slave
	t.Cleanup(func() { os.Stdout = old })
}

func TestAutoColoursATerminal(t *testing.T) {
	stdoutIsTerminal(t)
	setMode(t, Auto)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	if !Enabled() {
		t.Fatal("Enabled() = false for a terminal, want true")
	}
	if got, want := Error.Sprint("x"), "\x1b[31mx\x1b[0m"; got != want {
		t.Errorf("Sprint() = %q, want %q", got, want)
	}
}

func TestAutoRespectsNoColor(t *testing.T) {
	stdoutIsTerminal(t)
	setMode(t, Auto)
	t.Setenv("TERM", "xterm-256color")

	t.Setenv("NO_COLOR", "1")
	if Enabled() {
		t.Error("Enabled() = true with NO_COLOR set, want false")
	}
	// An empty NO_COLOR doesn't count: https://no-color.org asks for it to be ignored when it isn't set to something
	t.Setenv("NO_COLOR", "")
	if !Enabled() {
		t.Error("Enabled() = false with NO_COLOR empty, want true")
	}
}

func TestAutoRespectsDumbTerminal(t *testing.T) {
	stdoutIsTerminal(t)
	setMode(t, Auto)
	t.Setenv("NO_COLOR", "")

	t.Setenv("TERM", "dumb")
	if Enabled() {
		t.Error("Enabled() = true with TERM=dumb, want false")
	}
}

func TestNeverBeatsATerminal(t *testing.T) {
	stdoutIsTerminal(t)
	setMode(t, Never)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	if Enabled() {
		t.Error("Enabled() = true with the mode Never, want false")
	}
}

// A command prints to two streams, and each is a terminal or not on its own.
func TestAutoDecidesForEachStreamOnItsOwn(t *testing.T) {
	setMode(t, Auto)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	terminal := openTerminal(t)
	_, pipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	t.Cleanup(func() { _ = pipe.Close() })

	if !EnabledFor(terminal) {
		t.Error("EnabledFor(terminal) = false, want true")
	}
	if EnabledFor(pipe) {
		t.Error("EnabledFor(pipe) = true, want false")
	}

	t.Run("stdout is the terminal", func(t *testing.T) {
		old := os.Stdout
		os.Stdout = terminal
		t.Cleanup(func() { os.Stdout = old })

		if got, want := Error.Sprint("x"), "\x1b[31mx\x1b[0m"; got != want {
			t.Errorf("for stdout: Sprint() = %q, want %q", got, want)
		}
		if got := Error.For(pipe).Sprint("x"); got != "x" {
			t.Errorf("for a pipe while stdout is a terminal: Sprint() = %q, want it plain", got)
		}
	})

	t.Run("stdout is a pipe", func(t *testing.T) {
		old := os.Stdout
		os.Stdout = pipe
		t.Cleanup(func() { os.Stdout = old })

		if got := Error.Sprint("x"); got != "x" {
			t.Errorf("for stdout: Sprint() = %q, want it plain", got)
		}
		if got, want := Error.For(terminal).Sprint("x"), "\x1b[31mx\x1b[0m"; got != want {
			t.Errorf("for a terminal while stdout is a pipe: Sprint() = %q, want %q", got, want)
		}
	})
}
