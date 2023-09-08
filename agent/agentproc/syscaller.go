package agentproc

import (
	"syscall"
)

type Syscaller interface {
	SetPriority(pid int32, priority int) error
	GetPriority(pid int32) (int, error)
	Kill(pid int32, sig syscall.Signal) error
}
