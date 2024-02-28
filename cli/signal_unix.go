//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// StopSignals are used to gracefully exit.
// An example is exiting provisioner daemons but not canceling
// jobs, allowing a successful and clean exit.
var StopSignals = []os.Signal{
	syscall.SIGTERM,
}

// InterruptSignals are used to less gracefully exit.
// An example is canceling a job, waiting for a timeout,
// and then exiting.
var InterruptSignals = []os.Signal{
	// SIGINT
	os.Interrupt,
	syscall.SIGHUP,
}

// KillSignals will force exit.
var KillSignals = []os.Signal{
	// SIGKILL
	os.Kill,
}
