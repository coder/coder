//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// exitNodeStopSignals shut the exit node down. SIGHUP is deliberately absent
// because it reloads the policy instead.
var exitNodeStopSignals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
}

// exitNodeReloadSignals trigger a policy reload.
var exitNodeReloadSignals = []os.Signal{
	syscall.SIGHUP,
}
