//go:build !linux

package reaper

import "github.com/hashicorp/go-reap"

// IsChild returns true if we're the forked process.
func IsChild() bool {
	return false
}

// IsInitProcess returns true if the current process's PID is 1.
func IsInitProcess() bool {
	return false
}

func ForkReap(pids reap.PidCh) error {
	return nil
}
