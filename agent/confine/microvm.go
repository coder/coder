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
	Progress    func(string)
}

func reportMicroVMProgress(callback func(string), message string) {
	if callback != nil {
		callback(message)
	}
}

// embeddedAgentLogPath is where the guest agent writes its output inside the
// guest. Post-launch capture reads it back through guest exec because guest
// writes are not reliably visible in the host-side rootfs directory.
const embeddedAgentLogPath = "/var/log/coder-agent.log"

type embeddedVM interface {
	Exec(context.Context, string) (int, error)
	ExecOutput(context.Context, string) (int, []byte, error)
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

// AgentLog returns the tail of the guest agent's log. Guest writes are not
// reliably visible in the host-side rootfs directory, so the log is read
// through guest exec.
func (sandbox *EmbeddedSandbox) AgentLog(ctx context.Context) (string, error) {
	if sandbox == nil || sandbox.vm == nil {
		return "", xerrors.New("embedded sandbox is nil")
	}
	status, output, err := sandbox.vm.ExecOutput(ctx, "tail -c 4096 "+embeddedAgentLogPath+" 2>/dev/null || true")
	if err != nil {
		return "", err
	}
	if status != 0 {
		return "", xerrors.Errorf("read guest agent log: exit status %d", status)
	}
	return string(output), nil
}

// Close removes the embedded microVM and its per-VM state.
func (sandbox *EmbeddedSandbox) Close(ctx context.Context) error {
	if sandbox == nil || sandbox.vm == nil {
		return nil
	}
	return sandbox.vm.Close(ctx)
}
