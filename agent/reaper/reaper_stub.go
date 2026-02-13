//go:build !linux

package reaper

const (
	// StartCountFile tracks how many times the agent process has
	// started. A value > 1 indicates the agent was restarted.
	StartCountFile = "/tmp/coder-agent-start-count.txt"
	// KillSignalFile records the signal that terminated the
	// previous agent process.
	KillSignalFile = "/tmp/coder-agent-kill-signal.txt"
)

// IsInitProcess returns true if the current process's PID is 1.
func IsInitProcess() bool {
	return false
}

func ForkReap(_ ...Option) (int, error) {
	return 0, nil
}

// WriteStartCount is a no-op on non-Linux platforms.
func WriteStartCount(_ int) error {
	return nil
}

// WriteKillSignal is a no-op on non-Linux platforms.
func WriteKillSignal(_ string) error {
	return nil
}

// ReadKillSignal returns empty on non-Linux platforms.
func ReadKillSignal() string {
	return ""
}

// ParseKillSignal parses the kill signal file content on
// non-Linux platforms. Always returns empty strings.
func ParseKillSignal(_ string) (reason, value string) {
	return "", ""
}

// ClearRestartState is a no-op on non-Linux platforms.
func ClearRestartState() {}
