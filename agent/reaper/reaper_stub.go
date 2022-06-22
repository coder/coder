//go:build !linux

package reaper

// IsInitProcess returns true if the current process's PID is 1.
func IsInitProcess() bool {
	return false
}

func ForkReap(opt ...Option) error {
	return nil
}
