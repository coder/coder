package workspacetraffic

import (
	"context"
	"io"
	"sync"

	"github.com/coder/coder/v2/codersdk"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
)

func connectPTY(ctx context.Context, client *codersdk.Client, agentID, reconnect uuid.UUID) (*countReadWriteCloser, error) {
	conn, err := client.WorkspaceAgentReconnectingPTY(ctx, codersdk.WorkspaceAgentReconnectingPTYOpts{
		AgentID:   agentID,
		Reconnect: reconnect,
		Height:    25,
		Width:     80,
		Command:   "sh",
	})
	if err != nil {
		return nil, xerrors.Errorf("connect pty: %w", err)
	}

	// Wrap the conn in a countReadWriteCloser so we can monitor bytes sent/rcvd.
	crw := countReadWriteCloser{ctx: ctx, rwc: conn}
	return &crw, nil
}

func connectSSH(ctx context.Context, client *codersdk.Client, agentID uuid.UUID) (*countReadWriteCloser, error) {
	agentConn, err := client.DialWorkspaceAgent(ctx, agentID, &codersdk.DialWorkspaceAgentOptions{})
	if err != nil {
		return nil, xerrors.Errorf("dial workspace agent: %w", err)
	}
	agentConn.AwaitReachable(ctx)
	sshClient, err := agentConn.SSHClient(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get ssh client: %w", err)
	}
	sshSession, err := sshClient.NewSession()
	if err != nil {
		_ = agentConn.Close()
		return nil, xerrors.Errorf("new ssh session: %w", err)
	}
	wrappedConn := &wrappedSSHConn{ctx: ctx}
	// Do some plumbing to hook up the wrappedConn
	pr1, pw1 := io.Pipe()
	wrappedConn.stdout = pr1
	sshSession.Stdout = pw1
	pr2, pw2 := io.Pipe()
	sshSession.Stdin = pr2
	wrappedConn.stdin = pw2
	err = sshSession.RequestPty("xterm", 25, 80, gossh.TerminalModes{})
	if err != nil {
		_ = pr1.Close()
		_ = pr2.Close()
		_ = pw1.Close()
		_ = pw2.Close()
		_ = sshSession.Close()
		_ = agentConn.Close()
		return nil, xerrors.Errorf("request pty: %w", err)
	}
	err = sshSession.Shell()
	if err != nil {
		_ = sshSession.Close()
		_ = agentConn.Close()
		return nil, xerrors.Errorf("shell: %w", err)
	}

	closeFn := func() error {
		var merr error
		if err := sshSession.Close(); err != nil {
			merr = multierror.Append(merr, err)
		}
		if err := agentConn.Close(); err != nil {
			merr = multierror.Append(merr, err)
		}
		return merr
	}
	wrappedConn.close = closeFn

	crw := &countReadWriteCloser{ctx: ctx, rwc: wrappedConn}
	return crw, nil
}

// wrappedSSHConn wraps an ssh.Session to implement io.ReadWriteCloser.
type wrappedSSHConn struct {
	ctx       context.Context
	stdout    io.Reader
	stdin     io.Writer
	closeOnce sync.Once
	closeErr  error
	close     func() error
}

func (w *wrappedSSHConn) Close() error {
	w.closeOnce.Do(func() {
		_, _ = w.stdin.Write([]byte("exit\n"))
		w.closeErr = w.close()
	})
	return w.closeErr
}

func (w *wrappedSSHConn) Read(p []byte) (n int, err error) {
	select {
	case <-w.ctx.Done():
		return 0, xerrors.Errorf("read: %w", w.ctx.Err())
	default:
		return w.stdout.Read(p)
	}
}

func (w *wrappedSSHConn) Write(p []byte) (n int, err error) {
	select {
	case <-w.ctx.Done():
		return 0, xerrors.Errorf("write: %w", w.ctx.Err())
	default:
		return w.stdin.Write(p)
	}
}
