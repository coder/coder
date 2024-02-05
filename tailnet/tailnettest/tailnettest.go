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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/stun/stuntest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"
	"tailscale.com/types/nettype"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

//go:generate mockgen -destination ./multiagentmock.go -package tailnettest github.com/coder/coder/v2/tailnet MultiAgentConn
//go:generate mockgen -destination ./coordinatormock.go -package tailnettest github.com/coder/coder/v2/tailnet Coordinator
//go:generate mockgen -destination ./coordinateemock.go -package tailnettest github.com/coder/coder/v2/tailnet Coordinatee

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

type TestMultiAgent struct {
	t        testing.TB
	id       uuid.UUID
	a        tailnet.MultiAgentConn
	nodeKey  []byte
	discoKey string
}

func NewTestMultiAgent(t testing.TB, coord tailnet.Coordinator) *TestMultiAgent {
	nk, err := key.NewNode().Public().MarshalBinary()
	require.NoError(t, err)
	dk, err := key.NewDisco().Public().MarshalText()
	require.NoError(t, err)
	m := &TestMultiAgent{t: t, id: uuid.New(), nodeKey: nk, discoKey: string(dk)}
	m.a = coord.ServeMultiAgent(m.id)
	return m
}

func (m *TestMultiAgent) SendNodeWithDERP(d int32) {
	m.t.Helper()
	err := m.a.UpdateSelf(&proto.Node{
		Key:           m.nodeKey,
		Disco:         m.discoKey,
		PreferredDerp: d,
	})
	require.NoError(m.t, err)
}

func (m *TestMultiAgent) Close() {
	m.t.Helper()
	err := m.a.Close()
	require.NoError(m.t, err)
}

func (m *TestMultiAgent) RequireSubscribeAgent(id uuid.UUID) {
	m.t.Helper()
	err := m.a.SubscribeAgent(id)
	require.NoError(m.t, err)
}

func (m *TestMultiAgent) RequireUnsubscribeAgent(id uuid.UUID) {
	m.t.Helper()
	err := m.a.UnsubscribeAgent(id)
	require.NoError(m.t, err)
}

func (m *TestMultiAgent) RequireEventuallyHasDERPs(ctx context.Context, expected ...int) {
	m.t.Helper()
	for {
		resp, ok := m.a.NextUpdate(ctx)
		require.True(m.t, ok)
		nodes, err := tailnet.OnlyNodeUpdates(resp)
		require.NoError(m.t, err)
		if len(nodes) != len(expected) {
			m.t.Logf("expected %d, got %d nodes", len(expected), len(nodes))
			continue
		}

		derps := make([]int, 0, len(nodes))
		for _, n := range nodes {
			derps = append(derps, n.PreferredDERP)
		}
		for _, e := range expected {
			if !slices.Contains(derps, e) {
				m.t.Logf("expected DERP %d to be in %v", e, derps)
				continue
			}
			return
		}
	}
}

func (m *TestMultiAgent) RequireNeverHasDERPs(ctx context.Context, expected ...int) {
	m.t.Helper()
	for {
		resp, ok := m.a.NextUpdate(ctx)
		if !ok {
			return
		}
		nodes, err := tailnet.OnlyNodeUpdates(resp)
		require.NoError(m.t, err)
		if len(nodes) != len(expected) {
			m.t.Logf("expected %d, got %d nodes", len(expected), len(nodes))
			continue
		}

		derps := make([]int, 0, len(nodes))
		for _, n := range nodes {
			derps = append(derps, n.PreferredDERP)
		}
		for _, e := range expected {
			if !slices.Contains(derps, e) {
				m.t.Logf("expected DERP %d to be in %v", e, derps)
				continue
			}
			return
		}
	}
}

func (m *TestMultiAgent) RequireEventuallyClosed(ctx context.Context) {
	m.t.Helper()
	tkr := time.NewTicker(testutil.IntervalFast)
	defer tkr.Stop()
	for {
		select {
		case <-ctx.Done():
			m.t.Fatal("timeout")
			return // unhittable
		case <-tkr.C:
			if m.a.IsClosed() {
				return
			}
		}
	}
}

type FakeCoordinator struct {
	CoordinateCalls  chan *FakeCoordinate
	ServeClientCalls chan *FakeServeClient
}

func (*FakeCoordinator) ServeHTTPDebug(http.ResponseWriter, *http.Request) {
	panic("unimplemented")
}

func (*FakeCoordinator) Node(uuid.UUID) *tailnet.Node {
	panic("unimplemented")
}

func (f *FakeCoordinator) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	errCh := make(chan error)
	f.ServeClientCalls <- &FakeServeClient{
		Conn:  conn,
		ID:    id,
		Agent: agent,
		ErrCh: errCh,
	}
	return <-errCh
}

func (*FakeCoordinator) ServeAgent(net.Conn, uuid.UUID, string) error {
	panic("unimplemented")
}

func (*FakeCoordinator) Close() error {
	panic("unimplemented")
}

func (*FakeCoordinator) ServeMultiAgent(uuid.UUID) tailnet.MultiAgentConn {
	panic("unimplemented")
}

func (f *FakeCoordinator) Coordinate(ctx context.Context, id uuid.UUID, name string, a tailnet.TunnelAuth) (chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse) {
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
		CoordinateCalls:  make(chan *FakeCoordinate, 100),
		ServeClientCalls: make(chan *FakeServeClient, 100),
	}
}

type FakeCoordinate struct {
	Ctx   context.Context
	ID    uuid.UUID
	Name  string
	Auth  tailnet.TunnelAuth
	Reqs  chan *proto.CoordinateRequest
	Resps chan *proto.CoordinateResponse
}

type FakeServeClient struct {
	Conn  net.Conn
	ID    uuid.UUID
	Agent uuid.UUID
	ErrCh chan error
}
