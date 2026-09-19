//go:build !linux

package cmdtest

import (
	"os"
	"testing"
)

// OpenTerminal skips the test: a pseudo-terminal is only opened on Linux.
func OpenTerminal(t *testing.T) *os.File {
	t.Helper()
	t.Skip("pseudo-terminals are only opened on Linux")
	return nil
}
