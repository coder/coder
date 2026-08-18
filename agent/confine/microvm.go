package confine

import (
	"context"
	"regexp"

	"golang.org/x/xerrors"
)

const embeddedCoderGuestPath = "/opt/coder"

var embeddedMicroVMNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// MicroVMOptions configures an in-process microVM sandbox.
type MicroVMOptions struct {
	Image    string
	Name     string
	CacheDir string
	StateDir string

	CPUs      int
	MemoryMiB int

	CoderBinaryPath string
	AgentURL        string
	AgentToken      string
	SessionToken    string

	Policy      *PolicyEngine
	Destination DestinationOptions
	Event       EventCallback
}

type embeddedVM interface {
	Exec(context.Context, string) (int, error)
	Close(context.Context) error
}

// EmbeddedSandbox owns one embedded microVM and its gateway proxy.
type EmbeddedSandbox struct {
	vm embeddedVM
}

// Exec runs a noninteractive command in the embedded guest.
func (sandbox *EmbeddedSandbox) Exec(ctx context.Context, command string) (int, error) {
	if sandbox == nil || sandbox.vm == nil {
		return 0, xerrors.New("embedded sandbox is nil")
	}
	return sandbox.vm.Exec(ctx, command)
}

// Close removes the embedded microVM and its per-VM state.
func (sandbox *EmbeddedSandbox) Close(ctx context.Context) error {
	if sandbox == nil || sandbox.vm == nil {
		return nil
	}
	return sandbox.vm.Close(ctx)
}
