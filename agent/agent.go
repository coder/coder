package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/armon/circbuf"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"go.uber.org/atomic"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netlogtype"

	"cdr.dev/slog"
	"github.com/coder/coder/agent/agentssh"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/pty"
	"github.com/coder/coder/tailnet"
	"github.com/coder/retry"
)

const (
	ProtocolReconnectingPTY = "reconnecting-pty"
	ProtocolSSH             = "ssh"
	ProtocolDial            = "dial"
)

type Options struct {
	Filesystem             afero.Fs
	LogDir                 string
	TempDir                string
	ExchangeToken          func(ctx context.Context) (string, error)
	Client                 Client
	ReconnectingPTYTimeout time.Duration
	EnvironmentVariables   map[string]string
	Logger                 slog.Logger
	IgnorePorts            map[int]string
	SSHMaxTimeout          time.Duration
	TailnetListenPort      uint16
	Subsystem              codersdk.AgentSubsystem
	Addresses              []netip.Prefix

	PrometheusRegistry *prometheus.Registry
}

type Client interface {
	Manifest(ctx context.Context) (agentsdk.Manifest, error)
	Listen(ctx context.Context) (net.Conn, error)
	ReportStats(ctx context.Context, log slog.Logger, statsChan <-chan *agentsdk.Stats, setInterval func(time.Duration)) (io.Closer, error)
	PostLifecycle(ctx context.Context, state agentsdk.PostLifecycleRequest) error
	PostAppHealth(ctx context.Context, req agentsdk.PostAppHealthsRequest) error
	PostStartup(ctx context.Context, req agentsdk.PostStartupRequest) error
	PostMetadata(ctx context.Context, key string, req agentsdk.PostMetadataRequest) error
	PatchStartupLogs(ctx context.Context, req agentsdk.PatchStartupLogs) error
	GetServiceBanner(ctx context.Context) (codersdk.ServiceBannerConfig, error)
}

type Agent interface {
	HTTPDebug() http.Handler
	io.Closer
}

func New(options Options) Agent {
	if options.ReconnectingPTYTimeout == 0 {
		options.ReconnectingPTYTimeout = 5 * time.Minute
	}
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

	prometheusRegistry := options.PrometheusRegistry
	if prometheusRegistry == nil {
		prometheusRegistry = prometheus.NewRegistry()
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	a := &agent{
		tailnetListenPort:      options.TailnetListenPort,
		reconnectingPTYTimeout: options.ReconnectingPTYTimeout,
		logger:                 options.Logger,
		closeCancel:            cancelFunc,
		closed:                 make(chan struct{}),
		envVars:                options.EnvironmentVariables,
		client:                 options.Client,
		exchangeToken:          options.ExchangeToken,
		filesystem:             options.Filesystem,
		logDir:                 options.LogDir,
		tempDir:                options.TempDir,
		lifecycleUpdate:        make(chan struct{}, 1),
		lifecycleReported:      make(chan codersdk.WorkspaceAgentLifecycle, 1),
		lifecycleStates:        []agentsdk.PostLifecycleRequest{{State: codersdk.WorkspaceAgentLifecycleCreated}},
		ignorePorts:            options.IgnorePorts,
		connStatsChan:          make(chan *agentsdk.Stats, 1),
		sshMaxTimeout:          options.SSHMaxTimeout,
		subsystem:              options.Subsystem,
		addresses:              options.Addresses,

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
	subsystem   codersdk.AgentSubsystem

	reconnectingPTYs       sync.Map
	reconnectingPTYTimeout time.Duration

	connCloseWait sync.WaitGroup
	closeCancel   context.CancelFunc
	closeMutex    sync.Mutex
	closed        chan struct{}

	envVars map[string]string
	// manifest is atomic because values can change after reconnection.
	manifest atomic.Pointer[agentsdk.Manifest]
	// serviceBanner is atomic because it can change.
	serviceBanner atomic.Pointer[codersdk.ServiceBannerConfig]
	sessionToken  atomic.Pointer[string]
	sshServer     *agentssh.Server
	sshMaxTimeout time.Duration

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

	for retrier := retry.New(100*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		a.logger.Info(ctx, "connecting to coderd")
		err := a.run(ctx)
		// Cancel after the run is complete to clean up any leaked resources!
		if err == nil {
			continue
		}
		if errors.Is(err, context.Canceled) {
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

func (a *agent) collectMetadata(ctx context.Context, md codersdk.WorkspaceAgentMetadataDescription) *codersdk.WorkspaceAgentMetadataResult {
	var out bytes.Buffer
	result := &codersdk.WorkspaceAgentMetadataResult{
		// CollectedAt is set here for testing purposes and overrode by
		// coderd to the time of server receipt to solve clock skew.
		//
		// In the future, the server may accept the timestamp from the agent
		// if it can guarantee the clocks are synchronized.
		CollectedAt: time.Now(),
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

// adjustIntervalForTests returns a duration of testInterval milliseconds long
// for tests and interval seconds long otherwise.
func adjustIntervalForTests(interval time.Duration, testInterval time.Duration) time.Duration {
	// In tests we want to set shorter intervals because engineers are
	// impatient.
	if flag.Lookup("test.v") != nil {
		return testInterval
	}
	return interval
}

type metadataResultAndKey struct {
	result *codersdk.WorkspaceAgentMetadataResult
	key    string
}

type trySingleflight struct {
	m sync.Map
}

func (t *trySingleflight) Do(key string, fn func()) {
	_, loaded := t.m.LoadOrStore(key, struct{}{})
	if !loaded {
		// There is already a goroutine running for this key.
		return
	}

	defer t.m.Delete(key)
	fn()
}

func (a *agent) reportMetadataLoop(ctx context.Context) {
	baseInterval := adjustIntervalForTests(time.Second, time.Millisecond*100)

	const metadataLimit = 128

	var (
		baseTicker       = time.NewTicker(baseInterval)
		lastCollectedAts = make(map[string]time.Time)
		metadataResults  = make(chan metadataResultAndKey, metadataLimit)
	)
	defer baseTicker.Stop()

	// We use a custom singleflight that immediately returns if there is already
	// a goroutine running for a given key. This is to prevent a build-up of
	// goroutines waiting on Do when the script takes many multiples of
	// baseInterval to run.
	var flight trySingleflight

	for {
		select {
		case <-ctx.Done():
			return
		case mr := <-metadataResults:
			lastCollectedAts[mr.key] = mr.result.CollectedAt
			err := a.client.PostMetadata(ctx, mr.key, *mr.result)
			if err != nil {
				a.logger.Error(ctx, "agent failed to report metadata", slog.Error(err))
			}
		case <-baseTicker.C:
		}

		if len(metadataResults) > 0 {
			// The inner collection loop expects the channel is empty before spinning up
			// all the collection goroutines.
			a.logger.Debug(
				ctx, "metadata collection backpressured",
				slog.F("queue_len", len(metadataResults)),
			)
			continue
		}

		manifest := a.manifest.Load()
		if manifest == nil {
			continue
		}

		if len(manifest.Metadata) > metadataLimit {
			a.logger.Error(
				ctx, "metadata limit exceeded",
				slog.F("limit", metadataLimit), slog.F("got", len(manifest.Metadata)),
			)
			continue
		}

		// If the manifest changes (e.g. on agent reconnect) we need to
		// purge old cache values to prevent lastCollectedAt from growing
		// boundlessly.
		for key := range lastCollectedAts {
			if slices.IndexFunc(manifest.Metadata, func(md codersdk.WorkspaceAgentMetadataDescription) bool {
				return md.Key == key
			}) < 0 {
				delete(lastCollectedAts, key)
			}
		}

		// Spawn a goroutine for each metadata collection, and use a
		// channel to synchronize the results and avoid both messy
		// mutex logic and overloading the API.
		for _, md := range manifest.Metadata {
			collectedAt, ok := lastCollectedAts[md.Key]
			if ok {
				// If the interval is zero, we assume the user just wants
				// a single collection at startup, not a spinning loop.
				if md.Interval == 0 {
					continue
				}
				// The last collected value isn't quite stale yet, so we skip it.
				if collectedAt.Add(
					adjustIntervalForTests(
						time.Duration(md.Interval)*time.Second,
						time.Duration(md.Interval)*time.Millisecond*100),
				).After(time.Now()) {
					continue
				}
			}

			md := md
			// We send the result to the channel in the goroutine to avoid
			// sending the same result multiple times. So, we don't care about
			// the return values.
			go flight.Do(md.Key, func() {
				timeout := md.Timeout
				if timeout == 0 {
					timeout = md.Interval
				}
				ctx, cancel := context.WithTimeout(ctx,
					time.Duration(timeout)*time.Second,
				)
				defer cancel()

				select {
				case <-ctx.Done():
				case metadataResults <- metadataResultAndKey{
					key:    md.Key,
					result: a.collectMetadata(ctx, md),
				}:
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
		ChangedAt: database.Now(),
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
	ticker := time.NewTicker(adjustIntervalForTests(2*time.Minute, time.Millisecond*5))
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
		Subsystem:         a.subsystem,
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

		lifecycleState := codersdk.WorkspaceAgentLifecycleReady
		scriptDone := make(chan error, 1)
		err = a.trackConnGoroutine(func() {
			defer close(scriptDone)
			scriptDone <- a.runStartupScript(ctx, manifest.StartupScript)
		})
		if err != nil {
			return xerrors.Errorf("track startup script: %w", err)
		}
		go func() {
			var timeout <-chan time.Time
			// If timeout is zero, an older version of the coder
			// provider was used. Otherwise a timeout is always > 0.
			if manifest.StartupScriptTimeout > 0 {
				t := time.NewTimer(manifest.StartupScriptTimeout)
				defer t.Stop()
				timeout = t.C
			}

			var err error
			select {
			case err = <-scriptDone:
			case <-timeout:
				a.logger.Warn(ctx, "script timed out", slog.F("lifecycle", "startup"), slog.F("timeout", manifest.ShutdownScriptTimeout))
				a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleStartTimeout)
				err = <-scriptDone // The script can still complete after a timeout.
			}
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				lifecycleState = codersdk.WorkspaceAgentLifecycleStartError
			}
			a.setLifecycle(ctx, lifecycleState)
		}()
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
		network, err = a.createTailnet(ctx, manifest.AgentID, manifest.DERPMap, manifest.DisableDirectConnections)
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
		// Update the DERP map and allow/disallow direct connections.
		network.SetDERPMap(manifest.DERPMap)
		network.SetBlockEndpoints(manifest.DisableDirectConnections)
	}

	a.logger.Debug(ctx, "running tailnet connection coordinator")
	err = a.runCoordinator(ctx, network)
	if err != nil {
		return xerrors.Errorf("run coordinator: %w", err)
	}
	return nil
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

func (a *agent) createTailnet(ctx context.Context, agentID uuid.UUID, derpMap *tailcfg.DERPMap, disableDirectConnections bool) (_ *tailnet.Conn, err error) {
	network, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      a.wireguardAddresses(agentID),
		DERPMap:        derpMap,
		Logger:         a.logger.Named("tailnet"),
		ListenPort:     a.tailnetListenPort,
		BlockEndpoints: disableDirectConnections,
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
			logger.Debug(ctx, "accepted conn", slog.F("remote", conn.RemoteAddr().String()))
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
				_ = a.handleReconnectingPTY(ctx, logger, msg, conn)
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
				_ = speedtest.ServeConn(conn)
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

func (a *agent) runStartupScript(ctx context.Context, script string) error {
	return a.runScript(ctx, "startup", script)
}

func (a *agent) runShutdownScript(ctx context.Context, script string) error {
	return a.runScript(ctx, "shutdown", script)
}

func (a *agent) runScript(ctx context.Context, lifecycle, script string) (err error) {
	if script == "" {
		return nil
	}

	logger := a.logger.With(slog.F("lifecycle", lifecycle))

	logger.Info(ctx, fmt.Sprintf("running %s script", lifecycle), slog.F("script", script))
	fileWriter, err := a.filesystem.OpenFile(filepath.Join(a.logDir, fmt.Sprintf("coder-%s-script.log", lifecycle)), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return xerrors.Errorf("open %s script log file: %w", lifecycle, err)
	}
	defer func() {
		err := fileWriter.Close()
		if err != nil {
			logger.Warn(ctx, fmt.Sprintf("close %s script log file", lifecycle), slog.Error(err))
		}
	}()

	cmdPty, err := a.sshServer.CreateCommand(ctx, script, nil)
	if err != nil {
		return xerrors.Errorf("%s script: create command: %w", lifecycle, err)
	}
	cmd := cmdPty.AsExec()

	var stdout, stderr io.Writer = fileWriter, fileWriter
	if lifecycle == "startup" {
		send, flushAndClose := agentsdk.StartupLogsSender(a.client.PatchStartupLogs, logger)
		// If ctx is canceled here (or in a writer below), we may be
		// discarding logs, but that's okay because we're shutting down
		// anyway. We could consider creating a new context here if we
		// want better control over flush during shutdown.
		defer func() {
			if err := flushAndClose(ctx); err != nil {
				logger.Warn(ctx, "flush startup logs failed", slog.Error(err))
			}
		}()

		infoW := agentsdk.StartupLogsWriter(ctx, send, codersdk.LogLevelInfo)
		defer infoW.Close()
		errW := agentsdk.StartupLogsWriter(ctx, send, codersdk.LogLevelError)
		defer errW.Close()

		stdout = io.MultiWriter(fileWriter, infoW)
		stderr = io.MultiWriter(fileWriter, errW)
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	defer func() {
		end := time.Now()
		execTime := end.Sub(start)
		exitCode := 0
		if err != nil {
			exitCode = 255 // Unknown status.
			var exitError *exec.ExitError
			if xerrors.As(err, &exitError) {
				exitCode = exitError.ExitCode()
			}
			logger.Warn(ctx, fmt.Sprintf("%s script failed", lifecycle), slog.F("execution_time", execTime), slog.F("exit_code", exitCode), slog.Error(err))
		} else {
			logger.Info(ctx, fmt.Sprintf("%s script completed", lifecycle), slog.F("execution_time", execTime), slog.F("exit_code", exitCode))
		}
	}()

	err = cmd.Run()
	if err != nil {
		// cmd.Run does not return a context canceled error, it returns "signal: killed".
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return xerrors.Errorf("%s script: run: %w", lifecycle, err)
	}
	return nil
}

func (a *agent) handleReconnectingPTY(ctx context.Context, logger slog.Logger, msg codersdk.WorkspaceAgentReconnectingPTYInit, conn net.Conn) (retErr error) {
	defer conn.Close()
	a.metrics.connectionsTotal.Add(1)

	a.connCountReconnectingPTY.Add(1)
	defer a.connCountReconnectingPTY.Add(-1)

	connectionID := uuid.NewString()
	logger = logger.With(slog.F("message_id", msg.ID), slog.F("connection_id", connectionID))
	logger.Debug(ctx, "starting handler")

	defer func() {
		if err := retErr; err != nil {
			a.closeMutex.Lock()
			closed := a.isClosed()
			a.closeMutex.Unlock()

			// If the agent is closed, we don't want to
			// log this as an error since it's expected.
			if closed {
				logger.Debug(ctx, "reconnecting PTY failed with session error (agent closed)", slog.Error(err))
			} else {
				logger.Error(ctx, "reconnecting PTY failed with session error", slog.Error(err))
			}
		}
		logger.Debug(ctx, "session closed")
	}()

	var rpty *reconnectingPTY
	sendConnected := make(chan *reconnectingPTY, 1)
	// On store, reserve this ID to prevent multiple concurrent new connections.
	waitReady, ok := a.reconnectingPTYs.LoadOrStore(msg.ID, sendConnected)
	if ok {
		close(sendConnected) // Unused.
		logger.Debug(ctx, "connecting to existing session")
		c, ok := waitReady.(chan *reconnectingPTY)
		if !ok {
			return xerrors.Errorf("found invalid type in reconnecting pty map: %T", waitReady)
		}
		rpty, ok = <-c
		if !ok || rpty == nil {
			return xerrors.Errorf("reconnecting pty closed before connection")
		}
		c <- rpty // Put it back for the next reconnect.
	} else {
		logger.Debug(ctx, "creating new session")

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
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")

		// Default to buffer 64KiB.
		circularBuffer, err := circbuf.NewBuffer(64 << 10)
		if err != nil {
			return xerrors.Errorf("create circular buffer: %w", err)
		}

		ptty, process, err := pty.Start(cmd)
		if err != nil {
			a.metrics.reconnectingPTYErrors.WithLabelValues("start_command").Add(1)
			return xerrors.Errorf("start command: %w", err)
		}

		ctx, cancel := context.WithCancel(ctx)
		rpty = &reconnectingPTY{
			activeConns: map[string]net.Conn{
				// We have to put the connection in the map instantly otherwise
				// the connection won't be closed if the process instantly dies.
				connectionID: conn,
			},
			ptty: ptty,
			// Timeouts created with an after func can be reset!
			timeout:        time.AfterFunc(a.reconnectingPTYTimeout, cancel),
			circularBuffer: circularBuffer,
		}
		// We don't need to separately monitor for the process exiting.
		// When it exits, our ptty.OutputReader() will return EOF after
		// reading all process output.
		if err = a.trackConnGoroutine(func() {
			buffer := make([]byte, 1024)
			for {
				read, err := rpty.ptty.OutputReader().Read(buffer)
				if err != nil {
					// When the PTY is closed, this is triggered.
					// Error is typically a benign EOF, so only log for debugging.
					if errors.Is(err, io.EOF) {
						logger.Debug(ctx, "unable to read pty output, command might have exited", slog.Error(err))
					} else {
						logger.Warn(ctx, "unable to read pty output, command might have exited", slog.Error(err))
						a.metrics.reconnectingPTYErrors.WithLabelValues("output_reader").Add(1)
					}
					break
				}
				part := buffer[:read]
				rpty.circularBufferMutex.Lock()
				_, err = rpty.circularBuffer.Write(part)
				rpty.circularBufferMutex.Unlock()
				if err != nil {
					logger.Error(ctx, "write to circular buffer", slog.Error(err))
					break
				}
				rpty.activeConnsMutex.Lock()
				for cid, conn := range rpty.activeConns {
					_, err = conn.Write(part)
					if err != nil {
						logger.Warn(ctx,
							"error writing to active conn",
							slog.F("other_conn_id", cid),
							slog.Error(err),
						)
						a.metrics.reconnectingPTYErrors.WithLabelValues("write").Add(1)
					}
				}
				rpty.activeConnsMutex.Unlock()
			}

			// Cleanup the process, PTY, and delete it's
			// ID from memory.
			_ = process.Kill()
			rpty.Close()
			a.reconnectingPTYs.Delete(msg.ID)
		}); err != nil {
			_ = process.Kill()
			_ = ptty.Close()
			return xerrors.Errorf("start routine: %w", err)
		}
		connected = true
		sendConnected <- rpty
	}
	// Resize the PTY to initial height + width.
	err := rpty.ptty.Resize(msg.Height, msg.Width)
	if err != nil {
		// We can continue after this, it's not fatal!
		logger.Error(ctx, "reconnecting PTY initial resize failed, but will continue", slog.Error(err))
		a.metrics.reconnectingPTYErrors.WithLabelValues("resize").Add(1)
	}
	// Write any previously stored data for the TTY.
	rpty.circularBufferMutex.RLock()
	prevBuf := slices.Clone(rpty.circularBuffer.Bytes())
	rpty.circularBufferMutex.RUnlock()
	// Note that there is a small race here between writing buffered
	// data and storing conn in activeConns. This is likely a very minor
	// edge case, but we should look into ways to avoid it. Holding
	// activeConnsMutex would be one option, but holding this mutex
	// while also holding circularBufferMutex seems dangerous.
	_, err = conn.Write(prevBuf)
	if err != nil {
		a.metrics.reconnectingPTYErrors.WithLabelValues("write").Add(1)
		return xerrors.Errorf("write buffer to conn: %w", err)
	}
	// Multiple connections to the same TTY are permitted.
	// This could easily be used for terminal sharing, but
	// we do it because it's a nice user experience to
	// copy/paste a terminal URL and have it _just work_.
	rpty.activeConnsMutex.Lock()
	rpty.activeConns[connectionID] = conn
	rpty.activeConnsMutex.Unlock()
	// Resetting this timeout prevents the PTY from exiting.
	rpty.timeout.Reset(a.reconnectingPTYTimeout)

	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	heartbeat := time.NewTicker(a.reconnectingPTYTimeout / 2)
	defer heartbeat.Stop()
	go func() {
		// Keep updating the activity while this
		// connection is alive!
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
			}
			rpty.timeout.Reset(a.reconnectingPTYTimeout)
		}
	}()
	defer func() {
		// After this connection ends, remove it from
		// the PTYs active connections. If it isn't
		// removed, all PTY data will be sent to it.
		rpty.activeConnsMutex.Lock()
		delete(rpty.activeConns, connectionID)
		rpty.activeConnsMutex.Unlock()
	}()
	decoder := json.NewDecoder(conn)
	var req codersdk.ReconnectingPTYRequest
	for {
		err = decoder.Decode(&req)
		if xerrors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			logger.Warn(ctx, "reconnecting PTY failed with read error", slog.Error(err))
			return nil
		}
		_, err = rpty.ptty.InputWriter().Write([]byte(req.Data))
		if err != nil {
			logger.Warn(ctx, "reconnecting PTY failed with write error", slog.Error(err))
			a.metrics.reconnectingPTYErrors.WithLabelValues("input_writer").Add(1)
			return nil
		}
		// Check if a resize needs to happen!
		if req.Height == 0 || req.Width == 0 {
			continue
		}
		err = rpty.ptty.Resize(req.Height, req.Width)
		if err != nil {
			// We can continue after this, it's not fatal!
			logger.Error(ctx, "reconnecting PTY resize failed, but will continue", slog.Error(err))
			a.metrics.reconnectingPTYErrors.WithLabelValues("resize").Add(1)
		}
	}
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.closeMutex.Lock()
		network := a.network
		a.closeMutex.Unlock()

		if network == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("network is not ready yet"))
			return
		}

		if r.URL.Path == "/debug/magicsock" {
			network.MagicsockServeHTTPDebug(w, r)
		} else {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 not found"))
		}
	})
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
	if manifest := a.manifest.Load(); manifest != nil && manifest.ShutdownScript != "" {
		scriptDone := make(chan error, 1)
		go func() {
			defer close(scriptDone)
			scriptDone <- a.runShutdownScript(ctx, manifest.ShutdownScript)
		}()

		var timeout <-chan time.Time
		// If timeout is zero, an older version of the coder
		// provider was used. Otherwise a timeout is always > 0.
		if manifest.ShutdownScriptTimeout > 0 {
			t := time.NewTimer(manifest.ShutdownScriptTimeout)
			defer t.Stop()
			timeout = t.C
		}

		var err error
		select {
		case err = <-scriptDone:
		case <-timeout:
			a.logger.Warn(ctx, "script timed out", slog.F("lifecycle", "shutdown"), slog.F("timeout", manifest.ShutdownScriptTimeout))
			a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleShutdownTimeout)
			err = <-scriptDone // The script can still complete after a timeout.
		}
		if err != nil {
			lifecycleState = codersdk.WorkspaceAgentLifecycleShutdownError
		}
	}

	// Set final state and wait for it to be reported because context
	// cancellation will stop the report loop.
	a.setLifecycle(ctx, lifecycleState)

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

type reconnectingPTY struct {
	activeConnsMutex sync.Mutex
	activeConns      map[string]net.Conn

	circularBuffer      *circbuf.Buffer
	circularBufferMutex sync.RWMutex
	timeout             *time.Timer
	ptty                pty.PTYCmd
}

// Close ends all connections to the reconnecting
// PTY and clear the circular buffer.
func (r *reconnectingPTY) Close() {
	r.activeConnsMutex.Lock()
	defer r.activeConnsMutex.Unlock()
	for _, conn := range r.activeConns {
		_ = conn.Close()
	}
	_ = r.ptty.Close()
	r.circularBufferMutex.Lock()
	r.circularBuffer.Reset()
	r.circularBufferMutex.Unlock()
	r.timeout.Stop()
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
