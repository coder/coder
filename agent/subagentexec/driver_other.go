//go:build !unix

package subagentexec

import "os/exec"

// platformSupported reports that this build cannot isolate or signal a
// driver process group. The execution isolation proof of concept targets
// Linux; other platforms compile but refuse to launch.
const platformSupported = false

func configureCommand(*exec.Cmd) {}

func signalProcessGroup(*exec.Cmd, procSignal) error {
	return ErrUnsupportedPlatform
}
