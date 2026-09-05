package audio

import (
	"os/exec"
	"strconv"
	"sync"
)

// Supported reports whether system-output capture is available.
func Supported() bool { return backend() != "" }

// Backend names the capture tool that would be used, for the doctor report.
func Backend() string {
	if p := backend(); p != "" {
		return p
	}
	return "none (install pulseaudio-utils for parec)"
}

// backend is resolved once. It is asked for at startup, for every spectrum
// scene in the deck, and again by the doctor report, and a PATH walk per call
// is a cost with nothing to show for it. Installing the recorder while wakeart
// is running therefore needs a restart, which is a fair trade.
var backend = sync.OnceValue(func() string {
	p, err := exec.LookPath("parec")
	if err != nil {
		return ""
	}
	return p
})

// captureCmd builds the recorder. Arguments are passed as argv with no shell,
// so a device name cannot become a command — but it could still become a flag,
// which is what the validation in cleanDevice prevents.
func captureCmd(device string) (*exec.Cmd, error) {
	p := backend()
	if p == "" {
		return nil, ErrUnsupported
	}
	dev, err := cleanDevice(device)
	if err != nil {
		return nil, err
	}
	return exec.Command(p,
		"--format=float32le",
		"--rate="+strconv.Itoa(Rate),
		"--channels=1",
		"--device="+dev,
		"--latency-msec=50",
		"--client-name=wakeart",
	), nil
}
