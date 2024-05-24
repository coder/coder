//go:build linux
// +build linux

package integration_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/net/stun/stuntest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/nettype"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/test/integration"
	"github.com/coder/coder/v2/testutil"
)

const runTestEnv = "CODER_TAILNET_TESTS"

var (
	isSubprocess = flag.Bool("subprocess", false, "Signifies that this is a test subprocess")
	testID       = flag.String("test-name", "", "Which test is being run")
	role         = flag.String("role", "", "The role of the test subprocess: server, stun, client")

	// Role: server
	serverListenAddr = flag.String("server-listen-addr", "", "The address to listen on for the server")

	// Role: stun
	stunListenAddr = flag.String("stun-listen-addr", "", "The address to listen on for the STUN server")

	// Role: client
	clientName        = flag.String("client-name", "", "The name of the client for logs")
	clientNumber      = flag.Int("client-number", 0, "The number of the client")
	clientServerURL   = flag.String("client-server-url", "", "The url to connect to the server")
	clientDERPMapPath = flag.String("client-derp-map-path", "", "The path to the DERP map file to use on this client")
)

func TestMain(m *testing.M) {
	if run := os.Getenv(runTestEnv); run == "" {
		_, _ = fmt.Printf("skipping tests as %q is not set...\n", runTestEnv)
		return
	}
	if runtime.GOOS != "linux" {
		_, _ = fmt.Printf("GOOS %q is not linux", runtime.GOOS)
		os.Exit(1)
		return
	}
	if os.Getuid() != 0 {
		_, _ = fmt.Println("UID is not 0")
		os.Exit(1)
		return
	}

	flag.Parse()
	os.Exit(m.Run())
}

var topologies = []integration.TestTopology{
	{
		// Test that DERP over loopback works.
		Name:            "BasicLoopbackDERP",
		SetupNetworking: integration.SetupNetworkingLoopback,
		Server:          integration.SimpleServerOptions{},
		StartClient:     integration.StartClientDERP,
		RunTests:        integration.TestSuite,
	},
	{
		// Test that DERP over "easy" NAT works. The server, client 1 and client
		// 2 are on different networks with a shared router, and the router
		// masquerades the traffic.
		Name:            "EasyNATDERP",
		SetupNetworking: integration.SetupNetworkingEasyNAT,
		Server:          integration.SimpleServerOptions{},
		StartClient:     integration.StartClientDERP,
		RunTests:        integration.TestSuite,
	},
	{
		// Test that direct over "easy" NAT works. This should use local
		// endpoints to connect as routing is enabled between client 1 and
		// client 2.
		Name:            "EasyNATDirect",
		SetupNetworking: integration.SetupNetworkingEasyNATWithSTUN,
		Server:          integration.SimpleServerOptions{},
		StartClient:     integration.StartClientDirect,
		RunTests:        integration.TestSuite,
	},
	{
		// Test that DERP over WebSocket (as well as DERPForceWebSockets works).
		// This does not test the actual DERP failure detection code and
		// automatic fallback.
		Name:            "DERPForceWebSockets",
		SetupNetworking: integration.SetupNetworkingEasyNAT,
		Server: integration.SimpleServerOptions{
			FailUpgradeDERP:   false,
			DERPWebsocketOnly: true,
		},
		StartClient: integration.StartClientDERPWebSockets,
		RunTests:    integration.TestSuite,
	},
	{
		// Test that falling back to DERP over WebSocket works.
		Name:            "DERPFallbackWebSockets",
		SetupNetworking: integration.SetupNetworkingEasyNAT,
		Server: integration.SimpleServerOptions{
			FailUpgradeDERP:   true,
			DERPWebsocketOnly: false,
		},
		// Use a basic client that will try `Upgrade: derp` first.
		StartClient: integration.StartClientDERP,
		RunTests:    integration.TestSuite,
	},
	{
		Name:            "BasicLoopbackDERPNGINX",
		SetupNetworking: integration.SetupNetworkingLoopback,
		Server:          integration.NGINXServerOptions{},
		StartClient:     integration.StartClientDERP,
		RunTests:        integration.TestSuite,
	},
}

//nolint:paralleltest,tparallel
func TestIntegration(t *testing.T) {
	if *isSubprocess {
		handleTestSubprocess(t)
		return
	}

	for _, topo := range topologies {
		topo := topo
		t.Run(topo.Name, func(t *testing.T) {
			// These can run in parallel because every test should be in an
			// isolated NetNS.
			t.Parallel()

			log := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			networking := topo.SetupNetworking(t, log)

			// Useful for debugging network namespaces by avoiding cleanup.
			// t.Cleanup(func() {
			// 	time.Sleep(time.Minute * 15)
			// })

			closeServer := startServerSubprocess(t, topo.Name, networking)

			closeSTUN := func() error { return nil }
			if networking.STUN.ListenAddr != "" {
				closeSTUN = startSTUNSubprocess(t, topo.Name, networking)
			}

			// Write the DERP maps to a file.
			tempDir := t.TempDir()
			client1DERPMapPath := filepath.Join(tempDir, "client1-derp-map.json")
			client1DERPMap, err := networking.Client1.ResolveDERPMap()
			require.NoError(t, err, "resolve client 1 DERP map")
			err = writeDERPMapToFile(client1DERPMapPath, client1DERPMap)
			require.NoError(t, err, "write client 1 DERP map")
			client2DERPMapPath := filepath.Join(tempDir, "client2-derp-map.json")
			client2DERPMap, err := networking.Client2.ResolveDERPMap()
			require.NoError(t, err, "resolve client 2 DERP map")
			err = writeDERPMapToFile(client2DERPMapPath, client2DERPMap)
			require.NoError(t, err, "write client 2 DERP map")

			// client1 runs the tests.
			client1ErrCh, _ := startClientSubprocess(t, topo.Name, networking, integration.Client1, client1DERPMapPath)
			_, closeClient2 := startClientSubprocess(t, topo.Name, networking, integration.Client2, client2DERPMapPath)

			// Wait for client1 to exit.
			require.NoError(t, <-client1ErrCh, "client 1 exited")

			// Close client2 and the server.
			require.NoError(t, closeClient2(), "client 2 exited")
			require.NoError(t, closeSTUN(), "stun exited")
			require.NoError(t, closeServer(), "server exited")
		})
	}
}

func handleTestSubprocess(t *testing.T) {
	// Find the specific topology.
	var topo integration.TestTopology
	for _, t := range topologies {
		if t.Name == *testID {
			topo = t
			break
		}
	}
	require.NotEmptyf(t, topo.Name, "unknown test topology %q", *testID)
	require.Contains(t, []string{"server", "stun", "client"}, *role, "unknown role %q", *role)

	testName := topo.Name + "/"
	if *role == "server" || *role == "stun" {
		testName += *role
	} else {
		testName += *clientName
	}

	t.Run(testName, func(t *testing.T) {
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		switch *role {
		case "server":
			logger = logger.Named("server")
			topo.Server.StartServer(t, logger, *serverListenAddr)
			// no exit

		case "stun":
			launchSTUNServer(t, *stunListenAddr)
			// no exit

		case "client":
			logger = logger.Named(*clientName)
			if *clientNumber != int(integration.ClientNumber1) && *clientNumber != int(integration.ClientNumber2) {
				t.Fatalf("invalid client number %d", clientNumber)
			}
			me, peer := integration.Client1, integration.Client2
			if *clientNumber == int(integration.ClientNumber2) {
				me, peer = peer, me
			}

			serverURL, err := url.Parse(*clientServerURL)
			require.NoErrorf(t, err, "parse server url %q", *clientServerURL)

			// Load the DERP map.
			var derpMap tailcfg.DERPMap
			derpMapPath := *clientDERPMapPath
			f, err := os.Open(derpMapPath)
			require.NoErrorf(t, err, "open DERP map %q", derpMapPath)
			err = json.NewDecoder(f).Decode(&derpMap)
			_ = f.Close()
			require.NoErrorf(t, err, "decode DERP map %q", derpMapPath)

			waitForServerAvailable(t, serverURL)

			conn := topo.StartClient(t, logger, serverURL, &derpMap, me, peer)

			if me.ShouldRunTests {
				// Wait for connectivity.
				peerIP := tailnet.IPFromUUID(peer.ID)
				if !conn.AwaitReachable(testutil.Context(t, testutil.WaitLong), peerIP) {
					t.Fatalf("peer %v did not become reachable", peerIP)
				}

				topo.RunTests(t, logger, serverURL, conn, me, peer)
				// then exit
				return
			}
		}

		// Wait for signals.
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
		<-signals
	})
}

type forcedAddrPacketListener struct {
	addr string
}

var _ nettype.PacketListener = forcedAddrPacketListener{}

func (ln forcedAddrPacketListener) ListenPacket(ctx context.Context, network, _ string) (net.PacketConn, error) {
	return nettype.Std{}.ListenPacket(ctx, network, ln.addr)
}

func launchSTUNServer(t *testing.T, listenAddr string) {
	ln := forcedAddrPacketListener{addr: listenAddr}
	addr, cleanup := stuntest.ServeWithPacketListener(t, ln)
	t.Cleanup(cleanup)
	assert.Equal(t, listenAddr, addr.String(), "listen address should match forced addr")
}

func waitForServerAvailable(t *testing.T, serverURL *url.URL) {
	const delay = 100 * time.Millisecond
	const reqTimeout = 2 * time.Second
	const timeout = 30 * time.Second
	client := http.Client{
		Timeout: reqTimeout,
	}

	u, err := url.Parse(serverURL.String() + "/derp/latency-check")
	require.NoError(t, err)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(delay) {
		//nolint:noctx
		resp, err := client.Get(u.String())
		if err != nil {
			t.Logf("waiting for server to be available: %v", err)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Logf("waiting for server to be available: got status %d", resp.StatusCode)
			continue
		}
		return
	}

	t.Fatalf("server did not become available after %v", timeout)
}

func startServerSubprocess(t *testing.T, topologyName string, networking integration.TestNetworking) func() error {
	_, closeFn := startSubprocess(t, "server", networking.Server.Process.NetNS, []string{
		"--subprocess",
		"--test-name=" + topologyName,
		"--role=server",
		"--server-listen-addr=" + networking.Server.ListenAddr,
	})
	return closeFn
}

func startSTUNSubprocess(t *testing.T, topologyName string, networking integration.TestNetworking) func() error {
	_, closeFn := startSubprocess(t, "stun", networking.STUN.Process.NetNS, []string{
		"--subprocess",
		"--test-name=" + topologyName,
		"--role=stun",
		"--stun-listen-addr=" + networking.STUN.ListenAddr,
	})
	return closeFn
}

func startClientSubprocess(t *testing.T, topologyName string, networking integration.TestNetworking, me integration.Client, derpMapPath string) (<-chan error, func() error) {
	var (
		clientName          = fmt.Sprintf("client%d", me.Number)
		clientProcessConfig = networking.Client1
	)
	if me.Number == integration.ClientNumber2 {
		clientProcessConfig = networking.Client2
	}

	flags := []string{
		"--subprocess",
		"--test-name=" + topologyName,
		"--role=client",
		"--client-name=" + clientName,
		"--client-number=" + strconv.Itoa(int(me.Number)),
		"--client-server-url=" + clientProcessConfig.ServerAccessURL,
		"--client-derp-map-path=" + derpMapPath,
	}

	return startSubprocess(t, clientName, clientProcessConfig.Process.NetNS, flags)
}

// startSubprocess launches the test binary with the same flags as the test, but
// with additional flags added.
//
// See integration.ExecBackground for more details.
func startSubprocess(t *testing.T, processName string, netNS *os.File, flags []string) (<-chan error, func() error) {
	name := os.Args[0]
	// Always use verbose mode since it gets piped to the parent test anyways.
	args := append(os.Args[1:], append([]string{"-test.v=true"}, flags...)...) //nolint:gocritic
	return integration.ExecBackground(t, processName, netNS, name, args)
}

func writeDERPMapToFile(path string, derpMap *tailcfg.DERPMap) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(derpMap)
	if err != nil {
		return err
	}
	return nil
}
