package awake

import (
	"os"
	"os/exec"
	"strconv"
)

func inhibitor() string {
	p, err := exec.LookPath("caffeinate")
	if err != nil {
		return ""
	}
	return p
}

// -w ties the helper's life to ours, so even a crash cannot leave the machine awake.
func inhibitCmd() *exec.Cmd {
	p := inhibitor()
	if p == "" {
		return nil
	}
	return exec.Command(p, "-d", "-i", "-w", strconv.Itoa(os.Getpid()))
}
