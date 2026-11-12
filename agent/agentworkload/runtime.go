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
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/reconnectingpty"
	"github.com/coder/coder/v2/codersdk"
)

// Spec describes a workload to run. The supervisor has already applied the
// daemon's policy to it.
type Spec struct {
	// ID is the workload identity.
	ID   uuid.UUID
	Name string
	// Command is run by a shell inside the sandbox.
	Command string
	Image   string
	Network codersdk.WorkloadNetwork
	// Mounts are directories of the workspace to show in the sandbox at
	// the same path.
	Mounts []codersdk.WorkloadMount
	// Interactive gives the command a terminal to attach to.
	Interactive bool
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
	// workload, so exclusivity is the runtime's security duty. The channel
	// must work whatever the workload's network is.
	Start(ctx context.Context, spec Spec, channel http.Handler) (Sandbox, error)
	// Sweep removes sandboxes left behind by an earlier daemon process.
	Sweep(ctx context.Context) error
}

// AttachCommand runs a command in a terminal of the daemon's own and
// connects conn to it. It serves runtimes whose way to a workload's terminal
// is a command, such as `docker attach`.
func AttachCommand(ctx context.Context, logger slog.Logger, execer agentexec.Execer, conn net.Conn, height, width uint16, name string, args ...string) error {
	if execer == nil {
		execer = agentexec.DefaultExecer
	}
	terminal := reconnectingpty.New(ctx, logger, execer, execer.PTYCommandContext(ctx, name, args...), &reconnectingpty.Options{
		// The terminal belongs to this connection: the workload itself
		// keeps running in its sandbox when the caller goes away.
		Timeout:     time.Second,
		BackendType: "buffered",
	})
	defer terminal.Close(nil)
	return terminal.Attach(ctx, uuid.NewString(), conn, height, width, logger)
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
	// Close stops the workload, closes its channel and removes the sandbox.
	Close(ctx context.Context) error
}
