package xvnc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/x/agentdesktop/desktopruntime"
)

// x11SocketDir is where X servers place their unix sockets.
const x11SocketDir = "/tmp/.X11-unix"

// Start launches Xvnc and waits until its RFB port accepts connections. The
// server listens on 127.0.0.1 only, with no VNC authentication: callers are
// expected to broker access (the workspace agent authenticates and proxies
// connections over its own transport).
func Start(ctx context.Context, opts Options) (*Server, error) {
	if err := desktopruntime.Validate(opts.RuntimeDir); err != nil {
		return nil, xerrors.Errorf("invalid runtime dir: %w", err)
	}
	if opts.MinDisplay <= 0 {
		opts.MinDisplay = 10
	}
	if opts.Width <= 0 || opts.Height <= 0 {
		opts.Width, opts.Height = 1280, 800
	}
	if opts.Depth <= 0 {
		opts.Depth = 24
	}
	if opts.DesktopName == "" {
		opts.DesktopName = "Coder"
	}
	if opts.StartTimeout <= 0 {
		opts.StartTimeout = 15 * time.Second
	}
	if opts.Execer == nil {
		opts.Execer = agentexec.DefaultExecer
	}

	display := opts.Display
	if display == 0 {
		var err error
		display, err = freeDisplay(opts.MinDisplay)
		if err != nil {
			return nil, err
		}
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}

	// X clients connect through /tmp/.X11-unix. Xvnc only creates it when
	// running as root, so create it here; failures are non-fatal because
	// Linux clients fall back to the abstract socket namespace.
	if err := os.MkdirAll(x11SocketDir, 0o1777); err == nil {
		_ = os.Chmod(x11SocketDir, 0o1777)
	}

	runtimeBin := filepath.Join(opts.RuntimeDir, "bin")
	args := []string{
		":" + strconv.Itoa(display),
		"-rfbport", strconv.Itoa(port),
		"-localhost",
		"-SecurityTypes", "None",
		"-AlwaysShared",
		"-AcceptSetDesktopSize",
		"-geometry", fmt.Sprintf("%dx%d", opts.Width, opts.Height),
		"-depth", strconv.Itoa(opts.Depth),
		"-desktop", opts.DesktopName,
		"-xkbdir", filepath.Join(opts.RuntimeDir, "share", "xkb"),
		"-nolisten", "tcp",
	}
	args = append(args, opts.ExtraArgs...)

	cmd := opts.Execer.CommandContext(ctx, filepath.Join(opts.RuntimeDir, desktopruntime.XvncPath), args...)
	// Xvnc execs xkbcomp from PATH to compile keymaps.
	cmd.Env = append(os.Environ(), "PATH="+runtimeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if opts.Log != nil {
		cmd.Stdout = opts.Log
		cmd.Stderr = opts.Log
	}
	// Xvnc gets its own process group so that signals aimed at the caller
	// do not hit it directly, but Pdeathsig ensures it still exits if the
	// caller dies without running Stop.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second

	if err := cmd.Start(); err != nil {
		return nil, xerrors.Errorf("start Xvnc: %w", err)
	}
	srv := &Server{Display: display, Port: port, Cmd: cmd, done: make(chan struct{})}
	go func() {
		srv.err = cmd.Wait()
		close(srv.done)
	}()

	if err := srv.waitReady(ctx, opts.StartTimeout); err != nil {
		_ = srv.Stop(5 * time.Second)
		return nil, err
	}
	return srv, nil
}

// freeDisplay returns the first display number at or above minDisplay that
// has neither an X lock file nor an abstract or filesystem socket.
func freeDisplay(minDisplay int) (int, error) {
	for d := minDisplay; d < minDisplay+200; d++ {
		if _, err := os.Stat(fmt.Sprintf("/tmp/.X%d-lock", d)); err == nil {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/tmp/.X11-unix/X%d", d)); err == nil {
			continue
		}
		if conn, err := net.DialTimeout("unix", fmt.Sprintf("@/tmp/.X11-unix/X%d", d), 100*time.Millisecond); err == nil {
			_ = conn.Close()
			continue
		}
		return d, nil
	}
	return 0, xerrors.Errorf("no free X display in range %d-%d", minDisplay, minDisplay+199)
}
