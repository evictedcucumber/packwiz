//go:build windows

package modrinth

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup puts what a command runs in a process group of its own, so that Ctrl+C in the console isn't sent to it
func ownProcessGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// shellCommand runs a line in the shell
func shellCommand(line string) *exec.Cmd {
	return exec.Command("cmd", "/C", line)
}
