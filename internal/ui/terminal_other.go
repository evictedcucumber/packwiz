//go:build !windows

package ui

import (
	"os"

	"golang.org/x/term"
)

// isTerminal reports whether f is a terminal, which shows escape sequences.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// prepare gets f ready to show escape sequences, as far as that can be done, which it needn't be on terminals other
// than Windows'.
func prepare(*os.File) {}
