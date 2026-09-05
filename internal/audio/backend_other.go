//go:build !linux

package audio

import "os/exec"

// System-output capture is Linux-only. macOS has no route to it that works
// here: CoreAudio does not expose the output stream to a recorder, and the two
// APIs that do — ScreenCaptureKit and Core Audio taps — need cgo, which the
// release build disables. The alternative, a virtual loopback driver, is a
// system-wide install this program has no business assuming.
//
// Rather than draw a dead scene, the deck simply does not offer one here.

func Supported() bool { return false }

func Backend() string { return "none (Linux only)" }

func captureCmd(string) (*exec.Cmd, error) { return nil, ErrUnsupported }
