//go:build linux

package cmdtest

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// OpenTerminal opens a pseudo-terminal, and returns its terminal end: the one that a program would print to, which
// is a terminal to it. The test is skipped where there isn't one to be had, as in some sandboxes.
func OpenTerminal(t *testing.T) *os.File {
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
