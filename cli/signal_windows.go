//go:build windows

package cli

import (
	"os"
)

var InterruptSignals = []os.Signal{os.Interrupt}
