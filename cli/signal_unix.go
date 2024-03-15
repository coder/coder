//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// StopSignals is the list of signals that are used for handling
// shutdown behavior.
var StopSignals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
	syscall.SIGHUP,
}

// StopSignals is the list of signals that are used for handling
// graceful shutdown behavior.
var StopSignalsNoInterrupt = []os.Signal{
	syscall.SIGTERM,
	syscall.SIGHUP,
}

// InterruptSignals is the list of signals that are used for handling
// immediate shutdown behavior.
var InterruptSignals = []os.Signal{
	os.Interrupt,
}
