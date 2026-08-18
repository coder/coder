package cli

import (
	"context"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent/confine"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/serpent"
)

const (
	// #nosec G101, this is an environment variable name, not a credential.
	envSandboxAgentToken = "CODER_SANDBOX_AGENT_TOKEN"
	// #nosec G101, this is an environment variable name, not a credential.
	envSandboxPolicyToken = "CODER_AGENT_TOKEN"
	envSandboxImage       = "CODER_SANDBOX_IMAGE"
	envSandboxName        = "CODER_SANDBOX_NAME"
	envSandboxCPUs        = "CODER_SANDBOX_CPUS"
	envSandboxMemoryMiB   = "CODER_SANDBOX_MEMORY_MIB"
	envSandboxCacheDir    = "CODER_SANDBOX_CACHE_DIR"
	envSandboxStateDir    = "CODER_SANDBOX_STATE_DIR"

	sandboxShutdownTimeout = time.Minute
)

type agentSandboxInstance interface {
	Close(context.Context) error
}

type agentSandboxDeps struct {
	goos            string
	goarch          string
	executablePath  func() (string, error)
	userConfigDir   func() (string, error)
	newPolicyClient func(*url.URL, string, slog.Logger) confine.PolicyClient
	startMicroVM    func(context.Context, confine.MicroVMOptions) (agentSandboxInstance, error)
	shutdownTimeout time.Duration
}

func defaultAgentSandboxDeps() agentSandboxDeps {
	return agentSandboxDeps{
		goos:           runtime.GOOS,
		goarch:         runtime.GOARCH,
		executablePath: os.Executable,
		userConfigDir:  os.UserConfigDir,
		newPolicyClient: func(agentURL *url.URL, token string, logger slog.Logger) confine.PolicyClient {
			client := agentsdk.New(agentURL, agentsdk.WithFixedToken(token))
			client.SDK.SetLogger(logger)
			return client
		},
		startMicroVM: func(ctx context.Context, options confine.MicroVMOptions) (agentSandboxInstance, error) {
			return confine.StartEmbeddedMicroVM(ctx, options)
		},
		shutdownTimeout: sandboxShutdownTimeout,
	}
}

func agentSandbox(agentAuth *AgentAuth) *serpent.Command {
	return agentSandboxWithDeps(agentAuth, defaultAgentSandboxDeps())
}

func agentSandboxWithDeps(agentAuth *AgentAuth, deps agentSandboxDeps) *serpent.Command {
	var (
		policyToken string
		image       string
		name        string
		cpus        int64
		memoryMiB   int64
		cacheDir    string
		stateDir    string
	)
	if deps.shutdownTimeout == 0 {
		deps.shutdownTimeout = sandboxShutdownTimeout
	}
	configDir, _ := deps.userConfigDir()
	microVMDir := filepath.Join(configDir, "coder-ai", "microvm")

	cmd := &serpent.Command{
		Use:   "sandbox",
		Short: "Boots an embedded microVM and runs a Coder agent inside it.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			if deps.goos != "linux" || deps.goarch != "amd64" {
				return xerrors.Errorf("agent sandbox is supported only on linux/amd64, got %s/%s", deps.goos, deps.goarch)
			}
			if err := validateAgentSandboxOptions(agentAuth.agentToken, policyToken, &agentAuth.agentURL, cpus, memoryMiB); err != nil {
				return err
			}
			if strings.TrimSpace(cacheDir) == "" || strings.TrimSpace(stateDir) == "" {
				return xerrors.New("--cache-dir and --state-dir are required")
			}

			coderBinaryPath, err := deps.executablePath()
			if err != nil {
				return xerrors.Errorf("resolve Coder executable: %w", err)
			}
			// The command is daemonized by template scripts with stderr
			// redirected to a log file, so a human-readable sink on stderr
			// is the only observability this process has.
			logger := inv.Logger.
				AppendSinks(sloghuman.Sink(inv.Stderr)).
				Leveled(slog.LevelInfo).
				Named("agent-sandbox")
			destination, err := confine.ControlChannelDestinationOptions(&agentAuth.agentURL)
			if err != nil {
				return xerrors.Errorf("configure agent sandbox control channel: %w", err)
			}
			logger.Info(inv.Context(), "injected AI sandbox egress allowance",
				slog.F("host", destination.AlwaysAllowHost),
				slog.F("port", destination.AlwaysAllowPort),
				slog.F("reason", "platform control channel"),
			)
			client := deps.newPolicyClient(&agentAuth.agentURL, policyToken, logger)
			policyMonitor, err := confine.NewPolicyMonitor(confine.PolicyMonitorOptions{
				Client:    client,
				Logger:    logger,
				AccessURL: &agentAuth.agentURL,
			})
			if err != nil {
				return xerrors.Errorf("create AI egress policy monitor: %w", err)
			}

			ctx, stopNotify := signal.NotifyContext(inv.Context(), StopSignals...)
			defer stopNotify()
			policy, fetchErr := policyMonitor.Start(ctx)
			if fetchErr != nil {
				logger.Error(ctx, "ai egress policy fetch failed, continuing with deny-default policy",
					slog.Error(fetchErr),
				)
			} else {
				logger.Info(ctx, "initial AI egress policy fetched",
					slog.F("revision", policy.Revision),
					slog.F("rule_count", len(policy.Rules)),
				)
			}
			logger.Warn(ctx, "egress decisions are logged locally and are not retained server side")

			sandbox, err := deps.startMicroVM(ctx, confine.MicroVMOptions{
				Image:           image,
				Name:            name,
				CacheDir:        cacheDir,
				StateDir:        stateDir,
				CPUs:            int(cpus),
				MemoryMiB:       int(memoryMiB),
				CoderBinaryPath: coderBinaryPath,
				AgentURL:        agentAuth.agentURL.String(),
				AgentToken:      agentAuth.agentToken,
				Policy:          policyMonitor.Engine(),
				Destination:     destination,
				Event: func(event confine.NetworkEvent) {
					logger.Info(context.Background(), "ai sandbox egress decision",
						slog.F("action", event.Action),
						slog.F("protocol", event.Protocol),
						slog.F("host", event.Host),
						slog.F("port", event.Port),
						slog.F("policy_revision", event.PolicyRevision),
					)
				},
				Progress: func(message string) {
					logger.Info(ctx, message)
				},
			})
			if err != nil {
				return xerrors.Errorf("boot agent sandbox microVM: %w", err)
			}

			<-ctx.Done()
			logger.Info(context.Background(), "shutting down agent sandbox microVM")
			closeCtx, closeCancel := context.WithTimeout(context.Background(), deps.shutdownTimeout)
			closeErr := sandbox.Close(closeCtx)
			closeCancel()
			if closeErr != nil {
				return xerrors.Errorf("close agent sandbox microVM: %w", closeErr)
			}
			logger.Info(context.Background(), "agent sandbox microVM stopped")
			return nil
		},
	}
	cmd.Options = append(cmd.Options,
		serpent.Option{
			Name:        "Sandbox Agent Token",
			Description: "Agent authentication token used by the guest agent.",
			Flag:        "agent-token",
			Env:         envSandboxAgentToken,
			Value:       serpent.StringOf(&agentAuth.agentToken),
		},
		serpent.Option{
			Name:        "Sandbox Agent URL",
			Description: "URL used by the guest agent and host policy client to access Coder.",
			Flag:        "agent-url",
			Env:         envAgentURL,
			Value:       serpent.URLOf(&agentAuth.agentURL),
		},
		serpent.Option{
			Name:        "Policy Token",
			Description: "Host agent token used to fetch and watch egress policy.",
			Flag:        "policy-token",
			Env:         envSandboxPolicyToken,
			Value:       serpent.StringOf(&policyToken),
		},
		serpent.Option{
			Name:        "Sandbox Image",
			Description: "OCI image booted inside the microVM.",
			Flag:        "image",
			Env:         envSandboxImage,
			Default:     "ubuntu:24.04",
			Value:       serpent.StringOf(&image),
		},
		serpent.Option{
			Name:        "Sandbox Name",
			Description: "Name for the embedded microVM.",
			Flag:        "name",
			Env:         envSandboxName,
			Default:     "sandbox",
			Value:       serpent.StringOf(&name),
		},
		serpent.Option{
			Name:        "Sandbox CPUs",
			Description: "Virtual CPU count for the embedded microVM.",
			Flag:        "cpus",
			Env:         envSandboxCPUs,
			Default:     "1",
			Value:       serpent.Int64Of(&cpus),
		},
		serpent.Option{
			Name:        "Sandbox Memory",
			Description: "Guest memory for the embedded microVM, in MiB.",
			Flag:        "memory-mib",
			Env:         envSandboxMemoryMiB,
			Default:     "1024",
			Value:       serpent.Int64Of(&memoryMiB),
		},
		serpent.Option{
			Name:        "Sandbox Cache Directory",
			Description: "Directory for downloaded microVM runtime and image artifacts.",
			Flag:        "cache-dir",
			Env:         envSandboxCacheDir,
			Default:     filepath.Join(microVMDir, "cache"),
			Value:       serpent.StringOf(&cacheDir),
		},
		serpent.Option{
			Name:        "Sandbox State Directory",
			Description: "Directory for embedded microVM runtime state.",
			Flag:        "state-dir",
			Env:         envSandboxStateDir,
			Default:     filepath.Join(microVMDir, "state"),
			Value:       serpent.StringOf(&stateDir),
		},
	)
	return cmd
}

func validateAgentSandboxOptions(agentToken, policyToken string, agentURL *url.URL, cpus, memoryMiB int64) error {
	hasAgentToken := strings.TrimSpace(agentToken) != ""
	hasAgentURL := agentURL != nil && agentURL.Scheme != "" && agentURL.Hostname() != ""
	if !hasAgentToken || !hasAgentURL {
		return xerrors.New("--agent-token or CODER_SANDBOX_AGENT_TOKEN and --agent-url or CODER_AGENT_URL are required together")
	}
	if strings.TrimSpace(policyToken) == "" {
		return xerrors.New("--policy-token or CODER_AGENT_TOKEN is required")
	}
	if cpus <= 0 {
		return xerrors.New("--cpus must be positive")
	}
	if memoryMiB <= 0 {
		return xerrors.New("--memory-mib must be positive")
	}
	return nil
}
