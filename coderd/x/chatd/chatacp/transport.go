package chatacp

import (
	"context"
	"io"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
)

// Process is a running ACP adapter with stdio attached. Stdout carries
// newline-delimited JSON-RPC; stderr is logging only.
type Process interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	// Wait blocks until the process exits.
	Wait() error
	// Close terminates the process and releases the channel.
	Close() error
}

// Transport starts ACP adapter processes. Implementations must provide
// a clean byte stream: no shell banners or prompt noise on stdout.
type Transport interface {
	Start(ctx context.Context) (Process, error)
}

// SSHTransport runs the adapter over a non-PTY SSH exec channel to the
// workspace agent. Without a PTY the channel carries raw bytes, which
// JSON-RPC framing requires.
type SSHTransport struct {
	// Client is an established SSH client to the workspace agent.
	Client *gossh.Client
	// Command is the adapter invocation, normally Harness.Command.
	Command string
	// Env is applied to the adapter process (the harness-built
	// credentials). The agent SSH server forwards client env into the
	// exec environment.
	Env map[string]string
}

type sshProcess struct {
	session *gossh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
}

func (p *sshProcess) Stdin() io.WriteCloser { return p.stdin }
func (p *sshProcess) Stdout() io.Reader     { return p.stdout }
func (p *sshProcess) Wait() error           { return p.session.Wait() }

func (p *sshProcess) Close() error {
	_ = p.stdin.Close()
	return p.session.Close()
}

// Start opens an SSH session and executes the adapter command. The
// caller owns the returned process and must Close it.
func (t *SSHTransport) Start(_ context.Context) (Process, error) {
	if t.Client == nil {
		return nil, xerrors.New("ssh client is required")
	}
	if strings.TrimSpace(t.Command) == "" {
		return nil, xerrors.New("adapter command is required")
	}
	session, err := t.Client.NewSession()
	if err != nil {
		return nil, xerrors.Errorf("new ssh session: %w", err)
	}
	for k, v := range t.Env {
		if err := session.Setenv(k, v); err != nil {
			_ = session.Close()
			return nil, xerrors.Errorf("set env %s: %w", k, err)
		}
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, xerrors.Errorf("stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, xerrors.Errorf("stdout pipe: %w", err)
	}
	if err := session.Start(t.Command); err != nil {
		_ = session.Close()
		return nil, xerrors.Errorf("start adapter %q: %w", t.Command, err)
	}
	return &sshProcess{
		session: session,
		stdin:   stdin,
		stdout:  stdout,
	}, nil
}
