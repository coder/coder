// Package agentworkload runs a workload next to a workspace daemon that has
// a sandbox, such as a pod, to itself. The workload has an identity of its
// own: coderd hands the daemon a credential for it, which is all the
// workload gets to reach Coder with.
package agentworkload

import (
	"context"
	"io"
	"net"

	"github.com/google/uuid"
)

// Spec describes a workload to run.
type Spec struct {
	// ID is the workload identity.
	ID   uuid.UUID
	Name string
	// Command is run by a shell.
	Command string
	// Interactive gives the command a terminal to attach to.
	Interactive bool
	// SessionToken is the workload's own credential for Coder. It belongs
	// to the workload's identity and is narrowed to the identity's scopes.
	SessionToken string
}

// Runtime runs workloads. What isolates a workload is whatever isolates
// the sandbox the daemon runs in.
type Runtime interface {
	// Name identifies the runtime, for example "pod".
	Name() string
	// Start runs the workload.
	Start(ctx context.Context, spec Spec) (Sandbox, error)
	// Sweep removes what an earlier daemon process left behind.
	Sweep(ctx context.Context) error
}

// Sandbox is a running or finished workload.
type Sandbox interface {
	// ID identifies the sandbox to its runtime.
	ID() string
	// Logs copies the workload's output to w until the workload exits.
	Logs(ctx context.Context, w io.Writer) error
	// Attach connects conn to the terminal of an interactive workload
	// until either of them ends. conn carries what a reconnecting PTY
	// carries: JSON messages with input and window sizes one way, raw
	// output the other.
	Attach(ctx context.Context, conn net.Conn, height, width uint16) error
	// Wait blocks until the workload exits and returns its exit code.
	Wait(ctx context.Context) (int, error)
	// Close stops the workload and removes what it left behind.
	Close(ctx context.Context) error
}
