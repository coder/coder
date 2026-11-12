package agentworkload

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/pty"
)

// attachGrace is how long an interactive workload waits for somebody to
// attach before it is given up.
const attachGrace = 2 * time.Minute

// PodRuntime runs a workload as a process of the pod, or machine, that the
// workspace daemon itself runs in. It is for daemons that have a sandbox to
// themselves: the pod was made for this one workload, and the pod is the
// sandbox. What isolates the workload is whatever isolates the pod.
//
// The workload and the daemon are the same principal here. The workload can
// read whatever the daemon can, including the token that identifies the
// pod, so the channel is a convenience and not a boundary: it lets programs
// reach Coder as the workload without handling a credential. For the same
// reason the daemon can offer the channel on a loopback address itself, for
// programs that cannot use a unix socket.
type PodRuntime struct {
	// SandboxID names the sandbox, for example the pod's name.
	SandboxID string
	// StateDir holds one directory per workload with its channel socket
	// and its output.
	StateDir string
	// CLIPath is the coder binary, so that `coder` is on the workload's
	// path.
	CLIPath string
	// Execer runs the workload. It defaults to agentexec.DefaultExecer.
	Execer agentexec.Execer
	Logger slog.Logger
}

var _ Runtime = (*PodRuntime)(nil)

// Name implements Runtime.
func (*PodRuntime) Name() string { return "pod" }

// environ is what a workload's process sees: the pod's ordinary
// environment, without anything that configures the daemon.
func (p *PodRuntime) environ(spec Spec, socket, loopback string) []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "CODER_")
	})
	if p.CLIPath != "" && filepath.Base(p.CLIPath) == "coder" {
		// So that `coder` is on the path.
		for i, kv := range env {
			if path, ok := strings.CutPrefix(kv, "PATH="); ok {
				env[i] = "PATH=" + filepath.Dir(p.CLIPath) + string(os.PathListSeparator) + path
			}
		}
	}
	return append(env,
		codersdk.WorkloadSocketEnv+"="+socket,
		"CODER_URL=http://"+loopback,
		"CODER_WORKLOAD_NAME="+spec.Name,
	)
}

// Start implements Runtime.
func (p *PodRuntime) Start(ctx context.Context, spec Spec, channel http.Handler) (Sandbox, error) {
	if len(spec.Mounts) > 0 {
		return nil, xerrors.New("a workload that has a pod to itself sees the pod's files; directories cannot be shared with it")
	}
	dir := filepath.Join(p.StateDir, spec.ID.String())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, xerrors.Errorf("create workload directory: %w", err)
	}
	socket := filepath.Join(dir, "workload.sock")
	_ = os.Remove(socket)
	listener, err := (&net.ListenConfig{}).Listen(ctx, "unix", socket)
	if err != nil {
		return nil, xerrors.Errorf("listen on channel: %w", err)
	}
	// The same channel on a loopback address. Whatever can reach it runs
	// in the pod, and so is the workload.
	loopback, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		_ = listener.Close()
		return nil, xerrors.Errorf("listen on loopback: %w", err)
	}
	output, err := os.OpenFile(filepath.Join(dir, "output"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = listener.Close()
		_ = loopback.Close()
		return nil, xerrors.Errorf("create output file: %w", err)
	}
	server := &http.Server{Handler: channel, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = server.Serve(listener) }()
	go func() { _ = server.Serve(loopback) }()

	sandbox := &podSandbox{
		runtime: p, spec: spec, dir: dir, socket: socket, loopback: loopback.Addr().String(),
		server: server, output: output,
		done: make(chan struct{}),
	}
	sandbox.ctx, sandbox.cancel = context.WithCancel(context.Background())
	if spec.Interactive {
		// The command runs in the terminal of whoever attaches.
		sandbox.unattached = time.AfterFunc(attachGrace, func() {
			sandbox.finish(-1, xerrors.New("nobody attached to the workload's terminal"))
		})
		return sandbox, nil
	}

	execer := p.Execer
	if execer == nil {
		execer = agentexec.DefaultExecer
	}
	cmd := execer.CommandContext(sandbox.ctx, "sh", "-c", spec.Command)
	cmd.Env = p.environ(spec, socket, sandbox.loopback)
	cmd.Stdout, cmd.Stderr = output, output
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	// Ask first, so that the command can finish what it is doing.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		_ = sandbox.Close(ctx)
		return nil, xerrors.Errorf("start workload: %w", err)
	}
	go func() {
		err := cmd.Wait()
		var exit *exec.ExitError
		switch {
		case err == nil:
			sandbox.finish(0, nil)
		case errors.As(err, &exit) && exit.ExitCode() >= 0:
			sandbox.finish(exit.ExitCode(), nil)
		default:
			sandbox.finish(-1, err)
		}
	}()
	return sandbox, nil
}

// Sweep implements Runtime. Processes of an earlier daemon died with it, or
// with its pod.
func (p *PodRuntime) Sweep(context.Context) error {
	return os.RemoveAll(p.StateDir)
}

type podSandbox struct {
	runtime *PodRuntime
	spec    Spec
	dir     string
	socket  string
	// loopback is the address the channel is also served on.
	loopback string
	server   *http.Server
	output   *os.File
	// ctx ends the workload's process.
	ctx    context.Context
	cancel context.CancelFunc

	mu         sync.Mutex
	attached   bool
	unattached *time.Timer
	// done is closed when the command is over; code and err are valid
	// after that.
	done     chan struct{}
	finished bool
	code     int
	err      error
}

func (s *podSandbox) ID() string { return s.runtime.SandboxID }

func (s *podSandbox) finish(code int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.finished, s.code, s.err = true, code, err
	if s.unattached != nil {
		s.unattached.Stop()
	}
	_ = s.output.Close()
	close(s.done)
}

// Logs copies what the workload has written so far, and then what it goes
// on to write, until it is over.
func (s *podSandbox) Logs(ctx context.Context, w io.Writer) error {
	file, err := os.Open(filepath.Join(s.dir, "output"))
	if err != nil {
		return err
	}
	defer file.Close()
	over := false
	for {
		if _, err := io.Copy(w, file); err != nil {
			return err
		}
		if over {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			// Once more, for what was written last.
			over = true
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Attach runs the command of an interactive workload in a terminal and
// connects conn to it. The command ends with the connection.
func (s *podSandbox) Attach(ctx context.Context, conn net.Conn, height, width uint16) error {
	s.mu.Lock()
	if s.attached || s.finished {
		s.mu.Unlock()
		return xerrors.New("the workload's terminal can be attached to once")
	}
	s.attached = true
	if s.unattached != nil {
		s.unattached.Stop()
	}
	s.mu.Unlock()

	execer := s.runtime.Execer
	if execer == nil {
		execer = agentexec.DefaultExecer
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-s.ctx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	cmd := execer.PTYCommandContext(ctx, "sh", "-c", s.spec.Command)
	cmd.Env = append(s.runtime.environ(s.spec, s.socket, s.loopback), "TERM=xterm-256color")
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	terminal, process, err := pty.Start(cmd)
	if err != nil {
		s.finish(-1, xerrors.Errorf("start workload in a terminal: %w", err))
		return err
	}
	defer terminal.Close()
	if height > 0 && width > 0 {
		_ = terminal.Resize(height, width)
	}

	shown := make(chan struct{})
	go func() {
		defer close(shown)
		// What the terminal shows is the workload's output.
		_, _ = io.Copy(io.MultiWriter(conn, s.output), terminal.OutputReader())
	}()
	go func() {
		defer cancel()
		decoder := json.NewDecoder(conn)
		for {
			var req workspacesdk.ReconnectingPTYRequest
			if err := decoder.Decode(&req); err != nil {
				return
			}
			if _, err := terminal.InputWriter().Write([]byte(req.Data)); err != nil {
				return
			}
			if req.Height > 0 && req.Width > 0 {
				_ = terminal.Resize(req.Height, req.Width)
			}
		}
	}()

	err = process.Wait()
	// Let the last of the output through before the record is closed.
	select {
	case <-shown:
	case <-time.After(2 * time.Second):
	}
	var exit *exec.ExitError
	switch {
	case err == nil:
		s.finish(0, nil)
	case errors.As(err, &exit) && exit.ExitCode() >= 0 && ctx.Err() == nil:
		s.finish(exit.ExitCode(), nil)
	default:
		s.finish(-1, xerrors.Errorf("the workload's terminal went away: %w", err))
	}
	return nil
}

func (s *podSandbox) Wait(ctx context.Context) (int, error) {
	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	case <-s.done:
		return s.code, s.err
	}
}

func (s *podSandbox) Close(context.Context) error {
	s.cancel()
	_ = s.server.Close()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
	}
	// An interactive workload that nobody attached to never ran.
	s.finish(-1, xerrors.New("the workload was removed"))
	return os.RemoveAll(s.dir)
}
