package workspacetraffic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/websocket"
)

const (
	// Set a timeout for graceful close of the connection.
	connCloseTimeout = 30 * time.Second
	// Set a timeout for waiting for the connection to close.
	waitCloseTimeout = connCloseTimeout + 5*time.Second

	// In theory, we can send larger payloads to push bandwidth, but we need to
	// be careful not to send too much data at once or the server will close the
	// connection. We see this more readily as our JSON payloads approach 28KB.
	//
	// 	failed to write frame: WebSocket closed: received close frame: status = StatusMessageTooBig and reason = "read limited at 32769 bytes"
	//
	// Since we can't control fragmentation/buffer sizes, we keep it simple and
	// match the conservative payload size used by agent/reconnectingpty (1024).
	rptyJSONMaxDataSize = 1024
)

func connectRPTY(ctx context.Context, client *codersdk.Client, agentID, reconnect uuid.UUID, cmd string) (*countReadWriteCloser, error) {
	width, height := 80, 25
	conn, err := workspacesdk.New(client).AgentReconnectingPTY(ctx, workspacesdk.WorkspaceAgentReconnectingPTYOpts{
		AgentID:   agentID,
		Reconnect: reconnect,
		Width:     uint16(width),
		Height:    uint16(height),
		Command:   cmd,
	})
	if err != nil {
		return nil, xerrors.Errorf("connect pty: %w", err)
	}

	// Wrap the conn in a countReadWriteCloser so we can monitor bytes sent/rcvd.
	crw := countReadWriteCloser{rwc: newPTYConn(conn)}
	return &crw, nil
}

type rptyConn struct {
	conn io.ReadWriteCloser
	wenc *json.Encoder

	readOnce sync.Once
	readErr  chan error

	mu     sync.Mutex // Protects following.
	closed bool
}

func newPTYConn(conn io.ReadWriteCloser) *rptyConn {
	rc := &rptyConn{
		conn:    conn,
		wenc:    json.NewEncoder(conn),
		readErr: make(chan error, 1),
	}
	return rc
}

func (c *rptyConn) Read(p []byte) (int, error) {
	n, err := c.conn.Read(p)
	if err != nil {
		c.readOnce.Do(func() {
			c.readErr <- err
			close(c.readErr)
		})
		return n, err
	}
	return n, nil
}

func (c *rptyConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Early exit in case we're closing, this is to let call write Ctrl+C
	// without a flood of other writes.
	if c.closed {
		return 0, io.EOF
	}

	return c.writeNoLock(p)
}

func (c *rptyConn) writeNoLock(p []byte) (n int, err error) {
	// If we try to send more than the max payload size, the server will close the connection.
	for len(p) > 0 {
		pp := p
		if len(pp) > rptyJSONMaxDataSize {
			pp = p[:rptyJSONMaxDataSize]
		}
		p = p[len(pp):]
		req := workspacesdk.ReconnectingPTYRequest{Data: string(pp)}
		if err := c.wenc.Encode(req); err != nil {
			return n, xerrors.Errorf("encode pty request: %w", err)
		}
		n += len(pp)
	}
	return n, nil
}

func (c *rptyConn) Close() (err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	defer c.conn.Close()

	// Send Ctrl+C to interrupt the command.
	_, err = c.writeNoLock([]byte("\u0003"))
	if err != nil {
		return xerrors.Errorf("write ctrl+c: %w", err)
	}
	select {
	case <-time.After(connCloseTimeout):
		return xerrors.Errorf("timeout waiting for read to finish")
	case err = <-c.readErr:
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

//nolint:revive // Ignore requestPTY control flag.
func connectSSH(ctx context.Context, client *codersdk.Client, agentID uuid.UUID, cmd string, requestPTY bool) (rwc *countReadWriteCloser, err error) {
	var closers []func() error
	defer func() {
		if err != nil {
			for _, c := range closers {
				if err2 := c(); err2 != nil {
					err = errors.Join(err, err2)
				}
			}
		}
	}()

	agentConn, err := workspacesdk.New(client).DialAgent(ctx, agentID, &workspacesdk.DialAgentOptions{})
	if err != nil {
		return nil, xerrors.Errorf("dial workspace agent: %w", err)
	}
	closers = append(closers, agentConn.Close)

	sshClient, err := agentConn.SSHClient(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get ssh client: %w", err)
	}
	closers = append(closers, sshClient.Close)

	sshSession, err := sshClient.NewSession()
	if err != nil {
		return nil, xerrors.Errorf("new ssh session: %w", err)
	}
	closers = append(closers, sshSession.Close)

	wrappedConn := &wrappedSSHConn{}

	// Do some plumbing to hook up the wrappedConn
	pr1, pw1 := io.Pipe()
	closers = append(closers, pr1.Close, pw1.Close)
	wrappedConn.stdout = pr1
	sshSession.Stdout = pw1

	pr2, pw2 := io.Pipe()
	closers = append(closers, pr2.Close, pw2.Close)
	sshSession.Stdin = pr2
	wrappedConn.stdin = pw2

	if requestPTY {
		err = sshSession.RequestPty("xterm", 25, 80, gossh.TerminalModes{})
		if err != nil {
			return nil, xerrors.Errorf("request pty: %w", err)
		}
	}
	err = sshSession.Start(cmd)
	if err != nil {
		return nil, xerrors.Errorf("shell: %w", err)
	}
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- sshSession.Wait()
	}()

	closeFn := func() error {
		// Start by closing stdin so we stop writing to the ssh session.
		merr := pw2.Close()
		if err := sshSession.Signal(gossh.SIGHUP); err != nil {
			merr = errors.Join(merr, err)
		}
		select {
		case <-time.After(connCloseTimeout):
			merr = errors.Join(merr, xerrors.Errorf("timeout waiting for ssh session to close"))
		case err := <-waitErr:
			if err != nil {
				var exitErr *gossh.ExitError
				if xerrors.As(err, &exitErr) {
					// The exit status is 255 when the command is
					// interrupted by a signal. This is expected.
					if exitErr.ExitStatus() != 255 {
						merr = errors.Join(merr, xerrors.Errorf("ssh session exited with unexpected status: %d", int32(exitErr.ExitStatus())))
					}
				} else {
					merr = errors.Join(merr, err)
				}
			}
		}
		for _, c := range closers {
			if err := c(); err != nil {
				if !errors.Is(err, io.EOF) {
					merr = errors.Join(merr, err)
				}
			}
		}
		return merr
	}
	wrappedConn.close = closeFn

	crw := &countReadWriteCloser{rwc: wrappedConn}

	return crw, nil
}

// wrappedSSHConn wraps an ssh.Session to implement io.ReadWriteCloser.
type wrappedSSHConn struct {
	stdout    io.Reader
	stdin     io.WriteCloser
	closeOnce sync.Once
	closeErr  error
	close     func() error
}

func (w *wrappedSSHConn) Close() error {
	w.closeOnce.Do(func() {
		w.closeErr = w.close()
	})
	return w.closeErr
}

func (w *wrappedSSHConn) Read(p []byte) (n int, err error) {
	return w.stdout.Read(p)
}

func (w *wrappedSSHConn) Write(p []byte) (n int, err error) {
	return w.stdin.Write(p)
}

func appClientConn(ctx context.Context, client *codersdk.Client, url string) (*countReadWriteCloser, error) {
	headers := http.Header{}
	tokenHeader := codersdk.SessionTokenHeader
	if client.SessionTokenHeader != "" {
		tokenHeader = client.SessionTokenHeader
	}
	headers.Set(tokenHeader, client.SessionToken())

	//nolint:bodyclose // The websocket conn manages the body.
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: client.HTTPClient,
		HTTPHeader: headers,
	})
	if err != nil {
		return nil, xerrors.Errorf("websocket dial: %w", err)
	}

	netConn := websocketNetConn(conn, websocket.MessageBinary)

	// Wrap the conn in a countReadWriteCloser so we can monitor bytes sent/rcvd.
	crw := &countReadWriteCloser{rwc: netConn}
	return crw, nil
}

// wsNetConn wraps net.Conn created by websocket.NetConn(). Cancel func
// is called if a read or write error is encountered.
type wsNetConn struct {
	net.Conn

	writeMu sync.Mutex
	readMu  sync.Mutex

	cancel  context.CancelFunc
	closeMu sync.Mutex
	closed  bool
}

func (c *wsNetConn) Read(b []byte) (n int, err error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.isClosed() {
		return 0, io.EOF
	}
	n, err = c.Conn.Read(b)
	if err != nil {
		if c.isClosed() {
			return n, io.EOF
		}
		return n, err
	}
	return n, nil
}

func (c *wsNetConn) Write(b []byte) (n int, err error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isClosed() {
		return 0, io.EOF
	}

	for len(b) > 0 {
		bb := b
		if len(bb) > rptyJSONMaxDataSize {
			bb = b[:rptyJSONMaxDataSize]
		}
		b = b[len(bb):]
		nn, err := c.Conn.Write(bb)
		n += nn
		if err != nil {
			if c.isClosed() {
				return n, io.EOF
			}
			return n, err
		}
	}
	return n, nil
}

func (c *wsNetConn) isClosed() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closed
}

func (c *wsNetConn) Close() error {
	c.closeMu.Lock()
	closed := c.closed
	c.closed = true
	c.closeMu.Unlock()

	if closed {
		return nil
	}

	// Cancel before acquiring locks to speed up teardown.
	c.cancel()

	c.readMu.Lock()
	defer c.readMu.Unlock()
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_ = c.Conn.Close()
	return nil
}

func websocketNetConn(conn *websocket.Conn, msgType websocket.MessageType) net.Conn {
	// Since `websocket.NetConn` binds to a context for the lifetime of the
	// connection, we need to create a new context that can be canceled when
	// the connection is closed.
	ctx, cancel := context.WithCancel(context.Background())
	nc := websocket.NetConn(ctx, conn, msgType)
	return &wsNetConn{cancel: cancel, Conn: nc}
}
