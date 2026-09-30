package workspaceexec

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/workspaceartifacts"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// ControllerAgent combines observed process identity with complete file export.
type ControllerAgent interface {
	ProcessAgent
	workspaceartifacts.Bundler
}

// ControllerOptions supply existing server services and explicit cleanup policy.
type ControllerOptions struct {
	Database database.Store
	Clock    quartz.Clock
	// TickerClock schedules scans; Clock supplies lifecycle timestamps.
	TickerClock      quartz.Clock
	Logger           slog.Logger
	DialAgent        func(context.Context, uuid.UUID) (ControllerAgent, func(), error)
	FileCache        *files.Cache
	UsageChecker     func() wsbuilder.UsageChecker
	Pubsub           pubsub.Pubsub
	DeploymentValues *codersdk.DeploymentValues
	Experiments      codersdk.Experiments
	// AuthorizeCollection applies current deployment restrictions before reading files.
	AuthorizeCollection func(context.Context) error
	AllowDeletion       bool
	Interval            time.Duration
}

// Controller reconciles durable work independently of client connections.
type Controller struct {
	ControllerOptions
	once             sync.Once
	cancel           context.CancelFunc
	waiter           quartz.Waiter
	scanAfter        uuid.UUID
	maintenanceAfter uuid.UUID
}

// NewController configures reconciliation. Interval bounds retry polling and is
// not a lease, execution deadline, artifact retention, or licensing default.
func NewController(options ControllerOptions) *Controller {
	if options.Clock == nil {
		options.Clock = quartz.NewReal()
	}
	if options.TickerClock == nil {
		options.TickerClock = options.Clock
	}
	if options.Interval <= 0 {
		options.Interval = 10 * time.Second
	}
	return &Controller{ControllerOptions: options}
}

// Start begins autonomous reconciliation on the polling clock.
func (c *Controller) Start(ctx context.Context) {
	c.once.Do(func() {
		ctx, c.cancel = context.WithCancel(ctx)
		c.waiter = c.TickerClock.TickerFunc(ctx, c.Interval, func() error {
			//nolint:gocritic // The autonomous controller reconciles internal durable state.
			c.scan(dbauthz.AsSystemRestricted(ctx))
			return nil
		}, "workspace-execution-controller")
	})
}

// Close waits for reconciliation to stop.
func (c *Controller) Close() {
	if c.cancel != nil {
		c.cancel()
		_ = c.waiter.Wait()
	}
}

func (c *Controller) scan(ctx context.Context) {
	var sessions []database.WorkspaceExecutionSession
	for _, lane := range []struct {
		priority bool
		after    *uuid.UUID
	}{{true, &c.scanAfter}, {false, &c.maintenanceAfter}} {
		page, err := c.Database.GetReconciliableWorkspaceExecutionSessions(ctx, database.GetReconciliableWorkspaceExecutionSessionsParams{
			AfterID: *lane.after, Now: c.Clock.Now(), Priority: lane.priority,
			ObserveBefore: c.Clock.Now().Add(-c.Interval),
		})
		if err != nil {
			c.Logger.Warn(ctx, "list workspace execution sessions", slog.Error(err))
			return
		}
		if len(page) < 100 {
			*lane.after = uuid.Nil
		} else {
			*lane.after = page[len(page)-1].ID
		}
		sessions = append(sessions, page...)
	}
	// Independent sessions must not queue behind unreachable agents. A bounded
	// pool also limits concurrent preservation and control-plane requests.
	jobs := make(chan database.WorkspaceExecutionSession, len(sessions))
	for _, session := range sessions {
		jobs <- session
	}
	close(jobs)
	var workers sync.WaitGroup
	for range min(16, len(sessions)) {
		workers.Go(func() {
			for session := range jobs {
				if ctx.Err() != nil {
					return
				}
				err := c.Reconcile(ctx, session.ID)
				if err != nil && !xerrors.Is(err, ErrSessionChanged) {
					c.Logger.Warn(ctx, "reconcile workspace execution", slog.F("session_id", session.ID), slog.Error(err))
				}
			}
		})
	}
	workers.Wait()
}

// Reconcile advances one persisted session without retrying uncertain starts.
func (c *Controller) Reconcile(ctx context.Context, id uuid.UUID) error {
	parent := ctx
	ctx, cancelObservation := context.WithTimeout(parent, 30*time.Second)
	defer cancelObservation()
	session, err := c.Database.GetWorkspaceExecutionSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if session.State == "completed" || !session.WorkspaceID.Valid {
		return nil
	}
	receipts, err := c.Database.GetPendingWorkspaceExecutionReceipts(ctx, session.ID)
	if err != nil {
		return err
	}
	observedDue := receiptsDue(receipts, c.Clock.Now().Add(-c.Interval))
	if session.NextRetryAt.Valid && session.NextRetryAt.Time.After(c.Clock.Now()) && !observedDue {
		workspace, err := c.Database.GetWorkspaceByID(ctx, session.WorkspaceID.UUID)
		if err != nil {
			return err
		}
		if !maintenanceRetry(session) || !workspace.Deleted {
			return nil
		}
	}
	if session.State == "deleting" {
		return c.observeDeletion(ctx, session)
	}
	for _, receipt := range receipts {
		dialCtx, cancelDial := context.WithTimeout(ctx, 5*time.Second)
		agent, release, dialErr := c.DialAgent(dialCtx, receipt.AgentID)
		cancelDial()
		if dialErr != nil {
			err = dialErr
		} else {
			if receipt.Deadline.Valid && !receipt.Deadline.Time.After(c.Clock.Now()) {
				_, err = Cancel(ctx, c.Database, agent, receipt, 0, c.Clock.Now())
			} else {
				_, _, err = Observe(ctx, c.Database, agent, receipt, 0, c.Clock.Now())
			}
			if release != nil {
				release()
			}
		}
		if err != nil {
			// A timed-out network attempt must still rotate its durable place
			// in the bounded observation queue. Terminal receipts stay final.
			recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_, persistErr := recordObservation(recordCtx, c.Database, receipt, "unknown", nil, err.Error(), c.Clock.Now())
			cancel()
			c.Logger.Debug(ctx, "execution remains unsettled", slog.F("execution_id", receipt.ID), slog.Error(err))
			if persistErr != nil {
				c.Logger.Warn(ctx, "persist unsettled execution", slog.F("execution_id", receipt.ID), slog.Error(persistErr))
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	claimed := false
	err = c.Database.InTx(func(tx database.Store) error {
		current, workspace, err := lockSession(ctx, tx, session)
		if err != nil {
			return err
		}
		if current.Revision != session.Revision || current.State != session.State || current.ActorID != session.ActorID {
			return ErrSessionChanged
		}
		// Recheck scheduling under the same lock as the state mutation. A
		// replica that already advanced the retry time suppresses duplicate work.
		if current.NextRetryAt.Valid && current.NextRetryAt.Time.After(c.Clock.Now()) &&
			(!maintenanceRetry(current) || !workspace.Deleted) &&
			(!observedDue || current.NextRetryAt != session.NextRetryAt) {
			pending, err := tx.GetPendingWorkspaceExecutionReceipts(ctx, current.ID)
			if err != nil || !receiptsDue(pending, c.Clock.Now().Add(-c.Interval)) {
				return err
			}
		}
		if workspace.Deleted {
			return c.retireDeletedWorkspace(ctx, tx, current, workspace)
		}
		if current.Retained {
			return c.parkSession(ctx, tx, current, "workspace is retained")
		}
		if !c.AllowDeletion {
			return c.parkSession(ctx, tx, current, "automatic workspace execution cleanup is disabled")
		}
		if current.LeaseExpiresAt.After(c.Clock.Now()) {
			return c.scheduleSession(ctx, tx, current, current.State, "", min(max(c.Interval, 5*time.Minute), current.LeaseExpiresAt.Sub(c.Clock.Now())))
		}
		if !current.Disposable && current.State == "preserved" {
			return c.parkSession(ctx, tx, current, "results are preserved; this session does not own workspace deletion")
		}
		_, protection, err := otherSessionProtection(ctx, tx, current, c.Clock.Now())
		if err != nil {
			return err
		}
		if protection != "" {
			return c.deferSession(ctx, tx, current, current.State, protection)
		}
		pending, err := tx.HasPendingWorkspaceExecutionReceiptsByWorkspaceID(ctx, current.WorkspaceID)
		if err != nil {
			return err
		}
		if pending {
			return c.deferSession(ctx, tx, current, current.State, "execution is running or its outcome is unknown")
		}
		busy, err := tx.HasBusyWorkspaceExecutionChats(ctx, current.WorkspaceID)
		if err != nil {
			return err
		}
		if busy {
			return c.deferSession(ctx, tx, current, current.State, "chat work is active or queued")
		}
		next := current
		next.State = "preserving"
		next.Error = ""
		next.NextRetryAt = sql.NullTime{}
		if current.State == "active" {
			next.Revision++
		}
		session, err = saveSession(ctx, tx, current, next, c.Clock.Now())
		claimed = err == nil
		return err
	}, nil)
	if err != nil || !claimed {
		return err
	}
	cancelObservation()
	// Collection has its own budget matching the agent's bundle timeout. Each phase
	// remains bounded by the caller's deadline and server shutdown cancellation.
	collectionCtx, cancelCollection := context.WithTimeout(parent, 5*time.Minute)
	_, err = workspaceartifacts.Preserve(collectionCtx, c.Database, controllerBundler(func(ctx context.Context, request workspacesdk.BundleFilesRequest) ([]byte, error) {
		return c.collect(ctx, session, request)
	}), session, c.Clock.Now())
	cancelCollection()
	if err != nil {
		return c.fail(parent, session, "preservation_failed", err)
	}
	ctx, cancelFinalization := context.WithTimeout(parent, 30*time.Second)
	defer cancelFinalization()
	session, err = c.commitPreservedSession(ctx, session)
	if err != nil {
		return err
	}
	if !session.Disposable {
		return nil
	}
	if err := c.deleteWorkspace(ctx, session); err != nil {
		if xerrors.Is(err, ErrSessionChanged) {
			return err
		}
		return c.fail(ctx, session, "deletion_failed", err)
	}
	return nil
}

// Keep these wake conditions aligned with the scheduler query. Activity can
// bypass quiet maintenance; ordinary observation and failure retry stay bounded.
func maintenanceRetry(session database.WorkspaceExecutionSession) bool {
	switch session.State {
	case "preserving", "preservation_failed", "deleting", "deletion_failed":
		return false
	default:
		return session.NextRetryAt.Valid && session.NextRetryAt.Time.Sub(session.UpdatedAt) >= 5*time.Minute
	}
}

func receiptsDue(receipts []database.WorkspaceExecutionReceipt, before time.Time) bool {
	for _, receipt := range receipts {
		if !receipt.UpdatedAt.After(before) {
			return true
		}
	}
	return false
}

type controllerBundler func(context.Context, workspacesdk.BundleFilesRequest) ([]byte, error)

func (f controllerBundler) BundleFiles(ctx context.Context, request workspacesdk.BundleFilesRequest) ([]byte, error) {
	return f(ctx, request)
}

func (c *Controller) collect(ctx context.Context, session database.WorkspaceExecutionSession, request workspacesdk.BundleFilesRequest) ([]byte, error) {
	if c.AuthorizeCollection != nil {
		if err := c.AuthorizeCollection(ctx); err != nil {
			return nil, err
		}
	}
	var declarations struct {
		ResultAgentID   uuid.UUID `json:"result_agent_id"`
		ResultAgentName string    `json:"result_agent_name"`
	}
	if err := json.Unmarshal(session.Declarations, &declarations); err != nil {
		return nil, err
	}
	agents, err := c.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, session.WorkspaceID.UUID)
	if err != nil {
		return nil, err
	}
	id := declarations.ResultAgentID
	if declarations.ResultAgentName != "" {
		for _, agent := range agents {
			if agent.Name == declarations.ResultAgentName {
				if id != uuid.Nil {
					return nil, xerrors.New("declared result agent name is ambiguous")
				}
				id = agent.ID
			}
		}
		if id == uuid.Nil {
			return nil, xerrors.New("declared result agent name is unavailable")
		}
	}
	if id == uuid.Nil {
		if len(agents) != 1 {
			return nil, xerrors.New("result agent is unavailable or ambiguous")
		}
		id = agents[0].ID
	}
	found := false
	for _, agent := range agents {
		if agent.ID == id {
			found = true
			break
		}
	}
	if !found {
		return nil, xerrors.New("declared result agent is not in the latest workspace build")
	}
	agent, release, err := c.DialAgent(ctx, id)
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer release()
	}
	return agent.BundleFiles(ctx, request)
}

func (c *Controller) deferSession(ctx context.Context, tx database.Store, current database.WorkspaceExecutionSession, state, detail string) error {
	delay := c.Interval
	if state == "preservation_failed" || state == "deletion_failed" {
		delay *= time.Duration(1 << min(current.AttemptCount, 6))
	}
	return c.scheduleSession(ctx, tx, current, state, detail, delay)
}

func (c *Controller) parkSession(ctx context.Context, tx database.Store, current database.WorkspaceExecutionSession, detail string) error {
	if current.State == "preservation_failed" || current.State == "deletion_failed" {
		return c.deferSession(ctx, tx, current, current.State, detail)
	}
	// Keep this maintenance threshold aligned with the scheduler query.
	return c.scheduleSession(ctx, tx, current, current.State, detail, max(c.Interval, 5*time.Minute))
}

func (c *Controller) scheduleSession(ctx context.Context, tx database.Store, current database.WorkspaceExecutionSession, state, detail string, delay time.Duration) error {
	if current.State == state && current.Error == detail && current.NextRetryAt.Valid && current.NextRetryAt.Time.After(c.Clock.Now()) {
		return nil
	}
	next := current
	next.State = state
	next.Error = detail
	now := dbtime.Time(c.Clock.Now())
	next.NextRetryAt = sql.NullTime{Time: now.Add(delay), Valid: true}
	_, err := saveSession(ctx, tx, current, next, now)
	return err
}

func (c *Controller) deferFailure(ctx context.Context, tx database.Store, current database.WorkspaceExecutionSession, state, detail string) error {
	current.AttemptCount++
	return c.deferSession(ctx, tx, current, state, detail)
}

func (c *Controller) fail(ctx context.Context, expected database.WorkspaceExecutionSession, state string, cause error) error {
	// Persist the observed failure even when collection exhausted its request deadline.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return c.Database.InTx(func(tx database.Store) error {
		current, _, err := lockSession(ctx, tx, expected)
		if err != nil {
			return err
		}
		if current.Revision != expected.Revision || current.State != expected.State {
			return ErrSessionChanged
		}
		return c.deferFailure(ctx, tx, current, state, cause.Error())
	}, nil)
}
