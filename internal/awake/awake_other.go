//go:build !darwin && !linux

package awake

import "os/exec"

func inhibitor() string     { return "" }
func inhibitCmd() *exec.Cmd { return nil }
