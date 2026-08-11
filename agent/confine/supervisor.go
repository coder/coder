package confine

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/reaper"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

const (
	// EnvAgentEgressProxyURL carries the supervisor proxy URL into the child
	// agent without exposing the policy itself.
	EnvAgentEgressProxyURL = "CODER_AGENT_EGRESS_PROXY_URL"
	// EnvEgressProxy is inherited by agent-spawned processes as the Coder
	// specific proxy location.
	EnvEgressProxy = "CODER_EGRESS_PROXY"

	fetchTimeout       = 10 * time.Second
	reportTimeout      = 5 * time.Second
	eventFlushPeriod   = 5 * time.Second
	eventQueueSize     = 1024
	proxyListenAddress = "127.0.0.1:0"
)

// Mode selects the confinement mechanism.
type Mode string

const (
	ModeProxy Mode = "proxy"
	ModeNetNS Mode = "netns"
)

// AgentClient is the agent API used by the confinement supervisor.
type AgentClient interface {
	AIEgressPolicy(context.Context) (codersdk.AIEgressPolicy, error)
	WatchAIEgressPolicy(context.Context) (<-chan codersdk.AIEgressPolicy, io.Closer, error)
	PostAISandboxSession(context.Context, agentsdk.PostAISandboxSessionRequest) error
	PatchAISandboxNetworkEvents(context.Context, agentsdk.PatchAISandboxNetworkEventsRequest) error
	PatchLogs(context.Context, agentsdk.PatchLogs) error
}

// SupervisorOptions configures a confinement supervisor and child process.
type SupervisorOptions struct {
	Mode         Mode
	Client       AgentClient
	Logger       slog.Logger
	AccessURL    *url.URL
	ExecArgs     []string
	Env          []string
	CatchSignals []os.Signal
}

// Supervisor owns policy delivery, egress listeners, audit reporting, and the
// confined child process lifecycle.
type Supervisor struct {
	options SupervisorOptions
}

// NewSupervisor validates options for a confinement supervisor.
func NewSupervisor(options SupervisorOptions) (*Supervisor, error) {
	if options.Mode != ModeProxy && options.Mode != ModeNetNS {
		return nil, xerrors.Errorf("invalid confinement mode %q", options.Mode)
	}
	if options.Client == nil {
		return nil, xerrors.New("confinement agent client is required")
	}
	if options.AccessURL == nil || options.AccessURL.Hostname() == "" {
		return nil, xerrors.New("confinement access URL is required")
	}
	if len(options.ExecArgs) == 0 {
		return nil, xerrors.New("confinement child arguments are required")
	}
	return &Supervisor{options: options}, nil
}

// Run bootstraps default-deny policy before fork, runs the confined child, and
// retains the last policy through watch failures.
func (s *Supervisor) Run(ctx context.Context) (int, error) {
	host := s.options.AccessURL.Hostname()
	port, err := accessURLPort(s.options.AccessURL)
	if err != nil {
		return 1, err
	}
	engine := NewPolicyEngine(host, port)

	fetchCtx, fetchCancel := context.WithTimeout(ctx, fetchTimeout)
	policy, fetchErr := s.options.Client.AIEgressPolicy(fetchCtx)
	fetchCancel()
	if fetchErr != nil {
		s.signalDegraded("AI egress policy fetch failed; started deny-all (degraded)", fetchErr)
	} else {
		engine.Update(policy)
		s.options.Logger.Info(ctx, "applied ai egress policy",
			slog.F("revision", policy.Revision),
			slog.F("rule_count", len(policy.Rules)),
		)
	}

	netns, forced := s.prepareNetwork(ctx)
	if netns != nil {
		defer func() {
			_ = netns.Close()
		}()
	}

	listenAddress := proxyListenAddress
	if forced {
		listenAddress = net.JoinHostPort(netns.hostIP, "0")
	}

	sessionID := uuid.New()
	batcher := newEventBatcher(s.options.Client, s.options.Logger, sessionID, eventQueueSize)
	proxy, err := ListenProxy(listenAddress, engine, batcher.Add)
	if err != nil && forced {
		_ = netns.Close()
		netns = nil
		forced = false
		s.signalDegraded("AI egress network listener failed; using proxy-only advisory mode (degraded)", err)
		proxy, err = ListenProxy(proxyListenAddress, engine, batcher.Add)
	}
	if err != nil {
		return 1, xerrors.Errorf("start egress proxy: %w", err)
	}
	defer func() {
		_ = proxy.Close()
	}()

	var sni *SNIListener
	if forced {
		sni, err = ListenSNI(net.JoinHostPort(netns.hostIP, "0"), engine, batcher.Add)
		if err != nil {
			_ = proxy.Close()
			_ = netns.Close()
			netns = nil
			forced = false
			s.signalDegraded("AI egress SNI listener failed; using proxy-only advisory mode (degraded)", err)
			proxy, err = ListenProxy(proxyListenAddress, engine, batcher.Add)
			if err != nil {
				return 1, xerrors.Errorf("start fallback egress proxy: %w", err)
			}
		}
	}
	if sni != nil {
		defer func() {
			_ = sni.Close()
		}()
	}

	s.options.Logger.Info(ctx, "ai egress proxy started",
		slog.F("proxy_url", "http://"+proxy.Addr().String()),
		slog.F("forced", forced),
	)

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go watchPolicy(runCtx, s.options.Client, s.options.Logger, engine)
	go batcher.Run(runCtx, eventFlushPeriod)

	enforcement := codersdk.AISandboxEgressEnforcementAdvisory
	if forced {
		enforcement = codersdk.AISandboxEgressEnforcementForced
	}

	proxyURL := "http://" + proxy.Addr().String()
	childEnv := childEnvironment(s.options.Env, proxyURL, enforcement)
	childArgs := append([]string(nil), s.options.ExecArgs...)
	if forced {
		childArgs = netns.execArgs(childArgs)
	}

	var startedAt time.Time
	childStarted := make(chan struct{})
	sessionTriggered := make(chan struct{})
	sessionAbort := make(chan struct{})
	sessionReported := make(chan struct{})
	go func() {
		defer close(sessionReported)
		select {
		case <-childStarted:
			close(sessionTriggered)
			s.postSession(agentsdk.PostAISandboxSessionRequest{
				ID:                sessionID,
				EgressEnforcement: enforcement,
				StartedAt:         startedAt,
			})
		case <-sessionAbort:
		}
	}()

	childDidStart := false
	exitCode, forkErr := reaper.ForkReap(
		reaper.WithExecArgs(childArgs...),
		reaper.WithEnv(childEnv),
		reaper.WithStartCallback(func(_ int) {
			startedAt = time.Now()
			childDidStart = true
			close(childStarted)
			<-sessionTriggered
		}),
		reaper.WithCatchSignals(s.options.CatchSignals...),
		reaper.WithLogger(s.options.Logger),
	)
	if !childDidStart {
		close(sessionAbort)
	}
	<-sessionReported
	stop()
	batcher.Flush()
	if childDidStart {
		endedAt := time.Now()
		s.postSession(agentsdk.PostAISandboxSessionRequest{
			ID:                sessionID,
			EgressEnforcement: enforcement,
			StartedAt:         startedAt,
			EndedAt:           &endedAt,
		})
	}
	return exitCode, forkErr
}

func (s *Supervisor) prepareNetwork(ctx context.Context) (*networkNamespace, bool) {
	if s.options.Mode != ModeNetNS {
		return nil, false
	}
	netns, err := newNetworkNamespace(ctx)
	if err != nil {
		s.signalDegraded("AI egress network namespace setup failed; using proxy-only advisory mode (degraded)", err)
		return nil, false
	}
	return netns, true
}

func (s *Supervisor) signalDegraded(message string, cause error) {
	ctx := context.Background()
	s.options.Logger.Warn(ctx, message, slog.Error(cause))
	reportCtx, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()
	err := s.options.Client.PatchLogs(reportCtx, agentsdk.PatchLogs{
		Logs: []agentsdk.Log{{CreatedAt: time.Now(), Output: message, Level: codersdk.LogLevelWarn}},
	})
	if err != nil {
		s.logReportFailure(ctx, "report degraded confinement", err)
	}
}

func (s *Supervisor) postSession(request agentsdk.PostAISandboxSessionRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()
	if err := s.options.Client.PostAISandboxSession(ctx, request); err != nil {
		s.logReportFailure(ctx, "report AI sandbox session", err)
	}
}

func (s *Supervisor) logReportFailure(ctx context.Context, operation string, err error) {
	if isNotFound(err) {
		return
	}
	s.options.Logger.Warn(ctx, operation, slog.Error(err))
}

func childEnvironment(env []string, proxyURL string, enforcement codersdk.AISandboxEgressEnforcement) []string {
	env = unsetEnv(append([]string(nil), env...), "CODER_AGENT_CONFINE")
	env = setEnv(env, EnvAgentEgressProxyURL, proxyURL)
	if enforcement != codersdk.AISandboxEgressEnforcementForced {
		return env
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		env = setEnv(env, key, proxyURL)
	}
	return setEnv(env, "NO_PROXY", "localhost,127.0.0.1,::1")
}

func setEnv(env []string, key, value string) []string {
	env = unsetEnv(env, key)
	return append(env, key+"="+value)
}

func unsetEnv(env []string, key string) []string {
	prefix := key + "="
	return slices.DeleteFunc(env, func(value string) bool {
		return strings.HasPrefix(value, prefix)
	})
}

func accessURLPort(accessURL *url.URL) (int, error) {
	if value := accessURL.Port(); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || !validPort(port) {
			return 0, xerrors.Errorf("invalid access URL port %q", value)
		}
		return port, nil
	}
	switch strings.ToLower(accessURL.Scheme) {
	case "http", "ws":
		return defaultHTTPPort, nil
	case "https", "wss":
		return defaultHTTPSPort, nil
	default:
		return 0, xerrors.Errorf("access URL scheme %q has no default port", accessURL.Scheme)
	}
}

func waitBackoff(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isNotFound(err error) bool {
	var sdkErr *codersdk.Error
	return xerrors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound
}
