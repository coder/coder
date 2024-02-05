package agentssh

import (
	"os"

	"github.com/gliderlabs/ssh"
)

func osSignalFrom(sig ssh.Signal) os.Signal {
	switch sig {
	// Signals are not supported on Windows.
	default:
		return os.Kill
	}
}
