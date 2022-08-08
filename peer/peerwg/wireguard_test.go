package peerwg_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/stun/stuntest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/logger"
	"tailscale.com/types/nettype"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/peer/peerwg"
)

func TestConnect(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	c1IPv6 := peerwg.UUIDToNetaddr(uuid.New())
	wgn1, err := peerwg.New(logger.Named("c1"), []netip.Prefix{
		netip.PrefixFrom(c1IPv6, 128),
	})
	require.NoError(t, err)

	c2IPv6 := peerwg.UUIDToNetaddr(uuid.New())
	wgn2, err := peerwg.New(logger.Named("c2"), []netip.Prefix{
		netip.PrefixFrom(c2IPv6, 128),
	})
	require.NoError(t, err)
	err = wgn1.AddPeer(peerwg.Handshake{
		DiscoPublicKey: wgn2.DiscoPublicKey,
		NodePublicKey:  wgn2.NodePrivateKey.Public(),
		IPv6:           c2IPv6,
	})
	require.NoError(t, err)

	conn := make(chan struct{})
	go func() {
		listener, err := wgn1.Listen("tcp", ":35565")
		require.NoError(t, err)
		conn <- struct{}{}
		fmt.Printf("Started listening...\n")
		_, err = listener.Accept()
		fmt.Printf("Got connection!\n")
		require.NoError(t, err)
		conn <- struct{}{}
	}()

	err = wgn2.AddPeer(peerwg.Handshake{
		DiscoPublicKey: wgn1.DiscoPublicKey,
		NodePublicKey:  wgn1.NodePrivateKey.Public(),
		IPv6:           c1IPv6,
	})
	require.NoError(t, err)
	<-conn
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("\n\n\n\n\nDIALING TCP\n\n\n\n\n")
	_, err = wgn2.Netstack.DialContextTCP(context.Background(), netip.AddrPortFrom(c1IPv6, 35565))
	require.NoError(t, err)
	<-conn
}

func runDERPAndStun(t *testing.T, logf logger.Logf, l nettype.PacketListener, stunIP netip.Addr) (derpMap *tailcfg.DERPMap, cleanup func()) {
	d := derp.NewServer(key.NewNode(), logf)

	httpsrv := httptest.NewUnstartedServer(derphttp.Handler(d))
	httpsrv.Config.ErrorLog = logger.StdLogger(logf)
	httpsrv.Config.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	httpsrv.StartTLS()

	stunAddr, stunCleanup := stuntest.ServeWithPacketListener(t, l)

	m := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionCode: "test",
				Nodes: []*tailcfg.DERPNode{
					{
						Name:             "t1",
						RegionID:         1,
						HostName:         "test-node.unused",
						IPv4:             "127.0.0.1",
						IPv6:             "none",
						STUNPort:         stunAddr.Port,
						DERPPort:         httpsrv.Listener.Addr().(*net.TCPAddr).Port,
						InsecureForTests: true,
						STUNTestIP:       stunIP.String(),
					},
				},
			},
		},
	}

	cleanup = func() {
		httpsrv.CloseClientConnections()
		httpsrv.Close()
		d.Close()
		stunCleanup()
	}

	return m, cleanup
}
