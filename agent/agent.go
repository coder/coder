// Package agent provides functionalities for securely connecting a
// workspace to the Coder server.
package agent

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	"github.com/pion/stun"
	sdp "github.com/pion/webrtc/v3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/procfs"
	"github.com/spf13/afero"
	"golang.org/x/exp/maps"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"storj.io/drpc/drpcconn"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agenthelper"
	"github.com/coder/coder/v2/agent/agentrsa"
	"github.com/coder/coder/v2/agent/agentscripts"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/proto/agentproto_drpc"
	"github.com/coder/coder/v2/agent/reconnectingpty"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/pty/ptyhost"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/retry"
	"github.com/coder/waggy"
)

// All errors associated with creating an agent are important enough
// to log. This is essentially DI associated with the process lifecycle.
//
//nolint:revive
const (
	maxMetrics           = 1_000
	runTaskPollingPeriod = 5 * time.Second
)

// Agent enables connecting to a workspace remotely and enables workspaces
// to phone home to Coder.
type Agent struct {
	id         uuid.UUID
	auth       *AgentAuth
	options    *Options
	closer     io.Closer
	lifecycle  *lifecycleHandler
	fs         afero.Fs
	logger     slog.Logger
	conn       *websocket.Conn
	appReadyCh chan struct{}
	// The ready channel is closed when the initial connection is established.
	readyCh chan struct{}
	// There's a potential race when closing an agent because Tailscale dials
	// after we've closed.
	clientLock  sync.RWMutex
	clientProto agentproto_drpc.DRPCAgentClient
	client      *drpcconn.Conn

	collectMetricsMu sync.Mutex
	// metricSink allows for testing collection of metrics.
	metricSink func([]*agentproto.Stats_Metric)
	metrics    *agentMetrics

	drpcTailnet drpcTailnet
	tailnet     struct {
		coordination *tailnet.Coordination
		closer       io.Closer
		closed       atomic.Bool
	}

	reconnectionQuota ReconnectionQuota
	connectionStatus  connectionStatusReporter

	serverInfo string

	// Manifest state
	manifestMu      sync.RWMutex
	manifest        Manifest
	manifestVersion string

	// Coder app (dynamic portforwarding) state.
	scripts       *agentscripts.Scripts
	containerized bool
	appConn       *websocket.Conn
	appConnCtx    context.Context
	appConnCancel context.CancelFunc
	appSessCh     chan struct{}

	// Health state.
	healthMu   sync.RWMutex
	healthLogs atomic.Pointer[waggy.LogBucket]
	// healthErr is the most recent error that occurred when trying to reach
	// the Coder server. It is nil if the most recent connection was successful.
	healthErr error

	startupScriptMu      sync.RWMutex
	startupScriptStatus  agentproto.StartupScriptStatus
	startupScriptLogs    []string
	startupScriptErr     error
	startupScriptDoneCh  chan struct{}
	startupScriptTimeout bool
	// The time when the agent received the last startup script timeout update (in unix format).
	startupScriptTimeoutAt time.Time

	statsCh    chan struct{}
	apiVersion uint64

	// portForwardingEnabled is set to 1 if port forwarding is enabled, else 0.
	portForwardingEnabled atomic.Int32

	dialer    func(context.Context, string) (net.Conn, error)
	stunProxy *stunProxy
}

// AgentAuth is used to authenticate the agent with the server.
type AgentAuth struct {
	ClientID    uuid.UUID
	AccessToken uuid.UUID
}

// ReconnectionQuota determines how many reconnections an agent may make.
type ReconnectionQuota interface {
	// Consume returns the duration to delay the next reconnection attempt,
	// or returns an error if the quota is exhausted.
	Consume() (time.Duration, error)
}

// net.Conn isn't exposed to prevent bypassing tracking code,
// but functionality must be identical.
type trackedConn interface {
	io.ReadWriteCloser

	// For WebSocket connections:
	// net.Conn's SetDeadline cannot be fulfilled (and is not needed).
	// This is fine since the DRPC layer will maintain it's own heartbeat mechanism.
	// LocalAddr() net.Addr
	// RemoteAddr() net.Addr
	// SetDeadline(t time.Time) error
	// SetReadDeadline(t time.Time) error
	// SetWriteDeadline(t time.Time) error
}

// drpcTailnet is used by Agent to dial a Tailnet node.
type drpcTailnet interface {
	// Dial creates a connection to a Tailnet node.
	Dial(ctx context.Context, node tailcfg.NodeID, derpMap *tailcfg.DERPMap) (net.Conn, error)
	// Accept accepts a connection from a Tailnet node.
	// it expects an HTTP request to come through the connection
	// with a URL of: /agent
	Accept(ctx context.Context, supportReconnect bool) (*websocket.Conn, error)
}

// Options provide configuration for an Agent.
type Options struct {
	ReconnectingPTYTimeout time.Duration
	ReconnectingPTYOpts    *reconnectingpty.Options
	Dir                    string
	// StartupTimeout is the time the agent will wait for the startup script to complete.
	// If the startup script is still running after this period of time, the agent will
	// timeout the script and mark the agent as ready. The script will continue to run
	// in the background.
	StartupTimeout time.Duration
	ScratchDir     string
	NoScratchDirs  bool
	// Auth is credentials used to authenticate with the workspace proxy.
	Auth *AgentAuth
	// Logger is used to log output from the agent.
	Logger slog.Logger
	// AGPL
	ManifestVersion string
	ReconnectingPTY *reconnectingpty.Manager
	Filesystem      afero.Fs

	// ExternalAuth allows for the application to provide dynamic
	// authentication mechanisms.
	ExternalAuth []ExternalAuth

	// Used for unit tests.
	connectionStatus connectionStatusReporter

	EnvironmentVariables map[string]string

	// StartupScriptBehavior controls how the agent should handle startup scripts.
	StartupScriptBehavior StartupScriptBehavior

	// PortForwardingEnabled controls if port forwarding is enabled.
	PortForwardingEnabled bool

	// IgnoreOwnershipCheck allows the agent to initialize even if the SSH host key
	// is already owned by another agent.
	IgnoreOwnershipCheck bool

	// SSHUsername is the username to use for the SSH server. Defaults to the
	// current user.
	SSHUsername string

	// Stats configuration for the agent stats collector.
	Stats                   StatsOptions
	WireguardOpenConnection func(conn *drpcconn.Conn) (io.ReadWriteCloser, error)
	STUN                    *STUNOptions
}

// ExternalAuth is a mechanism for allowing dynamic authenticaion
// with external services.
type ExternalAuth struct {
	ID    string
	Regex string
}

// STUNOptions configure the STUN server for NAT traversal.
type STUNOptions struct {
	// URL is the STUN server to use, e.g. "stun:stun.l.google.com:19302"
	URL string
	// Log is whether to log all STUN traffic
	Log bool
	// ListenTCP is the bind address for TCP STUN proxy, e.g. "127.0.0.1:3478"
	//
	// If not set, no TCP proxy will be started. This proxy allows clients to use
	// TCP instead of UDP to connect to STUN.
	ListenTCP string
}

type reconnectingPTYHandler struct {
	server reconnectingpty.Server
}

type StartupScriptBehavior string

const (
	// StartupScriptBehaviorBlockStartup will block the agent from starting up.
	// This means applications will be hidden until the script completes.
	StartupScriptBehaviorBlockStartup StartupScriptBehavior = "blocking"
	// StartupScriptBehaviorNonBlocking will not block the agent from starting up.
	// This means applications will be visible while the script runs.
	StartupScriptBehaviorNonBlocking StartupScriptBehavior = "non-blocking"
)

type StatsCollectionKey struct {
	ClientID uuid.UUID
	AgentID  uuid.UUID
}

type StatsOptions struct {
	CollectionInterval time.Duration
	// ReportInterval is how often metrics are sent to the server.
	ReportInterval time.Duration
	// InitialDelay is how long to wait before starting the stats reporter.
	InitialDelay time.Duration
	// StalledTimeout is how long to wait before marking the connection as stalled.
	StalledTimeout time.Duration
	// DisableCollection disables stats for an agent (specifically lifecycle metrics).
	DisableCollection bool
	// CollectionKey ensures the collector has a unique ID.
	// It is optional, and is used to prevent multiple agents from submitting stats
	// for the same agent ID.
	CollectionKey *StatsCollectionKey
}

// ConnectionStatus reports the status of the connection to the coder server.
type ConnectionStatus string

const (
	// ConnectionStatusConnected indicates that the connection to the coder server
	// is active.
	ConnectionStatusConnected ConnectionStatus = "connected"
	// ConnectionStatusReconnecting indicates that the connection to the coder server
	// was lost and the agent is attempting to reconnect.
	ConnectionStatusReconnecting ConnectionStatus = "reconnecting"
	// ConnectionStatusTimeout indicates that the server has not responded to
	// the client within the expected window of time.
	ConnectionStatusTimeout ConnectionStatus = "timeout"
)

// connectionStatusReporter is a function that reports the status of the
// connection to the coder server. It exists mostly to facilitate tests.
type connectionStatusReporter func(ConnectionStatus)

type defaultConnectionStatusReporter func(ConnectionStatus)

func (d defaultConnectionStatusReporter) reportStatus(status ConnectionStatus) {
	d(status)
}

func defaultConnStatusReporter(logger slog.Logger) defaultConnectionStatusReporter {
	lastStatus := ConnectionStatusConnecting

	return func(status ConnectionStatus) {
		if status == lastStatus {
			return
		}

		switch status {
		case ConnectionStatusConnected:
			logger.Info(context.Background(), "connected to coder server")
		case ConnectionStatusReconnecting:
			logger.Info(context.Background(), "connection lost; attempting to reconnect...")
		case ConnectionStatusTimeout:
			logger.Info(context.Background(), "connection timed out; attempting to reconnect...")
		}

		lastStatus = status
	}
}

// ConnectionStatusConnecting is a status used by the defaultConnectionStatusReporter
// to indicate the first status. It shouldn't be emitted by agent code; the reporter
// ignores it.
const ConnectionStatusConnecting ConnectionStatus = "connecting"

// CreateAgent constructs and yields a new Agent.
func CreateAgent(ctx context.Context, logger slog.Logger, fs afero.Fs, id uuid.UUID, serverURL *url.URL, options *Options) (*Agent, error) {
	lifecycle, err := newLifecycleHandler(ctx, logger)
	if err != nil {
		return nil, xerrors.Errorf("setup lifecycle handler: %w", err)
	}

	if options.Dir == "" {
		dir, err := os.MkdirTemp("", "agent")
		if err != nil {
			return nil, xerrors.Errorf("create agent directory: %w", err)
		}
		options.Dir = dir
	}

	// The "health" bucket will always have the most recent logs
	// emitted by the agent for quick introspection.
	healthLogs := waggy.NewLogBucket()
	healthLogger := logger.With(slog.F("subsystem", "health"))
	waggyLogger := waggy.Logger{
		LogFn: func(level waggy.LogLevel, msg string, fields ...any) {
			switch level {
			case waggy.LevelDebug:
				healthLogger.Debug(ctx, msg, fields...)
			case waggy.LevelInfo:
				healthLogger.Info(ctx, msg, fields...)
			case waggy.LevelWarn:
				healthLogger.Warn(ctx, msg, fields...)
			case waggy.LevelError:
				healthLogger.Error(ctx, msg, fields...)
			default:
				healthLogger.Warn(ctx, msg, fields...)
			}
		},
		Bucket: healthLogs,
	}

	var serverInfoString string
	if options.Auth != nil {
		serverInfoString = fmt.Sprintf("Agent: %s, Client: %s, Workspace: %s, Server: %s",
			id, options.Auth.ClientID, options.Auth.AccessToken, options.ManifestVersion)
	} else {
		serverInfoString = fmt.Sprintf("Agent (no connection): %s %s", id, options.ManifestVersion)
	}

	stun := options.STUN
	var stunProxy *stunProxy
	if stun != nil && stun.URL != "" && stun.ListenTCP != "" {
		stunProxyLogger := logger.With(slog.F("subsystem", "stun"))
		stunProxy = newStunProxy(stunProxyLogger, stun.URL, stun.ListenTCP, stun.Log)
		err := stunProxy.start()
		if err != nil {
			return nil, xerrors.Errorf("start STUN proxy: %w", err)
		}
	}

	a := &Agent{
		id:                     id,
		auth:                   options.Auth,
		options:                options,
		lifecycle:              lifecycle,
		fs:                     fs,
		logger:                 logger,
		appReadyCh:             make(chan struct{}),
		readyCh:                make(chan struct{}),
		reconnectionQuota:      NewCappedExponentialBackoffReconnectionQuota(5*time.Minute, 1024),
		serverInfo:             serverInfoString,
		manifest:               NewEmptyManifest(),
		manifestVersion:        options.ManifestVersion,
		dialer:                 websocket.DefaultDialer.NetDialContext,
		statsCh:                make(chan struct{}, 1),
		startupScriptDoneCh:    make(chan struct{}),
		startupScriptStatus:    agentproto.StartupScriptStatus_STARTUP_SCRIPT_STATUS_PENDING,
		apiVersion:             agentproto.Version,
		stunProxy:              stunProxy,
		closer:                 agenthelper.NewMultiCloser(),
		containerized:          os.Getenv("CODER_AGENT_CONTAINERIZED") == "true",
		portForwardingEnabled:  *atomic.NewInt32(0),
		scripts:                agentscripts.New(fs, logger),
		connectionStatus:       options.connectionStatus,
		startupScriptTimeoutAt: time.Unix(0, 0), // Default to 0 (1970-01-01 00:00:00) for testing
	}
	healthLogPtr := atomic.Pointer[waggy.LogBucket]{}
	healthLogPtr.Store(healthLogs)
	a.healthLogs = healthLogPtr

	if a.connectionStatus == nil {
		a.connectionStatus = defaultConnStatusReporter(logger)
	}

	if a.options.StartupScriptBehavior == "" {
		a.options.StartupScriptBehavior = StartupScriptBehaviorBlockStartup
	}

	if options.PortForwardingEnabled {
		a.portForwardingEnabled.Store(1)
	}

	appSessCh := make(chan struct{}, 1)
	a.appSessCh = appSessCh
	if err := a.initializeReconnectingPTY(ctx, &waggyLogger); err != nil {
		return nil, xerrors.Errorf("initialize reconnecting pty: %w", err)
	}

	return a, nil
}

// PortForwardingEnabled returns whether port forwarding is enabled.
func (a *Agent) PortForwardingEnabled() bool {
	return a.portForwardingEnabled.Load() == 1
}

// SetPortForwardingEnabled sets whether port forwarding is enabled.
func (a *Agent) SetPortForwardingEnabled(enabled bool) {
	if enabled {
		a.portForwardingEnabled.Store(1)
	} else {
		a.portForwardingEnabled.Store(0)
	}
}

// Run starts an agent and manages it's lifecycle. It blocks until
// the agent is gracefully closed or it experiences a fatal error.
// Callers are required to use `agent.Close()` to signal for agent
// termination.
func (a *Agent) Run(ctx context.Context) error {
	// When the agent closes, cancel our context to stop everything.
	// Another goroutine will call Close() to trigger termination.
	defer func() {
		// We already closed.
		if a.client == nil {
			return
		}
		a.healthMu.Lock()
		defer a.healthMu.Unlock()
		a.client = nil
	}()

	// We might not run if close was called before we get a client
	// connection, so manually check if the agent has been closed.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if a.auth == nil {
		return xerrors.New("cannot run agent with nil auth")
	}

	if err := a.initializeScratchDir(); err != nil {
		return xerrors.Errorf("initialize scratch directory: %w", err)
	}

	// Set up metricss for this agent. We always do this even if stats
	// collection is disabled, so that the stats endpoint still works.
	err := a.initMetrics()
	if err != nil {
		return xerrors.Errorf("initialize metrics: %w", err)
	}

	egCtx, egCancel := context.WithCancel(ctx)
	defer egCancel()
	eg, egCtx := errgroup.WithContext(egCtx)

	// If we have a ReconnectingPTY manager, start it in the background.
	if a.options.ReconnectingPTY != nil {
		reconnectingPTYLogger := a.logger.With(slog.F("subsystem", "reconnecting-pty"))
		eg.Go(func() error {
			reconnectingPTYLogger.Debug(egCtx, "starting reconnecting pty")
			<-egCtx.Done()
			return nil
		})
	}

	a.connectionStatus(ConnectionStatusConnected)

	// Wait for startup scripts in a goroutine, so it can continue running
	// in the background even if we timeout. The Ready() channel will be
	// unblocked upon completion.
	eg.Go(func() error {
		return a.waitForStartupScript(egCtx)
	})

	// If startup timeout is enabled, we'll start a goroutine to timeout the
	// startup script after the timeout period.
	if a.options.StartupTimeout > 0 && a.options.StartupScriptBehavior == StartupScriptBehaviorBlockStartup {
		eg.Go(func() error {
			select {
			case <-egCtx.Done():
				return nil
			case <-a.startupScriptDoneCh:
				return nil
			case <-time.After(a.options.StartupTimeout):
				a.logger.Debug(egCtx, "startup script timeout", slog.F("timeout", a.options.StartupTimeout))
				a.timeoutStartupScript()
				return nil
			}
		})
	}

	if !a.options.Stats.DisableCollection {
		// The stats reporter forwards stats from this agent to the server.
		// It needs to start after tailnet is set up.
		eg.Go(func() error {
			// Wait for tailnet to start
			select {
			case <-a.readyCh:
			case <-egCtx.Done():
				return xerrors.Errorf("shutdown before agent was ready: %w", egCtx.Err())
			}

			statsInterval := a.options.Stats.ReportInterval
			// If not configured, default to 1 minute
			if statsInterval == 0 {
				statsInterval = time.Minute
			}

			// Initial 0-2s delay after startup before sending the first stats.
			initialDelay := a.options.Stats.InitialDelay
			if initialDelay == 0 {
				delay, err := cryptorand.Intn(2000)
				if err != nil {
					delay = 1000
				}
				initialDelay = time.Duration(delay) * time.Millisecond
			}

			select {
			case <-time.After(initialDelay):
			case <-egCtx.Done():
				return nil
			}

			stalled := time.Duration(0) // Default to never marking as stalled. This also avoids a division by zero later.
			if a.options.Stats.StalledTimeout > 0 {
				stalled = a.options.Stats.StalledTimeout
			}

			var (
				ticker       = time.NewTicker(statsInterval)
				collectTimer *time.Timer
				lastReported time.Time
				stalledTimer *time.Timer
			)
			if stalled > 0 {
				stalledTimer = time.NewTimer(stalled)
				defer stalledTimer.Stop()
			}
			defer ticker.Stop()

			a.logger.Debug(egCtx, "starting stats reporter", slog.F("interval", statsInterval), slog.F("stalled_timeout", stalled))

			for {
				select {
				case <-egCtx.Done():
					return nil
				case <-ticker.C:
					// Fall through to send stats.
				case <-a.statsCh:
					// Cancel the collect timer if it exists
					if collectTimer != nil {
						collectTimer.Stop()
					}

					// Avoid a stampede of agents reporting at the same time by adding jitter.
					delay, err := cryptorand.Intn(200)
					if err != nil {
						delay = 100
					}
					collectTimer = time.NewTimer(time.Duration(delay) * time.Millisecond)

					select {
					case <-collectTimer.C:
						// Fall through to send stats.
					case <-egCtx.Done():
						collectTimer.Stop()
						return nil
					}
				case <-stalledTimer.C:
					elapsedReports := time.Since(lastReported) / statsInterval
					a.logger.Warn(egCtx, "stats reporting stalled",
						slog.F("missed_reports", elapsedReports),
						slog.F("last_reported", lastReported),
						slog.F("stalled_timeout", stalled),
					)
					a.connectionStatus(ConnectionStatusTimeout)
					// We'd expect the timer to be reset after the next successful report.
					if stalled > 0 {
						stalledTimer.Reset(stalled)
					}
					continue
				}

				a.clientLock.RLock()
				proto := a.clientProto
				a.clientLock.RUnlock()
				if proto == nil {
					// This happens when tailnet's peer is nil.
					continue
				}

				// Generate stats, and send them to the server.
				stats := a.GetStats(egCtx)
				// Copy the stats before we send them so we can modify them.
				sendStats := *stats
				if a.metricSink != nil {
					a.metricSink(sendStats.Metrics)
				}

				// Filter out all metrics if the client doesn't support them.
				if a.apiVersion <= 1 {
					sendStats.Metrics = nil
				}

				err := proto.UpdateStats(egCtx, &sendStats)
				if err != nil {
					// Don't log context cancellation errors, since they're expected.
					if egCtx.Err() == nil && !errors.Is(err, context.Canceled) {
						a.logger.Warn(egCtx, "failed to send stats to server", slog.Error(err))
					}
					continue
				}

				lastReported = time.Now()
				if stalled > 0 {
					stalledTimer.Reset(stalled)
					a.connectionStatus(ConnectionStatusConnected)
				}
			}
		})
	}

	eg.Go(func() error {
		go a.lifecycle.Start()
		defer a.lifecycle.Stop()

		return a.connectedLoop(egCtx)
	})

	// This is a background service, so we'll wait until the context is canceled.
	return eg.Wait()
}

// Startup scripts are a tool to allow users to run one-off commands when starting a workspace.
// The agent has access to the startup script and logs through the manifest. These are
// read-only. The workspace owner can execute startup scripts as needed, and the agent will
// simply report on the status of those scripts.
func (a *Agent) waitForStartupScript(ctx context.Context) error {
	// Wait until the manifest is sent by the agent
	a.logger.Debug(ctx, "waiting for startup script manifest")
	for a.options.StartupScriptBehavior == StartupScriptBehaviorBlockStartup {
		a.manifestMu.RLock()
		script := a.manifest.StartupScript
		a.manifestMu.RUnlock()

		// Manifest is empty, or startup script is either empty or already failed,
		// so we're ready.
		if script.Script == "" {
			a.finishStartupScript(ctx)
			break
		}

		logs, status, err := a.RunStartupScript(ctx, script.Script)
		a.startupScriptMu.Lock()
		a.startupScriptStatus = status
		a.startupScriptLogs = logs
		a.startupScriptErr = err
		a.startupScriptMu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.startupScriptDoneCh:
			return nil
		}
	}

	// If we're in non-blocking mode we just mark our script as ready now.
	if a.options.StartupScriptBehavior == StartupScriptBehaviorNonBlocking {
		a.finishStartupScript(ctx)
	}

	<-ctx.Done()
	return ctx.Err()
}

// timeoutStartupScript marks the startup script as timed out.
func (a *Agent) timeoutStartupScript() {
	a.startupScriptMu.Lock()
	defer a.startupScriptMu.Unlock()

	a.startupScriptStatus = agentproto.StartupScriptStatus_STARTUP_SCRIPT_STATUS_TIMEOUT
	a.startupScriptTimeout = true
	a.startupScriptTimeoutAt = time.Now()
	a.startupScriptLogs = append(a.startupScriptLogs, fmt.Sprintf("Startup script timed out after %.0f minutes. Script will continue execution in the background.", a.options.StartupTimeout.Minutes()))

	select {
	case <-a.startupScriptDoneCh:
		// already closed so the startup script has already completed
		return
	default:
		close(a.startupScriptDoneCh)
	}
}

// finishStartupScript marks the startup script as completed successfully
// and that workspace is ready for use.
func (a *Agent) finishStartupScript(ctx context.Context) {
	a.startupScriptMu.Lock()
	defer a.startupScriptMu.Unlock()

	if a.startupScriptStatus != agentproto.StartupScriptStatus_STARTUP_SCRIPT_STATUS_TIMEOUT &&
		a.startupScriptStatus != agentproto.StartupScriptStatus_STARTUP_SCRIPT_STATUS_ERROR {
		a.startupScriptStatus = agentproto.StartupScriptStatus_STARTUP_SCRIPT_STATUS_SUCCESS
	}

	// Ensure the ready channel is closed, so clients that are waiting on us will continue.
	select {
	case <-a.startupScriptDoneCh:
		// Already closed, so the startup script has already completed
		return
	default:
		a.logger.Debug(ctx, "marking startup script as done")
		close(a.startupScriptDoneCh)
	}
}

// Handler returns a http.Handler used to accept and process agent connections.
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		// Just being connected is "healthy"
		rw.WriteHeader(http.StatusOK)
	})

	// Anything goes to the agent api
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.HandleConn(w, r)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
	})
}

// registerConn adds a new inbound connection (typically either an API or application connection)
// to the agent. They're handled almost the same, with different behaviors for the Connect message.
func registerConn(ctx context.Context, ws *websocket.Conn) (*websocket.Conn, error) {
	pingTicker := time.NewTicker(time.Second * 15)
	defer pingTicker.Stop()
	closeCh := make(chan struct{})
	defer close(closeCh)
	go func() {
		for {
			select {
			case <-pingTicker.C:
				err := ws.Ping(ctx)
				if err != nil {
					_ = ws.Close(websocket.StatusGoingAway, "ping failed")
					return
				}
			case <-closeCh:
				return
			}
		}
	}()
	receiveCtx, closeFunc := context.WithTimeout(ctx, time.Minute)
	defer closeFunc()
	messageType, msg, err := ws.Read(receiveCtx)
	if err != nil {
		_ = ws.Close(websocket.StatusNormalClosure, "failed to read connect message")
		return nil, xerrors.Errorf("read connect message: %w", err)
	}
	// This is the "Connect" message that the client sends to identify itself.
	if messageType != websocket.MessageText {
		_ = ws.Close(websocket.StatusInvalidMessageType, "connect message must be text")
		return nil, xerrors.Errorf("connect message must be text; got: %v", messageType)
	}
	var connectMsg *AgentConnectMessage
	if err := json.Unmarshal(msg, &connectMsg); err != nil {
		_ = ws.Close(websocket.StatusPolicyViolation, "connect message must be valid JSON")
		return nil, xerrors.Errorf("unmarshal connect message: %w", err)
	}
	return ws, nil
}

// HandleConn registers a websocket connection as an agent API connection.
func (a *Agent) HandleConn(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	if err == nil {
		// Wait for the initial connect message to see what type of connection this is.
		ws, err = registerConn(r.Context(), ws)
		if err == nil {
			// This is an API connection.
			a.handleAPIConn(r.Context(), ws)
		}
	}
}

// Ready returns a channel that is closed when the agent is ready to be used.
// For example, a workspace with a startup script will not be ready until the
// startup script has completed or timed out.
func (a *Agent) Ready() <-chan struct{} {
	return a.startupScriptDoneCh
}

// connectedLoop manages the connected lifecycle of an agent.
//
// Agents will continuously retry a connection to the tailnet,
// which provides a DRPC connection back to the workspace proxy.
// The agent will use reconnections to implement retry behavior
// (like establishing websocket-NG connections), and will continue
// to retry even after failures connecting to the tailnet coordinator.
func (a *Agent) connectedLoop(outCtx context.Context) error {
	defer func() {
		// Ensure our ready channel is closed, if possible.
		close(a.readyCh)
	}()

	var lastNoopTime time.Time

	// This backoff will increase gradually as the agent fails
	// to connect to the controller.
	backoff := retry.Backoff{
		Floor: time.Second,
		Ceil:  30 * time.Second,
		Factor: 1.5,
		Jitter: true,
	}

	for {
		// This context will be canceled if the connection fails.
		// It's a sub-context of outCtx, which signals that the agent
		// is shutting down.
		connCtx, connCancel := context.WithCancel(outCtx)

		err := a.connect(connCtx)

		// connCancel is called here at the end of connect(), regardless of success
		// or failure. This is because connect() never returns until the connection
		// succeeds or the context is canceled.
		connCancel()

		// If interrupted the parent context is shutdown.
		if outCtx.Err() != nil {
			// Clean close: most likely, the agent's Close() method was called
			return nil
		}

		// Check if there's a specific reason we should not reconnect.
		if err != nil && !defaultShouldReconnect(err) {
			return err
		}

		// Collect connection errors for health checks.
		// Store the error directly if it's from connect.
		// We still want to reconnect, but we want to report why
		// we needed to reconnect.
		a.healthMu.Lock()
		a.healthErr = err
		a.healthMu.Unlock()

		// Here's a check for readiness: we'll close the ready channel if it hasn't been closed yet. This
		// is idempotent, since the channel is only closed on the first success. I considered
		// using a sync.Once, but we do have an ordering dependency: first the ready channel,
		// then the app ready channel.
		select {
		case <-a.readyCh:
			// Already closed
		default:
			// Close it the first time
			close(a.readyCh)
		}

		// Figure out how long to wait before reconnection. Report a
		// "reconnection" status to the client to inform them of our status.
		delay, err := a.reconnectionQuota.Consume()
		if err != nil {
			// If we return here, all agent behavior stops. In particular,
			// a.health() will fail, breaking agent healthz checks. That's
			// a bad user experience, so the quota being full doesn't
			// actually stop us from sending requests when possible. We
			// do however log the error for debugging purposes.
			a.logger.Warn(outCtx, "reconnection quota exceeded")
		}

		delay = a.addReconnectNoopJitter(delay, &lastNoopTime)
		a.logger.Info(outCtx, "reconnecting to agent", slog.F("delay", delay.String()))
		a.connectionStatus(ConnectionStatusReconnecting)

		select {
		case <-time.After(delay):
			continue
		case <-outCtx.Done():
			return nil
		}
	}
}

// connect establishes a websocket connection to the coder server, sets up tailnet,
// and then waits for the context to be canceled.
func (a *Agent) connect(ctx context.Context) error {
	// This connection dial will block until the connection can be established.
	// If the context is canceled, we'll exit.
	conn, drpcTailnet, err := a.dial(ctx)
	if err != nil {
		return err
	}

	// Record in agent health that we're connected now (no error).
	// This intentionally clears the error on a successful connect.
	a.healthMu.Lock()
	a.healthErr = nil
	a.healthMu.Unlock()

	a.conn = conn
	a.drpcTailnet = drpcTailnet

	// Once we have a connection, we can close the DRPC connection directly.
	// This must be deferred AFTER the connection is established, otherwise
	// we'll have a nil dereference.
	defer func() {
		_ = conn.Close(websocket.StatusGoingAway, "close connection")
	}()

	return a.handleAPI(ctx)
}

// addReconnectNoopJitter adds some jitter to the reconnection delay if the last noop was
// recent. This is to avoid a thundering herd of reconnections from all agents at the
// same time. This is deterministic behavior for debugging, but happens on randomized
// intervals.
func (a *Agent) addReconnectNoopJitter(delay time.Duration, lastNoopTime *time.Time) time.Duration {
	if time.Since(*lastNoopTime) < time.Minute {
		// If we've recently received a noop signal, add a bit of random jitter.
		jitter, _ := cryptorand.Intn(3000)
		delay += time.Duration(jitter) * time.Millisecond

		// Mark the time so we don't keep adding jitter.
		*lastNoopTime = time.Time{}
	}
	return delay
}

// dial establishes a websocket connection to the coder server. It will retry until
// the context is canceled, or a permanent error is encountered.
func (a *Agent) dial(ctx context.Context) (*websocket.Conn, drpcTailnet, error) {
	// Always add the agent id in a query param to catch duplicate agent ids.
	agentIDBase := url.QueryEscape(a.id.String())
	clientIDBase := ""
	sessionTokenBase := ""
	if a.auth != nil {
		clientIDBase = url.QueryEscape(a.auth.ClientID.String())
		sessionTokenBase = url.QueryEscape(a.auth.AccessToken.String())
	}

	manifest, err := json.Marshal(a.manifest)
	manifestB64 := base64.StdEncoding.EncodeToString(manifest)

	// This backoff will increase gradually as the agent fails
	// to connect to the controller.
	var (
		backoff = retry.Backoff{
			Floor:   time.Second,
			Ceil:    30 * time.Second,
			Factor:  1.5,
			Jitter:  true,
		}
		// counter helps us keep track of how many iterations we've gone through.
		counter int
		tailnet drpcTailnet
	)

	health := a.logger.With(slog.F("subsystem", "health"))
	for {
		counter++
		err = nil

		queryVals := url.Values{}
		queryVals.Set("version", buildinfo.Version())
		queryVals.Set("agent_id", agentIDBase)
		queryVals.Set("client_id", clientIDBase)
		queryVals.Set("session_token", sessionTokenBase)
		queryVals.Set("api_version", strconv.FormatUint(agentproto.Version, 10))
		queryVals.Set("manifest", manifestB64)

		// Tailnet configuration
		queryVals.Set("tailnet", "true")
		queryVals.Set("wait_coordination", "true")

		// Environment variables are useful for the server to know about the agent
		queryVals.Set("environment_variables", a.environmentVariablesToB64())

		// Use HTTP header authentication for enterprise edition, but only use it
		// if we're using authentication.
		auth := ""
		headers := http.Header{}
		if a.auth != nil {
			auth = fmt.Sprintf("agent-id:%s client-id:%s session-token:%s", agentIDBase, clientIDBase, sessionTokenBase)
			headers.Set("Authorization", auth)
		}

		// This computes the coordination URL, which is the coder server URL
		// with /api/v2/tailnet/coordination?{params} appended.
		// Note the query value is pre-escaped.
		u := &url.URL{
			Scheme:   "wss",
			Host:     a.options.Auth.WorkspaceHost,
			Path:     path.Join("/api/v2/tailnet/coordination"),
			RawQuery: queryVals.Encode(),
		}

		// If we're debugging, or on iteration 5, log the URL (but omit the session token).
		if counter%5 == 0 || a.logger.Level() <= slog.LevelDebug {
			health.Debug(ctx, "connecting to tailnet coordination endpoint", slog.F("url", strings.Split(u.String(), "session_token=")[0]+"session_token=REDACTED"))
		} else {
			health.Info(ctx, "connecting to tailnet coordination endpoint")
		}

		// If we're on a reconnection, we're in "reconnecting" state already.
		// But on the first connection, we need to set this status.
		if counter == 1 {
			a.connectionStatus(ConnectionStatusReconnecting)
		}

		// Dialing will either return success, a specific error that should stop the agent,
		// or a default error. Default errors will be retried, but we may still report them
		// to admins for easier debugging.
		conn, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
			HTTPHeader: headers,
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					// Pass a custom dialer to use for establishing the connection.
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						return a.dialer(ctx, network+"|"+addr)
					},
				},
			},
		})

		// Indicates an explicit client close. Time to give up!
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return nil, nil, xerrors.Errorf("canceled: %w", err)
		}

		// We can return early if we get a specific signal that the server is rejecting us.
		// ConnectionClosed signals that the server is deliberately rejecting us from connecting.
		closeStatus := websocket.CloseStatus(err)
		if closeStatus != -1 {
			// The client is shutting down, or we had an explicit websocket error. Let's try to parse it!
			var closeMsg string
			if closeErr, ok := err.(*websocket.CloseError); ok {
				closeMsg = closeErr.Reason
				if closeErr.Code == 1001 {
					// This happens when the server is shutting down.
					health.Info(ctx, "server is shutting down, waiting to reconnect", slog.F("reason", closeMsg))
					continue
				}
			}

			// Should we try to reconnect again? We should if this is a network error, or the server
			// is telling us to back off.
			if closeStatus == websocket.StatusInternalError ||
				closeStatus == websocket.StatusBadGateway ||
				closeStatus == websocket.StatusServiceRestart {
				health.Warn(ctx, "server error, reconnecting", slog.F("status", closeStatus), slog.F("reason", closeMsg))

				// Wait a bit before reconnecting.
				time.Sleep(backoff.Next())
				continue
			}

			// Here, we're returning specific errors that are not just "try to reconnect again".
			if closeStatus == websocket.StatusGoingAway ||
				closeStatus == websocket.StatusPolicyViolation ||
				closeStatus == 4000 { // This is a tailnet-specific status code.
				// This means the workspace was deleted, or the agent ID no longer belongs to Coder.
				// Since this is a legitimate error, we'll log it.
				health.Error(ctx, "server rejected connection, not reconnecting", slog.F("status", closeStatus), slog.F("reason", closeMsg))
				if closeStatus == 4000 {
					closeMsg = fmt.Sprintf("invalid tailnet coordination URL: %s", closeMsg)
				}
				return nil, nil, agenthelper.InvalidWorkspace(closeMsg)
			}
		}

		// General network errors. Retryable.
		if err != nil {
			health.Warn(ctx, "connection to server failed", slog.Error(err))
			// Wait a bit before reconnecting.
			time.Sleep(backoff.Next())
			continue
		}

		// Reset our backoff on a successful connection.
		backoff.Reset()

		// Wait for the tailnet coordination packet to come through.
		coordinateCtx, cancelFunc := context.WithTimeout(ctx, time.Minute)
		defer cancelFunc()

		// Ask tailnet to make a connection for us. This provides us with a client
		// that dials through the connected websocket channel.
		//
		// We need to pass this context explicitly (rather than just withCancel(ctx))
		// in order to prevent infinite connection loops. Tailnet should be able to
		// connect or fail within a reasonably short time period.
		coordinator, err := tailnet.NewCoordinator(coordinateCtx, conn)
		if err != nil {
			health.Warn(ctx, "tailnet coordination failed", slog.Error(err))

			// Close the connection explicitly since we're passing along the error.
			_ = conn.Close(websocket.StatusNormalClosure, "tailnet coordination failed")
			time.Sleep(backoff.Next())
			continue
		}

		// Perform coordination.
		tailnet := &tailnet.ClientCoordination{
			Coordinator: coordinator,
		}

		health.Debug(ctx, "connection to server established")
		a.connectionStatus(ConnectionStatusConnected)
		return conn, tailnet, nil
	}
}

// defaultShouldReconnect decides whether errors deserve reconnection.
// By default we always reconnect unless there's a permanent error.
func defaultShouldReconnect(err error) bool {
	if err == nil {
		return true
	}

	// This means the workspace was deleted, or the agent was renamed.
	if efw := (agenthelper.ErrFixWorkspace{}); errors.As(err, &efw) {
		return false
	}

	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		// Going away means the agent is no longer valid.
		if closeErr.Code == websocket.StatusGoingAway {
			return false
		}
	}

	return true
}

// initializeScratchDir creates the scratch directory if enabled.
func (a *Agent) initializeScratchDir() error {
	if a.options.NoScratchDirs {
		return nil
	}
	if a.options.ScratchDir == "" {
		a.options.ScratchDir = filepath.Join(a.options.Dir, "scratch")
	}
	err := os.MkdirAll(a.options.ScratchDir, 0o700)
	if err != nil {
		return xerrors.Errorf("makedirs: %w", err)
	}
	return nil
}

// initializeReconnectingPTY creates the reconnecting PTY handler.
func (a *Agent) initializeReconnectingPTY(ctx context.Context, healthLogger *waggy.Logger) error {
	// We only need to initialize a reconnecting PTY if we're accepting
	// reconnecting PTY connections from the client.
	if a.options.ReconnectingPTY == nil {
		return nil
	}

	if a.options.ReconnectingPTYOpts == nil {
		a.options.ReconnectingPTYOpts = &reconnectingpty.Options{}
	}

	if a.options.ReconnectingPTYOpts.Timeout == 0 {
		// Default if zero: 10 minutes inactive timeout.
		a.options.ReconnectingPTYOpts.Timeout = 10 * time.Minute
	}

	// Host environment variables help users find their way around the container.
	if len(a.options.EnvironmentVariables) > 0 {
		a.options.ReconnectingPTYOpts.EnvironmentVariables = a.options.EnvironmentVariables
	}

	a.options.ReconnectingPTYOpts.Logger = healthLogger

	return nil
}

// environmentVariablesToB64 encodes environment variables that should be passed to the coordinator.
func (a *Agent) environmentVariablesToB64() string {
	if len(a.options.EnvironmentVariables) == 0 {
		return ""
	}

	data, err := json.Marshal(a.options.EnvironmentVariables)
	if err != nil {
		a.logger.Error(context.Background(), "marshal environment variables", slog.Error(err))
		return ""
	}

	return base64.StdEncoding.EncodeToString(data)
}

// StartSSHServer starts an SSH server on the given address.
// The SSH server allows users to connect to the workspace via a regular SSH client.
// The agent exposes the reconnecting PTY interface via this SSH server.
// This can only be called once per agent, before the agent is started, and cannot
// be called on an agent that has already been started.
func (a *Agent) StartSSHServer(ctx context.Context, addr string) (err error) {
	if a.options.ReconnectingPTY == nil {
		return xerrors.New("reconnecting PTY manager is required for SSH server")
	}

	// Generate Host Keys
	var sshServer *ssh.Server

	// Default to the current user if none is set.
	sshUsername := a.options.SSHUsername
	if sshUsername == "" {
		currentUser, err := user.Current()
		if err != nil {
			return xerrors.Errorf("get current user: %w", err)
		}
		sshUsername = currentUser.Username
	}

	a.logger.Debug(ctx, "starting SSH server",
		slog.F("addr", addr),
		slog.F("username", sshUsername),
	)

	agentDir := a.options.Dir

	// Initialize the SSH server.
	serverOptions := []agentssh.ServerOption{
		agentssh.WithLogger(a.logger),
		agentssh.WithHostKeyDir(agentDir),
		agentssh.WithPTYManager(a.options.ReconnectingPTY),
		agentssh.WithUserProvidedOptions(a.options.ReconnectingPTYOpts),
		agentssh.WithUsername(sshUsername),
	}
	// We may want to ignore ownership checks if we're using file-locking to prevent
	// duplicate agents.
	if a.options.IgnoreOwnershipCheck {
		serverOptions = append(serverOptions, agentssh.WithIgnoreOwnershipCheck())
	}

	sshServer, err = agentssh.NewServer(serverOptions...)
	if err != nil {
		return xerrors.Errorf("create ssh server: %w", err)
	}

	// Listen for connections. If this fails, clean up and return an error.
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return xerrors.Errorf("listen: %w", err)
	}

	// Run the server.
	go func() {
		err := sshServer.Serve(listener)

		a.logger.Info(ctx, "ssh server stopped", slog.Error(err))
	}()

	// We have a new service to close.
	a.options.closer = agenthelper.CloseFunc(func() error {
		sshErr := sshServer.Close()
		if sshErr != nil {
			return xerrors.Errorf("close ssh server: %w", sshErr)
		}
		return nil
	})

	return nil
}

// handleAPI handles the API connection to the coder server.
// This is where we handle all the messages from the server.
func (a *Agent) handleAPI(ctx context.Context) error {
	tailnetReady := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-tailnetReady:
			// We're connected to the tailnet, so we're ready to accept inbound
			// requests from the server. Close the readyCh to signal this.
			select {
			case <-a.readyCh:
				// Already closed
			default:
				close(a.readyCh)
			}

			// Now, we're ready to accept inbound connections to coder apps.
			// Close the appReadyCh to signal this.
			select {
			case <-a.appReadyCh:
				// Already closed
			default:
				close(a.appReadyCh)
			}

			return
		}
	}()

	// Time to set up the tailnet connection.
	cleanup, peer, err := a.setupTailnet(ctx, a.drpcTailnet)
	if err != nil {
		return xerrors.Errorf("setup tailnet: %w", err)
	}
	defer cleanup()

	// With local tailnet setup, we're officially as ready as we can be.
	close(tailnetReady)

	err = a.startRouting(ctx, peer)
	if err != nil {
		return xerrors.Errorf("start routing: %w", err)
	}

	// Now, sit and wait until the context is canceled. If the server
	// disconnects us (connection errors) then peer.Serve() would return
	// and we'd return an error -- but canceling the context will also
	// cause peer.Serve() to return.
	a.logger.Debug(ctx, "starting tailnet peer serve")
	err = peer.Serve(ctx)
	if !errors.Is(err, context.Canceled) {
		return xerrors.Errorf("tailnet peer serve: %w", err)
	}
	return nil
}

// setupTailnet initializes the tailnet connection and negotiates a DRPC
// connection to the coder server. It returns a cleanup function that should
// be called when the connection is done, a DRPC peer listener for the agent.
func (a *Agent) setupTailnet(ctx context.Context, coordinator drpcTailnet) (cleanup func(), peer *tailnet.Peer, err error) {
	defer func() {
		if err != nil && cleanup != nil {
			cleanup()
			cleanup = nil
		}
	}()

	var (
		newPeerNetwork = tailnet.PeerNetworkAgentConn
		newNode        = tailnet.NewNode
	)

	// Now coordinate to receive a node ID.
	tn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:         []netip.Prefix{},
		DERPMap:           nil,
		Logger:            a.logger,
		PeerClientDialer:  coordinator.Dial,
		DisableTailnetDNS: true,
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("new tailnet conn: %w", err)
	}

	network := newPeerNetwork(tn, a.logger)
	node, err := newNode(network, a.logger)
	if err != nil {
		_ = tn.Close()
		return nil, nil, xerrors.Errorf("new node: %w", err)
	}
	a.tailnet.coordination = tn
	a.tailnet.closed.Store(false)
	a.tailnet.closer = agenthelper.CloseFunc(func() error {
		if a.tailnet.closed.CompareAndSwap(false, true) {
			node.Close()
			_ = tn.Close()
		}
		return nil
	})

	cleanup = func() {
		_ = a.tailnet.closer.Close()
	}

	// Now, we're ready to start listening. The server expects agent DRPC
	// connections to come from the node after this point.
	//
	// The peer is used to process inbound connections.
	peer, err = tailnet.NewPeer(tailnet.PeerOptions{
		Node:   node,
		Logger: a.logger,
	})
	if err != nil {
		return cleanup, nil, xerrors.Errorf("new peer: %w", err)
	}

	// Register the agent DRPC service.
	service := &agentService{
		agent: a,
	}
	mux := drpcmux.New()
	err = agentproto_drpc.DRPCRegisterAgent(mux, service)
	if err != nil {
		return cleanup, nil, xerrors.Errorf("drpc register agent: %w", err)
	}

	// This is how we listen for inbound connections from the server.
	// The peer will forward them to the agent service.
	server := drpcserver.New(mux)
	err = peer.ServeHTTP("/agent",
		&tailnet.HandlerOptions{
			DRPC: server,
		})
	if err != nil {
		return cleanup, nil, xerrors.Errorf("listen: %w", err)
	}

	// For reconnecting PTY, the server binds the agent URL suffix.
	if a.options.ReconnectingPTY != nil {
		a.options.ReconnectingPTY.SetServer(&reconnectingPTYHandler{
			server: peer.HTTPServer(),
		})
	}

	return cleanup, peer, nil
}

// startRouting sets up the client DRPC connection to the coder server.
// This is how we send messages to the server.
func (a *Agent) startRouting(ctx context.Context, peer *tailnet.Peer) error {
	// Now finally, we send a request to the server to notify it that
	// we are online. The server will respond with its own handshake.
	var (
		client     *drpcconn.Conn
		clientChan = make(chan *drpcconn.Conn, 1)
		errChan    = make(chan error, 1)
	)

	// Dial the agent API endpoint on the server.
	go func() {
		a.logger.Debug(ctx, "dialing server agent API")
		conn, err := a.drpcTailnet.Dial(ctx, 0, nil)
		if err != nil {
			errChan <- xerrors.Errorf("dial api: %w", err)
			return
		}

		// Wrap the raw connection in a DRPC connection to the API.
		// Here, we're connected directly to the workspace coordinator proxy. It's the server
		// handling the other side.
		drpcConn := drpcconn.New(conn)
		clientChan <- drpcConn
	}()

	select {
	case client = <-clientChan:
		break
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	// Store the client for later use.
	a.clientLock.Lock()
	defer a.clientLock.Unlock()
	a.client = client

	a.clientProto = agentproto_drpc.NewDRPCAgentClient(a.client)

	return nil
}

// handleAPIConn handles a websocket connection that was set up for the
// agent/API connection. This is for inbound connections from the coder server.
func (a *Agent) handleAPIConn(ctx context.Context, conn *websocket.Conn) {
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "agent closed connection")
	}()

	// This is a message handler.
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return
		}

		// Just ping-pong. We aren't expecting any actual messages.
		_ = conn.Ping(ctx)
	}
}

// initMetrics initializes the metrics collectors for the agent.
func (a *Agent) initMetrics() error {
	a.collectMetricsMu.Lock()
	defer a.collectMetricsMu.Unlock()

	if a.metrics != nil {
		return nil
	}

	return a.newMetrics()
}

// GetStats returns statistics for the agent.
func (a *Agent) GetStats(ctx context.Context) *agentproto.Stats {
	stats := &agentproto.Stats{
		Version:                  buildinfo.Version(),
		SessionCount:             int64(a.sessionCount()),
		ConnectionCount:          0,
		ConnectionMedianLatencyMs: 0,
		RxBytes:                  a.rxBytes(),
		TxBytes:                  a.txBytes(),
		Metrics:                  nil,
	}

	return a.measureLatencyStats(ctx, stats)
}

// sessionCount returns how many reconnecting PTY sessions are open.
func (a *Agent) sessionCount() uint64 {
	if a.options.ReconnectingPTY == nil {
		return 0
	}
	return uint64(a.options.ReconnectingPTY.Count())
}

// rxBytes returns the received bytes counter of the agent.
func (a *Agent) rxBytes() uint64 {
	tailnet := a.tailnet.coordination
	if tailnet == nil {
		return 0
	}
	return tailnet.RxBytes()
}

// txBytes returns the sent bytes counter of the agent.
func (a *Agent) txBytes() uint64 {
	tailnet := a.tailnet.coordination
	if tailnet == nil {
		return 0
	}
	return tailnet.TxBytes()
}

// measureLatencyStats performs latency measurements for agent stats. It returns a new
// stats object with latency metrics added.
func (a *Agent) measureLatencyStats(ctx context.Context, stats *agentproto.Stats) *agentproto.Stats {
	// Create the stats and run all health checks.
	a.clientLock.RLock()
	client := a.clientProto
	a.clientLock.RUnlock()

	if client == nil {
		stats.ConnectionMedianLatencyMs = -1
		return stats
	}

	// Always report latency, we've done some refactoring to ensure it's cheap.
	var (
		wg             sync.WaitGroup
		durations      = make([]float64, 0)
		mu             sync.Mutex
		p2pConns int64 = 0
		derpConns      = 0
	)

	sendRequest := func(p2p bool) {
		wg.Add(1)
		go func() {
			// This goroutine captures durations, p2pConns, and derpConns, using mu to synchronize.
			defer wg.Done()

			// Report latency if we can.
			startTime := time.Now()
			resp, err := client.CheckHealth(ctx, &agentproto.HealthCheckRequest{
				P2P: p2p,
			})
			// How long did it take?
			duration := time.Since(startTime)
			if err != nil || resp == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			durations = append(durations, float64(duration.Microseconds()))
			if p2p {
				p2pConns++
			} else {
				derpConns++
			}
		}()
	}

	// Let's run two requests - one for p2p, one for derp.
	// Ideally we'd run more, and average them, but we're just
	// checking connectivity here so we'll be quick.
	sendRequest(true)
	sendRequest(false)
	wg.Wait()
	sort.Float64s(durations)
	durationsLength := len(durations)
	switch {
	case durationsLength == 0:
		stats.ConnectionMedianLatencyMs = -1
	case durationsLength%2 == 0:
		stats.ConnectionMedianLatencyMs = (durations[durationsLength/2-1] + durations[durationsLength/2]) / 2
	default:
		stats.ConnectionMedianLatencyMs = durations[durationsLength/2]
	}
	// Convert from microseconds to milliseconds.
	stats.ConnectionMedianLatencyMs /= 1000

	// Collect agent metrics.
	// Agent metrics are changing all the time, so there is no need to perform
	// reflect.DeepEqual to see if stats should be transferred.

	// currentConnections behaves like a hypothetical `GaugeFuncVec` and is only set at collection time.
	a.metrics.currentConnections.WithLabelValues("p2p").Set(float64(p2pConns))
	a.metrics.currentConnections.WithLabelValues("derp").Set(float64(derpConns))
	metricsCtx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFunc()
	a.logger.Debug(ctx, "collecting agent metrics for stats")
	stats.Metrics = a.collectMetrics(metricsCtx)

	return stats
}