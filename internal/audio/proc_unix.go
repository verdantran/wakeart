//go:build unix

package audio

import (
	"os/exec"
	"syscall"
)

// Its own process group, so the recorder does not take the terminal's Ctrl+C
// and die before we have cleaned up after it.
func detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func kill(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	if err := syscall.Kill(-c.Process.Pid, syscall.SIGTERM); err != nil {
		_ = c.Process.Kill()
	}
}
