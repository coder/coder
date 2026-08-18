package confine

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
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
	// EnvAISandboxMicroVM declares the embedded microVM sandbox mode.
	EnvAISandboxMicroVM = "CODER_AI_SANDBOX_MICROVM"
	// EnvAISandboxImage declares the embedded microVM OCI image.
	EnvAISandboxImage = "CODER_AI_SANDBOX_IMAGE"
	// EnvAISandboxMemoryMiB declares embedded microVM memory in MiB.
	EnvAISandboxMemoryMiB = "CODER_AI_SANDBOX_MEMORY_MIB"
	// EnvAISandboxCPUs declares the embedded microVM virtual CPU count.
	EnvAISandboxCPUs = "CODER_AI_SANDBOX_CPUS"
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
	// EnvAISandboxPolicyFile declares the coder/sandbox runtime network policy
	// file written by the controller.
	EnvAISandboxPolicyFile = "CODER_AI_SANDBOX_POLICY_FILE"
	// EnvAISandboxPolicyReloadScript declares the command run after each
	// successful policy file write.
	EnvAISandboxPolicyReloadScript = "CODER_AI_SANDBOX_POLICY_RELOAD_SCRIPT"

	// EnvAIAgentURL is the coderd URL passed to the sandbox create script.
	EnvAIAgentURL = "CODER_AI_AGENT_URL"
	// EnvAIAgentToken is the child agent token passed to the create script.
	// #nosec G101, this is an environment variable name, not a credential.
	EnvAIAgentToken = "CODER_AI_AGENT_TOKEN"
	// EnvAISessionToken is the scoped AI token passed only to the create script.
	EnvAISessionToken = "CODER_AI_SESSION_TOKEN"
	// EnvSandboxID is the sandbox lifecycle ID passed to sandbox scripts.
	EnvSandboxID = "CODER_SANDBOX_ID"

	defaultSandboxName          = "sandbox"
	defaultSandboxProxyAddress  = "127.0.0.1:0"
	defaultSandboxMicroVMImage  = "ubuntu:24.04"
	defaultSandboxMicroVMMemory = 1024
	defaultSandboxMicroVMCPUs   = 1
	createScriptTimeout         = 5 * time.Minute
	destroyScriptTimeout        = time.Minute
	sessionReportAttempts       = 3
	sessionReportWindow         = 20 * time.Second
)

var sandboxNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// SandboxMode selects how the agent creates and enforces an AI sandbox.
type SandboxMode string

const (
	// SandboxModeCreateScript runs an administrator-provided sandbox runtime.
	SandboxModeCreateScript SandboxMode = "create-script"
	// SandboxModeProxy runs only the parent-side proxy for a declared sandbox.
	SandboxModeProxy SandboxMode = "proxy"
	// SandboxModeMicroVM runs the platform's embedded microVM runtime.
	SandboxModeMicroVM SandboxMode = "microvm"
)

// SandboxDeclaration describes one interim environment-declared AI sandbox.
type SandboxDeclaration struct {
	Mode               SandboxMode
	CreateScript       string
	DestroyScript      string
	PolicyFile         string
	PolicyReloadScript string
	Name               string
	EgressEnforcement  codersdk.AISandboxEgressEnforcement
	MicroVMImage       string
	MicroVMMemoryMiB   int
	MicroVMCPUs        int
	// ProxyAddress defaults to parent loopback. Isolation technologies that
	// cannot reach parent loopback, such as Docker, must declare a
	// bridge-reachable parent address.
	ProxyAddress string
}

// SandboxDeclarationFromEnv parses the interim environment declaration used
// until the Terraform sandbox resource is available.
func SandboxDeclarationFromEnv(lookup func(string) (string, bool)) (SandboxDeclaration, error) {
	createScript, _ := lookup(EnvAISandboxCreateScript)
	hasCreateScript := strings.TrimSpace(createScript) != ""

	microVM := false
	if value, ok := lookup(EnvAISandboxMicroVM); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return SandboxDeclaration{}, xerrors.Errorf("parse %s: %w", EnvAISandboxMicroVM, err)
		}
		microVM = enabled
	}
	if microVM && hasCreateScript {
		return SandboxDeclaration{}, xerrors.Errorf(
			"%s and %s are mutually exclusive", EnvAISandboxMicroVM, EnvAISandboxCreateScript,
		)
	}

	mode := SandboxMode("")
	switch {
	case microVM:
		mode = SandboxModeMicroVM
	case hasCreateScript:
		mode = SandboxModeCreateScript
	default:
		// Proxy-only mode is used when an ai_bound coder_agent and ordinary
		// coder_script own the sandbox while this controller owns its proxy.
		enable, ok := lookup(EnvAIEgressProxy)
		if !ok || strings.TrimSpace(enable) == "" {
			return SandboxDeclaration{}, xerrors.Errorf(
				"one of %s, %s, or %s is required",
				EnvAISandboxMicroVM, EnvAISandboxCreateScript, EnvAIEgressProxy,
			)
		}
		mode = SandboxModeProxy
	}

	declaration := SandboxDeclaration{
		Mode:              mode,
		CreateScript:      createScript,
		Name:              defaultSandboxName,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementNone,
		ProxyAddress:      defaultSandboxProxyAddress,
	}
	if mode == SandboxModeMicroVM {
		declaration.MicroVMImage = defaultSandboxMicroVMImage
		declaration.MicroVMMemoryMiB = defaultSandboxMicroVMMemory
		declaration.MicroVMCPUs = defaultSandboxMicroVMCPUs
		if value, ok := lookup(EnvAISandboxImage); ok && strings.TrimSpace(value) != "" {
			declaration.MicroVMImage = strings.TrimSpace(value)
		}
		if value, ok := lookup(EnvAISandboxMemoryMiB); ok && strings.TrimSpace(value) != "" {
			memory, err := parseSandboxPositiveInt(EnvAISandboxMemoryMiB, value)
			if err != nil {
				return SandboxDeclaration{}, err
			}
			declaration.MicroVMMemoryMiB = memory
		}
		if value, ok := lookup(EnvAISandboxCPUs); ok && strings.TrimSpace(value) != "" {
			cpus, err := parseSandboxPositiveInt(EnvAISandboxCPUs, value)
			if err != nil {
				return SandboxDeclaration{}, err
			}
			declaration.MicroVMCPUs = cpus
		}
	}
	if value, ok := lookup(EnvAISandboxDestroyScript); ok {
		declaration.DestroyScript = value
	}
	if value, ok := lookup(EnvAISandboxPolicyFile); ok {
		declaration.PolicyFile = value
	}
	if value, ok := lookup(EnvAISandboxPolicyReloadScript); ok {
		declaration.PolicyReloadScript = value
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

func parseSandboxPositiveInt(name, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, xerrors.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
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

type runningMicroVMSandbox interface {
	Close(context.Context) error
}

type startMicroVMFunc func(context.Context, MicroVMOptions) (runningMicroVMSandbox, error)

// SandboxController reconciles one sandbox and owns its proxy and session.
type SandboxController struct {
	options          SandboxControllerOptions
	scriptEnvHandoff *sandboxScriptEnvHandoff
	startMicroVM     startMicroVMFunc
	coderBinaryPath  string
	microVMCacheDir  string
	microVMStateDir  string
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

	if options.Declaration.Mode == "" {
		if strings.TrimSpace(options.Declaration.CreateScript) == "" {
			options.Declaration.Mode = SandboxModeProxy
		} else {
			options.Declaration.Mode = SandboxModeCreateScript
		}
	}
	controller := &SandboxController{
		options:          options,
		scriptEnvHandoff: newSandboxScriptEnvHandoff(),
		startMicroVM: func(ctx context.Context, options MicroVMOptions) (runningMicroVMSandbox, error) {
			return StartEmbeddedMicroVM(ctx, options)
		},
	}
	switch options.Declaration.Mode {
	case SandboxModeCreateScript:
		if strings.TrimSpace(options.Declaration.CreateScript) == "" {
			return nil, xerrors.New("AI sandbox create script is required in create-script mode")
		}
		if options.Declaration.ProxyAddress == "" {
			return nil, xerrors.New("AI sandbox proxy address is required")
		}
	case SandboxModeProxy:
		if strings.TrimSpace(options.Declaration.CreateScript) != "" {
			return nil, xerrors.New("AI sandbox create script is not valid in proxy mode")
		}
		if options.Declaration.ProxyAddress == "" {
			return nil, xerrors.New("AI sandbox proxy address is required")
		}
	case SandboxModeMicroVM:
		if strings.TrimSpace(options.Declaration.CreateScript) != "" {
			return nil, xerrors.Errorf(
				"%s and %s are mutually exclusive", EnvAISandboxMicroVM, EnvAISandboxCreateScript,
			)
		}
		if err := validateSandboxMicroVMPlatform(runtime.GOOS, runtime.GOARCH); err != nil {
			return nil, err
		}
		if !embeddedMicroVMNamePattern.MatchString(options.Declaration.Name) {
			return nil, xerrors.Errorf("invalid embedded microVM name %q", options.Declaration.Name)
		}
		if strings.TrimSpace(options.Declaration.MicroVMImage) == "" {
			return nil, xerrors.New("AI sandbox microVM image is required")
		}
		if options.Declaration.MicroVMMemoryMiB <= 0 {
			return nil, xerrors.New("AI sandbox microVM memory must be positive")
		}
		if options.Declaration.MicroVMCPUs <= 0 {
			return nil, xerrors.New("AI sandbox microVM CPU count must be positive")
		}

		executablePath, err := os.Executable()
		if err != nil {
			return nil, xerrors.Errorf("resolve Coder executable: %w", err)
		}
		executablePath, err = filepath.Abs(executablePath)
		if err != nil {
			return nil, xerrors.Errorf("resolve Coder executable path: %w", err)
		}
		controller.coderBinaryPath, err = filepath.EvalSymlinks(executablePath)
		if err != nil {
			return nil, xerrors.Errorf("resolve Coder executable symlinks: %w", err)
		}
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, xerrors.Errorf("resolve user config directory: %w", err)
		}
		microVMDir := filepath.Join(configDir, "coder-ai", "microvm")
		controller.microVMCacheDir = filepath.Join(microVMDir, "cache")
		controller.microVMStateDir = filepath.Join(microVMDir, "state")
	default:
		return nil, xerrors.Errorf("invalid AI sandbox mode %q", options.Declaration.Mode)
	}
	if options.Execer == nil {
		controller.options.Execer = agentexec.DefaultExecer
	}
	return controller, nil
}

func validateSandboxMicroVMPlatform(goos, goarch string) error {
	if goos != "linux" || goarch != "amd64" {
		return xerrors.Errorf(
			"AI sandbox microVM mode is supported only on linux/amd64, got %s/%s",
			goos, goarch,
		)
	}
	return nil
}

// WaitForProxy waits until the proxy is listening and script environment is ready.
func (c *SandboxController) WaitForProxy(ctx context.Context) error {
	return c.scriptEnvHandoff.wait(ctx)
}

// ScriptExtraEnv returns the exec-time environment for workspace scripts.
func (c *SandboxController) ScriptExtraEnv() []string {
	return c.scriptEnvHandoff.environment()
}

// Run reconciles the declared sandbox, starts its selected runtime, and retains
// policy delivery and audit reporting until ctx is canceled.
func (c *SandboxController) Run(ctx context.Context) (retErr error) {
	defer func() {
		err := retErr
		if err == nil {
			err = xerrors.New("AI sandbox controller stopped before sandbox networking was ready")
		}
		c.scriptEnvHandoff.complete(nil, err)
	}()

	mode := c.options.Declaration.Mode
	proxyOnly := mode == SandboxModeProxy
	microVMMode := mode == SandboxModeMicroVM

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

	var afterPolicyUpdate func(codersdk.AIEgressPolicy)
	if !microVMMode {
		afterPolicyUpdate = func(policy codersdk.AIEgressPolicy) {
			c.exportSandboxPolicy(ctx, sandbox.ID, policy)
		}
	}
	policyMonitor, err := NewPolicyMonitor(PolicyMonitorOptions{
		Client:      c.options.Client,
		Logger:      c.options.Logger,
		AccessURL:   c.options.AccessURL,
		AfterUpdate: afterPolicyUpdate,
	})
	if err != nil {
		return xerrors.Errorf("create AI egress policy monitor: %w", err)
	}
	policy, fetchErr := policyMonitor.Start(ctx)
	if fetchErr != nil {
		c.signalDegraded("AI egress policy fetch failed; started deny-all (degraded)", fetchErr)
	}
	engine := policyMonitor.Engine()
	if !microVMMode {
		c.exportSandboxPolicy(ctx, sandbox.ID, policy)
	}

	sessionID := uuid.New()
	batcher := newEventBatcher(c.options.Client, c.options.Logger, sessionID, eventQueueSize)
	var proxy *Proxy
	if !microVMMode {
		proxy, err = ListenProxyWithOptions(
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
	} else {
		// The embedded guest can reach only hostvm's private gateway listener,
		// which serves the in-process proxy. A parent loopback listener would be
		// unreachable from the guest and would not enforce any additional path.
		c.options.Logger.Info(ctx, "using embedded microVM gateway proxy",
			slog.F("sandbox_id", sandbox.ID),
		)
	}

	runCtx, stop := context.WithCancel(ctx)
	batcherDone := make(chan struct{})
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

	var microVMSandbox runningMicroVMSandbox
	switch mode {
	case SandboxModeCreateScript:
		createEnv := c.createScriptEnvironment(sandbox, proxy.Addr().String())
		if err := c.runScript(ctx, "create", c.options.Declaration.CreateScript, createEnv, createScriptTimeout); err != nil {
			c.signalDegraded("AI sandbox create script failed; sandbox remains active (degraded)", err)
		}
	case SandboxModeMicroVM:
		bootCtx, bootCancel := context.WithTimeout(ctx, createScriptTimeout)
		microVMSandbox, err = c.startMicroVM(bootCtx, MicroVMOptions{
			Image:           c.options.Declaration.MicroVMImage,
			Name:            c.options.Declaration.Name,
			CacheDir:        c.microVMCacheDir,
			StateDir:        c.microVMStateDir,
			CPUs:            c.options.Declaration.MicroVMCPUs,
			MemoryMiB:       c.options.Declaration.MicroVMMemoryMiB,
			CoderBinaryPath: c.coderBinaryPath,
			AgentURL:        c.options.AccessURL.String(),
			AgentToken:      sandbox.AgentToken,
			SessionToken:    sandbox.SessionToken,
			Policy:          engine,
			Destination: DestinationOptions{
				AllowPrivateHost: c.options.AccessURL.Hostname(),
			},
			Event: batcher.Add,
		})
		bootCancel()
		if err != nil {
			c.signalDegraded("AI sandbox microVM boot failed; sandbox remains active (degraded)", err)
		} else {
			c.options.Logger.Info(ctx, "embedded AI sandbox microVM started",
				slog.F("sandbox_id", sandbox.ID),
				slog.F("name", c.options.Declaration.Name),
			)
		}
		c.scriptEnvHandoff.complete([]string{EnvSandboxID + "=" + sandbox.ID.String()}, nil)
	}

	<-ctx.Done()

	switch mode {
	case SandboxModeCreateScript:
		if strings.TrimSpace(c.options.Declaration.DestroyScript) != "" {
			destroyEnv := c.destroyScriptEnvironment(sandbox, proxy.Addr().String())
			if err := c.runScript(context.Background(), "destroy", c.options.Declaration.DestroyScript, destroyEnv, destroyScriptTimeout); err != nil {
				c.signalDegraded("AI sandbox destroy script failed (degraded)", err)
			}
		}
	case SandboxModeMicroVM:
		if microVMSandbox != nil {
			closeMicroVMCtx, closeMicroVMCancel := context.WithTimeout(context.Background(), destroyScriptTimeout)
			err := microVMSandbox.Close(closeMicroVMCtx)
			closeMicroVMCancel()
			if err != nil {
				c.signalDegraded("AI sandbox microVM shutdown failed (degraded)", err)
			}
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
	if proxy != nil {
		if err := proxy.Close(); err != nil {
			c.options.Logger.Warn(context.Background(), "close AI sandbox egress proxy", slog.Error(err))
		}
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
	env = setEnv(env, EnvSandboxID, sandbox.ID.String())
	if policyFile := strings.TrimSpace(c.options.Declaration.PolicyFile); policyFile != "" {
		env = setEnv(env, EnvAISandboxPolicyFile, policyFile)
	}
	return env
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

func watchPolicy(
	ctx context.Context,
	client PolicyClient,
	logger slog.Logger,
	engine *PolicyEngine,
	afterUpdate func(codersdk.AIEgressPolicy),
) {
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
			logger.Info(ctx, "applied AI egress policy update",
				slog.F("revision", policy.Revision),
				slog.F("rule_count", len(policy.Rules)),
			)
			if afterUpdate != nil {
				afterUpdate(policy)
			}
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
