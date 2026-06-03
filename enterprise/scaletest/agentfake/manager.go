package agentfake

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// ExternalAgentClient is the subset of *codersdk.Client the Manager
// uses to enumerate external-agent workspaces under a template and
// fetch each agent's auth token. *codersdk.Client satisfies this
// interface, so production callers pass their client directly; tests
// substitute a fake without standing up a real coderd.
type ExternalAgentClient interface {
	Workspaces(ctx context.Context, filter codersdk.WorkspaceFilter) (codersdk.WorkspacesResponse, error)
	WorkspaceExternalAgentCredentials(ctx context.Context, workspaceID uuid.UUID, agentName string) (codersdk.ExternalAgentCredentials, error)
}

const (
	enumeratePageSize        = 100
	maxEnumerateRetries      = 5
	initialEnumerateBackoff  = 1 * time.Second
	maxEnumerateRetryBackoff = 5 * time.Second
)

// TokenInfo is a single workspace-agent auth token retrieved for a coder external agent, along with the identifying
// metadata needed to report the agent in metrics and logs.
type TokenInfo struct {
	WorkspaceID   uuid.UUID
	WorkspaceName string
	AgentID       uuid.UUID
	AgentName     string
	Token         string
}

// ManagerOptions configures a Manager. Authentication is supplied via the *codersdk.Client passed to NewManager rather
// than here, so the CLI / caller can construct the client with whatever session token (operator-issued, admin,
// template-admin) suits its deployment.
type ManagerOptions struct {
	// Template restricts enumeration to workspaces of the given template name. Required.
	Template string
	// Owner restricts enumeration to workspaces owned by the given user. Optional; if empty, all owners are included.
	Owner string
}

// Manager supervises a set of fake Agents in one process. It enumerates the agents it owns from coderd at Run time
// (via coder_external_agent tokens on workspaces matching opts.Template), then opens a dRPC stream per agent and keeps
// them connected until ctx is canceled.
type Manager struct {
	coderURL *url.URL
	client   ExternalAgentClient
	logger   slog.Logger
	opts     ManagerOptions

	mu     sync.Mutex
	agents []*Agent
}

// NewManager returns an Agent Manager. The provided client must already be authenticated with sufficient privilege
// to list workspaces by template and to call the enterprise-only WorkspaceExternalAgentCredentials endpoint
// (template-admin or higher; FeatureWorkspaceExternalAgent must be enabled). coderURL is the URL the spawned
// fake agents will dial.
func NewManager(coderURL *url.URL, client ExternalAgentClient, logger slog.Logger, opts ManagerOptions) *Manager {
	return &Manager{
		coderURL: coderURL,
		client:   client,
		logger:   logger,
		opts:     opts,
	}
}

// Run enumerates the Manager's external agents from coderd, constructs one Agent per token, and runs the "fake agent"
// routines all until ctx is canceled or any Agent returns a non-context error.
// Enumeration is retried with exponential backoff for transient errors (network failures, 5xx, 429).
// Auth/permission/license/template-not-found errors (401, 403, 404) are treated as fatal.
// Run blocks until ctx is canceled, an Agent fails irrecoverably, or enumeration permanently fails.
func (m *Manager) Run(ctx context.Context) error {
	if m.opts.Template == "" {
		return xerrors.New("invalid manager options: Template is required")
	}

	tokens, err := m.enumerateWithRetry(ctx)
	if err != nil {
		return xerrors.Errorf("enumerate external agents: %w", err)
	}

	agents := make([]*Agent, 0, len(tokens))
	for i, ti := range tokens {
		agents = append(agents, NewAgent(m.coderURL, ti.Token,
			m.logger.Named("agent-"+strconv.Itoa(i))))
	}
	m.mu.Lock()
	m.agents = agents
	m.mu.Unlock()

	eg, egCtx := errgroup.WithContext(ctx)
	for _, a := range agents {
		eg.Go(func() error {
			return a.Run(egCtx)
		})
	}
	err = eg.Wait()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

// Close stops every Agent constructed during Run. Safe to call any
// number of times.
func (m *Manager) Close() {
	for _, a := range m.agents {
		a.Close()
	}
}

// enumerateWithRetry calls EnumerateExternalAgents with exponential backoff on transient failures.
// Fatal failures (auth, permission, missing template) exit immediately.
func (m *Manager) enumerateWithRetry(ctx context.Context) ([]TokenInfo, error) {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = initialEnumerateBackoff
	b.MaxInterval = maxEnumerateRetryBackoff
	bkoff := backoff.WithContext(backoff.WithMaxRetries(b, maxEnumerateRetries), ctx)

	var tokens []TokenInfo
	// for attempt := 0; attempt <= maxEnumerateRetries; attempt++ {
	err := backoff.Retry(func() error {
		var retryErr error
		tokens, retryErr = m.EnumerateExternalAgents(ctx)
		if retryErr == nil {
			return nil
		}
		if IsFatalEnumerationError(retryErr) {
			m.logger.Warn(ctx, "enumeration failed, will retry", slog.Error(retryErr))
			return backoff.Permanent(retryErr)
		}
		return retryErr
	}, bkoff)
	if err != nil {
		return nil, xerrors.Errorf("enumeration exhausted retries: %w", err)
	}
	return tokens, nil
}

// EnumerateExternalAgents asks coderd for the list of workspaces matching the configured template, walks each
// workspace's latest build for agents on builds with HasExternalAgent=true, and returns the auth tokens for every
// external agent. Per-agent credential failures are logged and skipped; a non-nil error is returned only if the
// workspace listing itself fails.
func (m *Manager) EnumerateExternalAgents(ctx context.Context) ([]TokenInfo, error) {
	var workspaces []codersdk.Workspace
	filter := codersdk.WorkspaceFilter{
		Template: m.opts.Template,
		Owner:    m.opts.Owner,
		Limit:    enumeratePageSize,
	}
	for {
		page, err := m.client.Workspaces(ctx, filter)
		if err != nil {
			return nil, xerrors.Errorf("list workspaces (offset=%d): %w", filter.Offset, err)
		}
		workspaces = append(workspaces, page.Workspaces...)
		if len(page.Workspaces) < filter.Limit {
			break
		}
		filter.Offset += len(page.Workspaces)
	}

	tokens := make([]TokenInfo, 0, len(workspaces))
	for _, ws := range workspaces {
		// The credentials endpoint requires WorkspaceBuild.HasExternalAgent=true (see
		// enterprise/coderd/workspaceagents.go:48). Skip workspaces whose latest build
		// doesn't carry the flag rather than 404 our way through every workspace in coderd.
		if ws.LatestBuild.HasExternalAgent == nil || !*ws.LatestBuild.HasExternalAgent {
			continue
		}
		for _, res := range ws.LatestBuild.Resources {
			for _, agent := range res.Agents {
				creds, err := m.client.WorkspaceExternalAgentCredentials(ctx, ws.ID, agent.Name)
				if err != nil {
					m.logger.Warn(ctx, "fetch external-agent credentials",
						slog.F("workspace_id", ws.ID),
						slog.F("workspace_name", ws.Name),
						slog.F("agent_name", agent.Name),
						slog.Error(err))
					continue
				}
				tokens = append(tokens, TokenInfo{
					WorkspaceID:   ws.ID,
					WorkspaceName: ws.Name,
					AgentID:       agent.ID,
					AgentName:     agent.Name,
					Token:         creds.AgentToken,
				})
			}
		}
	}
	return tokens, nil
}

// IsFatalEnumerationError reports whether err from a coderd API call indicates an unrecoverable misconfiguration that
// retrying will not fix: missing/invalid session token, insufficient permissions, missing license feature, or a template
// that does not exist.
// All other errors (network blips, 429, 5xx) are treated as transient and can be retried.
func IsFatalEnumerationError(err error) bool {
	if err == nil {
		return false
	}
	sdkErr, ok := codersdk.AsError(err)
	if !ok {
		return false
	}

	switch sdkErr.StatusCode() {
	case http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusBadRequest:
		return true
	}
	return false
}
