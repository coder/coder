package agentfake

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	maxEnumerateRetries        = 5
	initialEnumerateBackoff    = 1 * time.Second
	maxEnumerateRetryBackoff   = 5 * time.Second
	workspaceCountPollInterval = 5 * time.Second
)

// TokenInfo is a single workspace-agent auth token retrieved for a coder external agent, along with the identifying
// metadata needed to report the agent in metrics and logs.
type TokenInfo struct {
	WorkspaceID uuid.UUID
	AgentID     uuid.UUID
	AgentName   string
	Token       string
}

// ManagerOptions configures a Manager. Authentication is supplied via the *codersdk.Client passed to NewManager rather
// than here, so the CLI / caller can construct the client with whatever session token (operator-issued, admin,
// template-admin) suits its deployment.
type ManagerOptions struct {
	// Template restricts enumeration to workspaces of the given template name. Required.
	Template string
	// Owner restricts enumeration to workspaces owned by the given user. Optional; if empty, all owners are included.
	Owner string
	// Metrics collectors. Optional; nil disables metric reporting.
	Metrics *Metrics
	// ExpectedAgents, when non-zero, causes Run to poll until the workspace
	// count is within [ExpectedAgents-Tolerance, ExpectedAgents+Tolerance]
	// before enumerating.
	ExpectedAgents          int64
	ExpectedAgentsTolerance int64
	// Clock is used for the workspace-count polling interval.
	// Defaults to the real clock; override in tests with quartz.NewMock.
	Clock quartz.Clock
}

// Manager supervises a set of fake Agents in one process. It enumerates the agents it owns from coderd at Run time
// (via coder_external_agent tokens on workspaces matching opts.Template), then opens a dRPC stream per agent and keeps
// them connected until ctx is canceled.
type Manager struct {
	client *codersdk.Client
	db     database.Store
	logger slog.Logger
	opts   ManagerOptions

	mu     sync.Mutex
	agents []*Agent
}

// NewManager returns an Agent Manager. The provided client must already be authenticated with sufficient privilege
// to list workspaces by template (template-admin or higher). db must be a database.Store connected to the same
// Postgres database as the target coderd; it is used to bulk-fetch external-agent tokens for the enumerated workspaces.
func NewManager(client *codersdk.Client, db database.Store, logger slog.Logger, opts ManagerOptions) *Manager {
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	return &Manager{
		client: client,
		db:     db,
		logger: logger,
		opts:   opts,
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

	if m.opts.ExpectedAgents > 0 {
		if err := m.waitForWorkspaceCount(ctx); err != nil {
			return xerrors.Errorf("waiting for workspaces: %w", err)
		}
	}

	tokens, err := m.enumerateWithRetry(ctx)
	if err != nil {
		return xerrors.Errorf("enumerate external agents: %w", err)
	}

	agents := make([]*Agent, 0, len(tokens))
	for i, ti := range tokens {
		agents = append(agents, NewAgent(
			m.logger.Named("agent-"+strconv.Itoa(i)),
			m.client.URL, ti.Token, m.opts.Metrics))
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

	// Log connection-time stats once all agents have reported their first connect.
	// Run in a goroutine so agents that never connect don't block Run from returning.
	go func() {
		durations := make([]time.Duration, 0, len(agents))
		for _, a := range agents {
			select {
			case d := <-a.firstConnectDuration:
				durations = append(durations, d)
			case <-egCtx.Done():
				return
			}
		}
		if len(durations) == 0 {
			return
		}
		m.logger.Info(egCtx, "all agents connected",
			slog.F("count", len(durations)),
			slog.F("mean", meanDuration(durations)),
			slog.F("pct_ninety_five", percentileDuration(durations, 95)),
			slog.F("pct_ninety_nine", percentileDuration(durations, 99)),
		)
	}()

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
	start := time.Now()
	m.logger.Info(ctx, "enumerating external-agent workspaces",
		slog.F("template", m.opts.Template),
		slog.F("owner", m.opts.Owner))

	page, err := m.client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Template: m.opts.Template,
		Owner:    m.opts.Owner,
		Limit:    0,
	})
	if err != nil {
		return nil, xerrors.Errorf("list workspaces: %w", err)
	}
	workspaces := page.Workspaces

	wsIDs := make([]uuid.UUID, 0, len(workspaces))
	for _, ws := range workspaces {
		wsIDs = append(wsIDs, ws.ID)
	}

	// AsSystemRestricted is required because GetExternalAgentTokensByWorkspaceIDs
	// is gated by dbauthz on ResourceSystem read. This code path runs in the
	// agentfake scaletest manager pod, which holds a direct Postgres connection
	// and acts as a trusted system caller; the security boundary here is Postgres
	// authn (the coder-db-url secret), not a coder session token.
	// nolint:gocritic
	rows, err := m.db.GetExternalAgentTokensByWorkspaceIDs(dbauthz.AsSystemRestricted(ctx), wsIDs)
	if err != nil {
		return nil, xerrors.Errorf("fetch external-agent tokens: %w", err)
	}

	tokens := make([]TokenInfo, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, TokenInfo{
			WorkspaceID: row.WorkspaceID,
			AgentID:     row.AgentID,
			AgentName:   row.AgentName,
			Token:       row.AgentToken.String(),
		})
	}
	m.logger.Info(ctx, "enumerated external-agent workspaces",
		slog.F("template", m.opts.Template),
		slog.F("owner", m.opts.Owner),
		slog.F("workspaces", len(workspaces)),
		slog.F("tokens", len(tokens)),
		slog.F("duration", time.Since(start)))
	return tokens, nil
}

// waitForWorkspaceCount polls until the workspace count for the configured
// template is within [ExpectedAgents-Tolerance, ExpectedAgents+Tolerance].
// It uses limit=1 on each poll; the workspaces SQL query computes the total
// count in a CTE before applying LIMIT, so Count reflects the full result set
// regardless of page size.
func (m *Manager) waitForWorkspaceCount(ctx context.Context) error {
	lo := m.opts.ExpectedAgents - m.opts.ExpectedAgentsTolerance
	hi := m.opts.ExpectedAgents + m.opts.ExpectedAgentsTolerance

	// checkWorkspaceCount returns true if the current workspace count for the
	// template is within the expected tolerance range, or an error if the
	// workspaces endpoint fails.
	checkWorkspaceCount := func() (bool, error) {
		page, err := m.client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Template: m.opts.Template,
			Owner:    m.opts.Owner,
			Limit:    1,
		})
		if err != nil {
			return false, xerrors.Errorf("check workspace count: %w", err)
		}
		count := int64(page.Count)
		if count >= lo && count <= hi {
			m.logger.Info(ctx, "workspace count ready",
				slog.F("count", count),
				slog.F("expected", m.opts.ExpectedAgents),
				slog.F("tolerance", m.opts.ExpectedAgentsTolerance),
			)
			return true, nil
		}
		m.logger.Info(ctx, "waiting for workspaces",
			slog.F("count", count),
			slog.F("want_lo", lo),
			slog.F("want_hi", hi),
		)
		return false, nil
	}

	errDone := xerrors.New("done")
	var tickErr error
	waiter := m.opts.Clock.TickerFunc(ctx, workspaceCountPollInterval, func() error {
		done, err := checkWorkspaceCount()
		if err != nil {
			tickErr = err
			return err
		}
		if done {
			return errDone
		}
		return nil
	})
	if err := waiter.Wait(); err != nil && !errors.Is(err, errDone) {
		if tickErr != nil {
			return tickErr
		}
		return xerrors.Errorf("waiting for workspace count: %w", err)
	}
	return nil
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

// meanDuration returns the mean of d.
func meanDuration(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	var total time.Duration
	for _, v := range d {
		total += v
	}
	return total / time.Duration(len(d))
}

// percentileDuration returns the p-th percentile (0-100) using nearest-rank. Sorts d in place.
func percentileDuration(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	idx := int(p/100*float64(len(d))+0.5) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(d) {
		idx = len(d) - 1
	}
	return d[idx]
}
