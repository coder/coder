package tailnettest

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/stun/stuntest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"
	"tailscale.com/types/nettype"

	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

//go:generate mockgen -destination ./coordinatormock.go -package tailnettest github.com/coder/coder/v2/tailnet Coordinator
//go:generate mockgen -destination ./coordinateemock.go -package tailnettest github.com/coder/coder/v2/tailnet Coordinatee
//go:generate mockgen -destination ./workspaceupdatesprovidermock.go -package tailnettest github.com/coder/coder/v2/tailnet WorkspaceUpdatesProvider
//go:generate mockgen -destination ./subscriptionmock.go -package tailnettest github.com/coder/coder/v2/tailnet Subscription

type derpAndSTUNCfg struct {
	DisableSTUN    bool
	DERPIsEmbedded bool
}

type DERPAndStunOption func(cfg *derpAndSTUNCfg)

func DisableSTUN(cfg *derpAndSTUNCfg) {
	cfg.DisableSTUN = true
}

func DERPIsEmbedded(cfg *derpAndSTUNCfg) {
	cfg.DERPIsEmbedded = true
}

// RunDERPAndSTUN creates a DERP mapping for tests.
func RunDERPAndSTUN(t *testing.T, opts ...DERPAndStunOption) (*tailcfg.DERPMap, *derp.Server) {
	cfg := new(derpAndSTUNCfg)
	for _, o := range opts {
		o(cfg)
	}
	logf := tailnet.Logger(testutil.Logger(t))
	d := derp.NewServer(key.NewNode(), logf)
	server := httptest.NewUnstartedServer(derphttp.Handler(d))
	server.Config.ErrorLog = tslogger.StdLogger(logf)
	server.Config.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	server.StartTLS()
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
		d.Close()
	})

	stunPort := -1
	if !cfg.DisableSTUN {
		stunAddr, stunCleanup := stuntest.ServeWithPacketListener(t, nettype.Std{})
		t.Cleanup(func() {
			stunCleanup()
		})
		stunPort = stunAddr.Port
	}
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
						STUNPort:         stunPort,
						DERPPort:         tcpAddr.Port,
						InsecureForTests: true,
					},
				},
				EmbeddedRelay: cfg.DERPIsEmbedded,
			},
		},
	}, d
}

// RunDERPOnlyWebSockets creates a DERP mapping for tests that
// only allows WebSockets through it. Many proxies do not support
// upgrading DERP, so this is a good fallback.
func RunDERPOnlyWebSockets(t *testing.T) *tailcfg.DERPMap {
	logf := tailnet.Logger(testutil.Logger(t))
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

type FakeCoordinator struct {
	CoordinateCalls chan *FakeCoordinate
}

func (*FakeCoordinator) ServeHTTPDebug(http.ResponseWriter, *http.Request) {
	panic("unimplemented")
}

func (*FakeCoordinator) Node(uuid.UUID) *tailnet.Node {
	panic("unimplemented")
}

func (*FakeCoordinator) Close() error {
	panic("unimplemented")
}

func (f *FakeCoordinator) Coordinate(ctx context.Context, id uuid.UUID, name string, a tailnet.CoordinateeAuth) (chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse) {
	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	f.CoordinateCalls <- &FakeCoordinate{
		Ctx:   ctx,
		ID:    id,
		Name:  name,
		Auth:  a,
		Reqs:  reqs,
		Resps: resps,
	}
	return reqs, resps
}

func NewFakeCoordinator() *FakeCoordinator {
	return &FakeCoordinator{
		CoordinateCalls: make(chan *FakeCoordinate, 100),
	}
}

type FakeCoordinate struct {
	Ctx   context.Context
	ID    uuid.UUID
	Name  string
	Auth  tailnet.CoordinateeAuth
	Reqs  chan *proto.CoordinateRequest
	Resps chan *proto.CoordinateResponse
}

type FakeServeClient struct {
	Conn  net.Conn
	ID    uuid.UUID
	Agent uuid.UUID
	ErrCh chan error
}
