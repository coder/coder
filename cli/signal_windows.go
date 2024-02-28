//go:build windows

package cli

import (
	"os"
)

// Check the UNIX file for the comments on the signals

var StopSignals = []os.Signal{}

var InterruptSignals = []os.Signal{
	os.Interrupt,
}

var KillSignals = []os.Signal{
	os.Kill,
}
