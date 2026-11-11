// Package agentworkload runs workloads inside a workspace daemon. Each
// workload gets a sandbox and an identity of its own.
//
// The daemon is the only way from a sandbox to Coder as that workload. A
// sandbox holds no credential: the runtime gives it a private channel to the
// daemon, and the daemon forwards what arrives on that channel to coderd,
// vouching for the workload the channel belongs to. A workload therefore
// cannot act as another workload or as the daemon, as long as the runtime
// keeps sandboxes away from each other's channels and from the daemon.
package agentworkload

import (
	"context"
	"io"
	"net/http"

	"github.com/google/uuid"
)

// Spec describes a workload to run.
type Spec struct {
	// ID is the workload identity.
	ID   uuid.UUID
	Name string
	// Command is run by a shell inside the sandbox.
	Command string
	Image   string
}

// Runtime isolates workloads from each other and from the workspace daemon.
// How strong the isolation is depends on the implementation: containers,
// microVMs, or namespaces.
type Runtime interface {
	// Name identifies the runtime, for example "docker".
	Name() string
	// Start runs the workload in a new sandbox. The runtime serves channel
	// to that sandbox and to nothing else, and points Coder clients inside
	// the sandbox at it. Whatever reaches channel is treated as the
	// workload, so exclusivity is the runtime's security duty.
	Start(ctx context.Context, spec Spec, channel http.Handler) (Sandbox, error)
	// Sweep removes sandboxes left behind by an earlier daemon process.
	Sweep(ctx context.Context) error
}

// Sandbox is a running or finished workload.
type Sandbox interface {
	// ID identifies the sandbox to its runtime.
	ID() string
	// Logs copies the workload's output to w until the workload exits.
	Logs(ctx context.Context, w io.Writer) error
	// Wait blocks until the workload exits and returns its exit code.
	Wait(ctx context.Context) (int, error)
	// Close stops the workload, closes its channel and removes the sandbox.
	Close(ctx context.Context) error
}
