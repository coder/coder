package agent

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type upgradedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *upgradedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// handleTCPUpgrade upgrades an HTTP connection based on the port number,
// passing along any client session ID that exists on the request context.
func (a *agent) handleTCPUpgrade(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	port, err := strconv.ParseUint(chi.URLParam(r, "port"), 10, 16)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid port",
			Detail:  err.Error(),
		})
		return
	}

	supported := []uint64{workspacesdk.AgentStandardSSHPort}
	if !slices.Contains(supported, port) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
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

	a.sshServer.HandleUpgrade(&upgradedConn{
		Conn: conn,
		r:    brw.Reader,
	}, clientSessionID)
}
