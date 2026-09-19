package ui

import "strings"

// isCygwinPipeName reports whether name is the name of a pipe that Cygwin, MSYS2 (which Git Bash is made from) or their
// terminal, mintty, makes for a program's input or output: \msys-<id>-pty<n>-to-master, or \cygwin-<id>-pty<n>-from-master.
// It isn't only used on Windows, where it is asked, so that it can be tested elsewhere.
func isCygwinPipeName(name string) bool {
	tokens := strings.Split(name, "-")
	if len(tokens) < 5 {
		return false
	}
	switch tokens[0] {
	case `\msys`, `\cygwin`, `\Device\NamedPipe\msys`, `\Device\NamedPipe\cygwin`:
	default:
		return false
	}
	return tokens[1] != "" &&
		strings.HasPrefix(tokens[2], "pty") &&
		(tokens[3] == "from" || tokens[3] == "to") &&
		tokens[4] == "master"
}
