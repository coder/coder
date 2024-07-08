package agentproc

import (
	"syscall"
)

type Syscaller interface {
	SetPriority(pid int32, priority int) error
	GetPriority(pid int32) (int, error)
	Kill(pid int32, sig syscall.Signal) error
}

// nolint: unused // used on some but no all platforms
const defaultProcDir = "/proc"

type Process struct {
	Dir         string
	CmdLine     string
	PID         int32
	OOMScoreAdj int
}
