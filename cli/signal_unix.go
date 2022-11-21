//go:build !windows

package cli

import (
	"os"
	"syscall"
)

var InterruptSignals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
	syscall.SIGHUP,
}
