package agentworkload

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk"
)

const (
	dockerLabel = "com.coder.workload"
	// Paths inside a sandbox.
	sandboxChannelDir = "/run/coder"
	sandboxSocket     = sandboxChannelDir + "/workload.sock"
	sandboxCLI        = "/usr/local/bin/coder"
)

// DockerRuntime runs each workload in a container of a Docker daemon the
// workspace daemon can reach. The daemon and Docker must share a filesystem,
// because the channel is a unix socket in a directory that is mounted into
// the container.
//
// A container gets its own filesystem, process and network namespaces, no
// capabilities and an unprivileged user. Nothing of the workspace is mounted
// into it except its own channel and, read-only, the coder CLI. For
// containers to be unable to reach each other, run the Docker daemon with
// inter-container communication disabled, and do not let it listen on TCP.
type DockerRuntime struct {
	// Docker is the docker CLI. It defaults to "docker".
	Docker string
	// StateDir holds one directory per workload with its channel socket.
	StateDir string
	// CLIPath is a coder binary to offer inside sandboxes. Empty offers none.
	CLIPath string
	// Execer runs the docker CLI. It defaults to agentexec.DefaultExecer.
	Execer agentexec.Execer
}

var _ Runtime = (*DockerRuntime)(nil)

// Name implements Runtime.
func (*DockerRuntime) Name() string { return "docker" }

// command returns a docker CLI invocation. The arguments are built in this
// file; a workload's command only ever runs inside its container.
func (d *DockerRuntime) command(ctx context.Context, args ...string) *exec.Cmd {
	bin := d.Docker
	if bin == "" {
		bin = "docker"
	}
	execer := d.Execer
	if execer == nil {
		execer = agentexec.DefaultExecer
	}
	return execer.CommandContext(ctx, bin, args...)
}

func (d *DockerRuntime) docker(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := d.command(ctx, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", xerrors.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Start implements Runtime.
func (d *DockerRuntime) Start(ctx context.Context, spec Spec, channel http.Handler) (Sandbox, error) {
	if spec.Image == "" {
		return nil, xerrors.New("no image configured for workloads")
	}
	// Only the directory is mounted, read-only, so the container cannot
	// replace the socket. Connecting to a socket needs no write access to
	// the filesystem it is on.
	dir := filepath.Join(d.StateDir, spec.ID.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, xerrors.Errorf("create channel directory: %w", err)
	}
	socket := filepath.Join(dir, filepath.Base(sandboxSocket))
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, xerrors.Errorf("listen on channel: %w", err)
	}
	//nolint:gosec // The socket is reachable only from the sandbox it is mounted into, whose user is unprivileged.
	if err := os.Chmod(socket, 0o666); err != nil {
		_ = listener.Close()
		return nil, xerrors.Errorf("open channel to the sandbox user: %w", err)
	}
	server := &http.Server{Handler: channel, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = server.Serve(listener) }()

	args := []string{
		"run", "--detach",
		"--name", "coder-workload-" + spec.ID.String(),
		"--label", dockerLabel + "=" + spec.ID.String(),
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "1000:1000",
		"--pids-limit", "512",
		"--volume", dir + ":" + sandboxChannelDir + ":ro",
		"--env", codersdk.WorkloadSocketEnv + "=" + sandboxSocket,
		"--env", "HOME=/tmp",
		"--env", "CODER_WORKLOAD_NAME=" + spec.Name,
	}
	if d.CLIPath != "" {
		args = append(args, "--volume", d.CLIPath+":"+sandboxCLI+":ro")
	}
	args = append(args, spec.Image, "sh", "-c", spec.Command)
	containerID, err := d.docker(ctx, args...)
	if err != nil {
		_ = server.Close()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &dockerSandbox{runtime: d, containerID: containerID, dir: dir, server: server}, nil
}

// Sweep implements Runtime.
func (d *DockerRuntime) Sweep(ctx context.Context) error {
	ids, err := d.docker(ctx, "ps", "--all", "--quiet", "--filter", "label="+dockerLabel)
	if err != nil {
		return err
	}
	if ids = strings.TrimSpace(ids); ids != "" {
		if _, err := d.docker(ctx, append([]string{"rm", "--force"}, strings.Fields(ids)...)...); err != nil {
			return err
		}
	}
	return os.RemoveAll(d.StateDir)
}

type dockerSandbox struct {
	runtime     *DockerRuntime
	containerID string
	dir         string
	server      *http.Server
}

func (s *dockerSandbox) ID() string { return s.containerID }

func (s *dockerSandbox) Logs(ctx context.Context, w io.Writer) error {
	cmd := s.runtime.command(ctx, "logs", "--follow", s.containerID)
	cmd.Stdout, cmd.Stderr = w, w
	return cmd.Run()
}

func (s *dockerSandbox) Wait(ctx context.Context) (int, error) {
	out, err := s.runtime.docker(ctx, "wait", s.containerID)
	if err != nil {
		return -1, err
	}
	code, err := strconv.Atoi(out)
	if err != nil {
		return -1, xerrors.Errorf("parse exit code %q: %w", out, err)
	}
	return code, nil
}

func (s *dockerSandbox) Close(ctx context.Context) error {
	_ = s.server.Close()
	_, err := s.runtime.docker(ctx, "rm", "--force", s.containerID)
	_ = os.RemoveAll(s.dir)
	return err
}
