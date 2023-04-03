package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
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
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/armon/circbuf"
	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"github.com/spf13/afero"
	"go.uber.org/atomic"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netlogtype"

	"cdr.dev/slog"
	"github.com/coder/coder/agent/usershell"
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

	// MagicSessionErrorCode indicates that something went wrong with the session, rather than the
	// command just returning a nonzero exit code, and is chosen as an arbitrary, high number
	// unlikely to shadow other exit codes, which are typically 1, 2, 3, etc.
	MagicSessionErrorCode = 229

	// MagicSSHSessionTypeEnvironmentVariable is used to track the purpose behind an SSH connection.
	// This is stripped from any commands being executed, and is counted towards connection stats.
	MagicSSHSessionTypeEnvironmentVariable = "CODER_SSH_SESSION_TYPE"
	// MagicSSHSessionTypeVSCode is set in the SSH config by the VS Code extension to identify itself.
	MagicSSHSessionTypeVSCode = "vscode"
	// MagicSSHSessionTypeJetBrains is set in the SSH config by the JetBrains extension to identify itself.
	MagicSSHSessionTypeJetBrains = "jetbrains"
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
	AgentPorts             map[int]string
	SSHMaxTimeout          time.Duration
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
}

func New(options Options) io.Closer {
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
	ctx, cancelFunc := context.WithCancel(context.Background())
	a := &agent{
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
		ignorePorts:            options.AgentPorts,
		connStatsChan:          make(chan *agentsdk.Stats, 1),
		sshMaxTimeout:          options.SSHMaxTimeout,
	}
	a.init(ctx)
	return a
}

type agent struct {
	logger        slog.Logger
	client        Client
	exchangeToken func(ctx context.Context) (string, error)
	filesystem    afero.Fs
	logDir        string
	tempDir       string
	// ignorePorts tells the api handler which ports to ignore when
	// listing all listening ports. This is helpful to hide ports that
	// are used by the agent, that the user does not care about.
	ignorePorts map[int]string

	reconnectingPTYs       sync.Map
	reconnectingPTYTimeout time.Duration

	connCloseWait sync.WaitGroup
	closeCancel   context.CancelFunc
	closeMutex    sync.Mutex
	closed        chan struct{}

	envVars map[string]string
	// manifest is atomic because values can change after reconnection.
	manifest      atomic.Pointer[agentsdk.Manifest]
	sessionToken  atomic.Pointer[string]
	sshServer     *ssh.Server
	sshMaxTimeout time.Duration

	lifecycleUpdate   chan struct{}
	lifecycleReported chan codersdk.WorkspaceAgentLifecycle
	lifecycleMu       sync.RWMutex // Protects following.
	lifecycleState    codersdk.WorkspaceAgentLifecycle

	network       *tailnet.Conn
	connStatsChan chan *agentsdk.Stats
	latestStat    atomic.Pointer[agentsdk.Stats]

	connCountVSCode          atomic.Int64
	connCountJetBrains       atomic.Int64
	connCountReconnectingPTY atomic.Int64
	connCountSSHSession      atomic.Int64
}

// runLoop attempts to start the agent in a retry loop.
// Coder may be offline temporarily, a connection issue
// may be happening, but regardless after the intermittent
// failure, you'll want the agent to reconnect.
func (a *agent) runLoop(ctx context.Context) {
	go a.reportLifecycleLoop(ctx)
	go a.reportMetadataLoop(ctx)

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
		// the server to the time the server received the result to protect
		// against clock skew.
		//
		// In the future, the server may accept the timestamp from the agent
		// if it is certain the clocks are in sync.
		CollectedAt: time.Now(),
	}
	cmd, err := a.createCommand(ctx, md.Script, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	cmd.Stdout = &out
	cmd.Stderr = &out

	// The error isn't mutually exclusive with useful output.
	err = cmd.Run()

	const bufLimit = 10 << 10
	if out.Len() > bufLimit {
		err = errors.Join(
			err,
			xerrors.Errorf("output truncated from %v to %v bytes", out.Len(), bufLimit),
		)
		out.Truncate(bufLimit)
	}

	if err != nil {
		result.Error = err.Error()
	}
	result.Value = out.String()
	return result
}

func adjustIntervalForTests(i int64) time.Duration {
	// In tests we want to set shorter intervals because engineers are
	// impatient.
	base := time.Second
	if flag.Lookup("test.v") != nil {
		base = time.Millisecond * 100
	}
	return time.Duration(i) * base
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
	baseInterval := adjustIntervalForTests(1)

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
				a.logger.Error(ctx, "report metadata", slog.Error(err))
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
					adjustIntervalForTests(md.Interval),
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

// reportLifecycleLoop reports the current lifecycle state once.
// Only the latest state is reported, intermediate states may be
// lost if the agent can't communicate with the API.
func (a *agent) reportLifecycleLoop(ctx context.Context) {
	var lastReported codersdk.WorkspaceAgentLifecycle
	for {
		select {
		case <-a.lifecycleUpdate:
		case <-ctx.Done():
			return
		}

		for r := retry.New(time.Second, 15*time.Second); r.Wait(ctx); {
			a.lifecycleMu.RLock()
			state := a.lifecycleState
			a.lifecycleMu.RUnlock()

			if state == lastReported {
				break
			}

			a.logger.Debug(ctx, "reporting lifecycle state", slog.F("state", state))

			err := a.client.PostLifecycle(ctx, agentsdk.PostLifecycleRequest{
				State: state,
			})
			if err == nil {
				lastReported = state
				select {
				case a.lifecycleReported <- state:
				case <-a.lifecycleReported:
					a.lifecycleReported <- state
				}
				break
			}
			if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
				return
			}
			// If we fail to report the state we probably shouldn't exit, log only.
			a.logger.Error(ctx, "post state", slog.Error(err))
		}
	}
}

// setLifecycle sets the lifecycle state and notifies the lifecycle loop.
// The state is only updated if it's a valid state transition.
func (a *agent) setLifecycle(ctx context.Context, state codersdk.WorkspaceAgentLifecycle) {
	a.lifecycleMu.Lock()
	lastState := a.lifecycleState
	if slices.Index(codersdk.WorkspaceAgentLifecycleOrder, lastState) > slices.Index(codersdk.WorkspaceAgentLifecycleOrder, state) {
		a.logger.Warn(ctx, "attempted to set lifecycle state to a previous state", slog.F("last", lastState), slog.F("state", state))
		a.lifecycleMu.Unlock()
		return
	}
	a.lifecycleState = state
	a.logger.Debug(ctx, "set lifecycle state", slog.F("state", state), slog.F("last", lastState))
	a.lifecycleMu.Unlock()

	select {
	case a.lifecycleUpdate <- struct{}{}:
	default:
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

	manifest, err := a.client.Manifest(ctx)
	if err != nil {
		return xerrors.Errorf("fetch metadata: %w", err)
	}
	a.logger.Info(ctx, "fetched manifest", slog.F("manifest", manifest))

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
		scriptStart := time.Now()
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
				a.logger.Warn(ctx, "startup script timed out")
				a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleStartTimeout)
				err = <-scriptDone // The script can still complete after a timeout.
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			// Only log if there was a startup script.
			if manifest.StartupScript != "" {
				execTime := time.Since(scriptStart)
				if err != nil {
					a.logger.Warn(ctx, "startup script failed", slog.F("execution_time", execTime), slog.Error(err))
					lifecycleState = codersdk.WorkspaceAgentLifecycleStartError
				} else {
					a.logger.Info(ctx, "startup script completed", slog.F("execution_time", execTime))
				}
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
		network, err = a.createTailnet(ctx, manifest.DERPMap)
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
		// Update the DERP map!
		network.SetDERPMap(manifest.DERPMap)
	}

	a.logger.Debug(ctx, "running tailnet connection coordinator")
	err = a.runCoordinator(ctx, network)
	if err != nil {
		return xerrors.Errorf("run coordinator: %w", err)
	}
	return nil
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

func (a *agent) createTailnet(ctx context.Context, derpMap *tailcfg.DERPMap) (_ *tailnet.Conn, err error) {
	network, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(codersdk.WorkspaceAgentIP, 128)},
		DERPMap:   derpMap,
		Logger:    a.logger.Named("tailnet"),
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
		var wg sync.WaitGroup
		for {
			conn, err := sshListener.Accept()
			if err != nil {
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
				a.sshServer.HandleConn(conn)
			}()
		}
		wg.Wait()
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

func (a *agent) runScript(ctx context.Context, lifecycle, script string) error {
	if script == "" {
		return nil
	}

	a.logger.Info(ctx, "running script", slog.F("lifecycle", lifecycle), slog.F("script", script))
	fileWriter, err := a.filesystem.OpenFile(filepath.Join(a.logDir, fmt.Sprintf("coder-%s-script.log", lifecycle)), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return xerrors.Errorf("open %s script log file: %w", lifecycle, err)
	}
	defer func() {
		_ = fileWriter.Close()
	}()

	var writer io.Writer = fileWriter
	if lifecycle == "startup" {
		// Create pipes for startup logs reader and writer
		logsReader, logsWriter := io.Pipe()
		defer func() {
			_ = logsReader.Close()
		}()
		writer = io.MultiWriter(fileWriter, logsWriter)
		flushedLogs, err := a.trackScriptLogs(ctx, logsReader)
		if err != nil {
			return xerrors.Errorf("track script logs: %w", err)
		}
		defer func() {
			_ = logsWriter.Close()
			<-flushedLogs
		}()
	}

	cmd, err := a.createCommand(ctx, script, nil)
	if err != nil {
		return xerrors.Errorf("create command: %w", err)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err = cmd.Run()
	if err != nil {
		// cmd.Run does not return a context canceled error, it returns "signal: killed".
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return xerrors.Errorf("run: %w", err)
	}
	return nil
}

func (a *agent) trackScriptLogs(ctx context.Context, reader io.Reader) (chan struct{}, error) {
	// Initialize variables for log management
	queuedLogs := make([]agentsdk.StartupLog, 0)
	var flushLogsTimer *time.Timer
	var logMutex sync.Mutex
	logsFlushed := sync.NewCond(&sync.Mutex{})
	var logsSending bool
	defer func() {
		logMutex.Lock()
		if flushLogsTimer != nil {
			flushLogsTimer.Stop()
		}
		logMutex.Unlock()
	}()

	// sendLogs function uploads the queued logs to the server
	sendLogs := func() {
		// Lock logMutex and check if logs are already being sent
		logMutex.Lock()
		if logsSending {
			logMutex.Unlock()
			return
		}
		if flushLogsTimer != nil {
			flushLogsTimer.Stop()
		}
		if len(queuedLogs) == 0 {
			logMutex.Unlock()
			return
		}
		// Move the current queued logs to logsToSend and clear the queue
		logsToSend := queuedLogs
		logsSending = true
		queuedLogs = make([]agentsdk.StartupLog, 0)
		logMutex.Unlock()

		// Retry uploading logs until successful or a specific error occurs
		for r := retry.New(time.Second, 5*time.Second); r.Wait(ctx); {
			err := a.client.PatchStartupLogs(ctx, agentsdk.PatchStartupLogs{
				Logs: logsToSend,
			})
			if err == nil {
				break
			}
			var sdkErr *codersdk.Error
			if errors.As(err, &sdkErr) {
				if sdkErr.StatusCode() == http.StatusRequestEntityTooLarge {
					a.logger.Warn(ctx, "startup logs too large, dropping logs")
					break
				}
			}
			a.logger.Error(ctx, "upload startup logs", slog.Error(err), slog.F("to_send", logsToSend))
		}
		// Reset logsSending flag
		logMutex.Lock()
		logsSending = false
		flushLogsTimer.Reset(100 * time.Millisecond)
		logMutex.Unlock()
		logsFlushed.Broadcast()
	}
	// queueLog function appends a log to the queue and triggers sendLogs if necessary
	queueLog := func(log agentsdk.StartupLog) {
		logMutex.Lock()
		defer logMutex.Unlock()

		// Append log to the queue
		queuedLogs = append(queuedLogs, log)

		// If there are more than 100 logs, send them immediately
		if len(queuedLogs) > 100 {
			// Don't early return after this, because we still want
			// to reset the timer just in case logs come in while
			// we're sending.
			go sendLogs()
		}
		// Reset or set the flushLogsTimer to trigger sendLogs after 100 milliseconds
		if flushLogsTimer != nil {
			flushLogsTimer.Reset(100 * time.Millisecond)
			return
		}
		flushLogsTimer = time.AfterFunc(100*time.Millisecond, sendLogs)
	}

	// It's important that we either flush or drop all logs before returning
	// because the startup state is reported after flush.
	//
	// It'd be weird for the startup state to be ready, but logs are still
	// coming in.
	logsFinished := make(chan struct{})
	err := a.trackConnGoroutine(func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			queueLog(agentsdk.StartupLog{
				CreatedAt: database.Now(),
				Output:    scanner.Text(),
			})
		}
		defer close(logsFinished)
		logsFlushed.L.Lock()
		for {
			logMutex.Lock()
			if len(queuedLogs) == 0 {
				logMutex.Unlock()
				break
			}
			logMutex.Unlock()
			logsFlushed.Wait()
		}
	})
	if err != nil {
		return nil, xerrors.Errorf("track conn goroutine: %w", err)
	}
	return logsFinished, nil
}

func (a *agent) init(ctx context.Context) {
	// Clients' should ignore the host key when connecting.
	// The agent needs to authenticate with coderd to SSH,
	// so SSH authentication doesn't improve security.
	randomHostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	randomSigner, err := gossh.NewSignerFromKey(randomHostKey)
	if err != nil {
		panic(err)
	}

	sshLogger := a.logger.Named("ssh-server")
	forwardHandler := &ssh.ForwardedTCPHandler{}
	unixForwardHandler := &forwardedUnixHandler{log: a.logger}

	a.sshServer = &ssh.Server{
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip":                   ssh.DirectTCPIPHandler,
			"direct-streamlocal@openssh.com": directStreamLocalHandler,
			"session":                        ssh.DefaultSessionHandler,
		},
		ConnectionFailedCallback: func(conn net.Conn, err error) {
			sshLogger.Info(ctx, "ssh connection ended", slog.Error(err))
		},
		Handler: func(session ssh.Session) {
			err := a.handleSSHSession(session)
			var exitError *exec.ExitError
			if xerrors.As(err, &exitError) {
				a.logger.Debug(ctx, "ssh session returned", slog.Error(exitError))
				_ = session.Exit(exitError.ExitCode())
				return
			}
			if err != nil {
				a.logger.Warn(ctx, "ssh session failed", slog.Error(err))
				// This exit code is designed to be unlikely to be confused for a legit exit code
				// from the process.
				_ = session.Exit(MagicSessionErrorCode)
				return
			}
			_ = session.Exit(0)
		},
		HostSigners: []ssh.Signer{randomSigner},
		LocalPortForwardingCallback: func(ctx ssh.Context, destinationHost string, destinationPort uint32) bool {
			// Allow local port forwarding all!
			sshLogger.Debug(ctx, "local port forward",
				slog.F("destination-host", destinationHost),
				slog.F("destination-port", destinationPort))
			return true
		},
		PtyCallback: func(ctx ssh.Context, pty ssh.Pty) bool {
			return true
		},
		ReversePortForwardingCallback: func(ctx ssh.Context, bindHost string, bindPort uint32) bool {
			// Allow reverse port forwarding all!
			sshLogger.Debug(ctx, "local port forward",
				slog.F("bind-host", bindHost),
				slog.F("bind-port", bindPort))
			return true
		},
		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":                          forwardHandler.HandleSSHRequest,
			"cancel-tcpip-forward":                   forwardHandler.HandleSSHRequest,
			"streamlocal-forward@openssh.com":        unixForwardHandler.HandleSSHRequest,
			"cancel-streamlocal-forward@openssh.com": unixForwardHandler.HandleSSHRequest,
		},
		ServerConfigCallback: func(ctx ssh.Context) *gossh.ServerConfig {
			return &gossh.ServerConfig{
				NoClientAuth: true,
			}
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(session ssh.Session) {
				ctx := session.Context()

				// Typically sftp sessions don't request a TTY, but if they do,
				// we must ensure the gliderlabs/ssh CRLF emulation is disabled.
				// Otherwise sftp will be broken. This can happen if a user sets
				// `RequestTTY force` in their SSH config.
				session.DisablePTYEmulation()

				var opts []sftp.ServerOption
				// Change current working directory to the users home
				// directory so that SFTP connections land there.
				homedir, err := userHomeDir()
				if err != nil {
					sshLogger.Warn(ctx, "get sftp working directory failed, unable to get home dir", slog.Error(err))
				} else {
					opts = append(opts, sftp.WithServerWorkingDirectory(homedir))
				}

				server, err := sftp.NewServer(session, opts...)
				if err != nil {
					sshLogger.Debug(ctx, "initialize sftp server", slog.Error(err))
					return
				}
				defer server.Close()

				err = server.Serve()
				if errors.Is(err, io.EOF) {
					// Unless we call `session.Exit(0)` here, the client won't
					// receive `exit-status` because `(*sftp.Server).Close()`
					// calls `Close()` on the underlying connection (session),
					// which actually calls `channel.Close()` because it isn't
					// wrapped. This causes sftp clients to receive a non-zero
					// exit code. Typically sftp clients don't echo this exit
					// code but `scp` on macOS does (when using the default
					// SFTP backend).
					_ = session.Exit(0)
					return
				}
				sshLogger.Warn(ctx, "sftp server closed with error", slog.Error(err))
				_ = session.Exit(1)
			},
		},
		MaxTimeout: a.sshMaxTimeout,
	}

	go a.runLoop(ctx)
}

// createCommand processes raw command input with OpenSSH-like behavior.
// If the script provided is empty, it will default to the users shell.
// This injects environment variables specified by the user at launch too.
func (a *agent) createCommand(ctx context.Context, script string, env []string) (*exec.Cmd, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, xerrors.Errorf("get current user: %w", err)
	}
	username := currentUser.Username

	shell, err := usershell.Get(username)
	if err != nil {
		return nil, xerrors.Errorf("get user shell: %w", err)
	}

	manifest := a.manifest.Load()
	if manifest == nil {
		return nil, xerrors.Errorf("no metadata was provided")
	}

	// OpenSSH executes all commands with the users current shell.
	// We replicate that behavior for IDE support.
	caller := "-c"
	if runtime.GOOS == "windows" {
		caller = "/c"
	}
	args := []string{caller, script}

	// gliderlabs/ssh returns a command slice of zero
	// when a shell is requested.
	if len(script) == 0 {
		args = []string{}
		if runtime.GOOS != "windows" {
			// On Linux and macOS, we should start a login
			// shell to consume juicy environment variables!
			args = append(args, "-l")
		}
	}

	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Dir = manifest.Directory

	// If the metadata directory doesn't exist, we run the command
	// in the users home directory.
	_, err = os.Stat(cmd.Dir)
	if cmd.Dir == "" || err != nil {
		// Default to user home if a directory is not set.
		homedir, err := userHomeDir()
		if err != nil {
			return nil, xerrors.Errorf("get home dir: %w", err)
		}
		cmd.Dir = homedir
	}
	cmd.Env = append(os.Environ(), env...)
	executablePath, err := os.Executable()
	if err != nil {
		return nil, xerrors.Errorf("getting os executable: %w", err)
	}
	// Set environment variables reliable detection of being inside a
	// Coder workspace.
	cmd.Env = append(cmd.Env, "CODER=true")
	cmd.Env = append(cmd.Env, fmt.Sprintf("USER=%s", username))
	// Git on Windows resolves with UNIX-style paths.
	// If using backslashes, it's unable to find the executable.
	unixExecutablePath := strings.ReplaceAll(executablePath, "\\", "/")
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_SSH_COMMAND=%s gitssh --`, unixExecutablePath))

	// Specific Coder subcommands require the agent token exposed!
	cmd.Env = append(cmd.Env, fmt.Sprintf("CODER_AGENT_TOKEN=%s", *a.sessionToken.Load()))

	// Set SSH connection environment variables (these are also set by OpenSSH
	// and thus expected to be present by SSH clients). Since the agent does
	// networking in-memory, trying to provide accurate values here would be
	// nonsensical. For now, we hard code these values so that they're present.
	srcAddr, srcPort := "0.0.0.0", "0"
	dstAddr, dstPort := "0.0.0.0", "0"
	cmd.Env = append(cmd.Env, fmt.Sprintf("SSH_CLIENT=%s %s %s", srcAddr, srcPort, dstPort))
	cmd.Env = append(cmd.Env, fmt.Sprintf("SSH_CONNECTION=%s %s %s %s", srcAddr, srcPort, dstAddr, dstPort))

	// This adds the ports dialog to code-server that enables
	// proxying a port dynamically.
	cmd.Env = append(cmd.Env, fmt.Sprintf("VSCODE_PROXY_URI=%s", manifest.VSCodePortProxyURI))

	// Hide Coder message on code-server's "Getting Started" page
	cmd.Env = append(cmd.Env, "CS_DISABLE_GETTING_STARTED_OVERRIDE=true")

	// Load environment variables passed via the agent.
	// These should override all variables we manually specify.
	for envKey, value := range manifest.EnvironmentVariables {
		// Expanding environment variables allows for customization
		// of the $PATH, among other variables. Customers can prepend
		// or append to the $PATH, so allowing expand is required!
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", envKey, os.ExpandEnv(value)))
	}

	// Agent-level environment variables should take over all!
	// This is used for setting agent-specific variables like "CODER_AGENT_TOKEN".
	for envKey, value := range a.envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", envKey, value))
	}

	return cmd, nil
}

func (a *agent) handleSSHSession(session ssh.Session) (retErr error) {
	ctx := session.Context()
	env := session.Environ()
	var magicType string
	for index, kv := range env {
		if !strings.HasPrefix(kv, MagicSSHSessionTypeEnvironmentVariable) {
			continue
		}
		magicType = strings.TrimPrefix(kv, MagicSSHSessionTypeEnvironmentVariable+"=")
		env = append(env[:index], env[index+1:]...)
	}
	switch magicType {
	case MagicSSHSessionTypeVSCode:
		a.connCountVSCode.Add(1)
		defer a.connCountVSCode.Add(-1)
	case MagicSSHSessionTypeJetBrains:
		a.connCountJetBrains.Add(1)
		defer a.connCountJetBrains.Add(-1)
	case "":
		a.connCountSSHSession.Add(1)
		defer a.connCountSSHSession.Add(-1)
	default:
		a.logger.Warn(ctx, "invalid magic ssh session type specified", slog.F("type", magicType))
	}

	cmd, err := a.createCommand(ctx, session.RawCommand(), env)
	if err != nil {
		return err
	}

	if ssh.AgentRequested(session) {
		l, err := ssh.NewAgentListener()
		if err != nil {
			return xerrors.Errorf("new agent listener: %w", err)
		}
		defer l.Close()
		go ssh.ForwardAgentConnections(l, session)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SSH_AUTH_SOCK", l.Addr().String()))
	}

	sshPty, windowSize, isPty := session.Pty()
	if isPty {
		// Disable minimal PTY emulation set by gliderlabs/ssh (NL-to-CRNL).
		// See https://github.com/coder/coder/issues/3371.
		session.DisablePTYEmulation()

		if !isQuietLogin(session.RawCommand()) {
			manifest := a.manifest.Load()
			if manifest != nil {
				err = showMOTD(session, manifest.MOTDFile)
				if err != nil {
					a.logger.Error(ctx, "show MOTD", slog.Error(err))
				}
			} else {
				a.logger.Warn(ctx, "metadata lookup failed, unable to show MOTD")
			}
		}

		cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", sshPty.Term))

		// The pty package sets `SSH_TTY` on supported platforms.
		ptty, process, err := pty.Start(cmd, pty.WithPTYOption(
			pty.WithSSHRequest(sshPty),
			pty.WithLogger(slog.Stdlib(ctx, a.logger, slog.LevelInfo)),
		))
		if err != nil {
			return xerrors.Errorf("start command: %w", err)
		}
		var wg sync.WaitGroup
		defer func() {
			defer wg.Wait()
			closeErr := ptty.Close()
			if closeErr != nil {
				a.logger.Warn(ctx, "failed to close tty", slog.Error(closeErr))
				if retErr == nil {
					retErr = closeErr
				}
			}
		}()
		go func() {
			for win := range windowSize {
				resizeErr := ptty.Resize(uint16(win.Height), uint16(win.Width))
				// If the pty is closed, then command has exited, no need to log.
				if resizeErr != nil && !errors.Is(resizeErr, pty.ErrClosed) {
					a.logger.Warn(ctx, "failed to resize tty", slog.Error(resizeErr))
				}
			}
		}()
		// We don't add input copy to wait group because
		// it won't return until the session is closed.
		go func() {
			_, _ = io.Copy(ptty.Input(), session)
		}()

		// In low parallelism scenarios, the command may exit and we may close
		// the pty before the output copy has started. This can result in the
		// output being lost. To avoid this, we wait for the output copy to
		// start before waiting for the command to exit. This ensures that the
		// output copy goroutine will be scheduled before calling close on the
		// pty. This shouldn't be needed because of `pty.Dup()` below, but it
		// may not be supported on all platforms.
		outputCopyStarted := make(chan struct{})
		ptyOutput := func() io.ReadCloser {
			defer close(outputCopyStarted)
			// Try to dup so we can separate stdin and stdout closure.
			// Once the original pty is closed, the dup will return
			// input/output error once the buffered data has been read.
			stdout, err := ptty.Dup()
			if err == nil {
				return stdout
			}
			// If we can't dup, we shouldn't close
			// the fd since it's tied to stdin.
			return readNopCloser{ptty.Output()}
		}
		wg.Add(1)
		go func() {
			// Ensure data is flushed to session on command exit, if we
			// close the session too soon, we might lose data.
			defer wg.Done()

			stdout := ptyOutput()
			defer stdout.Close()

			_, _ = io.Copy(session, stdout)
		}()
		<-outputCopyStarted

		err = process.Wait()
		var exitErr *exec.ExitError
		// ExitErrors just mean the command we run returned a non-zero exit code, which is normal
		// and not something to be concerned about.  But, if it's something else, we should log it.
		if err != nil && !xerrors.As(err, &exitErr) {
			a.logger.Warn(ctx, "wait error", slog.Error(err))
		}
		return err
	}

	cmd.Stdout = session
	cmd.Stderr = session.Stderr()
	// This blocks forever until stdin is received if we don't
	// use StdinPipe. It's unknown what causes this.
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return xerrors.Errorf("create stdin pipe: %w", err)
	}
	go func() {
		_, _ = io.Copy(stdinPipe, session)
		_ = stdinPipe.Close()
	}()
	err = cmd.Start()
	if err != nil {
		return xerrors.Errorf("start: %w", err)
	}
	return cmd.Wait()
}

type readNopCloser struct{ io.Reader }

// Close implements io.Closer.
func (readNopCloser) Close() error { return nil }

func (a *agent) handleReconnectingPTY(ctx context.Context, logger slog.Logger, msg codersdk.WorkspaceAgentReconnectingPTYInit, conn net.Conn) (retErr error) {
	defer conn.Close()

	a.connCountReconnectingPTY.Add(1)
	defer a.connCountReconnectingPTY.Add(-1)

	connectionID := uuid.NewString()
	logger = logger.With(slog.F("id", msg.ID), slog.F("connection_id", connectionID))

	defer func() {
		if err := retErr; err != nil {
			a.closeMutex.Lock()
			closed := a.isClosed()
			a.closeMutex.Unlock()

			// If the agent is closed, we don't want to
			// log this as an error since it's expected.
			if closed {
				logger.Debug(ctx, "session error after agent close", slog.Error(err))
			} else {
				logger.Error(ctx, "session error", slog.Error(err))
			}
		}
		logger.Debug(ctx, "session closed")
	}()

	var rpty *reconnectingPTY
	rawRPTY, ok := a.reconnectingPTYs.Load(msg.ID)
	if ok {
		logger.Debug(ctx, "connecting to existing session")
		rpty, ok = rawRPTY.(*reconnectingPTY)
		if !ok {
			return xerrors.Errorf("found invalid type in reconnecting pty map: %T", rawRPTY)
		}
	} else {
		logger.Debug(ctx, "creating new session")

		// Empty command will default to the users shell!
		cmd, err := a.createCommand(ctx, msg.Command, nil)
		if err != nil {
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
			return xerrors.Errorf("start command: %w", err)
		}

		ctx, cancelFunc := context.WithCancel(ctx)
		rpty = &reconnectingPTY{
			activeConns: map[string]net.Conn{
				// We have to put the connection in the map instantly otherwise
				// the connection won't be closed if the process instantly dies.
				connectionID: conn,
			},
			ptty: ptty,
			// Timeouts created with an after func can be reset!
			timeout:        time.AfterFunc(a.reconnectingPTYTimeout, cancelFunc),
			circularBuffer: circularBuffer,
		}
		a.reconnectingPTYs.Store(msg.ID, rpty)
		go func() {
			// CommandContext isn't respected for Windows PTYs right now,
			// so we need to manually track the lifecycle.
			// When the context has been completed either:
			// 1. The timeout completed.
			// 2. The parent context was canceled.
			<-ctx.Done()
			_ = process.Kill()
		}()
		go func() {
			// If the process dies randomly, we should
			// close the pty.
			_ = process.Wait()
			rpty.Close()
		}()
		if err = a.trackConnGoroutine(func() {
			buffer := make([]byte, 1024)
			for {
				read, err := rpty.ptty.Output().Read(buffer)
				if err != nil {
					// When the PTY is closed, this is triggered.
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
				for _, conn := range rpty.activeConns {
					_, _ = conn.Write(part)
				}
				rpty.activeConnsMutex.Unlock()
			}

			// Cleanup the process, PTY, and delete it's
			// ID from memory.
			_ = process.Kill()
			rpty.Close()
			a.reconnectingPTYs.Delete(msg.ID)
		}); err != nil {
			return xerrors.Errorf("start routine: %w", err)
		}
	}
	// Resize the PTY to initial height + width.
	err := rpty.ptty.Resize(msg.Height, msg.Width)
	if err != nil {
		// We can continue after this, it's not fatal!
		logger.Error(ctx, "resize", slog.Error(err))
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
			logger.Warn(ctx, "read conn", slog.Error(err))
			return nil
		}
		_, err = rpty.ptty.Input().Write([]byte(req.Data))
		if err != nil {
			logger.Warn(ctx, "write to pty", slog.Error(err))
			return nil
		}
		// Check if a resize needs to happen!
		if req.Height == 0 || req.Width == 0 {
			continue
		}
		err = rpty.ptty.Resize(req.Height, req.Width)
		if err != nil {
			// We can continue after this, it's not fatal!
			logger.Error(ctx, "resize", slog.Error(err))
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
		stats.SessionCountSSH = a.connCountSSHSession.Load()
		stats.SessionCountVSCode = a.connCountVSCode.Load()
		stats.SessionCountJetBrains = a.connCountJetBrains.Load()
		stats.SessionCountReconnectingPTY = a.connCountReconnectingPTY.Load()

		// Compute the median connection latency!
		var wg sync.WaitGroup
		var mu sync.Mutex
		status := a.network.Status()
		durations := []float64{}
		ctx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
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
				duration, _, _, err := a.network.Ping(ctx, addresses[0].Addr())
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

		lastStat := a.latestStat.Load()
		if lastStat != nil && reflect.DeepEqual(lastStat, stats) {
			a.logger.Info(ctx, "skipping stat because nothing changed")
			return
		}
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
		a.logger.Error(ctx, "report stats", slog.Error(err))
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

func (a *agent) Close() error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.isClosed() {
		return nil
	}

	ctx := context.Background()
	a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleShuttingDown)

	lifecycleState := codersdk.WorkspaceAgentLifecycleOff
	if manifest := a.manifest.Load(); manifest != nil && manifest.ShutdownScript != "" {
		scriptDone := make(chan error, 1)
		scriptStart := time.Now()
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
			a.logger.Warn(ctx, "shutdown script timed out")
			a.setLifecycle(ctx, codersdk.WorkspaceAgentLifecycleShutdownTimeout)
			err = <-scriptDone // The script can still complete after a timeout.
		}
		execTime := time.Since(scriptStart)
		if err != nil {
			a.logger.Warn(ctx, "shutdown script failed", slog.F("execution_time", execTime), slog.Error(err))
			lifecycleState = codersdk.WorkspaceAgentLifecycleShutdownError
		} else {
			a.logger.Info(ctx, "shutdown script completed", slog.F("execution_time", execTime))
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
	ptty                pty.PTY
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

// Bicopy copies all of the data between the two connections and will close them
// after one or both of them are done writing. If the context is canceled, both
// of the connections will be closed.
func Bicopy(ctx context.Context, c1, c2 io.ReadWriteCloser) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	defer func() {
		_ = c1.Close()
		_ = c2.Close()
	}()

	var wg sync.WaitGroup
	copyFunc := func(dst io.WriteCloser, src io.Reader) {
		defer func() {
			wg.Done()
			// If one side of the copy fails, ensure the other one exits as
			// well.
			cancel()
		}()
		_, _ = io.Copy(dst, src)
	}

	wg.Add(2)
	go copyFunc(c1, c2)
	go copyFunc(c2, c1)

	// Convert waitgroup to a channel so we can also wait on the context.
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

// isQuietLogin checks if the SSH server should perform a quiet login or not.
//
// https://github.com/openssh/openssh-portable/blob/25bd659cc72268f2858c5415740c442ee950049f/session.c#L816
func isQuietLogin(rawCommand string) bool {
	// We are always quiet unless this is a login shell.
	if len(rawCommand) != 0 {
		return true
	}

	// Best effort, if we can't get the home directory,
	// we can't lookup .hushlogin.
	homedir, err := userHomeDir()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(homedir, ".hushlogin"))
	return err == nil
}

// showMOTD will output the message of the day from
// the given filename to dest, if the file exists.
//
// https://github.com/openssh/openssh-portable/blob/25bd659cc72268f2858c5415740c442ee950049f/session.c#L784
func showMOTD(dest io.Writer, filename string) error {
	if filename == "" {
		return nil
	}

	f, err := os.Open(filename)
	if err != nil {
		if xerrors.Is(err, os.ErrNotExist) {
			// This is not an error, there simply isn't a MOTD to show.
			return nil
		}
		return xerrors.Errorf("open MOTD: %w", err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		// Carriage return ensures each line starts
		// at the beginning of the terminal.
		_, err = fmt.Fprint(dest, s.Text()+"\r\n")
		if err != nil {
			return xerrors.Errorf("write MOTD: %w", err)
		}
	}
	if err := s.Err(); err != nil {
		return xerrors.Errorf("read MOTD: %w", err)
	}

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
	return os.ExpandEnv(dir), nil
}
