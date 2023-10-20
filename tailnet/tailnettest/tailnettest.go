package tailnettest

import (
	"crypto/tls"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/stun/stuntest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"
	"tailscale.com/types/nettype"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
)

//go:generate mockgen -destination ./multiagentmock.go -package tailnettest github.com/coder/coder/v2/tailnet MultiAgentConn

// RunDERPAndSTUN creates a DERP mapping for tests.
func RunDERPAndSTUN(t *testing.T) (*tailcfg.DERPMap, *derp.Server) {
	logf := tailnet.Logger(slogtest.Make(t, nil))
	d := derp.NewServer(key.NewNode(), logf)
	server := httptest.NewUnstartedServer(derphttp.Handler(d))
	server.Config.ErrorLog = tslogger.StdLogger(logf)
	server.Config.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	server.StartTLS()

	stunAddr, stunCleanup := stuntest.ServeWithPacketListener(t, nettype.Std{})
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
		d.Close()
		stunCleanup()
	})
	tcpAddr, ok := server.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.FailNow()
	}

	return &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionCode: "test",
				RegionName: "Test",
				Nodes: []*tailcfg.DERPNode{
					{
						Name:             "t2",
						RegionID:         1,
						IPv4:             "127.0.0.1",
						IPv6:             "none",
						STUNPort:         stunAddr.Port,
						DERPPort:         tcpAddr.Port,
						InsecureForTests: true,
					},
				},
			},
		},
	}, d
}

// RunDERPOnlyWebSockets creates a DERP mapping for tests that
// only allows WebSockets through it. Many proxies do not support
// upgrading DERP, so this is a good fallback.
func RunDERPOnlyWebSockets(t *testing.T) *tailcfg.DERPMap {
	logf := tailnet.Logger(slogtest.Make(t, nil))
	d := derp.NewServer(key.NewNode(), logf)
	handler := derphttp.Handler(d)
	var closeFunc func()
	handler, closeFunc = tailnet.WithWebsocketSupport(d, handler)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/derp" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
			return
		}
		if r.Header.Get("Upgrade") != "websocket" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf(`Invalid "Upgrade" header: %s`, html.EscapeString(r.Header.Get("Upgrade")))))
			return
		}
		handler.ServeHTTP(w, r)
	}))
	server.Config.ErrorLog = tslogger.StdLogger(logf)
	server.Config.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	server.StartTLS()
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
		closeFunc()
		d.Close()
	})

	tcpAddr, ok := server.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.FailNow()
	}

	return &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionCode: "test",
				RegionName: "Test",
				Nodes: []*tailcfg.DERPNode{
					{
						Name:             "t1",
						RegionID:         1,
						IPv4:             "127.0.0.1",
						IPv6:             "none",
						DERPPort:         tcpAddr.Port,
						InsecureForTests: true,
					},
				},
			},
		},
	}
}
