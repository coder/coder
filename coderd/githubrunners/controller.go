package githubrunners

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/quartz"
)

// Options binds the controller to Coder's existing workspace lifecycle.
type Options struct {
	Database        database.Store
	Logger          slog.Logger
	Clock           quartz.Clock
	NewClient       func(database.GithubRunnerPool) (Client, error)
	CreateWorkspace func(context.Context, database.GithubRunnerPool, database.GithubRunner, string) (uuid.UUID, error)
	WorkspaceStatus func(context.Context, uuid.UUID) (string, error)
	DeleteWorkspace func(context.Context, database.GithubRunner) error
}

// Controller discovers pools and maintains one leased worker for each pool.
type Controller struct {
	Options
	done chan struct{}
}

// New starts polling and reconciliation until ctx is canceled.
func New(ctx context.Context, opts Options) *Controller {
	c := &Controller{Options: opts, done: make(chan struct{})}
	if c.Clock == nil {
		c.Clock = quartz.NewReal()
	}
	if c.NewClient == nil {
		c.NewClient = NewClient
	}
	go c.run(dbauthz.AsGitHubRunnerController(ctx)) //nolint:gocritic // Background reconciliation requires the restricted runner-controller identity; workspace creation separately assumes the actual owner.
	return c
}

// Wait waits for the controller and its pool workers to stop.
func (c *Controller) Wait() { <-c.done }

func (c *Controller) run(ctx context.Context) {
	defer close(c.done)
	var workers sync.WaitGroup
	active := make(map[uuid.UUID]chan struct{})
	ticker := c.Clock.NewTicker(5*time.Second, "githubrunners", "discovery")
	defer ticker.Stop()
	for {
		pools, err := c.Database.GetGitHubRunnerPools(ctx, uuid.Nil)
		if err != nil && ctx.Err() == nil {
			c.Logger.Error(ctx, "discover GitHub runner pools", slog.Error(err))
		}
		for _, pool := range pools {
			if pool.State == "deleted" {
				continue
			}
			if done, ok := active[pool.ID]; ok {
				select {
				case <-done:
					delete(active, pool.ID)
				default:
					continue
				}
			}
			owner := uuid.New()
			if _, err = c.Database.AcquireGitHubRunnerPoolLease(ctx, database.AcquireGitHubRunnerPoolLeaseParams{ID: pool.ID, LeaseOwner: uuid.NullUUID{UUID: owner, Valid: true}}); err != nil {
				continue
			}
			done := make(chan struct{})
			active[pool.ID] = done
			workers.Go(func() {
				defer close(done)
				pool.LeaseOwner = uuid.NullUUID{UUID: owner, Valid: true}
				w := worker{controller: c, pool: pool, owner: owner}
				if err := w.run(ctx); err != nil && ctx.Err() == nil {
					w.report(ctx, err)
				}
				releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				if err := c.Database.ReleaseGitHubRunnerPoolLease(releaseCtx, database.ReleaseGitHubRunnerPoolLeaseParams{ID: pool.ID, LeaseOwner: uuid.NullUUID{UUID: owner, Valid: true}}); err != nil {
					c.Logger.Warn(releaseCtx, "release GitHub runner pool lease", slog.Error(err))
				}
			})
		}
		select {
		case <-ctx.Done():
			workers.Wait()
			return
		case <-ticker.C:
		}
	}
}

type worker struct {
	controller *Controller
	mu         sync.Mutex
	pool       database.GithubRunnerPool
	owner      uuid.UUID
	client     Client
}

func (w *worker) run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	// Renew independently of provisioning and long polling. Losing ownership
	// cancels external work; short state transactions also verify ownership.
	poolID := w.pool.ID
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := w.controller.Clock.NewTicker(30*time.Second, "githubrunners", "lease")
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				leaseCtx, leaseCancel := context.WithTimeout(ctx, 10*time.Second)
				_, err := w.controller.Database.AcquireGitHubRunnerPoolLease(leaseCtx, database.AcquireGitHubRunnerPoolLeaseParams{ID: poolID, LeaseOwner: uuid.NullUUID{UUID: w.owner, Valid: true}})
				leaseCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-heartbeatDone }()
	var err error
	w.client, err = w.controller.NewClient(w.pool)
	if err != nil {
		return err
	}
	set, err := w.client.GetRunnerScaleSet(ctx, int(w.pool.RunnerGroupID), ScaleSetName(w.pool))
	if err != nil {
		return err
	}
	if set == nil {
		set, err = w.client.CreateRunnerScaleSet(ctx, &scaleset.RunnerScaleSet{
			Name: ScaleSetName(w.pool), RunnerGroupID: int(w.pool.RunnerGroupID),
			Labels:        []scaleset.Label{{Name: w.pool.Name, Type: "System"}},
			RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		})
		if err != nil {
			return err
		}
	}
	w.pool.ScaleSetID = int64(set.ID)
	if err = w.saveStatus(ctx, nil); err != nil {
		return err
	}
	session, err := w.client.OpenSession(ctx, set.ID, w.owner.String())
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer closeCancel()
		if err := session.Close(closeCtx); err != nil {
			w.controller.Logger.Debug(closeCtx, "close GitHub runner message session", slog.Error(err))
		}
	}()
	if stats := session.Session().Statistics; stats != nil {
		w.pool.AssignedJobs = assignedJobCount(stats.TotalAssignedJobs)
	}
	if err = w.saveStatus(ctx, nil); err != nil {
		return err
	}
	pollDone := make(chan error, 1)
	go func() { pollDone <- w.poll(ctx, session) }()
	defer func() { cancel(); <-pollDone }()
	ticker := w.controller.Clock.NewTicker(5*time.Second, "githubrunners", "reconcile")
	defer ticker.Stop()
	for {
		w.mu.Lock()
		_, err = w.controller.Database.AcquireGitHubRunnerPoolLease(ctx, database.AcquireGitHubRunnerPoolLeaseParams{ID: w.pool.ID, LeaseOwner: uuid.NullUUID{UUID: w.owner, Valid: true}})
		if err == nil {
			w.pool, err = w.controller.Database.GetGitHubRunnerPoolByID(ctx, w.pool.ID)
		}
		if err == nil {
			err = w.reconcile(ctx)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
			w.report(ctx, err)
		}
		w.mu.Unlock()
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case pollErr := <-pollDone:
			pollDone <- pollErr
			return pollErr
		case <-ticker.C:
		}
	}
}

func (w *worker) poll(ctx context.Context, s Session) error {
	lastID := 0
	for ctx.Err() == nil {
		w.mu.Lock()
		capacity := int(w.pool.MaxRunners)
		if w.pool.State != "active" {
			capacity = 0
		}
		w.mu.Unlock()
		msg, err := s.GetMessage(ctx, lastID, capacity)
		if err != nil {
			return xerrors.Errorf("poll scale set: %w", err)
		}
		if msg == nil {
			continue
		}
		w.mu.Lock()
		err = w.processMessage(ctx, s, msg)
		w.mu.Unlock()
		if err != nil {
			return err
		}
		if err = s.DeleteMessage(ctx, msg.MessageID); err != nil {
			return xerrors.Errorf("acknowledge scale set message: %w", err)
		}
		lastID = msg.MessageID
	}
	return ctx.Err()
}

func (w *worker) processMessage(ctx context.Context, s Session, msg *scaleset.RunnerScaleSetMessage) error {
	if w.pool.State == "active" && len(msg.JobAvailableMessages) > 0 {
		ids := make([]int64, 0, len(msg.JobAvailableMessages))
		for _, job := range msg.JobAvailableMessages {
			ids = append(ids, job.RunnerRequestID)
		}
		if _, err := s.AcquireJobs(ctx, ids); err != nil {
			return xerrors.Errorf("acquire jobs: %w", err)
		}
	}
	return w.controller.Database.InTx(func(tx database.Store) error {
		if _, err := tx.AcquireGitHubRunnerPoolLease(ctx, database.AcquireGitHubRunnerPoolLeaseParams{ID: w.pool.ID, LeaseOwner: uuid.NullUUID{UUID: w.owner, Valid: true}}); err != nil {
			return err
		}
		runners, err := tx.GetGitHubRunnersByPoolID(ctx, w.pool.ID)
		if err != nil {
			return err
		}
		for _, r := range runners {
			if r.State == "deleted" || r.State == "deleting" || r.State == "finishing" {
				continue
			}
			changed := false
			for _, job := range msg.JobStartedMessages {
				if job.RunnerName == r.Name && int64(job.RunnerID) == r.GithubRunnerID && r.State != "busy" {
					r.State = "busy"
					r.JobRequestID = job.RunnerRequestID
					changed = true
				}
			}
			for _, job := range msg.JobCompletedMessages {
				if job.RunnerName == r.Name && int64(job.RunnerID) == r.GithubRunnerID {
					r.State = "finishing"
					r.JobRequestID = job.RunnerRequestID
					r.JobResult = job.Result
					changed = true
				}
			}
			if changed {
				if _, err = SaveRunner(ctx, tx, r, w.owner); err != nil {
					return err
				}
			}
		}
		if msg.Statistics != nil {
			w.pool.AssignedJobs = assignedJobCount(msg.Statistics.TotalAssignedJobs)
		}
		return tx.UpdateGitHubRunnerPoolStatus(ctx, database.UpdateGitHubRunnerPoolStatusParams{ID: w.pool.ID, LeaseOwner: uuid.NullUUID{UUID: w.owner, Valid: true}, ScaleSetID: w.pool.ScaleSetID, AssignedJobs: w.pool.AssignedJobs, LastError: ""})
	}, nil)
}

func (w *worker) reconcile(ctx context.Context) error {
	runners, err := w.controller.Database.GetGitHubRunnersByPoolID(ctx, w.pool.ID)
	if err != nil {
		return err
	}
	capacity := 0
	var reconcileErrors []error
	for _, r := range runners {
		if r.State == "deleted" {
			continue
		}
		if err = w.reconcileRunner(ctx, r); err != nil {
			reconcileErrors = append(reconcileErrors, err)
		}
		// Include cleanup in the infrastructure budget until a later observation
		// confirms deletion. This bounds resources during slow destroys.
		capacity++
	}
	if len(reconcileErrors) > 0 {
		return errors.Join(reconcileErrors...)
	}
	if w.pool.State == "draining" && capacity == 0 {
		if err = w.client.DeleteRunnerScaleSet(ctx, int(w.pool.ScaleSetID)); err != nil {
			return err
		}
		_, err = w.controller.Database.UpdateGitHubRunnerPool(ctx, database.UpdateGitHubRunnerPoolParams{ID: w.pool.ID, State: "deleted", MaxRunners: w.pool.MaxRunners})
		return err
	}
	if w.pool.State != "active" {
		return nil
	}
	desired := min(int(w.pool.MaxRunners), int(w.pool.AssignedJobs))
	// Only one create per reconciliation bounds bursts and leaves time for
	// message processing and lease renewal between workspace builds.
	if capacity < desired {
		id := uuid.New()
		err = w.controller.Database.InTx(func(tx database.Store) error {
			if _, err := tx.AcquireGitHubRunnerPoolLease(ctx, database.AcquireGitHubRunnerPoolLeaseParams{ID: w.pool.ID, LeaseOwner: uuid.NullUUID{UUID: w.owner, Valid: true}}); err != nil {
				return err
			}
			_, err := tx.InsertGitHubRunner(ctx, database.InsertGitHubRunnerParams{ID: id, PoolID: w.pool.ID, OrganizationID: w.pool.OrganizationID, Name: "gha-" + strings.ReplaceAll(id.String(), "-", "")[:28]})
			return err
		}, nil)
	}
	return err
}

func (w *worker) reconcileRunner(ctx context.Context, r database.GithubRunner) error {
	store := w.controller.Database
	now := w.controller.Clock.Now()
	if r.State == "pending" && w.pool.State == "paused" {
		return nil
	}
	if r.State == "pending" && w.pool.State == "draining" {
		r.State = "deleting"
	}
	if r.State == "pending" {
		// A name containing the allocation UUID makes a partially committed
		// registration discoverable without adopting another pool's runner.
		existing, err := w.client.GetRunnerByName(ctx, r.Name)
		if err != nil && !errors.Is(err, scaleset.RunnerNotFoundError) {
			return err
		}
		if existing != nil {
			if int64(existing.RunnerScaleSetID) != w.pool.ScaleSetID {
				return xerrors.Errorf("runner %q belongs to another scale set", r.Name)
			}
			if err = w.client.RemoveRunner(ctx, int64(existing.ID)); err != nil {
				return err
			}
		}
		jit, err := w.client.GenerateJitRunnerConfig(ctx, &scaleset.RunnerScaleSetJitRunnerSetting{Name: r.Name}, int(w.pool.ScaleSetID))
		if err != nil {
			return err
		}
		if jit == nil || jit.Runner == nil || jit.Runner.ID == 0 || jit.EncodedJITConfig == "" {
			return xerrors.New("GitHub returned an incomplete JIT runner configuration")
		}
		r.GithubRunnerID = int64(jit.Runner.ID)
		r, err = SaveRunner(ctx, store, r, w.owner)
		if err != nil {
			return err
		}
		workspaceID, err := w.controller.CreateWorkspace(ctx, w.pool, r, jit.EncodedJITConfig)
		if err != nil {
			// Creation may have committed before response conversion failed.
			latest, loadErr := store.GetGitHubRunnerByID(ctx, r.ID)
			if loadErr != nil {
				return loadErr
			}
			latest.State = "deleting"
			latest.LastError = w.safeError(err)
			_, saveErr := SaveRunner(ctx, store, latest, w.owner)
			return saveErr
		}
		r.WorkspaceID = uuid.NullUUID{UUID: workspaceID, Valid: true}
		r.State = "provisioning"
		_, err = SaveRunner(ctx, store, r, w.owner)
		return err
	}
	if r.State == "finishing" && now.Sub(r.UpdatedAt) >= 15*time.Second {
		r.State = "deleting"
	}
	if (r.State == "provisioning" || r.State == "idle") && now.Sub(r.CreatedAt) > time.Duration(w.pool.StartupTimeoutSeconds)*time.Second {
		r.State = "deleting"
		r.LastError = "Runner startup deadline exceeded."
	}
	if r.State == "busy" && now.Sub(r.UpdatedAt) > time.Duration(w.pool.JobTimeoutSeconds)*time.Second {
		r.State = "deleting"
		r.LastError = "Runner job deadline exceeded."
	}
	if w.pool.State == "draining" && r.State != "busy" && r.State != "finishing" {
		r.State = "deleting"
	}
	// Bound the allocation even when a lost JobStarted event prevents the
	// observed state from becoming busy. This deadline survives retries.
	if now.Sub(r.CreatedAt) > time.Duration(w.pool.StartupTimeoutSeconds+w.pool.JobTimeoutSeconds)*time.Second {
		r.State = "deleting"
		r.LastError = "Runner allocation deadline exceeded."
	}
	if r.State == "deleting" {
		return w.deleteRunner(ctx, r)
	}
	if r.WorkspaceID.Valid {
		status, err := w.controller.WorkspaceStatus(ctx, r.WorkspaceID.UUID)
		if err != nil {
			return err
		}
		if status == "failed" || status == "canceled" || status == "deleted" || status == "stopped" {
			r.State = "deleting"
			r.LastError = "Workspace stopped before runner disposal."
			_, err = SaveRunner(ctx, store, r, w.owner)
			return err
		}
	}
	return nil
}

func (w *worker) deleteRunner(ctx context.Context, r database.GithubRunner) error {
	// Resolve GitHub's assignment/removal race before destroying infrastructure.
	if r.GithubRunnerID != 0 {
		err := w.client.RemoveRunner(ctx, r.GithubRunnerID)
		if err != nil && !errors.Is(err, scaleset.RunnerNotFoundError) {
			// Hard deadlines dispose of busy workspaces; ordinary draining waits.
			deadline := r.LastError == "Runner job deadline exceeded." || r.LastError == "Runner allocation deadline exceeded."
			if !errors.Is(err, scaleset.JobStillRunningError) || !deadline || !r.WorkspaceID.Valid {
				return err
			}
			if _, err = w.deleteWorkspace(ctx, r); err != nil {
				return err
			}
			_, err = SaveRunner(ctx, w.controller.Database, r, w.owner)
			return err
		}
	}
	deleted, err := w.deleteWorkspace(ctx, r)
	if err != nil {
		return err
	}
	if deleted {
		r.State = "deleted"
	}
	_, err = SaveRunner(ctx, w.controller.Database, r, w.owner)
	return err
}

func (w *worker) deleteWorkspace(ctx context.Context, r database.GithubRunner) (bool, error) {
	if !r.WorkspaceID.Valid {
		return true, nil
	}
	status, err := w.controller.WorkspaceStatus(ctx, r.WorkspaceID.UUID)
	if err != nil {
		return false, err
	}
	if status == "deleted" {
		return true, nil
	}
	if status == "deleting" {
		return false, nil
	}
	return false, w.controller.DeleteWorkspace(ctx, r)
}

func assignedJobCount(count int) int32 {
	if count < 0 {
		return 0
	}
	if count > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(count)
}

// SaveRunner persists a complete runner lifecycle observation.
func SaveRunner(ctx context.Context, store database.Store, r database.GithubRunner, owner uuid.UUID) (database.GithubRunner, error) {
	var result database.GithubRunner
	err := store.InTx(func(tx database.Store) error {
		if _, err := tx.AcquireGitHubRunnerPoolLease(ctx, database.AcquireGitHubRunnerPoolLeaseParams{ID: r.PoolID, LeaseOwner: uuid.NullUUID{UUID: owner, Valid: true}}); err != nil {
			return err
		}
		var err error
		result, err = tx.UpdateGitHubRunner(ctx, database.UpdateGitHubRunnerParams{ID: r.ID, WorkspaceID: r.WorkspaceID, GithubRunnerID: r.GithubRunnerID, State: r.State, JobRequestID: r.JobRequestID, JobResult: r.JobResult, LastError: r.LastError})
		return err
	}, nil)
	return result, err
}

func (w *worker) safeError(err error) string {
	if err == nil {
		return ""
	}
	if w.pool.GithubToken == "" {
		return err.Error()
	}
	return strings.ReplaceAll(err.Error(), w.pool.GithubToken, "[REDACTED]")
}

func (w *worker) saveStatus(ctx context.Context, err error) error {
	return w.controller.Database.UpdateGitHubRunnerPoolStatus(ctx, database.UpdateGitHubRunnerPoolStatusParams{ID: w.pool.ID, LeaseOwner: uuid.NullUUID{UUID: w.owner, Valid: true}, ScaleSetID: w.pool.ScaleSetID, AssignedJobs: w.pool.AssignedJobs, LastError: w.safeError(err)})
}

func (w *worker) report(ctx context.Context, err error) {
	w.controller.Logger.Warn(ctx, "reconcile GitHub runner pool", slog.F("pool_id", w.pool.ID), slog.F("error", w.safeError(err)))
	if saveErr := w.saveStatus(ctx, err); saveErr != nil {
		w.controller.Logger.Warn(ctx, "record GitHub runner pool error", slog.Error(saveErr))
	}
}
