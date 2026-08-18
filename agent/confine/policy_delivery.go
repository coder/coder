package confine

import (
	"context"
	"io"
	"net/url"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// PolicyClient fetches and watches the materialized egress policy for an agent.
type PolicyClient interface {
	AIEgressPolicy(context.Context) (codersdk.AIEgressPolicy, error)
	WatchAIEgressPolicy(context.Context) (<-chan codersdk.AIEgressPolicy, io.Closer, error)
}

// PolicyMonitorOptions configures policy bootstrap and live updates.
type PolicyMonitorOptions struct {
	Client      PolicyClient
	Logger      slog.Logger
	AccessURL   *url.URL
	AfterUpdate func(codersdk.AIEgressPolicy)
}

// ControlChannelDestinationOptions returns the exact host and effective port
// allowance required for the agent's platform control channel.
func ControlChannelDestinationOptions(accessURL *url.URL) (DestinationOptions, error) {
	if accessURL == nil || accessURL.Hostname() == "" {
		return DestinationOptions{}, xerrors.New("AI egress policy access URL is required")
	}
	port, err := accessURLPort(accessURL)
	if err != nil {
		return DestinationOptions{}, err
	}
	host := normalizeHost(accessURL.Hostname())
	return DestinationOptions{
		AllowPrivateHost: host,
		AlwaysAllowHost:  host,
		AlwaysAllowPort:  port,
	}, nil
}

// PolicyMonitor owns a fail-closed policy engine and its live policy watcher.
type PolicyMonitor struct {
	options PolicyMonitorOptions
	engine  *PolicyEngine
}

// NewPolicyMonitor constructs a deny-default engine for the control-plane URL.
func NewPolicyMonitor(options PolicyMonitorOptions) (*PolicyMonitor, error) {
	if options.Client == nil {
		return nil, xerrors.New("AI egress policy client is required")
	}
	destination, err := ControlChannelDestinationOptions(options.AccessURL)
	if err != nil {
		return nil, err
	}
	return &PolicyMonitor{
		options: options,
		engine:  NewPolicyEngine(destination.AlwaysAllowHost, destination.AlwaysAllowPort),
	}, nil
}

// Engine returns the shared policy engine updated by Start.
func (m *PolicyMonitor) Engine() *PolicyEngine {
	return m.engine
}

// Start fetches the initial policy and starts the SSE watcher. The engine stays
// deny-default when the initial fetch fails.
func (m *PolicyMonitor) Start(ctx context.Context) (codersdk.AIEgressPolicy, error) {
	fetchCtx, fetchCancel := context.WithTimeout(ctx, fetchTimeout)
	policy, fetchErr := m.options.Client.AIEgressPolicy(fetchCtx)
	fetchCancel()
	if fetchErr == nil {
		m.engine.Update(policy)
		m.options.Logger.Info(ctx, "applied AI egress policy",
			slog.F("revision", policy.Revision),
			slog.F("rule_count", len(policy.Rules)),
		)
	}
	go watchPolicy(ctx, m.options.Client, m.options.Logger, m.engine, m.options.AfterUpdate)
	return policy, fetchErr
}
