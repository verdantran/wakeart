package awake

import "os/exec"

func inhibitor() string {
	p, err := exec.LookPath("systemd-inhibit")
	if err != nil {
		return ""
	}
	return p
}

// systemd-inhibit holds the lock only while its child runs, so it needs one.
func inhibitCmd() *exec.Cmd {
	p := inhibitor()
	if p == "" {
		return nil
	}
	return exec.Command(p,
		"--what=idle:sleep", "--who=wakeart", "--why=ASCII art on screen",
		"--mode=block", "sleep", "infinity")
}
