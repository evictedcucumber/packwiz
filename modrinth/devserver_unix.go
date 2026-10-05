//go:build !windows

package modrinth

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup puts what a command runs in a process group of its own, so that Ctrl+C in the terminal isn't sent to it
func ownProcessGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// shellCommand runs a line in the shell
func shellCommand(line string) *exec.Cmd {
	return exec.Command("sh", "-c", line)
}
