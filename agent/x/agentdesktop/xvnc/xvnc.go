// Package xvnc starts and supervises an Xvnc server from an unpacked desktop
// runtime. It is Linux only; the runtime archive is not built for other
// platforms.
package xvnc

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentexec"
)

// Options configures an Xvnc server.
type Options struct {
	// RuntimeDir is the unpacked desktop runtime root.
	RuntimeDir string
	// Display is the X display number to use. Zero picks the first free
	// display at or above MinDisplay.
	Display int
	// MinDisplay is the lowest display number to try when Display is zero.
	// Defaults to 10 to stay clear of displays used by real X sessions.
	MinDisplay int
	// Width and Height set the initial framebuffer geometry. Defaults to
	// 1280x800. Clients may still resize via SetDesktopSize.
	Width, Height int
	// Depth is the framebuffer depth. Defaults to 24.
	Depth int
	// DesktopName is the name advertised to VNC clients. Defaults to
	// "Coder".
	DesktopName string
	// StartTimeout bounds how long to wait for the RFB port to accept
	// connections. Defaults to 15s.
	StartTimeout time.Duration
	// Log receives Xvnc's stdout and stderr. Defaults to discarding.
	Log *os.File
	// ExtraArgs are appended to the Xvnc command line.
	ExtraArgs []string
	// Execer creates the Xvnc process. Defaults to agentexec.DefaultExecer;
	// the agent passes its own so process priority management applies.
	Execer agentexec.Execer
}

// Server is a running Xvnc process.
type Server struct {
	Display int
	// Port is the loopback TCP port the RFB server listens on.
	Port int
	Cmd  *exec.Cmd

	done chan struct{}
	err  error
}

// Addr returns the loopback address of the RFB server.
func (s *Server) Addr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port))
}

// Done is closed once the Xvnc process has exited.
func (s *Server) Done() <-chan struct{} {
	return s.done
}

// Err returns the exit error of the Xvnc process once Done is closed.
func (s *Server) Err() error {
	select {
	case <-s.done:
		return s.err
	default:
		return nil
	}
}

// Stop terminates Xvnc, escalating to SIGKILL if it has not exited within
// grace.
func (s *Server) Stop(grace time.Duration) error {
	select {
	case <-s.done:
		return nil
	default:
	}
	_ = s.Cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-s.done:
		return nil
	case <-time.After(grace):
	}
	_ = s.Cmd.Process.Kill()
	<-s.done
	return nil
}

// waitReady polls the RFB port until the server answers with a protocol
// version banner, the process exits, or the deadline passes.
func (s *Server) waitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return xerrors.Errorf("Xvnc exited during startup: %w", s.err)
		default:
		}
		if time.Now().After(deadline) {
			return xerrors.Errorf("Xvnc did not accept connections on %s within %s", s.Addr(), timeout)
		}
		if err := probeRFB(s.Addr()); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// probeRFB connects to addr and checks for the "RFB 003." banner every VNC
// server sends first.
func probeRFB(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	banner, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(banner, "RFB 003.") {
		return xerrors.Errorf("unexpected RFB banner %q", banner)
	}
	return nil
}

// freePort asks the kernel for an unused loopback TCP port. There is a small
// window in which another process could claim it before Xvnc binds, in which
// case Xvnc fails to start and the caller can retry.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, xerrors.Errorf("find free port: %w", err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, xerrors.Errorf("unexpected listener address type %T", ln.Addr())
	}
	return addr.Port, nil
}
