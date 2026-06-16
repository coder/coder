package agentfake

import (
	"context"
	"errors"
	"net/http"
	"net/url"
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

// ExternalAgentClient is the subset of *codersdk.Client the Manager uses to
// resolve the template/owner the operator named on the command line and to
// poll the workspace count gate. The actual external-agent auth tokens are
// fetched in-process via a direct database query (see
// GetExternalAgentTokensByTemplateID), not via this client. *codersdk.Client
// satisfies this interface, so production callers pass their client
// directly; tests substitute a fake without standing up a real coderd.
type ExternalAgentClient interface {
	User(ctx context.Context, userIdent string) (codersdk.User, error)
	Template(ctx context.Context, id uuid.UUID) (codersdk.Template, error)
	TemplatesByOrganization(ctx context.Context, orgID uuid.UUID) ([]codersdk.Template, error)
	Workspaces(ctx context.Context, filter codersdk.WorkspaceFilter) (codersdk.WorkspacesResponse, error)
}

const (
	maxEnumerateRetries        = 5
	initialEnumerateBackoff    = 1 * time.Second
	maxEnumerateRetryBackoff   = 5 * time.Second
	workspaceCountPollInterval = 5 * time.Second
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
	// Metrics collectors. Optional; nil disables metric reporting.
	Metrics *Metrics
	// ExpectedAgents, when non-zero, causes Run to poll until the workspace
	// count is within [ExpectedAgents-Tolerance, ExpectedAgents+Tolerance]
	// before enumerating.
	ExpectedAgents          int64
	ExpectedAgentsTolerance int64
	// A zero ConnectionReportInterval or ConnectionReportDuration disables
	// synthetic SSH connection reporting.
	ConnectionReportInterval time.Duration
	ConnectionReportDuration time.Duration
	// Clock is used for the workspace-count polling interval.
	// Defaults to the real clock; override in tests with quartz.NewMock.
	Clock quartz.Clock
}

// Manager supervises a set of fake Agents in one process. It enumerates the agents it owns from coderd at Run time
// (via coder_external_agent tokens on workspaces matching opts.Template), then opens a dRPC stream per agent and keeps
// them connected until ctx is canceled.
type Manager struct {
	coderURL *url.URL
	client   ExternalAgentClient
	db       database.Store
	logger   slog.Logger
	opts     ManagerOptions

	// templateID + ownerID are resolved once during Run from opts.Template /
	// opts.Owner (names). ownerID stays uuid.Nil when opts.Owner is empty, which
	// the GetExternalAgentTokensByTemplateID query treats as "match any owner".
	templateID uuid.UUID
	ownerID    uuid.UUID

	mu     sync.Mutex
	agents []*Agent
}

// NewManager returns an Agent Manager. The provided client must already be
// authenticated with sufficient privilege to list workspaces, look up the
// configured template, and (when --owner is set) look up the named user
// (template-admin or higher). db must be a database.Store connected to the
// same Postgres database as the target coderd; it is used to bulk-fetch
// external-agent tokens for the enumerated workspaces. coderURL is the URL
// the spawned fake agents will dial.
func NewManager(logger slog.Logger, coderURL *url.URL, client ExternalAgentClient, db database.Store, opts ManagerOptions) *Manager {
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	return &Manager{
		coderURL: coderURL,
		client:   client,
		db:       db,
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

	if m.opts.ExpectedAgents > 0 {
		if err := m.waitForWorkspaceCount(ctx); err != nil {
			return xerrors.Errorf("waiting for workspaces: %w", err)
		}
	}

	if err := m.ResolveTemplateAndOwner(ctx); err != nil {
		return xerrors.Errorf("resolve template/owner: %w", err)
	}

	tokens, err := m.enumerateWithRetry(ctx)
	if err != nil {
		return xerrors.Errorf("enumerate external agents: %w", err)
	}

	numAgents := len(tokens)

	// Buffered so a stalled collector can never block any agent's send.
	firstConnectCh := make(chan time.Duration, numAgents)

	agents := make([]*Agent, 0, numAgents)
	for i, ti := range tokens {
		agents = append(agents, NewAgent(
			m.logger.Named("agent-"+strconv.Itoa(i)),
			m.coderURL, ti.Token,
			WithMetrics(m.opts.Metrics),
			WithFirstConnect(firstConnectCh),
			WithConnectionReports(m.opts.ConnectionReportInterval, m.opts.ConnectionReportDuration)))
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

	// Bound to Run's lifetime rather than egCtx so the collector can't
	// outlive Run when every agent returns nil (errgroup never cancels
	// egCtx on clean shutdown).
	collectorCtx, cancelCollector := context.WithCancel(ctx)
	defer cancelCollector()
	go func() {
		durations := collectFirstConnect(collectorCtx, firstConnectCh, numAgents)
		if len(durations) == 0 {
			return
		}
		// Mean is order-independent and is computed before the sort so the
		// dependency between the two percentile calls and sortedness is
		// localized here.
		mean := meanDuration(durations)
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		m.logger.Info(collectorCtx, "all agents connected",
			slog.F("count", len(durations)),
			slog.F("mean", mean),
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

// collectFirstConnect drains ch until expected values arrive or ctx is
// canceled. The single shared channel ensures one stalled agent cannot
// hold up reports from the others.
func collectFirstConnect(ctx context.Context, ch <-chan time.Duration, expected int) []time.Duration {
	if expected <= 0 {
		return nil
	}
	durations := make([]time.Duration, 0, expected)
	for len(durations) < expected {
		select {
		case d := <-ch:
			durations = append(durations, d)
		case <-ctx.Done():
			return durations
		}
	}
	return durations
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

// EnumerateExternalAgents bulk-fetches the auth tokens for every external agent on a running workspace of the
// configured template (optionally filtered by owner) via a single direct Postgres query. resolveTemplateAndOwner
// must have been called once before any invocation; Run handles that, but tests that call this method directly
// must do the same.
func (m *Manager) EnumerateExternalAgents(ctx context.Context) ([]TokenInfo, error) {
	start := time.Now()
	m.logger.Info(ctx, "enumerating external-agent workspaces",
		slog.F("template", m.opts.Template),
		slog.F("template_id", m.templateID),
		slog.F("owner", m.opts.Owner))

	// AsSystemRestricted is required because GetExternalAgentTokensByTemplateID
	// is gated by dbauthz on ResourceSystem read. This code path runs in the
	// agentfake scaletest manager pod, which holds a direct Postgres connection
	// and acts as a trusted system caller; the security boundary here is Postgres
	// authn (the coder-db-url secret), not a coder session token.
	// nolint:gocritic
	rows, err := m.db.GetExternalAgentTokensByTemplateID(dbauthz.AsSystemRestricted(ctx), database.GetExternalAgentTokensByTemplateIDParams{
		TemplateID: m.templateID,
		OwnerID:    m.ownerID,
	})
	if err != nil {
		return nil, xerrors.Errorf("fetch external-agent tokens: %w", err)
	}

	tokens := make([]TokenInfo, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, TokenInfo{
			WorkspaceID:   row.WorkspaceID,
			WorkspaceName: row.WorkspaceName,
			AgentID:       row.AgentID,
			AgentName:     row.AgentName,
			Token:         row.AgentToken.String(),
		})
	}
	m.logger.Info(ctx, "enumerated external-agent workspaces",
		slog.F("template", m.opts.Template),
		slog.F("template_id", m.templateID),
		slog.F("owner", m.opts.Owner),
		slog.F("tokens", len(tokens)),
		slog.F("duration", time.Since(start)))
	return tokens, nil
}

// ResolveTemplateAndOwner looks up the configured template name (and, when set,
// owner username) once and caches the resulting UUIDs on the Manager so that
// EnumerateExternalAgents can issue a single by-ID DB query per cycle.
// Run calls this automatically; tests that exercise EnumerateExternalAgents
// directly must call it themselves first.
//
// Template resolution walks every organization the calling user belongs to,
// matching scaletest convention (see cli.parseTemplate). Owner resolution is
// skipped when opts.Owner is empty; the cached uuid.Nil is interpreted by the
// underlying query as "match workspaces of any owner".
func (m *Manager) ResolveTemplateAndOwner(ctx context.Context) error {
	me, err := m.client.User(ctx, codersdk.Me)
	if err != nil {
		return xerrors.Errorf("get current user: %w", err)
	}
	tpl, err := parseTemplate(ctx, m.client, me.OrganizationIDs, m.opts.Template)
	if err != nil {
		return xerrors.Errorf("resolve template %q: %w", m.opts.Template, err)
	}
	m.templateID = tpl.ID

	if m.opts.Owner != "" {
		owner, err := m.client.User(ctx, m.opts.Owner)
		if err != nil {
			return xerrors.Errorf("resolve owner %q: %w", m.opts.Owner, err)
		}
		m.ownerID = owner.ID
	}
	return nil
}

// parseTemplate is duplicated from cli/exp_scaletest.go (AGPL) to avoid
// exporting an internal helper as part of that package's public API for the
// sole benefit of this enterprise consumer. Keep behavior in sync with the
// original: accept either a UUID or a template name, search all of the user's
// organizations for a name match.
func parseTemplate(ctx context.Context, client ExternalAgentClient, organizationIDs []uuid.UUID, template string) (tpl codersdk.Template, err error) {
	if id, err := uuid.Parse(template); err == nil && id != uuid.Nil {
		tpl, err = client.Template(ctx, id)
		if err != nil {
			return tpl, xerrors.Errorf("get template by ID %q: %w", template, err)
		}
	} else {
		// List templates in all orgs until we find a match.
	orgLoop:
		for _, orgID := range organizationIDs {
			tpls, err := client.TemplatesByOrganization(ctx, orgID)
			if err != nil {
				return tpl, xerrors.Errorf("list templates in org %q: %w", orgID, err)
			}
			for _, t := range tpls {
				if t.Name == template {
					tpl = t
					break orgLoop
				}
			}
		}
	}
	if tpl.ID == uuid.Nil {
		return tpl, xerrors.Errorf("could not find template %q in any organization", template)
	}
	return tpl, nil
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

// meanDuration returns the mean of d, or zero if d is empty.
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

// percentileDuration returns the p-th percentile (0-100) using nearest-rank.
// Expects d to be sorted ascending; callers sort once before invoking this
// for multiple percentiles.
func percentileDuration(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	idx := int(p/100*float64(len(d))+0.5) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(d) {
		idx = len(d) - 1
	}
	return d[idx]
}
