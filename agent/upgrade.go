package agent

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type upgradedConn struct {
	net.Conn
	clientSessionID string
	r               *bufio.Reader
}

func (c *upgradedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c *upgradedConn) ClientSessionID() string    { return c.clientSessionID }

type httpUpgradeListener struct {
	mu     sync.Mutex
	logger slog.Logger
	addr   net.Addr
	conn   chan *upgradedConn
	closed chan struct{}
}

func (ln *httpUpgradeListener) Accept() (net.Conn, error) {
	var c *upgradedConn
	select {
	case c = <-ln.conn:
	case <-ln.closed:
		return nil, xerrors.Errorf("tcp upgrade listener: %w", net.ErrClosed)
	}
	return c, nil
}

func (ln *httpUpgradeListener) Addr() net.Addr { return ln.addr }
func (ln *httpUpgradeListener) Close() error {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	select {
	case <-ln.closed:
	default:
		close(ln.closed)
	}
	return nil
}

// handler upgrades an HTTP connection based on the port number, passing along
// any client session ID that exists on the request context.
func (ln *httpUpgradeListener) handler(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	port, err := strconv.ParseUint(chi.URLParam(r, "port"), 10, 16)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Invalid port",
			Detail:  err.Error(),
		})
		return
	}

	supported := []uint64{workspacesdk.AgentStandardSSHPort}
	if !slices.Contains(supported, port) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Unsupported port for upgrading",
			Detail:  fmt.Sprintf("Supported ports: %v", supported),
		})
		return
	}

	if r.Header.Get("upgrade") == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing upgrade header",
		})
		return
	}

	if !strings.EqualFold(r.Header.Get("upgrade"), "tcp") {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid upgrade header",
		})
		return
	}

	clientSessionID := tracing.ClientSessionID(r)
	if clientSessionID == "" {
		ln.logger.Warn(ctx, "client session ID is missing")
	}

	hijacker, ok := rw.(http.Hijacker)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Hijacking not supported",
		})
		return
	}

	conn, brw, err := hijacker.Hijack()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to upgrade connection",
			Detail:  err.Error(),
		})
		return
	}

	_, err = conn.Write([]byte(strings.Join([]string{
		"HTTP/1.1 101 Switching Protocols",
		"Connection: Upgrade",
		"Upgrade: tcp",
	}, "\r\n") + "\r\n\r\n"))
	if err != nil {
		_ = conn.Close()
		return
	}

	uconn := &upgradedConn{
		Conn:            conn,
		clientSessionID: clientSessionID,
		r:               brw.Reader,
	}

	t := time.NewTimer(time.Second)
	defer t.Stop()
	select {
	case ln.conn <- uconn:
		return
	case <-ln.closed:
		ln.logger.Info(ctx, "listener closed; closing connection")
	case <-t.C:
		ln.logger.Info(ctx, "listener timed out accepting; closing connection")
	}
	_ = conn.Close()
}
