package confine

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

const (
	// EnvAISandboxCreateScript declares the sandbox create command.
	EnvAISandboxCreateScript = "CODER_AI_SANDBOX_CREATE_SCRIPT"
	// EnvAIEgressProxy runs the egress proxy without a platform-managed
	// sandbox, for templates that declare the sandbox as an ai_bound
	// coder_agent plus an ordinary coder_script. Any non-empty value enables
	// it; it is ignored when a create script is declared.
	EnvAIEgressProxy = "CODER_AI_EGRESS_PROXY"
	// EnvAISandboxDestroyScript declares the optional sandbox destroy command.
	EnvAISandboxDestroyScript = "CODER_AI_SANDBOX_DESTROY_SCRIPT"
	// EnvAISandboxName declares the sandbox reconciliation name.
	EnvAISandboxName = "CODER_AI_SANDBOX_NAME"
	// EnvAISandboxEgressEnforcement declares the admin egress attestation.
	EnvAISandboxEgressEnforcement = "CODER_AI_SANDBOX_EGRESS_ENFORCEMENT"
	// EnvAISandboxProxyAddress declares the parent-side proxy listen address.
	EnvAISandboxProxyAddress = "CODER_AI_SANDBOX_PROXY_ADDRESS"

	// EnvAIAgentURL is the coderd URL passed to the sandbox create script.
	EnvAIAgentURL = "CODER_AI_AGENT_URL"
	// EnvAIAgentToken is the child agent token passed to the create script.
	// #nosec G101, this is an environment variable name, not a credential.
	EnvAIAgentToken = "CODER_AI_AGENT_TOKEN"
	// EnvAISessionToken is the scoped AI token passed only to the create script.
	EnvAISessionToken = "CODER_AI_SESSION_TOKEN"
	// EnvSandboxID is the sandbox lifecycle ID passed to sandbox scripts.
	EnvSandboxID = "CODER_SANDBOX_ID"

	defaultSandboxName         = "sandbox"
	defaultSandboxProxyAddress = "127.0.0.1:0"
	createScriptTimeout        = 5 * time.Minute
	destroyScriptTimeout       = time.Minute
	sessionReportAttempts      = 3
	sessionReportWindow        = 20 * time.Second
)

var sandboxNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// SandboxDeclaration describes one interim environment-declared AI sandbox.
type SandboxDeclaration struct {
	CreateScript      string
	DestroyScript     string
	Name              string
	EgressEnforcement codersdk.AISandboxEgressEnforcement
	// ProxyAddress defaults to parent loopback. Isolation technologies that
	// cannot reach parent loopback, such as Docker, must declare a
	// bridge-reachable parent address.
	ProxyAddress string
}

// SandboxDeclarationFromEnv parses the interim environment declaration used
// until the Terraform sandbox resource is available.
func SandboxDeclarationFromEnv(lookup func(string) (string, bool)) (SandboxDeclaration, error) {
	createScript, ok := lookup(EnvAISandboxCreateScript)
	if !ok || strings.TrimSpace(createScript) == "" {
		// Proxy-only mode. A template that declares its sandbox with an
		// ai_bound coder_agent and an ordinary coder_script does not hand the
		// agent a create script: the script runner owns the sandbox, and the
		// agent owns only the egress proxy. The proxy still has to be
		// listening before any script runs, so the controller is still what
		// starts it.
		if enable, ok := lookup(EnvAIEgressProxy); !ok || strings.TrimSpace(enable) == "" {
			return SandboxDeclaration{}, xerrors.Errorf(
				"%s or %s is required", EnvAISandboxCreateScript, EnvAIEgressProxy,
			)
		}
		createScript = ""
	}

	declaration := SandboxDeclaration{
		CreateScript:      createScript,
		Name:              defaultSandboxName,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementNone,
		ProxyAddress:      defaultSandboxProxyAddress,
	}
	if value, ok := lookup(EnvAISandboxDestroyScript); ok {
		declaration.DestroyScript = value
	}
	if value, ok := lookup(EnvAISandboxName); ok && value != "" {
		declaration.Name = value
	}
	if !sandboxNamePattern.MatchString(declaration.Name) {
		return SandboxDeclaration{}, xerrors.Errorf("invalid AI sandbox name %q", declaration.Name)
	}
	if value, ok := lookup(EnvAISandboxEgressEnforcement); ok && value != "" {
		declaration.EgressEnforcement = codersdk.AISandboxEgressEnforcement(value)
	}
	switch declaration.EgressEnforcement {
	case codersdk.AISandboxEgressEnforcementForced,
		codersdk.AISandboxEgressEnforcementAdvisory,
		codersdk.AISandboxEgressEnforcementNone:
	default:
		return SandboxDeclaration{}, xerrors.Errorf(
			"invalid AI sandbox egress enforcement %q", declaration.EgressEnforcement,
		)
	}
	if value, ok := lookup(EnvAISandboxProxyAddress); ok && value != "" {
		declaration.ProxyAddress = value
	}
	return declaration, nil
}

// SandboxClient is the parent-agent API used by the sandbox controller.
type SandboxClient interface {
	AgentClient
	CreateAISandbox(context.Context, agentsdk.CreateAISandboxRequest) (agentsdk.CreateAISandboxResponse, error)
	AISandboxes(context.Context) ([]agentsdk.AISandbox, error)
	DeleteAISandbox(context.Context, uuid.UUID) error
}

// SandboxControllerOptions configures an environment-declared sandbox.
type SandboxControllerOptions struct {
	Declaration SandboxDeclaration
	Client      SandboxClient
	Logger      slog.Logger
	LogDir      string
	AccessURL   *url.URL
	Execer      agentexec.Execer
}

// SandboxController reconciles one sandbox and owns its proxy and session.
type SandboxController struct {
	options          SandboxControllerOptions
	scriptEnvHandoff *sandboxScriptEnvHandoff
}

type sandboxScriptEnvHandoff struct {
	ready chan struct{}
	once  sync.Once
	mu    sync.RWMutex
	env   []string
	err   error
}

func newSandboxScriptEnvHandoff() *sandboxScriptEnvHandoff {
	return &sandboxScriptEnvHandoff{ready: make(chan struct{})}
}

func (e *sandboxScriptEnvHandoff) complete(env []string, err error) {
	e.once.Do(func() {
		e.mu.Lock()
		e.env = append([]string(nil), env...)
		e.err = err
		e.mu.Unlock()
		close(e.ready)
	})
}

func (e *sandboxScriptEnvHandoff) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.ready:
		e.mu.RLock()
		defer e.mu.RUnlock()
		return e.err
	}
}

func (e *sandboxScriptEnvHandoff) environment() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]string(nil), e.env...)
}

// NewSandboxController validates and constructs a sandbox controller.
func NewSandboxController(options SandboxControllerOptions) (*SandboxController, error) {
	if options.Client == nil {
		return nil, xerrors.New("AI sandbox agent client is required")
	}
	if options.AccessURL == nil || options.AccessURL.Hostname() == "" {
		return nil, xerrors.New("AI sandbox access URL is required")
	}
	if options.LogDir == "" {
		return nil, xerrors.New("AI sandbox log directory is required")
	}
	// An empty create script selects proxy-only mode; see
	// SandboxDeclarationFromEnv.
	if !sandboxNamePattern.MatchString(options.Declaration.Name) {
		return nil, xerrors.Errorf("invalid AI sandbox name %q", options.Declaration.Name)
	}
	switch options.Declaration.EgressEnforcement {
	case codersdk.AISandboxEgressEnforcementForced,
		codersdk.AISandboxEgressEnforcementAdvisory,
		codersdk.AISandboxEgressEnforcementNone:
	default:
		return nil, xerrors.Errorf(
			"invalid AI sandbox egress enforcement %q", options.Declaration.EgressEnforcement,
		)
	}
	if options.Declaration.ProxyAddress == "" {
		return nil, xerrors.New("AI sandbox proxy address is required")
	}
	if options.Execer == nil {
		options.Execer = agentexec.DefaultExecer
	}
	return &SandboxController{
		options:          options,
		scriptEnvHandoff: newSandboxScriptEnvHandoff(),
	}, nil
}

// WaitForProxy waits until the proxy is listening and script environment is ready.
func (c *SandboxController) WaitForProxy(ctx context.Context) error {
	return c.scriptEnvHandoff.wait(ctx)
}

// ScriptExtraEnv returns the exec-time environment for workspace scripts.
func (c *SandboxController) ScriptExtraEnv() []string {
	return c.scriptEnvHandoff.environment()
}

// Run reconciles the declared sandbox, executes its scripts, and retains the
// proxy and audit session until ctx is canceled.
func (c *SandboxController) Run(ctx context.Context) (retErr error) {
	defer func() {
		err := retErr
		if err == nil {
			err = xerrors.New("AI sandbox controller stopped before proxy was ready")
		}
		c.scriptEnvHandoff.complete(nil, err)
	}()
	// Proxy-only mode: the template owns the sandbox through an ai_bound
	// coder_agent and a coder_script, so there is no platform-managed sandbox
	// to reconcile or create. The controller contributes the egress proxy and
	// the policy it enforces, and nothing else.
	proxyOnly := strings.TrimSpace(c.options.Declaration.CreateScript) == ""

	var sandbox agentsdk.CreateAISandboxResponse
	if proxyOnly {
		// A local ID correlates this proxy's logs with the script that used
		// it. It is deliberately not a server-side sandbox record: see the
		// audit note where the session would otherwise be posted.
		sandbox = agentsdk.CreateAISandboxResponse{ID: uuid.New()}
	} else {
		if err := c.deleteStaleSandboxes(ctx); err != nil {
			return err
		}

		createCtx, createCancel := context.WithTimeout(ctx, fetchTimeout)
		created, err := c.options.Client.CreateAISandbox(createCtx, agentsdk.CreateAISandboxRequest{
			Name:              c.options.Declaration.Name,
			EgressEnforcement: c.options.Declaration.EgressEnforcement,
		})
		createCancel()
		if err != nil {
			return xerrors.Errorf("create AI sandbox: %w", err)
		}
		sandbox = created
	}
	switch {
	case proxyOnly:
		c.options.Logger.Info(ctx, "running AI egress proxy without a platform-managed sandbox",
			slog.F("correlation_id", sandbox.ID),
		)
	case sandbox.Reconciled:
		c.options.Logger.Info(ctx, "reconciled AI sandbox",
			slog.F("sandbox_id", sandbox.ID),
			slog.F("child_agent_id", sandbox.ChildAgentID),
			slog.F("name", c.options.Declaration.Name),
		)
	default:
		c.options.Logger.Info(ctx, "created AI sandbox",
			slog.F("sandbox_id", sandbox.ID),
			slog.F("child_agent_id", sandbox.ChildAgentID),
			slog.F("name", c.options.Declaration.Name),
		)
	}

	port, err := accessURLPort(c.options.AccessURL)
	if err != nil {
		return err
	}
	engine := NewPolicyEngine(c.options.AccessURL.Hostname(), port)
	fetchCtx, fetchCancel := context.WithTimeout(ctx, fetchTimeout)
	policy, fetchErr := c.options.Client.AIEgressPolicy(fetchCtx)
	fetchCancel()
	if fetchErr != nil {
		c.signalDegraded("AI egress policy fetch failed; started deny-all (degraded)", fetchErr)
	} else {
		engine.Update(policy)
		c.options.Logger.Info(ctx, "applied AI egress policy",
			slog.F("revision", policy.Revision),
			slog.F("rule_count", len(policy.Rules)),
		)
	}

	sessionID := uuid.New()
	batcher := newEventBatcher(c.options.Client, c.options.Logger, sessionID, eventQueueSize)
	proxy, err := ListenProxyWithOptions(
		c.options.Declaration.ProxyAddress,
		engine,
		batcher.Add,
		DestinationOptions{
			// The exact control-plane host may resolve to a private address in
			// local and on-prem deployments. It is policy-allowed implicitly,
			// and this exemption lets only that hostname pass destination range
			// validation; every other private destination remains denied.
			AllowPrivateHost: c.options.AccessURL.Hostname(),
		},
	)
	if err != nil {
		return xerrors.Errorf("start AI sandbox egress proxy: %w", err)
	}
	c.options.Logger.Info(ctx, "ai sandbox egress proxy started",
		slog.F("proxy_address", proxy.Addr().String()),
		slog.F("sandbox_id", sandbox.ID),
	)
	c.scriptEnvHandoff.complete([]string{
		EnvEgressProxy + "=" + proxy.Addr().String(),
		EnvSandboxID + "=" + sandbox.ID.String(),
	}, nil)

	runCtx, stop := context.WithCancel(ctx)
	batcherDone := make(chan struct{})
	go watchPolicy(runCtx, c.options.Client, c.options.Logger, engine)
	go func() {
		defer close(batcherDone)
		batcher.Run(runCtx, eventFlushPeriod)
	}()

	startedAt := time.Now()
	if !proxyOnly {
		c.postSession(runCtx, agentsdk.PostAISandboxSessionRequest{
			ID:                sessionID,
			ChildAgentID:      sandbox.ChildAgentID,
			EgressEnforcement: c.options.Declaration.EgressEnforcement,
			StartedAt:         startedAt,
		})
	} else {
		// KNOWN GAP. The session and network-event endpoints attribute a flow
		// through the reporting agent's CHILD, so they require the bound agent
		// to be a sub-agent of this one. A Terraform-declared ai_bound agent is
		// a sibling instead, with no parent_id, so its flows cannot be
		// attributed yet and are logged locally rather than retained server
		// side. Closing this needs a parent relationship between the host agent
		// and the declared agent, which is tracked as future work in
		// AI_AGENT_SANDBOX_SPEC.md.
		c.options.Logger.Warn(ctx, "egress events are not retained server side in proxy-only mode",
			slog.F("correlation_id", sandbox.ID),
		)
	}

	if !proxyOnly {
		createEnv := c.createScriptEnvironment(sandbox, proxy.Addr().String())
		if err := c.runScript(ctx, "create", c.options.Declaration.CreateScript, createEnv, createScriptTimeout); err != nil {
			c.signalDegraded("AI sandbox create script failed; sandbox remains active (degraded)", err)
		}
	}

	<-ctx.Done()

	if strings.TrimSpace(c.options.Declaration.DestroyScript) != "" {
		destroyEnv := c.destroyScriptEnvironment(sandbox, proxy.Addr().String())
		if err := c.runScript(context.Background(), "destroy", c.options.Declaration.DestroyScript, destroyEnv, destroyScriptTimeout); err != nil {
			c.signalDegraded("AI sandbox destroy script failed (degraded)", err)
		}
	}

	endedAt := time.Now()
	closeCtx, closeCancel := context.WithTimeout(context.Background(), sessionReportWindow)
	c.postSession(closeCtx, agentsdk.PostAISandboxSessionRequest{
		ID:                sessionID,
		ChildAgentID:      sandbox.ChildAgentID,
		EgressEnforcement: c.options.Declaration.EgressEnforcement,
		StartedAt:         startedAt,
		EndedAt:           &endedAt,
	})
	closeCancel()

	stop()
	<-batcherDone
	batcher.Flush()
	if err := proxy.Close(); err != nil {
		c.options.Logger.Warn(context.Background(), "close AI sandbox egress proxy", slog.Error(err))
	}
	return nil
}

func (c *SandboxController) deleteStaleSandboxes(ctx context.Context) error {
	listCtx, listCancel := context.WithTimeout(ctx, fetchTimeout)
	sandboxes, err := c.options.Client.AISandboxes(listCtx)
	listCancel()
	if err != nil {
		return xerrors.Errorf("list AI sandboxes: %w", err)
	}
	for _, sandbox := range sandboxes {
		if sandbox.Name == c.options.Declaration.Name {
			continue
		}
		deleteCtx, deleteCancel := context.WithTimeout(ctx, fetchTimeout)
		err := c.options.Client.DeleteAISandbox(deleteCtx, sandbox.ID)
		deleteCancel()
		if err != nil {
			return xerrors.Errorf("delete stale AI sandbox %q: %w", sandbox.Name, err)
		}
		c.options.Logger.Info(ctx, "deleted stale AI sandbox record",
			slog.F("sandbox_id", sandbox.ID),
			slog.F("child_agent_id", sandbox.ChildAgentID),
			slog.F("name", sandbox.Name),
		)
	}
	return nil
}

func (c *SandboxController) createScriptEnvironment(
	sandbox agentsdk.CreateAISandboxResponse,
	proxyAddress string,
) []string {
	env := c.scriptEnvironment(sandbox, proxyAddress)
	return setEnv(env, EnvAISessionToken, sandbox.SessionToken)
}

func (c *SandboxController) destroyScriptEnvironment(
	sandbox agentsdk.CreateAISandboxResponse,
	proxyAddress string,
) []string {
	return unsetEnv(c.scriptEnvironment(sandbox, proxyAddress), EnvAISessionToken)
}

func (c *SandboxController) scriptEnvironment(
	sandbox agentsdk.CreateAISandboxResponse,
	proxyAddress string,
) []string {
	env := append([]string(nil), os.Environ()...)
	env = setEnv(env, EnvAIAgentURL, c.options.AccessURL.String())
	env = setEnv(env, EnvAIAgentToken, sandbox.AgentToken)
	env = setEnv(env, EnvEgressProxy, proxyAddress)
	return setEnv(env, EnvSandboxID, sandbox.ID.String())
}

func (c *SandboxController) runScript(
	ctx context.Context,
	kind string,
	script string,
	env []string,
	timeout time.Duration,
) error {
	if err := os.MkdirAll(c.options.LogDir, 0o755); err != nil {
		return xerrors.Errorf("create AI sandbox log directory: %w", err)
	}
	logPath := filepath.Join(c.options.LogDir, "coder-ai-sandbox.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return xerrors.Errorf("open AI sandbox log: %w", err)
	}
	defer logFile.Close()

	scriptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := c.options.Execer.CommandContext(scriptCtx, "sh", "-c", script)
	command.Env = env
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Run(); err != nil {
		if scriptCtx.Err() != nil {
			return xerrors.Errorf("AI sandbox %s script: %w", kind, scriptCtx.Err())
		}
		return xerrors.Errorf("AI sandbox %s script: %w", kind, err)
	}
	return nil
}

func (c *SandboxController) postSession(ctx context.Context, request agentsdk.PostAISandboxSessionRequest) {
	backoff := time.Second
	var lastErr error
	for attempt := 1; attempt <= sessionReportAttempts; attempt++ {
		reportCtx, cancel := context.WithTimeout(ctx, reportTimeout)
		lastErr = c.options.Client.PostAISandboxSession(reportCtx, request)
		cancel()
		if lastErr == nil || isNotFound(lastErr) {
			return
		}
		if attempt == sessionReportAttempts || !waitBackoff(ctx, backoff) {
			break
		}
		backoff *= 2
	}
	c.options.Logger.Warn(context.Background(), "report AI sandbox session", slog.Error(lastErr))
}

func (c *SandboxController) signalDegraded(message string, cause error) {
	ctx := context.Background()
	c.options.Logger.Warn(ctx, message, slog.Error(cause))
	reportCtx, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()
	err := c.options.Client.PatchLogs(reportCtx, agentsdk.PatchLogs{
		Logs: []agentsdk.Log{{CreatedAt: time.Now(), Output: message, Level: codersdk.LogLevelWarn}},
	})
	if err != nil && !isNotFound(err) {
		c.options.Logger.Warn(ctx, "report degraded AI sandbox", slog.Error(err))
	}
}

func watchPolicy(ctx context.Context, client AgentClient, logger slog.Logger, engine *PolicyEngine) {
	backoff := time.Second
	for ctx.Err() == nil {
		policies, closer, err := client.WatchAIEgressPolicy(ctx)
		if err != nil {
			if !isNotFound(err) {
				logger.Warn(ctx, "watch AI egress policy", slog.Error(err))
			}
			if !waitBackoff(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		for policy := range policies {
			engine.Update(policy)
		}
		if closer != nil {
			_ = closer.Close()
		}
		if !waitBackoff(ctx, backoff) {
			return
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}
