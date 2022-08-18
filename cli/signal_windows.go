//go:build windows

package cli

import (
	"os"
)

var interruptSignals = []os.Signal{os.Interrupt}
