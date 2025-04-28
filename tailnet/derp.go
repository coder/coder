package tailnet

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"strings"
	"sync"

	"tailscale.com/derp"

	"github.com/coder/websocket"
)

// WithWebsocketSupport returns an http.Handler that upgrades
// connections to the "derp" subprotocol to WebSockets and
// passes them to the DERP server.
// Taken from: https://github.com/tailscale/tailscale/blob/e3211ff88ba85435f70984cf67d9b353f3d650d8/cmd/derper/websocket.go#L21
func WithWebsocketSupport(s *derp.Server, base http.Handler) (http.Handler, func()) {
	var mu sync.Mutex
	var waitGroup sync.WaitGroup
	ctx, cancelFunc := context.WithCancel(context.Background())

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			up := strings.ToLower(r.Header.Get("Upgrade"))

			// Very early versions of Tailscale set "Upgrade: WebSocket" but didn't actually
			// speak WebSockets (they still assumed DERP's binary framing). So to distinguish
			// clients that actually want WebSockets, look for an explicit "derp" subprotocol.
			if up != "websocket" || !strings.Contains(r.Header.Get("Sec-Websocket-Protocol"), "derp") {
				base.ServeHTTP(w, r)
				return
			}

			mu.Lock()
			if ctx.Err() != nil {
				mu.Unlock()
				return
			}
			waitGroup.Add(1)
			mu.Unlock()
			defer waitGroup.Done()
			c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				Subprotocols:   []string{"derp"},
				OriginPatterns: []string{"*"},
				// Disable compression because we transmit WireGuard messages that
				// are not compressible.
				// Additionally, Safari has a broken implementation of compression
				// (see https://github.com/nhooyr/websocket/issues/218) that makes
				// enabling it actively harmful.
				CompressionMode: websocket.CompressionDisabled,
			})
			if err != nil {
				log.Printf("websocket.Accept: %v", err)
				return
			}
			defer c.Close(websocket.StatusInternalError, "closing")
			if c.Subprotocol() != "derp" {
				c.Close(websocket.StatusPolicyViolation, "client must speak the derp subprotocol")
				return
			}
			wc := websocket.NetConn(ctx, c, websocket.MessageBinary)
			brw := bufio.NewReadWriter(bufio.NewReader(wc), bufio.NewWriter(wc))
			s.Accept(ctx, wc, brw, r.RemoteAddr)
		}), func() {
			cancelFunc()
			mu.Lock()
			waitGroup.Wait()
			mu.Unlock()
		}
}
