package agentssh

import (
	"os"

	"github.com/gliderlabs/ssh"
)

func osSignalFrom(sig ssh.Signal) os.Signal {
	switch sig {
	case ssh.SIGINT:
		return os.Interrupt
	default:
		return os.Kill
	}
}
