package sandbox

import (
	"context"

	"golang.org/x/xerrors"
)

// ErrRuntimeOwnership prevents rollback from deleting a conflicting allocation.
var ErrRuntimeOwnership = xerrors.New("sandbox allocation ownership mismatch")

// Runtime owns only compute and host networking, never Coder registration.
type Runtime interface {
	Start(context.Context, RuntimeSpec) error
	Inspect(context.Context, string) (bool, error)
	Delete(context.Context, string) error
	List(context.Context, string) ([]RuntimeAllocation, error)
	Close() error
}

// RuntimeOptions are operator settings, not template-controlled host access.
type RuntimeOptions struct {
	Address        string
	Namespace      string
	StateDirectory string
	CNIConfigDir   string
	CNIBinDir      string
}

// RuntimeSpec identifies a single start operation and its isolated compute.
type RuntimeSpec struct {
	ID          string
	WorkspaceID string
	BuildID     string
	BuildNumber int32
	Image       string
	CPU         float64
	MemoryBytes int64
	Workdir     string
	AgentToken  string
	CoderURL    string
}

// RuntimeAllocation describes host resources without exposing credentials.
type RuntimeAllocation struct {
	ID          string
	WorkspaceID string
	BuildID     string
	BuildNumber int32
	Image       string
}
