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

	forced := s.options.Mode == ModeNetNS
	sessionID := uuid.New()
	batcher := newEventBatcher(s.options.Client, s.options.Logger, sessionID, eventQueueSize)

	// Namespace mode is a structural claim. Any failure to establish it is
	// fatal rather than a downgrade to advisory: silently serving a weaker
	// boundary than the shape attests would misreport the enforcement that
	// actually applies.
	var netns *NetworkNamespace
	listenAddress := proxyListenAddress
	if forced {
		if err := PreflightNetworkNamespace(ctx); err != nil {
			return 1, xerrors.Errorf("network confinement unavailable: %w", err)
		}
		opened, err := OpenNetworkNamespace(ctx, NetworkNamespaceOptions{})
		if err != nil {
			return 1, xerrors.Errorf("create network namespace: %w", err)
		}
		netns = opened
		defer func() {
			_ = netns.Close()
		}()
		listenAddress = net.JoinHostPort(netns.HostIP(), "0")
	}

	proxy, err := ListenProxy(listenAddress, engine, batcher.Add)
	if err != nil {
		return 1, xerrors.Errorf("start egress proxy: %w", err)
	}
	defer func() {
		_ = proxy.Close()
	}()

	var (
		sni   *SNIListener
		relay *Relay
	)
	if forced {
		sni, err = ListenSNI(listenAddress, engine, batcher.Add)
		if err != nil {
			return 1, xerrors.Errorf("start SNI listener: %w", err)
		}
		defer func() {
			_ = sni.Close()
		}()

		// Transparent interception requires name resolution inside the
		// namespace: a client must resolve a name before it can open the
		// connection the platform intends to intercept.
		// DNS decisions are logged rather than batched into the egress
		// audit stream: ai_sandbox_network_events constrains protocol to
		// connect, http, sni, and tcp, so a dns value needs a schema change
		// before these can be retained server side.
		relay, err = ListenRelay(listenAddress, engine, func(event DNSQueryEvent) {
			s.options.Logger.Debug(ctx, "ai egress dns query",
				slog.F("qname", event.QName),
				slog.F("qtype", event.QType),
				slog.F("decision", string(event.Decision)),
				slog.F("policy_revision", event.PolicyRevision),
			)
		})
		if err != nil {
			return 1, xerrors.Errorf("start DNS relay: %w", err)
		}
		defer func() {
			_ = relay.Close()
		}()

		ports, perr := listenerPorts(proxy.Addr(), sni.Addr(), relay.Addr())
		if perr != nil {
			return 1, perr
		}
		if err := netns.ConfigureEgress(ctx, ports); err != nil {
			return 1, xerrors.Errorf("configure network confinement: %w", err)
		}
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
		// Refuses when the namespace is unconfigured or closed, so a
		// confinement failure cannot silently run the child unconfined.
		childArgs, err = netns.CommandArgs(childArgs)
		if err != nil {
			return 1, xerrors.Errorf("build confined child arguments: %w", err)
		}
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

// listenerPorts extracts the ports the namespace must redirect to. The
// listeners bind an ephemeral port each, so the rules can only be built
// after they are up.
func listenerPorts(proxyAddr, sniAddr, relayAddr net.Addr) (NetworkNamespacePorts, error) {
	var ports NetworkNamespacePorts
	for _, entry := range []struct {
		addr net.Addr
		name string
		out  *uint16
	}{
		{proxyAddr, "http proxy", &ports.HTTP},
		{sniAddr, "sni listener", &ports.SNI},
		{relayAddr, "dns relay", &ports.DNS},
	} {
		if entry.addr == nil {
			return NetworkNamespacePorts{}, xerrors.Errorf("%s has no address", entry.name)
		}
		_, port, err := net.SplitHostPort(entry.addr.String())
		if err != nil {
			return NetworkNamespacePorts{}, xerrors.Errorf("parse %s address: %w", entry.name, err)
		}
		parsed, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return NetworkNamespacePorts{}, xerrors.Errorf("parse %s port: %w", entry.name, err)
		}
		*entry.out = uint16(parsed)
	}
	return ports, nil
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
