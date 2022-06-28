package tailnet_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"inet.af/netaddr"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	tslogger "tailscale.com/types/logger"

	"github.com/coder/coder/tailnet"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTailnet(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	derpMap := runDERPAndStun(t, tailnet.Logger(logger.Named("derp")))

	w1IP := tailnet.IP()
	w1, err := tailnet.New(&tailnet.Options{
		Addresses: []netaddr.IPPrefix{netaddr.IPPrefixFrom(w1IP, 128)},
		Logger:    logger.Named("w1"),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)

	// When a new connection occurs, we want those nodes to exist for the lifetime of the connection.
	// As soon as the connection ends, the nodes can be removed.

	// The workspace agent creates a Tailnet on start. It updates keys and
	// begins listening for connection messages.
	//
	// A new connection starts by concurrently sending a POST request with
	// it's keys, and using a GET request on the workspace agent.
	//
	// Internally, the agent WebSocket listens for these messages.
	// If the agent dies and comes back to life,

	w2, err := tailnet.New(&tailnet.Options{
		Addresses: []netaddr.IPPrefix{netaddr.IPPrefixFrom(tailnet.IP(), 128)},
		Logger:    logger.Named("w2"),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = w1.Close()
		_ = w2.Close()
	})
	w1.SetNodeCallback(func(node *tailnet.Node) {
		w2.UpdateNodes([]*tailnet.Node{node})
	})
	w2.SetNodeCallback(func(node *tailnet.Node) {
		w1.UpdateNodes([]*tailnet.Node{node})
	})

	conn := make(chan struct{})
	go func() {
		listener, err := w1.Listen("tcp", ":35565")
		assert.NoError(t, err)
		defer listener.Close()
		nc, err := listener.Accept()
		assert.NoError(t, err)
		_ = nc.Close()
		conn <- struct{}{}
	}()

	nc, err := w2.DialContextTCP(context.Background(), netaddr.IPPortFrom(w1IP, 35565))
	require.NoError(t, err)
	_ = nc.Close()
	<-conn

	time.Sleep(time.Minute)

	w1.Close()
	w2.Close()
}

func runDERPAndStun(t *testing.T, logf tslogger.Logf) (derpMap *tailcfg.DERPMap) {
	d := derp.NewServer(key.NewNode(), logf)
	server := httptest.NewUnstartedServer(derphttp.Handler(d))
	server.Config.ErrorLog = tslogger.StdLogger(logf)
	server.Config.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	server.StartTLS()

	go func() {
		time.Sleep(5 * time.Second)
		fmt.Printf("\n\n\n\n\nSHUTTING IT DOWN\n\n\n\n\n")
		server.CloseClientConnections()
		server.Close()
		d.Close()
	}()

	// stunAddr, stunCleanup := stuntest.ServeWithPacketListener(t, nettype.Std{})
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
		d.Close()
		// stunCleanup()
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
				RegionName: "Testlandia",
				Nodes: []*tailcfg.DERPNode{
					{
						Name:             "t1",
						RegionID:         1,
						HostName:         "test-node.dns",
						IPv4:             "127.0.0.1",
						IPv6:             "none",
						STUNPort:         -1,
						DERPPort:         tcpAddr.Port,
						InsecureForTests: true,
						STUNTestIP:       "127.0.0.1",
					},
				},
			},
		},
	}
}
