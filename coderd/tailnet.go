package coderd

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tailscale/wireguard-go/device"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/site"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

var tailnetTransport *http.Transport

func init() {
	tp, valid := http.DefaultTransport.(*http.Transport)
	if !valid {
		panic("dev error: default transport is the wrong type")
	}
	tailnetTransport = tp.Clone()
	// We do not want to respect the proxy settings from the environment, since
	// all network traffic happens over wireguard.
	tailnetTransport.Proxy = nil
}

var _ workspaceapps.AgentProvider = (*ServerTailnet)(nil)

// NewServerTailnet creates a new tailnet intended for use by coderd.
func NewServerTailnet(
	ctx context.Context,
	logger slog.Logger,
	derpServer *derp.Server,
	dialer tailnet.ControlProtocolDialer,
	derpForceWebSockets bool,
	blockEndpoints bool,
	traceProvider trace.TracerProvider,
) (*ServerTailnet, error) {
	logger = logger.Named("servertailnet")
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
		DERPForceWebSockets: derpForceWebSockets,
		Logger:              logger,
		BlockEndpoints:      blockEndpoints,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet conn: %w", err)
	}
	serverCtx, cancel := context.WithCancel(ctx)

	derpConnects := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coder",
		Subsystem: "servertailnet",
		Name:      "derp_connects_total",
		Help:      "Number of times the server tailnet connected to the embedded DERP relay.",
	})

	// This is set to allow local DERP traffic to be proxied through memory
	// instead of needing to hit the external access URL. Don't use the ctx
	// given in this callback, it's only valid while connecting.
	if derpServer != nil {
		conn.SetDERPRegionDialer(func(_ context.Context, region *tailcfg.DERPRegion) net.Conn {
			// Don't set up the embedded relay if we're shutting down
			if !region.EmbeddedRelay || ctx.Err() != nil {
				return nil
			}
			derpConnects.Inc()
			logger.Debug(ctx, "connecting to embedded DERP via in-memory pipe")
			left, right := net.Pipe()
			go func() {
				defer left.Close()
				defer right.Close()
				brw := bufio.NewReadWriter(bufio.NewReader(right), bufio.NewWriter(right))
				derpServer.Accept(ctx, right, brw, "internal")
			}()
			return left
		})
	}

	tracer := traceProvider.Tracer(tracing.TracerName)

	controller := tailnet.NewController(logger, dialer)
	// it's important to set the DERPRegionDialer above _before_ we set the DERP map so that if
	// there is an embedded relay, we use the local in-memory dialer.
	controller.DERPCtrl = tailnet.NewBasicDERPController(logger, nil, conn)
	coordCtrl := NewMultiAgentController(serverCtx, logger, tracer, conn)
	controller.CoordCtrl = coordCtrl
	// TODO: support controller.TelemetryCtrl

	tn := &ServerTailnet{
		ctx:                 serverCtx,
		cancel:              cancel,
		logger:              logger,
		tracer:              tracer,
		conn:                conn,
		coordinatee:         conn,
		controller:          controller,
		coordCtrl:           coordCtrl,
		transport:           tailnetTransport.Clone(),
		clock:               quartz.NewReal(),
		readPeerDiagnostics: conn.GetPeerDiagnostics,
		peerDiagnosticsSlot: make(chan struct{}, 1),
		connsPerAgent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "servertailnet",
			Name:      "open_connections",
			Help:      "Total number of TCP connections currently open to workspace agents.",
		}, []string{"network"}),
		totalConns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "servertailnet",
			Name:      "connections_total",
			Help:      "Total number of TCP connections made to workspace agents.",
		}, []string{"network"}),
		derpConnects: derpConnects,
		agentUnreachable: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "servertailnet",
			Name:      "agent_unreachable_total",
			Help:      "Number of connection attempts where the workspace agent did not answer in time, by reason: no_node, peer_lost, no_handshake, handshake_stale, handshake_ok, diagnostics_timeout, or client_canceled.",
		}, []string{"reason"}),
		awaitReachable: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coder",
			Subsystem: "servertailnet",
			Name:      "await_reachable_seconds",
			Help:      "Time until a workspace agent answered the first ping when a new connection was opened.",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		}),
	}
	tn.transport.DialContext = tn.dialContext
	// These options are mostly just picked at random, and they can likely be
	// fine-tuned further. Generally, users are running applications in dev mode
	// which can generate hundreds of requests per page load, so we increased
	// MaxIdleConnsPerHost from 2 to 6 and removed the limit of total idle
	// conns.
	tn.transport.MaxIdleConnsPerHost = 6
	tn.transport.MaxIdleConns = 0
	tn.transport.IdleConnTimeout = 10 * time.Minute
	// We intentionally don't verify the certificate chain here.
	// The connection to the workspace is already established and most
	// apps are already going to be accessed over plain HTTP, this config
	// simply allows apps being run over HTTPS to be accessed without error --
	// many of which may be using self-signed certs.
	tn.transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		//nolint:gosec
		InsecureSkipVerify: true,
	}

	tn.controller.Run(tn.ctx)
	return tn, nil
}

// Conn is used to access the underlying tailnet conn of the ServerTailnet. It
// should only be used for read-only purposes.
func (s *ServerTailnet) Conn() *tailnet.Conn {
	return s.conn
}

func (s *ServerTailnet) Describe(descs chan<- *prometheus.Desc) {
	s.connsPerAgent.Describe(descs)
	s.totalConns.Describe(descs)
	s.derpConnects.Describe(descs)
	s.agentUnreachable.Describe(descs)
	s.awaitReachable.Describe(descs)
}

func (s *ServerTailnet) Collect(metrics chan<- prometheus.Metric) {
	s.connsPerAgent.Collect(metrics)
	s.totalConns.Collect(metrics)
	s.derpConnects.Collect(metrics)
	s.agentUnreachable.Collect(metrics)
	s.awaitReachable.Collect(metrics)
}

type ServerTailnet struct {
	ctx    context.Context
	cancel func()

	logger slog.Logger
	tracer trace.Tracer

	// in prod, these are the same, but coordinatee is a subset of Conn's
	// methods which makes some tests easier.
	conn        *tailnet.Conn
	coordinatee tailnet.Coordinatee

	controller *tailnet.Controller
	coordCtrl  *MultiAgentController

	transport *http.Transport
	clock     quartz.Clock

	connsPerAgent    *prometheus.GaugeVec
	totalConns       *prometheus.CounterVec
	derpConnects     prometheus.Counter
	agentUnreachable *prometheus.CounterVec
	awaitReachable   prometheus.Histogram

	// peerDiagnosticsSlot holds one token while a peer diagnostics read is
	// in flight.
	peerDiagnosticsSlot chan struct{}
	readPeerDiagnostics func(agentID uuid.UUID) tailnet.PeerDiagnostics
}

func (s *ServerTailnet) ReverseProxy(targetURL, dashboardURL *url.URL, agentID uuid.UUID, app appurl.ApplicationURL, wildcardHostname string) *httputil.ReverseProxy {
	// Rewrite the targetURL's Host to point to the agent's IP. This is
	// necessary because due to TCP connection caching, each agent needs to be
	// addressed invidivually. Otherwise, all connections get dialed as
	// "localhost:port", causing connections to be shared across agents.
	tgt := *targetURL
	_, port, _ := net.SplitHostPort(tgt.Host)
	tgt.Host = net.JoinHostPort(tailnet.TailscaleServicePrefix.AddrFromUUID(agentID).String(), port)

	proxy := httputil.NewSingleHostReverseProxy(&tgt)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, theErr error) {
		theErr = s.reportProxyDialFailure(r.Context(), agentID, theErr)
		var (
			desc           = "Failed to proxy request to application: " + theErr.Error()
			additionalInfo = ""
			actions        = []site.Action{}
		)

		var tlsError tls.RecordHeaderError
		if (errors.As(theErr, &tlsError) && tlsError.Msg == "first record does not look like a TLS handshake") ||
			errors.Is(theErr, http.ErrSchemeMismatch) {
			// If the error is due to an HTTP/HTTPS mismatch, we can provide a
			// more helpful error message with redirect buttons.
			switchURL := url.URL{
				Scheme: dashboardURL.Scheme,
			}
			_, protocol, isPort := app.PortInfo()
			if isPort {
				targetProtocol := "https"
				if protocol == "https" {
					targetProtocol = "http"
				}
				app = app.ChangePortProtocol(targetProtocol)

				switchURL.Host = fmt.Sprintf("%s%s", app.String(), strings.TrimPrefix(wildcardHostname, "*"))
				actions = append(actions, site.Action{
					URL:  switchURL.String(),
					Text: fmt.Sprintf("Switch to %s", strings.ToUpper(targetProtocol)),
				})
				additionalInfo += fmt.Sprintf("This error seems to be due to an app protocol mismatch, try switching to %s.", strings.ToUpper(targetProtocol))
			}
		}

		site.RenderStaticErrorPage(w, r, site.ErrorPageData{
			Status:      http.StatusBadGateway,
			Title:       "Bad Gateway",
			Description: desc,
			Actions: append(actions, []site.Action{
				{
					Text: "Retry",
				},
				{
					URL:  dashboardURL.String(),
					Text: "Back to site",
				},
			}...),
			AdditionalInfo: additionalInfo,
		})
	}
	proxy.Director = s.director(agentID, proxy.Director)
	proxy.Transport = s.transport

	return proxy
}

type agentIDKey struct{}

// director makes sure agentIDKey and a dialState are set on the context in the
// reverse proxy. The transport uses the agent ID to pick which agent to dial,
// and the dial and the error handler share the dialState.
func (*ServerTailnet) director(agentID uuid.UUID, prev func(req *http.Request)) func(req *http.Request) {
	return func(req *http.Request) {
		ds := &dialState{}
		ctx := context.WithValue(req.Context(), agentIDKey{}, agentID)
		ctx = context.WithValue(ctx, dialStateKey{}, ds)
		// A pooled connection skips dialContext, so the dial phases cannot show
		// that the request has a connection. GotConn fires either way.
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			GotConn: func(httptrace.GotConnInfo) { ds.markGotConn() },
		})
		*req = *req.WithContext(ctx)
		prev(req)
	}
}

// proxyDialTimeout caps a reverse proxy dial. The HTTP transport keeps dialing
// after the request ends, so without a cap an unreachable agent is pinged
// until the ServerTailnet closes.
const proxyDialTimeout = time.Minute

// dialPhase is the step of a reverse proxy dial that was in progress when the
// request ended.
type dialPhase int

const (
	dialPhaseNone dialPhase = iota
	dialPhaseAwaitReachable
	dialPhaseTCPDial
	dialPhaseDone
)

func (p dialPhase) String() string {
	switch p {
	case dialPhaseAwaitReachable:
		return "await_reachable"
	case dialPhaseTCPDial:
		return "tcp_dial"
	case dialPhaseDone:
		return "done"
	default:
		return "none"
	}
}

// dialState is shared by the request goroutine and the transport's dial
// goroutine through the request context. The error handler reads it to tell
// a dial that never finished from a failure on an established connection.
type dialState struct {
	mu      sync.Mutex
	phase   dialPhase
	since   time.Time
	gotConn bool
}

type dialStateKey struct{}

func dialStateFromContext(ctx context.Context) *dialState {
	ds, _ := ctx.Value(dialStateKey{}).(*dialState)
	return ds
}

func (d *dialState) set(phase dialPhase) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.phase = phase
	d.since = time.Now()
}

// get returns the current phase and how long it has been in progress.
func (d *dialState) get() (dialPhase, time.Duration) {
	if d == nil {
		return dialPhaseNone, 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.phase, time.Since(d.since)
}

// startDial marks the start of a dial. A dial that starts after GotConn is the
// transport retrying on a new connection, so the earlier connection no longer
// explains a failure.
func (d *dialState) startDial() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.phase = dialPhaseAwaitReachable
	d.since = time.Now()
	d.gotConn = false
}

func (d *dialState) markGotConn() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.gotConn = true
}

// hadConn reports whether the transport gave the request a connection.
func (d *dialState) hadConn() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gotConn
}

func (s *ServerTailnet) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	agentID, ok := ctx.Value(agentIDKey{}).(uuid.UUID)
	if !ok {
		return nil, xerrors.Errorf("no agent id attached")
	}
	// Only the connect steps below use the context. The returned conn is not
	// tied to it, so canceling after the dial is safe.
	ctx, cancel := context.WithTimeout(ctx, proxyDialTimeout)
	defer cancel()

	// The reverse proxy's error handler records unreachable agents on the
	// request goroutine when the request ends, which can be before this dial
	// returns, so this path uses acquireAgent, which neither logs nor counts.
	ds := dialStateFromContext(ctx)
	ds.startDial()
	conn, release, err := s.acquireAgent(ctx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("acquire agent conn: %w", err)
	}

	ds.set(dialPhaseTCPDial)
	nc, err := conn.DialContext(ctx, network, addr)
	if err != nil {
		release()
		return nil, xerrors.Errorf("dial context: %w", err)
	}
	ds.set(dialPhaseDone)

	s.connsPerAgent.WithLabelValues("tcp").Inc()
	s.totalConns.WithLabelValues("tcp").Inc()
	return &instrumentedConn{
		Conn:          &netConnCloser{Conn: nc, close: release},
		agentID:       agentID,
		connsPerAgent: s.connsPerAgent,
	}, nil
}

// reportProxyDialFailure records a reverse proxy request that ended while its
// dial was still in progress. It runs on the request goroutine before the 502
// is written, which is the last point where the request log line can still
// take fields: the transport detaches the dial from the request deadline, so
// the dial goroutine learns of the failure later, or not at all. Errors on an
// established connection are returned unchanged.
func (s *ServerTailnet) reportProxyDialFailure(ctx context.Context, agentID uuid.UUID, err error) error {
	ds := dialStateFromContext(ctx)
	// The transport may give the request a pooled connection while the dial it
	// started is still waiting on the agent. An error after that is from the
	// connection the request used, so the dial phase does not describe it.
	if ds.hadConn() {
		return err
	}
	phase, elapsed := ds.get()
	switch phase {
	case dialPhaseAwaitReachable:
		// A request that is still waiting can also see an error from before
		// the first ping, such as a failed coordinator send. AgentConn does
		// not count those, so neither does this path.
		if !errors.Is(err, errAgentUnreachable) && ctx.Err() == nil {
			return err
		}
		unreachable := s.recordAgentUnreachable(ctx, agentID, elapsed, slog.F("dial_phase", phase.String()))
		if errors.Is(err, errAgentUnreachable) {
			return err
		}
		return xerrors.Errorf("%s: %w", unreachable.Error(), err)
	case dialPhaseTCPDial:
		// The agent answered a ping, so the tunnel is up and the failure is
		// above it: the netstack dial or an app that is not accepting. Name
		// the phase on the log line, but do not count it as unreachable.
		if rl := loggermw.RequestLoggerFromContext(ctx); rl != nil {
			rl.WithFields(
				slog.F("agent_id", agentID),
				slog.F("dial_phase", phase.String()),
				slog.F("tcp_dial_after", elapsed),
			)
		}
		return err
	default:
		return err
	}
}

// errAgentUnreachable is returned by acquireAgent when the agent did not
// answer a ping before the context ended.
var errAgentUnreachable = xerrors.New("agent is unreachable")

func (s *ServerTailnet) AgentConn(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	start := time.Now()
	conn, release, err := s.acquireAgent(ctx, agentID)
	cause := context.Cause(ctx)
	if errors.Is(err, errAgentUnreachable) &&
		!errors.Is(cause, workspacesdk.ErrDialAbandoned) &&
		!errors.Is(cause, workspacesdk.ErrReadinessProbeTimeout) {
		return nil, nil, s.recordAgentUnreachable(ctx, agentID, time.Since(start))
	}
	return conn, release, err
}

// acquireAgent adds the agent to the coordinator and waits for it to answer a
// ping. On timeout it returns errAgentUnreachable and leaves logging and
// metrics to the caller, which knows whether a request is still waiting.
func (s *ServerTailnet) acquireAgent(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	var (
		conn workspacesdk.AgentConn
		ret  func()
	)

	s.logger.Debug(s.ctx, "acquiring agent", slog.F("agent_id", agentID))
	err := s.coordCtrl.ensureAgent(agentID)
	if err != nil {
		return nil, nil, xerrors.Errorf("ensure agent: %w", err)
	}
	ret = s.coordCtrl.acquireTicket(agentID)

	conn = workspacesdk.NewAgentConn(s.conn, workspacesdk.AgentConnOptions{
		AgentID:   agentID,
		CloseFunc: func() error { return workspacesdk.ErrSkipClose },
		Logger:    s.logger,
	})

	// Since we now have an open conn, be careful to close it if we error
	// without returning it to the user.

	start := time.Now()
	reachable := conn.AwaitReachable(ctx)
	if !reachable {
		ret()
		return nil, nil, errAgentUnreachable
	}
	s.awaitReachable.Observe(time.Since(start).Seconds())

	return conn, ret, nil
}

// peerDiagnosticsTimeout caps how long a failed connection waits for peer
// diagnostics, which need WireGuard engine locks that may be busy during an
// incident.
const peerDiagnosticsTimeout = time.Second

// clientCanceledWait is the longest wait that still counts as the client
// leaving when the context was canceled. A connected agent answers its first
// ping well within this time.
const clientCanceledWait = 5 * time.Second

// recordAgentUnreachable counts a connection attempt that the agent did not
// answer, adds the peer state to the request log line, and returns an error
// carrying the same fields. A client that left quickly gets a plain error so
// callers do not log it as an unreachable agent.
func (s *ServerTailnet) recordAgentUnreachable(ctx context.Context, agentID uuid.UUID, after time.Duration, extra ...slog.Field) error {
	fields := append([]slog.Field{
		slog.F("agent_id", agentID),
		slog.F("unreachable_after", after),
	}, extra...)
	reason := "diagnostics_timeout"

	// A quick cancellation is the client leaving, not the agent failing.
	// A long wait counts as unreachable however the context ended, since a
	// load balancer giving up on a hung request also cancels it.
	clientCanceled := errors.Is(ctx.Err(), context.Canceled) && after < clientCanceledWait
	if clientCanceled {
		reason = "client_canceled"
	} else if d, ok := s.peerDiagnostics(agentID); ok {
		fields = append(fields, unreachableFields(d)...)
		reason = unreachableReason(d, time.Now())
	}
	fields = append(fields, slog.F("reason", reason))
	s.agentUnreachable.WithLabelValues(reason).Inc()

	if rl := loggermw.RequestLoggerFromContext(ctx); rl != nil {
		rl.WithFields(fields...)
	} else if !clientCanceled {
		// Callers such as chatd have no request log line, so without this the
		// counter rises with no record of which agent failed.
		s.logger.Warn(ctx, "agent is unreachable", fields...)
	}
	if clientCanceled {
		return xerrors.New("agent is unreachable: client canceled")
	}
	return &workspaceapps.AgentUnreachableError{Fields: fields}
}

// peerDiagnostics reads the peer state within peerDiagnosticsTimeout. The
// read takes WireGuard engine locks that may be held during an incident, so
// one read runs at a time and other callers wait for the slot within the
// same timeout instead of each parking a goroutine on the lock.
func (s *ServerTailnet) peerDiagnostics(agentID uuid.UUID) (tailnet.PeerDiagnostics, bool) {
	timeout := s.clock.NewTimer(peerDiagnosticsTimeout, "peerDiagnostics")
	defer timeout.Stop()
	select {
	case s.peerDiagnosticsSlot <- struct{}{}:
	case <-timeout.C:
		return tailnet.PeerDiagnostics{}, false
	}
	diagCh := make(chan tailnet.PeerDiagnostics, 1)
	go func() {
		defer func() { <-s.peerDiagnosticsSlot }()
		diagCh <- s.readPeerDiagnostics(agentID)
	}()
	select {
	case d := <-diagCh:
		return d, true
	case <-timeout.C:
		return tailnet.PeerDiagnostics{}, false
	}
}

// unreachableFields describes the peer state for the request log line.
// server_preferred_derp is this server's home relay. The peer_* fields
// describe the agent.
func unreachableFields(d tailnet.PeerDiagnostics) []slog.Field {
	fields := []slog.Field{
		slog.F("peer_node_received", d.ReceivedNode != nil),
		slog.F("server_preferred_derp", d.PreferredDERP),
	}
	if !d.LastWireguardHandshake.IsZero() {
		fields = append(fields, slog.F("peer_last_handshake", d.LastWireguardHandshake))
	}
	if d.ReceivedNode != nil {
		// The node's DERP is "127.3.3.40:N" where N is the region ID.
		region := d.ReceivedNode.DERP
		if _, after, found := strings.Cut(region, ":"); found {
			region = after
		}
		fields = append(fields,
			slog.F("peer_lost", d.Lost),
			slog.F("peer_preferred_derp", region),
			slog.F("peer_tx_bytes", d.TxBytes),
			slog.F("peer_rx_bytes", d.RxBytes),
		)
	}
	return fields
}

// unreachableReason is the reason label for agent_unreachable_total. A lost
// peer comes before the handshake checks because the tunnel can look healthy
// while the control plane is down. A handshake older than WireGuard's session
// limit means our handshakes went unanswered while we waited.
func unreachableReason(d tailnet.PeerDiagnostics, now time.Time) string {
	switch {
	// The node was never received, or was removed after a disconnect or a
	// lost timeout.
	case d.ReceivedNode == nil:
		return "no_node"
	// The agent's coordinator connection dropped, or this server's connection
	// to the coordinator ended.
	case d.Lost:
		return "peer_lost"
	case d.LastWireguardHandshake.IsZero():
		return "no_handshake"
	case now.Sub(d.LastWireguardHandshake) > device.RejectAfterTime:
		return "handshake_stale"
	default:
		return "handshake_ok"
	}
}

func (s *ServerTailnet) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	s.conn.MagicsockServeHTTPDebug(w, r)
}

type netConnCloser struct {
	net.Conn
	close func()
}

func (c *netConnCloser) Close() error {
	c.close()
	return c.Conn.Close()
}

func (s *ServerTailnet) Close() error {
	s.logger.Info(s.ctx, "closing server tailnet")
	defer s.logger.Debug(s.ctx, "server tailnet close complete")
	s.cancel()
	_ = s.conn.Close()
	s.transport.CloseIdleConnections()
	s.coordCtrl.Close()
	<-s.controller.Closed()
	return nil
}

type instrumentedConn struct {
	net.Conn

	agentID       uuid.UUID
	closeOnce     sync.Once
	connsPerAgent *prometheus.GaugeVec
}

func (c *instrumentedConn) Close() error {
	c.closeOnce.Do(func() {
		c.connsPerAgent.WithLabelValues("tcp").Dec()
	})
	return c.Conn.Close()
}

// MultiAgentController is a tailnet.CoordinationController for connecting to multiple workspace
// agents.  It keeps track of connection times to the agents, and removes them on a timer if they
// have no active connections and haven't been used in a while.
type MultiAgentController struct {
	*tailnet.BasicCoordinationController

	logger slog.Logger
	tracer trace.Tracer

	mu sync.Mutex
	// connectionTimes is a map of agents the server wants to keep a connection to. It
	// contains the last time the agent was connected to.
	connectionTimes map[uuid.UUID]time.Time
	// tickets is a map of destinations to a set of connection tickets, representing open
	// connections to the destination
	tickets      map[uuid.UUID]map[uuid.UUID]struct{}
	coordination *tailnet.BasicCoordination

	cancel              context.CancelFunc
	expireOldAgentsDone chan struct{}
}

func (m *MultiAgentController) New(client tailnet.CoordinatorClient) tailnet.CloserWaiter {
	b := m.NewCoordination(client)
	// resync all destinations
	m.mu.Lock()
	defer m.mu.Unlock()
	m.coordination = b
	for agentID := range m.connectionTimes {
		err := b.SendRequest(&proto.CoordinateRequest{
			AddTunnel: &proto.CoordinateRequest_Tunnel{Id: agentID[:]},
		})
		if err != nil {
			m.logger.Error(context.Background(), "failed to re-add tunnel", slog.F("agent_id", agentID),
				slog.Error(err))
			b.SendErr(err)
			_ = client.Close()
			m.coordination = nil
			break
		}
	}
	return b
}

func (m *MultiAgentController) ensureAgent(agentID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.connectionTimes[agentID]
	// If we don't have the agent, subscribe.
	if !ok {
		m.logger.Debug(context.Background(),
			"subscribing to agent", slog.F("agent_id", agentID))
		if m.coordination != nil {
			err := m.coordination.SendRequest(&proto.CoordinateRequest{
				AddTunnel: &proto.CoordinateRequest_Tunnel{Id: agentID[:]},
			})
			if err != nil {
				err = xerrors.Errorf("subscribe agent: %w", err)
				m.coordination.SendErr(err)
				_ = m.coordination.CloseClient()
				m.coordination = nil
				return err
			}
		}
		m.tickets[agentID] = map[uuid.UUID]struct{}{}
	}
	m.connectionTimes[agentID] = time.Now()
	return nil
}

func (m *MultiAgentController) acquireTicket(agentID uuid.UUID) (release func()) {
	id := uuid.New()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets[agentID][id] = struct{}{}

	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.tickets[agentID], id)
	}
}

func (m *MultiAgentController) expireOldAgents(ctx context.Context) {
	defer close(m.expireOldAgentsDone)
	defer m.logger.Debug(context.Background(), "stopped expiring old agents")
	const (
		tick   = 5 * time.Minute
		cutoff = 30 * time.Minute
	)

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		m.doExpireOldAgents(ctx, cutoff)
	}
}

func (m *MultiAgentController) doExpireOldAgents(ctx context.Context, cutoff time.Duration) {
	// TODO: add some attrs to this.
	ctx, span := m.tracer.Start(ctx, tracing.FuncName())
	defer span.End()

	start := time.Now()
	deletedCount := 0

	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger.Debug(ctx, "pruning inactive agents", slog.F("agent_count", len(m.connectionTimes)))
	for agentID, lastConnection := range m.connectionTimes {
		// If no one has connected since the cutoff and there are no active
		// connections, remove the agent.
		if time.Since(lastConnection) > cutoff && len(m.tickets[agentID]) == 0 {
			if m.coordination != nil {
				err := m.coordination.SendRequest(&proto.CoordinateRequest{
					RemoveTunnel: &proto.CoordinateRequest_Tunnel{Id: agentID[:]},
				})
				if err != nil {
					m.logger.Debug(ctx, "unsubscribe expired agent", slog.Error(err), slog.F("agent_id", agentID))
					m.coordination.SendErr(xerrors.Errorf("unsubscribe expired agent: %w", err))
					// close the client because we do not want to do a graceful disconnect by
					// closing the coordination.
					_ = m.coordination.CloseClient()
					m.coordination = nil
					// Here we continue deleting any inactive agents: there is no point in
					// re-establishing tunnels to expired agents when we eventually reconnect.
				}
			}
			deletedCount++
			delete(m.connectionTimes, agentID)
		}
	}
	m.logger.Debug(ctx, "pruned inactive agents",
		slog.F("deleted", deletedCount),
		slog.F("took", time.Since(start)),
	)
}

func (m *MultiAgentController) Close() {
	m.cancel()
	<-m.expireOldAgentsDone
}

func NewMultiAgentController(ctx context.Context, logger slog.Logger, tracer trace.Tracer, coordinatee tailnet.Coordinatee) *MultiAgentController {
	m := &MultiAgentController{
		BasicCoordinationController: &tailnet.BasicCoordinationController{
			Logger:      logger,
			Coordinatee: coordinatee,
			SendAcks:    false, // we are a client, connecting to multiple agents
			Initiator:   codersdk.DisconnectInitiatorServer,
			Direction:   codersdk.ConnectionDirectionServerToAgent,
		},
		logger:              logger,
		tracer:              tracer,
		connectionTimes:     make(map[uuid.UUID]time.Time),
		tickets:             make(map[uuid.UUID]map[uuid.UUID]struct{}),
		expireOldAgentsDone: make(chan struct{}),
	}
	ctx, m.cancel = context.WithCancel(ctx)
	go m.expireOldAgents(ctx)
	return m
}

type Pinger interface {
	Ping(context.Context) (time.Duration, error)
}

// InmemTailnetDialer is a tailnet.ControlProtocolDialer that connects to a Coordinator and DERPMap
// service running in the same memory space.
type InmemTailnetDialer struct {
	CoordPtr *atomic.Pointer[tailnet.Coordinator]
	DERPFn   func() *tailcfg.DERPMap
	Logger   slog.Logger
	ClientID uuid.UUID
	// DatabaseHealthCheck is used to validate that the store is reachable.
	DatabaseHealthCheck Pinger
	// Clock is optional and defaults to the real clock.
	Clock quartz.Clock
}

func (a *InmemTailnetDialer) Dial(ctx context.Context, _ tailnet.ResumeTokenController) (tailnet.ControlProtocolClients, error) {
	if a.DatabaseHealthCheck != nil {
		if _, err := a.DatabaseHealthCheck.Ping(ctx); err != nil {
			return tailnet.ControlProtocolClients{}, xerrors.Errorf("%w: %v", codersdk.ErrDatabaseNotReachable, err)
		}
	}

	coord := a.CoordPtr.Load()
	if coord == nil {
		return tailnet.ControlProtocolClients{}, xerrors.Errorf("tailnet coordinator not initialized")
	}
	coordClient := tailnet.NewInMemoryCoordinatorClient(
		a.Logger, a.ClientID, tailnet.SingleTailnetCoordinateeAuth{}, *coord)
	clock := a.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	derpClient := newPollingDERPClient(a.DERPFn, a.Logger, clock)
	return tailnet.ControlProtocolClients{
		Closer:      closeAll{coord: coordClient, derp: derpClient},
		Coordinator: coordClient,
		DERP:        derpClient,
	}, nil
}

func newPollingDERPClient(derpFn func() *tailcfg.DERPMap, logger slog.Logger, clock quartz.Clock) tailnet.DERPClient {
	ctx, cancel := context.WithCancel(context.Background())
	a := &pollingDERPClient{
		fn:       derpFn,
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
		clock:    clock,
		ch:       make(chan *tailcfg.DERPMap),
		loopDone: make(chan struct{}),
	}
	go a.pollDERP()
	return a
}

// pollingDERPClient is a DERP client that just calls a function on a polling
// interval
type pollingDERPClient struct {
	fn          func() *tailcfg.DERPMap
	logger      slog.Logger
	clock       quartz.Clock
	ctx         context.Context
	cancel      context.CancelFunc
	loopDone    chan struct{}
	lastDERPMap *tailcfg.DERPMap
	ch          chan *tailcfg.DERPMap
}

// Close the DERP client
func (a *pollingDERPClient) Close() error {
	a.cancel()
	<-a.loopDone
	return nil
}

func (a *pollingDERPClient) Recv() (*tailcfg.DERPMap, error) {
	select {
	case <-a.ctx.Done():
		return nil, a.ctx.Err()
	case dm := <-a.ch:
		return dm, nil
	}
}

func (a *pollingDERPClient) pollDERP() {
	defer close(a.loopDone)
	defer a.logger.Debug(a.ctx, "polling DERPMap exited")

	// Fetch immediately so the tailnet can publish its node without waiting
	// for the first tick.
	ticker := a.clock.NewTicker(5*time.Second, "pollDERP")
	defer ticker.Stop()

	for {
		newDerpMap := a.fn()
		if !tailnet.CompareDERPMaps(a.lastDERPMap, newDerpMap) {
			select {
			case <-a.ctx.Done():
				return
			case a.ch <- newDerpMap:
			}
		}

		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type closeAll struct {
	coord tailnet.CoordinatorClient
	derp  tailnet.DERPClient
}

func (c closeAll) Close() error {
	cErr := c.coord.Close()
	dErr := c.derp.Close()
	if cErr != nil {
		return cErr
	}
	return dErr
}
