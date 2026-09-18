//go:build windows

package cli

import (
	"os"
)

// exitNodeStopSignals shut the exit node down.
var exitNodeStopSignals = []os.Signal{
	os.Interrupt,
}

// exitNodeReloadSignals is empty on Windows, which has no SIGHUP; the policy
// can only be changed by restarting the process.
var exitNodeReloadSignals []os.Signal
