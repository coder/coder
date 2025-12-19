package tailnet

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"tailscale.com/derp"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3"

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

type DERPMapRewriter interface {
	RewriteDERPMap(derpMap *tailcfg.DERPMap)
}

// RewriteDERPMapDefaultRelay rewrites the DERP map to use the given access URL
// as the "embedded relay" access URL. The passed derp map is modified in place.
//
// This is used by clients and agents to rewrite the default DERP relay to use
// their preferred access URL. Both of these clients can use a different access
// URL than the deployment has configured (with `--access-url`), so we need to
// accommodate that and respect the locally configured access URL.
//
// Note: passed context is only used for logging.
func RewriteDERPMapDefaultRelay(ctx context.Context, logger slog.Logger, derpMap *tailcfg.DERPMap, accessURL *url.URL) {
	if derpMap == nil {
		return
	}

	accessPort := 80
	if accessURL.Scheme == "https" {
		accessPort = 443
	}
	if accessURL.Port() != "" {
		parsedAccessPort, err := strconv.Atoi(accessURL.Port())
		if err != nil {
			// This should never happen because URL.Port() returns the empty string
			// if the port is not valid.
			logger.Critical(ctx, "failed to parse URL port, using default port",
				slog.F("port", accessURL.Port()),
				slog.F("access_url", accessURL))
		} else {
			accessPort = parsedAccessPort
		}
	}

	for _, region := range derpMap.Regions {
		if !region.EmbeddedRelay {
			continue
		}

		for _, node := range region.Nodes {
			if node.STUNOnly {
				continue
			}
			node.HostName = accessURL.Hostname()
			node.DERPPort = accessPort
			node.ForceHTTP = accessURL.Scheme == "http"
		}
	}
}
