//go:build windows

package cli

import (
	"os"
)

var StopSignals = []os.Signal{
	os.Interrupt,
}

var StopSignalsNoInterrupt = []os.Signal{}

var InterruptSignals = []os.Signal{
	os.Interrupt,
}
