//go:build unix

package awake

import (
	"os/exec"
	"syscall"
)

// Its own group, so stopping the inhibitor also stops whatever it wraps.
func detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func kill(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	if err := syscall.Kill(-c.Process.Pid, syscall.SIGTERM); err != nil {
		_ = c.Process.Kill()
	}
}
