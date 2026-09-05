//go:build !unix

package audio

import "os/exec"

func detach(c *exec.Cmd) {}

func kill(c *exec.Cmd) {
	if c.Process != nil {
		_ = c.Process.Kill()
	}
}
