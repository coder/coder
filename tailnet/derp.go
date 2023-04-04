package tailnet

import (
	"bufio"
	"net/http"
	"strings"

	"nhooyr.io/websocket"
	"tailscale.com/derp"
	"tailscale.com/net/wsconn"
)

type HandleWebsocket func(rw http.ResponseWriter, r *http.Request, options *websocket.AcceptOptions, f func(conn *websocket.Conn))

// WithWebsocketSupport returns an http.Handler that upgrades
// connections to the "derp" subprotocol to WebSockets and
// passes them to the DERP server.
// Taken from: https://github.com/tailscale/tailscale/blob/e3211ff88ba85435f70984cf67d9b353f3d650d8/cmd/derper/websocket.go#L21
// The accept function is used to accept the websocket connection and allows the caller to
// also affect the lifecycle of the websocket connection. (Eg. to close the connection on shutdown)
func WithWebsocketSupport(accept HandleWebsocket, s *derp.Server, base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := strings.ToLower(r.Header.Get("Upgrade"))

		// Very early versions of Tailscale set "Upgrade: WebSocket" but didn't actually
		// speak WebSockets (they still assumed DERP's binary framing). So to distinguish
		// clients that actually want WebSockets, look for an explicit "derp" subprotocol.
		if up != "websocket" || !strings.Contains(r.Header.Get("Sec-Websocket-Protocol"), "derp") {
			base.ServeHTTP(w, r)
			return
		}

		accept(w, r, &websocket.AcceptOptions{
			Subprotocols:   []string{"derp"},
			OriginPatterns: []string{"*"},
			// Disable compression because we transmit WireGuard messages that
			// are not compressible.
			// Additionally, Safari has a broken implementation of compression
			// (see https://github.com/nhooyr/websocket/issues/218) that makes
			// enabling it actively harmful.
			CompressionMode: websocket.CompressionDisabled,
		}, func(conn *websocket.Conn) {
			defer conn.Close(websocket.StatusInternalError, "closing")
			if conn.Subprotocol() != "derp" {
				conn.Close(websocket.StatusPolicyViolation, "client must speak the derp subprotocol")
				return
			}
			wc := wsconn.NetConn(r.Context(), conn, websocket.MessageBinary)
			brw := bufio.NewReadWriter(bufio.NewReader(wc), bufio.NewWriter(wc))
			s.Accept(r.Context(), wc, brw, r.RemoteAddr)
		})
	})
}
