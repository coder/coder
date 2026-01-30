//go:build !linux

package reaper

// IsInitProcess returns true if the current process's PID is 1.
func IsInitProcess() bool {
	return false
}

func ForkReap(_ ...Option) (int, error) {
	return 0, nil
}
