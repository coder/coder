//go:build linux
// +build linux

package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/packet"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/wgengine/capture"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

type ClientNumber int

const (
	ClientNumber1 ClientNumber = 1
	ClientNumber2 ClientNumber = 2
)

type Client struct {
	Number         ClientNumber
	ID             uuid.UUID
	ListenPort     uint16
	ShouldRunTests bool
	TunnelSrc      bool
}

var Client1 = Client{
	Number:         ClientNumber1,
	ID:             uuid.MustParse("00000000-0000-0000-0000-000000000001"),
	ListenPort:     client1Port,
	ShouldRunTests: true,
	TunnelSrc:      true,
}

var Client2 = Client{
	Number:         ClientNumber2,
	ID:             uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	ListenPort:     client2Port,
	ShouldRunTests: false,
	TunnelSrc:      false,
}

type TestTopology struct {
	Name string

	NetworkingProvider NetworkingProvider

	// Server is the server starter for the test. It is executed in the server
	// subprocess.
	Server ServerStarter
	// ClientStarter.StartClient gets called in each client subprocess. It's expected to
	// create the tailnet.Conn and ensure connectivity to it's peer.
	ClientStarter ClientStarter

	// RunTests is the main test function. It's called in each of the client
	// subprocesses. If tests can only run once, they should check the client ID
	// and return early if it's not the expected one.
	RunTests func(t *testing.T, logger slog.Logger, serverURL *url.URL, conn *tailnet.Conn, me Client, peer Client)
}

type ServerStarter interface {
	// StartServer should start the server and return once it's listening. It
	// should not block once it's listening. Cleanup should be handled by
	// t.Cleanup.
	StartServer(t *testing.T, logger slog.Logger, listenAddr string)
}

type NetworkingProvider interface {
	// SetupNetworking creates interfaces and network namespaces for the test.
	// The most simple implementation is NetworkSetupDefault, which only creates
	// a network namespace shared for all tests.
	SetupNetworking(t *testing.T, logger slog.Logger) TestNetworking
}

type ClientStarter interface {
	StartClient(t *testing.T, logger slog.Logger, serverURL *url.URL, derpMap *tailcfg.DERPMap, me Client, peer Client) *tailnet.Conn
}

type SimpleServerOptions struct {
	// FailUpgradeDERP will make the DERP server fail to handle the initial DERP
	// upgrade in a way that causes the client to fallback to
	// DERP-over-WebSocket fallback automatically.
	// Incompatible with DERPWebsocketOnly.
	FailUpgradeDERP bool
	// DERPWebsocketOnly will make the DERP server only accept WebSocket
	// connections. If a DERP request is received that is not using WebSocket
	// fallback, the test will fail.
	// Incompatible with FailUpgradeDERP.
	DERPWebsocketOnly bool
}

var _ ServerStarter = SimpleServerOptions{}

type connManager struct {
	mu    sync.Mutex
	conns map[uuid.UUID]net.Conn
}

func (c *connManager) Add(id uuid.UUID, conn net.Conn) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conns[id] = conn
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.conns, id)
	}
}

func (c *connManager) CloseAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		_ = conn.Close()
	}
	c.conns = make(map[uuid.UUID]net.Conn)
}

type derpServer struct {
	http.Handler
	srv     *derp.Server
	closeFn func()
}

func newDerpServer(t *testing.T, logger slog.Logger) *derpServer {
	derpSrv := derp.NewServer(key.NewNode(), tailnet.Logger(logger.Named("derp")))
	derpHandler, derpCloseFunc := tailnet.WithWebsocketSupport(derpSrv, derphttp.Handler(derpSrv))
	t.Cleanup(derpCloseFunc)
	return &derpServer{
		srv:     derpSrv,
		Handler: derpHandler,
		closeFn: derpCloseFunc,
	}
}

func (s *derpServer) Close() {
	s.srv.Close()
	s.closeFn()
}

//nolint:revive
func (o SimpleServerOptions) Router(t *testing.T, logger slog.Logger) *chi.Mux {
	coord := tailnet.NewCoordinator(logger)
	var coordPtr atomic.Pointer[tailnet.Coordinator]
	coordPtr.Store(&coord)
	t.Cleanup(func() { _ = coord.Close() })

	cm := connManager{
		conns: make(map[uuid.UUID]net.Conn),
	}

	csvc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                 logger,
		CoordPtr:               &coordPtr,
		DERPMapUpdateFrequency: 10 * time.Minute,
		DERPMapFn: func() *tailcfg.DERPMap {
			return &tailcfg.DERPMap{
				// Clients will set their own based on their custom access URL.
				Regions: map[int]*tailcfg.DERPRegion{},
			}
		},
		NetworkTelemetryHandler: func(batch []*tailnetproto.TelemetryEvent) {},
		ResumeTokenProvider:     tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	derpServer := atomic.Pointer[derpServer]{}
	derpServer.Store(newDerpServer(t, logger))
	t.Cleanup(func() {
		derpServer.Load().Close()
	})

	r := chi.NewRouter()
	r.Use(
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				logger.Debug(r.Context(), "start "+r.Method, slog.F("path", r.URL.Path), slog.F("remote_ip", r.RemoteAddr))
				next.ServeHTTP(w, r)
			})
		},
		tracing.StatusWriterMiddleware,
		loggermw.Logger(logger),
	)

	r.Route("/derp", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			logger.Info(r.Context(), "start derp request", slog.F("path", r.URL.Path), slog.F("remote_ip", r.RemoteAddr))

			upgrade := strings.ToLower(r.Header.Get("Upgrade"))
			if upgrade != "derp" && upgrade != "websocket" {
				http.Error(w, "invalid DERP upgrade header", http.StatusBadRequest)
				t.Errorf("invalid DERP upgrade header: %s", upgrade)
				return
			}

			if o.FailUpgradeDERP && upgrade == "derp" {
				// 4xx status codes will cause the client to fallback to
				// DERP-over-WebSocket.
				http.Error(w, "test derp upgrade failure", http.StatusBadRequest)
				return
			}
			if o.DERPWebsocketOnly && upgrade != "websocket" {
				logger.Error(r.Context(), "non-websocket DERP request received", slog.F("path", r.URL.Path), slog.F("remote_ip", r.RemoteAddr))
				http.Error(w, "non-websocket DERP request received", http.StatusBadRequest)
				t.Error("non-websocket DERP request received")
				return
			}

			derpServer.Load().ServeHTTP(w, r)
		})
		r.Get("/latency-check", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		r.Post("/restart", func(w http.ResponseWriter, r *http.Request) {
			oldServer := derpServer.Swap(newDerpServer(t, logger))
			oldServer.Close()
			w.WriteHeader(http.StatusOK)
		})
	})

	// /restart?derp=[true|false]&coordinator=[true|false]
	r.Post("/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("derp") == "true" {
			logger.Info(r.Context(), "killing DERP server")
			oldServer := derpServer.Swap(newDerpServer(t, logger))
			oldServer.Close()
			logger.Info(r.Context(), "restarted DERP server")
		}

		if r.URL.Query().Get("coordinator") == "true" {
			logger.Info(r.Context(), "simulating coordinator restart")
			cm.CloseAll()
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/api/v2/workspaceagents/{id}/coordinate", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParamFromCtx(ctx, "id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			logger.Warn(ctx, "bad agent ID passed in URL params", slog.F("id_str", idStr), slog.Error(err))
			httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
				Message: "Bad agent id.",
				Detail:  err.Error(),
			})
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			logger.Warn(ctx, "failed to accept websocket", slog.Error(err))
			httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to accept websocket.",
				Detail:  err.Error(),
			})
			return
		}

		ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
		defer wsNetConn.Close()

		cleanFn := cm.Add(id, wsNetConn)
		defer cleanFn()

		err = csvc.ServeConnV2(ctx, wsNetConn, tailnet.StreamID{
			Name: "client-" + id.String(),
			ID:   id,
			Auth: tailnet.SingleTailnetCoordinateeAuth{},
		})
		if err != nil && !xerrors.Is(err, io.EOF) && !xerrors.Is(err, context.Canceled) {
			logger.Warn(ctx, "failed to serve conn", slog.Error(err))
			_ = conn.Close(websocket.StatusInternalError, err.Error())
			return
		}
	})

	return r
}

func (o SimpleServerOptions) StartServer(t *testing.T, logger slog.Logger, listenAddr string) {
	srv := http.Server{
		Addr:        listenAddr,
		Handler:     o.Router(t, logger),
		ReadTimeout: 10 * time.Second,
	}
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		err := srv.ListenAndServe()
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			t.Error("HTTP server error:", err)
		}
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		<-serveDone
	})
}

type NGINXServerOptions struct {
	SimpleServerOptions
}

var _ ServerStarter = NGINXServerOptions{}

func (o NGINXServerOptions) StartServer(t *testing.T, logger slog.Logger, listenAddr string) {
	host, nginxPortStr, err := net.SplitHostPort(listenAddr)
	require.NoError(t, err)

	nginxPort, err := strconv.Atoi(nginxPortStr)
	require.NoError(t, err)

	serverPort := nginxPort + 1
	serverListenAddr := net.JoinHostPort(host, strconv.Itoa(serverPort))

	o.SimpleServerOptions.StartServer(t, logger, serverListenAddr)
	startNginx(t, nginxPortStr, serverListenAddr)
}

func startNginx(t *testing.T, listenPort, serverAddr string) {
	cfg := `events {}
http {
	server {
		listen ` + listenPort + `;
		server_name _;
		location / {
			proxy_pass http://` + serverAddr + `;
			proxy_http_version 1.1;
			proxy_set_header Upgrade $http_upgrade;
			proxy_set_header Connection "upgrade";
			proxy_set_header Host $host;
			proxy_set_header X-Real-IP $remote_addr;
			proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
			proxy_set_header X-Forwarded-Proto $scheme;
			proxy_set_header X-Forwarded-Host $server_name;
		}
	}
}
`

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nginx.conf")
	err := os.WriteFile(cfgPath, []byte(cfg), 0o600)
	require.NoError(t, err)

	// ExecBackground will handle cleanup.
	_, _ = ExecBackground(t, "server.nginx", nil, "nginx", []string{"-c", cfgPath})
}

type BasicClientStarter struct {
	BlockEndpoints      bool
	DERPForceWebsockets bool
	// WaitForConnection means wait for (any) peer connection before returning from StartClient
	WaitForConnection bool
	// WaitForConnection means wait for a direct peer connection before returning from StartClient
	WaitForDirect bool
	// Service is a network service (e.g. an echo server) to start on the client. If Wait* is set, the service is
	// started prior to waiting.
	Service    NetworkService
	LogPackets bool
}

type NetworkService interface {
	StartService(t *testing.T, logger slog.Logger, conn *tailnet.Conn)
}

func (b BasicClientStarter) StartClient(t *testing.T, logger slog.Logger, serverURL *url.URL, derpMap *tailcfg.DERPMap, me, peer Client) *tailnet.Conn {
	var hook capture.Callback
	if b.LogPackets {
		pktLogger := packetLogger{logger}
		hook = pktLogger.LogPacket
	}
	conn := startClientOptions(t, logger, serverURL, me, peer, &tailnet.Options{
		Addresses:           []netip.Prefix{tailnet.TailscaleServicePrefix.PrefixFromUUID(me.ID)},
		DERPMap:             derpMap,
		BlockEndpoints:      b.BlockEndpoints,
		Logger:              logger,
		DERPForceWebSockets: b.DERPForceWebsockets,
		ListenPort:          me.ListenPort,
		// These tests don't have internet connection, so we need to force
		// magicsock to do anything.
		ForceNetworkUp: true,
		CaptureHook:    hook,
	})

	if b.Service != nil {
		b.Service.StartService(t, logger, conn)
	}

	if b.WaitForConnection || b.WaitForDirect {
		// Wait for connection to be established.
		peerIP := tailnet.TailscaleServicePrefix.AddrFromUUID(peer.ID)
		require.Eventually(t, func() bool {
			t.Log("attempting ping to peer to judge direct connection")
			ctx := testutil.Context(t, testutil.WaitShort)
			_, p2p, pong, err := conn.Ping(ctx, peerIP)
			if err != nil {
				t.Logf("ping failed: %v", err)
				return false
			}
			if !p2p && b.WaitForDirect {
				t.Log("ping succeeded, but not direct yet")
				return false
			}
			t.Logf("ping succeeded, p2p=%t, endpoint=%s", p2p, pong.Endpoint)
			return true
		}, testutil.WaitLong, testutil.IntervalMedium)
	}

	return conn
}

const EchoPort = 2381

type UDPEchoService struct{}

func (UDPEchoService) StartService(t *testing.T, logger slog.Logger, _ *tailnet.Conn) {
	// tailnet doesn't handle UDP connections "in-process" the way we do for TCP, so we need to listen in the OS,
	// and tailnet will forward packets.
	l, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv6zero, // all interfaces
		Port: EchoPort,
	})
	require.NoError(t, err)

	// set path MTU discovery so that we don't fragment the responses.
	c, err := l.SyscallConn()
	require.NoError(t, err)
	var sockErr error
	err = c.Control(func(fd uintptr) {
		sockErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_MTU_DISCOVER, unix.IP_PMTUDISC_DO)
	})
	require.NoError(t, err)
	require.NoError(t, sockErr)
	logger.Info(context.Background(), "started UDPEcho server")
	t.Cleanup(func() {
		lCloseErr := l.Close()
		if lCloseErr != nil {
			t.Logf("error closing UDPEcho listener: %v", lCloseErr)
		}
	})
	go func() {
		buf := make([]byte, 1500)
		for {
			n, remote, readErr := l.ReadFromUDP(buf)
			if readErr != nil {
				logger.Info(context.Background(), "error reading UDPEcho listener", slog.Error(readErr))
				return
			}
			logger.Info(context.Background(), "received UDPEcho packet",
				slog.F("len", n), slog.F("remote", remote))
			n, writeErr := l.WriteToUDP(buf[:n], remote)
			if writeErr != nil {
				logger.Info(context.Background(), "error writing UDPEcho listener", slog.Error(writeErr))
				return
			}
			logger.Info(context.Background(), "wrote UDPEcho packet",
				slog.F("len", n), slog.F("remote", remote))
		}
	}()
}

func startClientOptions(t *testing.T, logger slog.Logger, serverURL *url.URL, me, peer Client, options *tailnet.Options) *tailnet.Conn {
	u, err := serverURL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", me.ID.String()))
	require.NoError(t, err)
	//nolint:bodyclose
	ws, _, err := websocket.Dial(context.Background(), u.String(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ws.Close(websocket.StatusNormalClosure, "closing websocket")
	})

	client, err := tailnet.NewDRPCClient(
		websocket.NetConn(context.Background(), ws, websocket.MessageBinary),
		logger,
	)
	require.NoError(t, err)

	coord, err := client.Coordinate(context.Background())
	require.NoError(t, err)

	conn, err := tailnet.NewConn(options)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	var coordination tailnet.CloserWaiter
	if me.TunnelSrc {
		ctrl := tailnet.NewTunnelSrcCoordController(logger, conn)
		ctrl.AddDestination(peer.ID)
		coordination = ctrl.New(coord)
	} else {
		// use the "Agent" controller so that we act as a tunnel destination and send "ReadyForHandshake" acks.
		ctrl := tailnet.NewAgentCoordinationController(logger, conn)
		coordination = ctrl.New(coord)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		_ = coordination.Close(cctx)
	})

	return conn
}

func basicDERPMap(serverURLStr string) (*tailcfg.DERPMap, error) {
	serverURL, err := url.Parse(serverURLStr)
	if err != nil {
		return nil, xerrors.Errorf("parse server URL %q: %w", serverURLStr, err)
	}

	portStr := serverURL.Port()
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, xerrors.Errorf("parse port %q: %w", portStr, err)
	}

	hostname := serverURL.Hostname()
	ipv4 := "none"
	ipv6 := "none"
	ip, err := netip.ParseAddr(hostname)
	if err == nil {
		hostname = ""
		if ip.Is4() {
			ipv4 = ip.String()
		}
		if ip.Is6() {
			ipv6 = ip.String()
		}
	}

	return &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionCode: "test",
				RegionName: "test server",
				Nodes: []*tailcfg.DERPNode{
					{
						Name:             "test0",
						RegionID:         1,
						HostName:         hostname,
						IPv4:             ipv4,
						IPv6:             ipv6,
						DERPPort:         port,
						STUNPort:         -1,
						ForceHTTP:        true,
						InsecureForTests: true,
					},
				},
			},
		},
	}, nil
}

// ExecBackground starts a subprocess with the given flags and returns a
// channel that will receive the error when the subprocess exits. The returned
// function can be used to close the subprocess.
//
// processName is used to identify the subprocess in logs.
//
// Optionally, a network namespace can be passed to run the subprocess in.
//
// Do not call close then wait on the channel. Use the returned value from the
// function instead in this case.
//
// Cleanup is handled automatically if you don't care about monitoring the
// process manually.
func ExecBackground(t *testing.T, processName string, netNS *os.File, name string, args []string) (<-chan error, func() error) {
	if netNS != nil {
		// We use nsenter to enter the namespace.
		// We can't use `setns` easily from Golang in the parent process because
		// you can't execute the syscall in the forked child thread before it
		// execs.
		// We can't use `setns` easily from Golang in the child process because
		// by the time you call it, the process has already created multiple
		// threads.
		args = append([]string{"--net=/proc/self/fd/3", name}, args...)
		name = "nsenter"
	}

	cmd := exec.Command(name, args...)
	if netNS != nil {
		cmd.ExtraFiles = []*os.File{netNS}
	}

	out := &testWriter{
		name: processName,
		t:    t,
	}
	t.Cleanup(out.Flush)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}
	err := cmd.Start()
	require.NoError(t, err)

	waitErr := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil && strings.Contains(err.Error(), "signal: terminated") {
			err = nil
		}
		waitErr <- err
		close(waitErr)
	}()

	closeFn := func() error {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		case err := <-waitErr:
			return err
		}
		return <-waitErr
	}

	t.Cleanup(func() {
		select {
		case err := <-waitErr:
			if err != nil {
				t.Log("subprocess exited: " + err.Error())
			}
			return
		default:
		}

		_ = closeFn()
	})

	return waitErr, closeFn
}

type testWriter struct {
	mut  sync.Mutex
	name string
	t    *testing.T

	capturedLines []string
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.mut.Lock()
	defer w.mut.Unlock()
	str := string(p)
	split := strings.Split(str, "\n")
	for _, s := range split {
		if s == "" {
			continue
		}

		// If a line begins with "\s*--- (PASS|FAIL)" or is just PASS or FAIL,
		// then it's a test result line. We want to capture it and log it later.
		trimmed := strings.TrimSpace(s)
		if strings.HasPrefix(trimmed, "--- PASS") || strings.HasPrefix(trimmed, "--- FAIL") || trimmed == "PASS" || trimmed == "FAIL" {
			// Also fail the test if we see a FAIL line.
			if strings.Contains(trimmed, "FAIL") {
				w.t.Errorf("subprocess logged test failure: %s: \t%s", w.name, s)
			}

			w.capturedLines = append(w.capturedLines, s)
			continue
		}

		w.t.Logf("%s output: \t%s", w.name, s)
	}
	return len(p), nil
}

func (w *testWriter) Flush() {
	w.mut.Lock()
	defer w.mut.Unlock()
	for _, s := range w.capturedLines {
		w.t.Logf("%s output: \t%s", w.name, s)
	}
	w.capturedLines = nil
}

type packetLogger struct {
	l slog.Logger
}

func (p packetLogger) LogPacket(path capture.Path, when time.Time, pkt []byte, _ packet.CaptureMeta) {
	q := new(packet.Parsed)
	q.Decode(pkt)
	p.l.Info(context.Background(), "Packet",
		slog.F("path", pathString(path)),
		slog.F("when", when),
		slog.F("decode", q.String()),
		slog.F("len", len(pkt)),
	)
}

func pathString(path capture.Path) string {
	switch path {
	case capture.FromLocal:
		return "Local"
	case capture.FromPeer:
		return "Peer"
	case capture.SynthesizedToLocal:
		return "SynthesizedToLocal"
	case capture.SynthesizedToPeer:
		return "SynthesizedToPeer"
	case capture.PathDisco:
		return "Disco"
	default:
		return "<<UNKNOWN>>"
	}
}
