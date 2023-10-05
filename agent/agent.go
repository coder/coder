package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"go.uber.org/atomic"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netlogtype"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/agentscripts"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/reconnectingpty"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/gitauth"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/retry"
)

const (
	ProtocolReconnectingPTY = "reconnecting-pty"
	ProtocolSSH             = "ssh"
	ProtocolDial            = "dial"
)

// EnvProcPrioMgmt determines whether we attempt to manage
// process CPU and OOM Killer priority.
const EnvProcPrioMgmt = "CODER_PROC_PRIO_MGMT"

type Options struct {
	Filesystem                   afero.Fs
	LogDir                       string
	TempDir                      string
	ExchangeToken                func(ctx context.Context) (string, error)
	Client                       Client
	ReconnectingPTYTimeout       time.Duration
	EnvironmentVariables         map[string]string
	Logger                       slog.Logger
	IgnorePorts                  map[int]string
	SSHMaxTimeout                time.Duration
	TailnetListenPort            uint16
	Subsystems                   []codersdk.AgentSubsystem
	Addresses                    []netip.Prefix
	PrometheusRegistry           *prometheus.Registry
	ReportMetadataInterval       time.Duration
	ServiceBannerRefreshInterval time.Duration
	Syscaller                    agentproc.Syscaller
	// ModifiedProcesses is used for testing process priority management.
	ModifiedProcesses chan []*agentproc.Process
	// ProcessManagementTick is used for testing process priority management.
	ProcessManagementTick <-chan time.Time
}

type Client interface {
	Manifest(ctx context.Context) (agentsdk.Manifest, error)
	Listen(ctx context.Context) (net.Conn, error)
	DERPMapUpdates(ctx context.Context) (<-chan agentsdk.DERPMapUpdate, io.Closer, error)
	ReportStats(ctx context.Context, log slog.Logger, statsChan <-chan *agentsdk.Stats, setInterval func(time.Duration)) (io.Closer, error)
	PostLifecycle(ctx context.Context, state agentsdk.PostLifecycleRequest) error
	PostAppHealth(ctx context.Context, req agentsdk.PostAppHealthsRequest) error
	PostStartup(ctx context.Context, req agentsdk.PostStartupRequest) error
	PostMetadata(ctx context.Context, req agentsdk.PostMetadataRequest) error
	PatchLogs(ctx context.Context, req agentsdk.PatchLogs) error
	GetServiceBanner(ctx context.Context) (codersdk.ServiceBannerConfig, error)
}

type Agent interface {
	HTTPDebug() http.Handler
	// TailnetConn may be nil.
	TailnetConn() *tailnet.Conn
	io.Closer
}

func New(options Options) Agent {
	if options.Filesystem == nil {
		options.Filesystem = afero.NewOsFs()
	}
	if options.TempDir == "" {
		options.TempDir = os.TempDir()
	}
	if options.LogDir == "" {
		if options.TempDir != os.TempDir() {
			options.Logger.Debug(context.Background(), "log dir not set, using temp dir", slog.F("temp_dir", options.TempDir))
		}
		options.LogDir = options.TempDir
	}
	if options.ExchangeToken == nil {
		options.ExchangeToken = func(ctx context.Context) (string, error) {
			return "", nil
		}
	}
	if options.ReportMetadataInterval == 0 {
		options.ReportMetadataInterval = time.Second
	}
	if options.ServiceBannerRefreshInterval == 0 {
		options.ServiceBannerRefreshInterval = 2 * time.Minute
	}

	prometheusRegistry := options.PrometheusRegistry
	if prometheusRegistry == nil {
		prometheusRegistry = prometheus.NewRegistry()
	}

	if options.Syscaller == nil {
		options.Syscaller = agentproc.NewSyscaller()
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	a := &agent{
		tailnetListenPort:            options.TailnetListenPort,
		reconnectingPTYTimeout:       options.ReconnectingPTYTimeout,
		logger:                       options.Logger,
		closeCancel:                  cancelFunc,
		closed:                       make(chan struct{}),
		envVars:                      options.EnvironmentVariables,
		client:                       options.Client,
		exchangeToken:                options.ExchangeToken,
		filesystem:                   options.Filesystem,
		logDir:                       options.LogDir,
		tempDir:                      options.TempDir,
		lifecycleUpdate:              make(chan struct{}, 1),
		lifecycleReported:            make(chan codersdk.WorkspaceAgentLifecycle, 1),
		lifecycleStates:              []agentsdk.PostLifecycleRequest{{State: codersdk.WorkspaceAgentLifecycleCreated}},
		ignorePorts:                  options.IgnorePorts,
		connStatsChan:                make(chan *agentsdk.Stats, 1),
		reportMetadataInterval:       options.ReportMetadataInterval,
		serviceBannerRefreshInterval: options.ServiceBannerRefreshInterval,
		sshMaxTimeout:                options.SSHMaxTimeout,
		subsystems:                   options.Subsystems,
		addresses:                    options.Addresses,
		syscaller:                    options.Syscaller,
		modifiedProcs:                options.ModifiedProcesses,
		processManagementTick:        options.ProcessManagementTick,

		prometheusRegistry: prometheusRegistry,
		metrics:            newAgentMetrics(prometheusRegistry),
	}
	a.init(ctx)
	return a
}

type agent struct {
	logger            slog.Logger
	client            Client
	exchangeToken     func(ctx context.Context) (string, error)
	tailnetListenPort uint16
	filesystem        afero.Fs
	logDir            string
	tempDir           string
	// ignorePorts tells the api handler which ports to ignore when
	// listing all listening ports. This is helpful to hide ports that
	// are used by the agent, that the user does not care about.
	ignorePorts map[int]string
	subsystems  []codersdk.AgentSubsystem

	reconnectingPTYs       sync.Map
	reconnectingPTYTimeout time.Duration

	connCloseWait sync.WaitGroup
	closeCancel   context.CancelFunc
	closeMutex    sync.Mutex
	closed        chan struct{}

	envVars map[string]string

	manifest                     atomic.Pointer[agentsdk.Manifest] // manifest is atomic because values can change after reconnection.
	reportMetadataInterval       time.Duration
	scriptRunner                 *agentscripts.Runner
	serviceBanner                atomic.Pointer[codersdk.ServiceBannerConfig] // serviceBanner is atomic because it is periodically updated.
	serviceBannerRefreshInterval time.Duration
	sessionToken                 atomic.Pointer[string]
	sshServer                    *agentssh.Server
	sshMaxTimeout                time.Duration

	lifecycleUpdate   chan struct{}
	lifecycleReported chan codersdk.WorkspaceAgentLifecycle
	lifecycleMu       sync.RWMutex // Protects following.
	lifecycleStates   []agentsdk.PostLifecycleRequest

	network       *tailnet.Conn
	addresses     []netip.Prefix
	connStatsChan chan *agentsdk.Stats
	latestStat    atomic.Pointer[agentsdk.Stats]

	connCountReconnectingPTY atomic.Int64

	prometheusRegistry *prometheus.Registry
	metrics            *agentMetrics
	syscaller          agentproc.Syscaller

	// modifiedProcs is used for testing process priority management.
	modifiedProcs chan []*agentproc.Process
	// processManagementTick is used for testing process priority management.
	processManagementTick <-chan time.Time
}

func (a *agent) TailnetConn() *tailnet.Conn {
	return a.network
}

func (a *agent) init(ctx context.Context) {
	sshSrv, err := agentssh.NewServer(ctx, a.logger.Named("ssh-server"), a.prometheusRegistry, a.filesystem, a.sshMaxTimeout, "")
	if err != nil {
		panic(err)
	}
	sshSrv.Env = a.envVars
	sshSrv.AgentToken = func() string { return *a.sessionToken.Load() }
	sshSrv.Manifest = &a.manifest
	sshSrv.ServiceBanner = &a.serviceBanner
	a.sshServer = sshSrv
	a.scriptRunner = agentscripts.New(agentscripts.Options{
		LogDir:     a.logDir,
		Logger:     a.logger,
		SSHServer:  sshSrv,
		Filesystem: a.filesystem,
		PatchLogs:  a.client.PatchLogs,
	})
	go a.runLoop(ctx)
}

// runLoop attempts to start the agent in a retry loop.
// Coder may be offline temporarily, a connection issue
// may be happening, but regardless after the intermittent
// failure, you'll want the agent to reconnect.
func (a *agent) runLoop(ctx context.Context) {
	go a.reportLifecycleLoop(ctx)
	go a.reportMetadataLoop(ctx)
	go a.fetchServiceBannerLoop(ctx)
	go a.manageProcessPriorityLoop(ctx)

	for retrier := retry.New(100*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		a.logger.Info(ctx, "connecting to coderd")
		err := a.run(ctx)
		// Cancel after the run is complete to clean up any leaked resources!
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			// Context canceled errors may come from websocket pings, so we
			// don't want to use `errors.Is(err, context.Canceled)` here.
			return
		}
		if a.isClosed() {
			return
		}
		if errors.Is(err, io.EOF) {
			a.logger.Info(ctx, "disconnected from coderd")
			continue
		}
		a.logger.Warn(ctx, "run exited with error", slog.Error(err))
	}
}

func (a *agent) collectMetadata(ctx context.Context, md codersdk.WorkspaceAgentMetadataDescription, now time.Time) *codersdk.WorkspaceAgentMetadataResult {
	var out bytes.Buffer
	result := &codersdk.WorkspaceAgentMetadataResult{
		// CollectedAt is set here for testing purposes and overrode by
		// coderd to the time of server receipt to solve clock skew.
		//
		// In the future, the server may accept the timestamp from the agent
		// if it can guarantee the clocks are synchronized.
		CollectedAt: now,
	}
	cmdPty, err := a.sshServer.CreateCommand(ctx, md.Script, nil)
	if err != nil {
		result.Error = fmt.Sprintf("create cmd: %+v", err)
		return result
	}
	cmd := cmdPty.AsExec()

	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Stdin = io.LimitReader(nil, 0)

	// We split up Start and Wait instead of calling Run so that we can return a more precise error.
	err = cmd.Start()
	if err != nil {
		result.Error = fmt.Sprintf("start cmd: %+v", err)
		return result
	}

	// This error isn't mutually exclusive with useful output.
	err = cmd.Wait()
	const bufLimit = 10 << 10
	if out.Len() > bufLimit {
		err = errors.Join(
			err,
			xerrors.Errorf("output truncated from %v to %v bytes", out.Len(), bufLimit),
		)
		out.Truncate(bufLimit)
	}

	// Important: if the command times out, we may see a misleading error like
	// "exit status 1", so it's important to include the context error.
	err = errors.Join(err, ctx.Err())

	if err != nil {
		result.Error = fmt.Sprintf("run cmd: %+v", err)
	}
	result.Value = out.String()
	return result
}

type metadataResultAndKey struct {
	result *codersdk.WorkspaceAgentMetadataResult
	key    string
}

type trySingleflight struct {
	mu sync.Mutex
	m  map[string]struct{}
}

func (t *trySingleflight) Do(key string, fn func()) {
	t.mu.Lock()
	_, ok := t.m[key]
	if ok {
		t.mu.Unlock()
		return
	}

	t.m[key] = struct{}{}
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.m, key)
		t.mu.Unlock()
	}()

	fn()
}

func (a *agent) reportMetadataLoop(ctx context.Context) {
	const metadataLimit = 128

	var (
		baseTicker        = time.NewTicker(a.reportMetadataInterval)
		lastCollectedAtMu sync.RWMutex
		lastCollectedAts  = make(map[string]time.Time)
		metadataResults   = make(chan metadataResultAndKey, metadataLimit)
		logger            = a.logger.Named("metadata")
	)
	defer baseTicker.Stop()

	// We use a custom singleflight that immediately returns if there is already
	// a goroutine running for a given key. This is to prevent a build-up of
	// goroutines waiting on Do when the script takes many multiples of
	// baseInterval to run.
	flight := trySingleflight{m: map[string]struct{}{}}

	postMetadata := func(mr metadataResultAndKey) {
		err := a.client.PostMetadata(ctx, agentsdk.PostMetadataRequest{
			Metadata: []agentsdk.Metadata{
				{
					Key:                          mr.key,
					WorkspaceAgentMetadataResult: *mr.result,
				},
			},
		})
		if err != nil {
			a.logger.Error(ctx, "agent failed to report metadata", slog.Error(err))
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case mr := <-metadataResults:
			postMetadata(mr)
			continue
		case <-baseTicker.C:
		}

		if len(metadataResults) > 0 {
			// The inner collection loop expects the channel is empty before spinning up
			// all the collection goroutines.
			logger.Debug(ctx, "metadata collection backpressured",
				slog.F("queue_len", len(metadataResults)),
			)
			continue
		}

		manifest := a.manifest.Load()
		if manifest == nil {
			continue
		}

		if len(manifest.Metadata) > metadataLimit {
			logger.Error(
				ctx, "metadata limit exceeded",
				slog.F("limit", metadataLimit), slog.F("got", len(manifest.Metadata)),
			)
			continue
		}

		// If the manifest changes (e.g. on agent reconnect) we need to
		// purge old cache values to prevent lastCollectedAt from growing
		// boundlessly.
		lastCollectedAtMu.Lock()
		for key := range lastCollectedAts {
			if slices.IndexFunc(manifest.Metadata, func(md codersdk.WorkspaceAgentMetadataDescription) bool {
				return md.Key == key
			}) < 0 {
				logger.Debug(ctx, "deleting lastCollected key, missing from manifest",
					slog.F("key", key),
				)
				delete(lastCollectedAts, key)
			}
		}
		lastCollectedAtMu.Unlock()

		// Spawn a goroutine for each metadata collection, and use a
		// channel to synchronize the results and avoid both messy
		// mutex logic and overloading the API.
		for _, md := range manifest.Metadata {
			md := md
			// We send the result to the channel in the goroutine to avoid
			// sending the same result multiple times. So, we don't care about
			// the return values.
			go flight.Do(md.Key, func() {
				ctx := slog.With(ctx, slog.F("key", md.Key))
				lastCollectedAtMu.RLock()
				collectedAt, ok := lastCollectedAts[md.Key]
				lastCollectedAtMu.RUnlock()
				if ok {
					// If the interval is zero, we assume the user just wants
					// a single collection at startup, not a spinning loop.
					if md.Interval == 0 {
						return
					}
					intervalUnit := time.Second
					// reportMetadataInterval is only less than a second in tests,
					// so adjust the interval unit for them.
					if a.reportMetadataInterval < time.Second {
						intervalUnit = 100 * time.Millisecond
					}
					// The last collected value isn't quite stale yet, so we skip it.
					if collectedAt.Add(time.Duration(md.Interval) * intervalUnit).After(time.Now()) {
						return
					}
				}

				timeout := md.Timeout
				if timeout == 0 {
					if md.Interval != 0 {
						timeout = md.Interval
					} else if interval := int64(a.reportMetadataInterval.Seconds()); interval != 0 {
						// Fallback to the report interval
						timeout = interval * 3
					} else {
						// If the interval is still 0 (possible if the interval
						// is less than a second), default to 5. This was
						// randomly picked.
						timeout = 5
					}
				}
				ctxTimeout := time.Duration(timeout) * time.Second
				ctx, cancel := context.WithTimeout(ctx, ctxTimeout)
				defer cancel()

				now := time.Now()
				select {
				case <-ctx.Done():
					logger.Warn(ctx, "metadata collection timed out", slog.F("timeout", ctxTimeout))
				case metadataResults <- metadataResultAndKey{
					key:    md.Key,
					result: a.collectMetadata(ctx, md, now),
				}:
					lastCollectedAtMu.Lock()
					lastCollectedAts[md.Key] = now
					lastCollectedAtMu.Unlock()
				}
			})
		}
	}
}

// reportLifecycleLoop reports the current lifecycle state once. All state
// changes are reported in order.
func (a *agent) reportLifecycleLoop(ctx context.Context) {
	lastReportedIndex := 0 // Start off with the created state without reporting it.
	for {
		select {
		case <-a.lifecycleUpdate:
		case <-ctx.Done():
			return
		}

		for r := retry.New(time.Second, 15*time.Second); r.Wait(ctx); {
			a.lifecycleMu.RLock()
			lastIndex := len(a.lifecycleStates) - 1
			report := a.lifecycleStates[lastReportedIndex]
			if len(a.lifecycleStates) > lastReportedIndex+1 {
				report = a.lifecycleStates[lastReportedIndex+1]
			}
			a.lifecycleMu.RUnlock()

			if lastIndex == lastReportedIndex {
				break
			}

			a.logger.Debug(ctx, "reporting lifecycle state", slog.F("payload", report))

			err := a.client.PostLifecycle(ctx, report)
			if err == nil {
				lastReportedIndex++
				select {
				case a.lifecycleReported <- report.State:
				case <-a.lifecycleReported:
					a.lifecycleReported <- report.State
				}
				if lastReportedIndex < lastIndex {
					// Keep reporting until we've sent all messages, we can't
					// rely on the channel triggering us before the backlog is
					// consumed.
					continue
				}
				break
			}
			if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
				return
			}
			// If we fail to report the state we probably shouldn't exit, log only.
			a.logger.Error(ctx, "agent failed to report the lifecycle state", slog.Error(err))
		}
	}
}

// setLifecycle sets the lifecycle state and notifies the lifecycle loop.
// The state is only updated if it's a valid state transition.
func (a *agent) setLifecycle(ctx context.Context, state codersdk.WorkspaceAgentLifecycle) {
	report := agentsdk.PostLifecycleRequest{
		State:     state,
		ChangedAt: dbtime.Now(),
	}

	a.lifecycleMu.Lock()
	lastReport := a.lifecycleStates[len(a.lifecycleStates)-1]
	if slices.Index(codersdk.WorkspaceAgentLifecycleOrder, lastReport.State) >= slices.Index(codersdk.WorkspaceAgentLifecycleOrder, report.State) {
		a.logger.Warn(ctx, "attempted to set lifecycle state to a previous state", slog.F("last", lastReport), slog.F("current", report))
		a.lifecycleMu.Unlock()
		return
	}
	a.lifecycleStates = append(a.lifecycleStates, report)
	a.logger.Debug(ctx, "set lifecycle state", slog.F("current", report), slog.F("last", lastReport))
	a.lifecycleMu.Unlock()

	select {
	case a.lifecycleUpdate <- struct{}{}:
	default:
	}
}

// fetchServiceBannerLoop fetches the service banner on an interval.  It will
// not be fetched immediately; the expectation is that it is primed elsewhere
// (and must be done before the session actually starts).
func (a *agent) fetchServiceBannerLoop(ctx context.Context) {
	ticker := time.NewTicker(a.serviceBannerRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			serviceBanner, err := a.client.GetServiceBanner(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				a.logger.Error(ctx, "failed to update service banner", slog.Error(err))
				continue
			}
			a.serviceBanner.Store(&serviceBanner)
		}
	}
}

func (a *agent) run(ctx context.Context) error {
	// This allows the agent to refresh it's token if necessary.
	// For instance identity this is required, since the instance
	// may not have re-provisioned, but a new agent ID was created.
	sessionToken, err := a.exchangeToken(ctx)
	if err != nil {
		return xerrors.Errorf("exchange token: %w", err)
	}
	a.sessionToken.Store(&sessionToken)

	serviceBanner, err := a.client.GetServiceBanner(ctx)
	if err != nil {
		return xerrors.Errorf("fetch service banner: %w", err)
	}
	a.serviceBanner.Store(&serviceBanner)

	manifest, err := a.client.Manifest(ctx)
	if err != nil {
		return xerrors.Errorf("fetch metadata: %w", err)
	}
	a.logger.Info(ctx, "fetched manifest", slog.F("manifest", manifest))

	if manifest.AgentID == uuid.Nil {
		return xerrors.New("nil agentID returned by manifest")
	}

	// Expand the directory and send it back to coderd so external
	// applications that rely on the directory can use it.
	//
	// An example is VS Code Remote, which must know the directory
	// before initializing a connection.
	manifest.Directory, err = expandDirectory(manifest.Directory)
	if err != nil {
		return xerrors.Errorf("expand directory: %w", err)
	}
	err = a.client.PostStartup(ctx, agentsdk.PostStartupRequest{
		Version:           buildinfo.Version(),
		ExpandedDirectory: manifest.Directory,
		Subsystems:        a.subsystems,
	})
	if err != nil {
		return xerrors.Errorf("update workspace agent version: %w", err)
	}

	oldManifest := a.manifest.Swap(&manifest)

	// The startup script should only execute on the first run!
	if oldManifest == nil {
		a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleStarting)

		// Perform overrides early so that Git auth can work even if users
		// connect to a workspace that is not yet ready. We don't run this
		// concurrently with the startup script to avoid conflicts between
		// them.
		if manifest.GitAuthConfigs > 0 {
			// If this fails, we should consider surfacing the error in the
			// startup log and setting the lifecycle state to be "start_error"
			// (after startup script completion), but for now we'll just log it.
			err := gitauth.OverrideVSCodeConfigs(a.filesystem)
			if err != nil {
				a.logger.Warn(ctx, "failed to override vscode git auth configs", slog.Error(err))
			}
		}

		err = a.scriptRunner.Init(manifest.Scripts)
		if err != nil {
			return xerrors.Errorf("init script runner: %w", err)
		}
		err = a.trackConnGoroutine(func() {
			err := a.scriptRunner.Execute(ctx, func(script codersdk.WorkspaceAgentScript) bool {
				return script.RunOnStart
			})
			if err != nil {
				a.logger.Warn(ctx, "startup script failed", slog.Error(err))
				if errors.Is(err, agentscripts.ErrTimeout) {
					a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleStartTimeout)
				} else {
					a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleStartError)
				}
			} else {
				a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleReady)
			}
			a.scriptRunner.StartCron()
		})
		if err != nil {
			return xerrors.Errorf("track conn goroutine: %w", err)
		}
	}

	// This automatically closes when the context ends!
	appReporterCtx, appReporterCtxCancel := context.WithCancel(ctx)
	defer appReporterCtxCancel()
	go NewWorkspaceAppHealthReporter(
		a.logger, manifest.Apps, a.client.PostAppHealth)(appReporterCtx)

	a.closeMutex.Lock()
	network := a.network
	a.closeMutex.Unlock()
	if network == nil {
		network, err = a.createTailnet(ctx, manifest.AgentID, manifest.DERPMap, manifest.DERPForceWebSockets, manifest.DisableDirectConnections)
		if err != nil {
			return xerrors.Errorf("create tailnet: %w", err)
		}
		a.closeMutex.Lock()
		// Re-check if agent was closed while initializing the network.
		closed := a.isClosed()
		if !closed {
			a.network = network
		}
		a.closeMutex.Unlock()
		if closed {
			_ = network.Close()
			return xerrors.New("agent is closed")
		}

		a.startReportingConnectionStats(ctx)
	} else {
		// Update the wireguard IPs if the agent ID changed.
		err := network.SetAddresses(a.wireguardAddresses(manifest.AgentID))
		if err != nil {
			a.logger.Error(ctx, "update tailnet addresses", slog.Error(err))
		}
		// Update the DERP map, force WebSocket setting and allow/disallow
		// direct connections.
		network.SetDERPMap(manifest.DERPMap)
		network.SetDERPForceWebSockets(manifest.DERPForceWebSockets)
		network.SetBlockEndpoints(manifest.DisableDirectConnections)
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		a.logger.Debug(egCtx, "running tailnet connection coordinator")
		err := a.runCoordinator(egCtx, network)
		if err != nil {
			return xerrors.Errorf("run coordinator: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		a.logger.Debug(egCtx, "running derp map subscriber")
		err := a.runDERPMapSubscriber(egCtx, network)
		if err != nil {
			return xerrors.Errorf("run derp map subscriber: %w", err)
		}
		return nil
	})

	return eg.Wait()
}

func (a *agent) wireguardAddresses(agentID uuid.UUID) []netip.Prefix {
	if len(a.addresses) == 0 {
		return []netip.Prefix{
			// This is the IP that should be used primarily.
			netip.PrefixFrom(tailnet.IPFromUUID(agentID), 128),
			// We also listen on the legacy codersdk.WorkspaceAgentIP. This
			// allows for a transition away from wsconncache.
			netip.PrefixFrom(codersdk.WorkspaceAgentIP, 128),
		}
	}

	return a.addresses
}

func (a *agent) trackConnGoroutine(fn func()) error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.isClosed() {
		return xerrors.New("track conn goroutine: agent is closed")
	}
	a.connCloseWait.Add(1)
	go func() {
		defer a.connCloseWait.Done()
		fn()
	}()
	return nil
}

func (a *agent) createTailnet(ctx context.Context, agentID uuid.UUID, derpMap *tailcfg.DERPMap, derpForceWebSockets, disableDirectConnections bool) (_ *tailnet.Conn, err error) {
	network, err := tailnet.NewConn(&tailnet.Options{
		ID:                  agentID,
		Addresses:           a.wireguardAddresses(agentID),
		DERPMap:             derpMap,
		DERPForceWebSockets: derpForceWebSockets,
		Logger:              a.logger.Named("net.tailnet"),
		ListenPort:          a.tailnetListenPort,
		BlockEndpoints:      disableDirectConnections,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}
	defer func() {
		if err != nil {
			network.Close()
		}
	}()

	sshListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.WorkspaceAgentSSHPort))
	if err != nil {
		return nil, xerrors.Errorf("listen on the ssh port: %w", err)
	}
	defer func() {
		if err != nil {
			_ = sshListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		_ = a.sshServer.Serve(sshListener)
	}); err != nil {
		return nil, err
	}

	reconnectingPTYListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.WorkspaceAgentReconnectingPTYPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for reconnecting pty: %w", err)
	}
	defer func() {
		if err != nil {
			_ = reconnectingPTYListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		logger := a.logger.Named("reconnecting-pty")
		var wg sync.WaitGroup
		for {
			conn, err := reconnectingPTYListener.Accept()
			if err != nil {
				if !a.isClosed() {
					logger.Debug(ctx, "accept pty failed", slog.Error(err))
				}
				break
			}
			clog := logger.With(
				slog.F("remote", conn.RemoteAddr().String()),
				slog.F("local", conn.LocalAddr().String()))
			clog.Info(ctx, "accepted conn")
			wg.Add(1)
			closed := make(chan struct{})
			go func() {
				select {
				case <-closed:
				case <-a.closed:
					_ = conn.Close()
				}
				wg.Done()
			}()
			go func() {
				defer close(closed)
				// This cannot use a JSON decoder, since that can
				// buffer additional data that is required for the PTY.
				rawLen := make([]byte, 2)
				_, err = conn.Read(rawLen)
				if err != nil {
					return
				}
				length := binary.LittleEndian.Uint16(rawLen)
				data := make([]byte, length)
				_, err = conn.Read(data)
				if err != nil {
					return
				}
				var msg codersdk.WorkspaceAgentReconnectingPTYInit
				err = json.Unmarshal(data, &msg)
				if err != nil {
					logger.Warn(ctx, "failed to unmarshal init", slog.F("raw", data))
					return
				}
				_ = a.handleReconnectingPTY(ctx, clog, msg, conn)
			}()
		}
		wg.Wait()
	}); err != nil {
		return nil, err
	}

	speedtestListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.WorkspaceAgentSpeedtestPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for speedtest: %w", err)
	}
	defer func() {
		if err != nil {
			_ = speedtestListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		var wg sync.WaitGroup
		for {
			conn, err := speedtestListener.Accept()
			if err != nil {
				if !a.isClosed() {
					a.logger.Debug(ctx, "speedtest listener failed", slog.Error(err))
				}
				break
			}
			clog := a.logger.Named("speedtest").With(
				slog.F("remote", conn.RemoteAddr().String()),
				slog.F("local", conn.LocalAddr().String()))
			clog.Info(ctx, "accepted conn")
			wg.Add(1)
			closed := make(chan struct{})
			go func() {
				select {
				case <-closed:
				case <-a.closed:
					_ = conn.Close()
				}
				wg.Done()
			}()
			go func() {
				defer close(closed)
				sErr := speedtest.ServeConn(conn)
				if sErr != nil {
					clog.Error(ctx, "test ended with error", slog.Error(sErr))
					return
				}
				clog.Info(ctx, "test ended")
			}()
		}
		wg.Wait()
	}); err != nil {
		return nil, err
	}

	apiListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.WorkspaceAgentHTTPAPIServerPort))
	if err != nil {
		return nil, xerrors.Errorf("api listener: %w", err)
	}
	defer func() {
		if err != nil {
			_ = apiListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		defer apiListener.Close()
		server := &http.Server{
			Handler:           a.apiHandler(),
			ReadTimeout:       20 * time.Second,
			ReadHeaderTimeout: 20 * time.Second,
			WriteTimeout:      20 * time.Second,
			ErrorLog:          slog.Stdlib(ctx, a.logger.Named("http_api_server"), slog.LevelInfo),
		}
		go func() {
			select {
			case <-ctx.Done():
			case <-a.closed:
			}
			_ = server.Close()
		}()

		err := server.Serve(apiListener)
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) && !strings.Contains(err.Error(), "use of closed network connection") {
			a.logger.Critical(ctx, "serve HTTP API server", slog.Error(err))
		}
	}); err != nil {
		return nil, err
	}

	return network, nil
}

// runCoordinator runs a coordinator and returns whether a reconnect
// should occur.
func (a *agent) runCoordinator(ctx context.Context, network *tailnet.Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	coordinator, err := a.client.Listen(ctx)
	if err != nil {
		return err
	}
	defer coordinator.Close()
	a.logger.Info(ctx, "connected to coordination endpoint")
	sendNodes, errChan := tailnet.ServeCoordinator(coordinator, func(nodes []*tailnet.Node) error {
		return network.UpdateNodes(nodes, false)
	})
	network.SetNodeCallback(sendNodes)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

// runDERPMapSubscriber runs a coordinator and returns if a reconnect should occur.
func (a *agent) runDERPMapSubscriber(ctx context.Context, network *tailnet.Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	updates, closer, err := a.client.DERPMapUpdates(ctx)
	if err != nil {
		return err
	}
	defer closer.Close()

	a.logger.Info(ctx, "connected to derp map endpoint")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			if update.Err != nil {
				return update.Err
			}
			if update.DERPMap != nil && !tailnet.CompareDERPMaps(network.DERPMap(), update.DERPMap) {
				a.logger.Info(ctx, "updating derp map due to detected changes")
				network.SetDERPMap(update.DERPMap)
			}
		}
	}
}

func (a *agent) handleReconnectingPTY(ctx context.Context, logger slog.Logger, msg codersdk.WorkspaceAgentReconnectingPTYInit, conn net.Conn) (retErr error) {
	defer conn.Close()
	a.metrics.connectionsTotal.Add(1)

	a.connCountReconnectingPTY.Add(1)
	defer a.connCountReconnectingPTY.Add(-1)

	connectionID := uuid.NewString()
	connLogger := logger.With(slog.F("message_id", msg.ID), slog.F("connection_id", connectionID))
	connLogger.Debug(ctx, "starting handler")

	defer func() {
		if err := retErr; err != nil {
			a.closeMutex.Lock()
			closed := a.isClosed()
			a.closeMutex.Unlock()

			// If the agent is closed, we don't want to
			// log this as an error since it's expected.
			if closed {
				connLogger.Info(ctx, "reconnecting pty failed with attach error (agent closed)", slog.Error(err))
			} else {
				connLogger.Error(ctx, "reconnecting pty failed with attach error", slog.Error(err))
			}
		}
		connLogger.Info(ctx, "reconnecting pty connection closed")
	}()

	var rpty reconnectingpty.ReconnectingPTY
	sendConnected := make(chan reconnectingpty.ReconnectingPTY, 1)
	// On store, reserve this ID to prevent multiple concurrent new connections.
	waitReady, ok := a.reconnectingPTYs.LoadOrStore(msg.ID, sendConnected)
	if ok {
		close(sendConnected) // Unused.
		connLogger.Debug(ctx, "connecting to existing reconnecting pty")
		c, ok := waitReady.(chan reconnectingpty.ReconnectingPTY)
		if !ok {
			return xerrors.Errorf("found invalid type in reconnecting pty map: %T", waitReady)
		}
		rpty, ok = <-c
		if !ok || rpty == nil {
			return xerrors.Errorf("reconnecting pty closed before connection")
		}
		c <- rpty // Put it back for the next reconnect.
	} else {
		connLogger.Debug(ctx, "creating new reconnecting pty")

		connected := false
		defer func() {
			if !connected && retErr != nil {
				a.reconnectingPTYs.Delete(msg.ID)
				close(sendConnected)
			}
		}()

		// Empty command will default to the users shell!
		cmd, err := a.sshServer.CreateCommand(ctx, msg.Command, nil)
		if err != nil {
			a.metrics.reconnectingPTYErrors.WithLabelValues("create_command").Add(1)
			return xerrors.Errorf("create command: %w", err)
		}

		rpty = reconnectingpty.New(ctx, cmd, &reconnectingpty.Options{
			Timeout: a.reconnectingPTYTimeout,
			Metrics: a.metrics.reconnectingPTYErrors,
		}, logger.With(slog.F("message_id", msg.ID)))

		if err = a.trackConnGoroutine(func() {
			rpty.Wait()
			a.reconnectingPTYs.Delete(msg.ID)
		}); err != nil {
			rpty.Close(err)
			return xerrors.Errorf("start routine: %w", err)
		}

		connected = true
		sendConnected <- rpty
	}
	return rpty.Attach(ctx, connectionID, conn, msg.Height, msg.Width, connLogger)
}

// startReportingConnectionStats runs the connection stats reporting goroutine.
func (a *agent) startReportingConnectionStats(ctx context.Context) {
	reportStats := func(networkStats map[netlogtype.Connection]netlogtype.Counts) {
		stats := &agentsdk.Stats{
			ConnectionCount:    int64(len(networkStats)),
			ConnectionsByProto: map[string]int64{},
		}
		for conn, counts := range networkStats {
			stats.ConnectionsByProto[conn.Proto.String()]++
			stats.RxBytes += int64(counts.RxBytes)
			stats.RxPackets += int64(counts.RxPackets)
			stats.TxBytes += int64(counts.TxBytes)
			stats.TxPackets += int64(counts.TxPackets)
		}

		// The count of active sessions.
		sshStats := a.sshServer.ConnStats()
		stats.SessionCountSSH = sshStats.Sessions
		stats.SessionCountVSCode = sshStats.VSCode
		stats.SessionCountJetBrains = sshStats.JetBrains

		stats.SessionCountReconnectingPTY = a.connCountReconnectingPTY.Load()

		// Compute the median connection latency!
		var wg sync.WaitGroup
		var mu sync.Mutex
		status := a.network.Status()
		durations := []float64{}
		pingCtx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFunc()
		for nodeID, peer := range status.Peer {
			if !peer.Active {
				continue
			}
			addresses, found := a.network.NodeAddresses(nodeID)
			if !found {
				continue
			}
			if len(addresses) == 0 {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				duration, _, _, err := a.network.Ping(pingCtx, addresses[0].Addr())
				if err != nil {
					return
				}
				mu.Lock()
				durations = append(durations, float64(duration.Microseconds()))
				mu.Unlock()
			}()
		}
		wg.Wait()
		sort.Float64s(durations)
		durationsLength := len(durations)
		if durationsLength == 0 {
			stats.ConnectionMedianLatencyMS = -1
		} else if durationsLength%2 == 0 {
			stats.ConnectionMedianLatencyMS = (durations[durationsLength/2-1] + durations[durationsLength/2]) / 2
		} else {
			stats.ConnectionMedianLatencyMS = durations[durationsLength/2]
		}
		// Convert from microseconds to milliseconds.
		stats.ConnectionMedianLatencyMS /= 1000

		// Collect agent metrics.
		// Agent metrics are changing all the time, so there is no need to perform
		// reflect.DeepEqual to see if stats should be transferred.

		metricsCtx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFunc()
		stats.Metrics = a.collectMetrics(metricsCtx)

		a.latestStat.Store(stats)

		select {
		case a.connStatsChan <- stats:
		case <-a.closed:
		}
	}

	// Report statistics from the created network.
	cl, err := a.client.ReportStats(ctx, a.logger, a.connStatsChan, func(d time.Duration) {
		a.network.SetConnStatsCallback(d, 2048,
			func(_, _ time.Time, virtual, _ map[netlogtype.Connection]netlogtype.Counts) {
				reportStats(virtual)
			},
		)
	})
	if err != nil {
		a.logger.Error(ctx, "agent failed to report stats", slog.Error(err))
	} else {
		if err = a.trackConnGoroutine(func() {
			// This is OK because the agent never re-creates the tailnet
			// and the only shutdown indicator is agent.Close().
			<-a.closed
			_ = cl.Close()
		}); err != nil {
			a.logger.Debug(ctx, "report stats goroutine", slog.Error(err))
			_ = cl.Close()
		}
	}
}

var prioritizedProcs = []string{"coder agent"}

func (a *agent) manageProcessPriorityLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			a.logger.Critical(ctx, "recovered from panic",
				slog.F("panic", r),
				slog.F("stack", string(debug.Stack())),
			)
		}
	}()

	if val := a.envVars[EnvProcPrioMgmt]; val == "" || runtime.GOOS != "linux" {
		a.logger.Debug(ctx, "process priority not enabled, agent will not manage process niceness/oom_score_adj ",
			slog.F("env_var", EnvProcPrioMgmt),
			slog.F("value", val),
			slog.F("goos", runtime.GOOS),
		)
		return
	}

	if a.processManagementTick == nil {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		a.processManagementTick = ticker.C
	}

	for {
		procs, err := a.manageProcessPriority(ctx)
		if err != nil {
			a.logger.Error(ctx, "manage process priority",
				slog.Error(err),
			)
		}
		if a.modifiedProcs != nil {
			a.modifiedProcs <- procs
		}

		select {
		case <-a.processManagementTick:
		case <-ctx.Done():
			return
		}
	}
}

func (a *agent) manageProcessPriority(ctx context.Context) ([]*agentproc.Process, error) {
	const (
		niceness = 10
	)

	procs, err := agentproc.List(a.filesystem, a.syscaller)
	if err != nil {
		return nil, xerrors.Errorf("list: %w", err)
	}

	var (
		modProcs = []*agentproc.Process{}
		logger   slog.Logger
	)

	for _, proc := range procs {
		logger = a.logger.With(
			slog.F("cmd", proc.Cmd()),
			slog.F("pid", proc.PID),
		)

		containsFn := func(e string) bool {
			contains := strings.Contains(proc.Cmd(), e)
			return contains
		}

		// If the process is prioritized we should adjust
		// it's oom_score_adj and avoid lowering its niceness.
		if slices.ContainsFunc[[]string, string](prioritizedProcs, containsFn) {
			continue
		}

		score, err := proc.Niceness(a.syscaller)
		if err != nil {
			logger.Warn(ctx, "unable to get proc niceness",
				slog.Error(err),
			)
			continue
		}

		// We only want processes that don't have a nice value set
		// so we don't override user nice values.
		// Getpriority actually returns priority for the nice value
		// which is niceness + 20, so here 20 = a niceness of 0 (aka unset).
		if score != 20 {
			// We don't log here since it can get spammy
			continue
		}

		err = proc.SetNiceness(a.syscaller, niceness)
		if err != nil {
			logger.Warn(ctx, "unable to set proc niceness",
				slog.F("niceness", niceness),
				slog.Error(err),
			)
			continue
		}

		modProcs = append(modProcs, proc)
	}
	return modProcs, nil
}

// isClosed returns whether the API is closed or not.
func (a *agent) isClosed() bool {
	select {
	case <-a.closed:
		return true
	default:
		return false
	}
}

func (a *agent) HTTPDebug() http.Handler {
	r := chi.NewRouter()

	requireNetwork := func(w http.ResponseWriter) (*tailnet.Conn, bool) {
		a.closeMutex.Lock()
		network := a.network
		a.closeMutex.Unlock()

		if network == nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("network is not ready yet"))
			return nil, false
		}

		return network, true
	}

	r.Get("/debug/magicsock", func(w http.ResponseWriter, r *http.Request) {
		network, ok := requireNetwork(w)
		if !ok {
			return
		}
		network.MagicsockServeHTTPDebug(w, r)
	})

	r.Get("/debug/magicsock/debug-logging/{state}", func(w http.ResponseWriter, r *http.Request) {
		state := chi.URLParam(r, "state")
		stateBool, err := strconv.ParseBool(state)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, "invalid state %q, must be a boolean", state)
			return
		}

		network, ok := requireNetwork(w)
		if !ok {
			return
		}

		network.MagicsockSetDebugLoggingEnabled(stateBool)
		a.logger.Info(r.Context(), "updated magicsock debug logging due to debug request", slog.F("new_state", stateBool))

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "updated magicsock debug logging to %v", stateBool)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("404 not found"))
	})

	return r
}

func (a *agent) Close() error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.isClosed() {
		return nil
	}

	ctx := context.Background()
	a.logger.Info(ctx, "shutting down agent")
	a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleShuttingDown)

	// Attempt to gracefully shut down all active SSH connections and
	// stop accepting new ones.
	err := a.sshServer.Shutdown(ctx)
	if err != nil {
		a.logger.Error(ctx, "ssh server shutdown", slog.Error(err))
	}

	lifecycleState := codersdk.WorkspaceAgentLifecycleOff
	err = a.scriptRunner.Execute(ctx, func(script codersdk.WorkspaceAgentScript) bool {
		return script.RunOnStop
	})
	if err != nil {
		if errors.Is(err, agentscripts.ErrTimeout) {
			lifecycleState = codersdk.WorkspaceAgentLifecycleShutdownTimeout
		} else {
			lifecycleState = codersdk.WorkspaceAgentLifecycleShutdownError
		}
	}
	a.setLifecycle(ctx, lifecycleState)

	err = a.scriptRunner.Close()
	if err != nil {
		a.logger.Error(ctx, "script runner close", slog.Error(err))
	}

	// Wait for the lifecycle to be reported, but don't wait forever so
	// that we don't break user expectations.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
lifecycleWaitLoop:
	for {
		select {
		case <-ctx.Done():
			break lifecycleWaitLoop
		case s := <-a.lifecycleReported:
			if s == lifecycleState {
				break lifecycleWaitLoop
			}
		}
	}

	close(a.closed)
	a.closeCancel()
	_ = a.sshServer.Close()
	if a.network != nil {
		_ = a.network.Close()
	}
	a.connCloseWait.Wait()

	return nil
}

// userHomeDir returns the home directory of the current user, giving
// priority to the $HOME environment variable.
func userHomeDir() (string, error) {
	// First we check the environment.
	homedir, err := os.UserHomeDir()
	if err == nil {
		return homedir, nil
	}

	// As a fallback, we try the user information.
	u, err := user.Current()
	if err != nil {
		return "", xerrors.Errorf("current user: %w", err)
	}
	return u.HomeDir, nil
}

// expandDirectory converts a directory path to an absolute path.
// It primarily resolves the home directory and any environment
// variables that may be set
func expandDirectory(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	if dir[0] == '~' {
		home, err := userHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, dir[1:])
	}
	dir = os.ExpandEnv(dir)

	if !filepath.IsAbs(dir) {
		home, err := userHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, dir)
	}
	return dir, nil
}

// EnvAgentSubsystem is the environment variable used to denote the
// specialized environment in which the agent is running
// (e.g. envbox, envbuilder).
const EnvAgentSubsystem = "CODER_AGENT_SUBSYSTEM"
