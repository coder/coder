package wsbuildorchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/pproflabel"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	subscribeMaxBackoff = 10 * time.Second
	// Pubsub should wake the worker promptly, while occasional
	// polling prevents missed wakes from leaving rows pending
	// indefinitely.
	backupPollInterval = 30 * time.Second
	maxAttempts        = 3
	retryDelay         = 30 * time.Second
)

// Orchestrator fulfills workspace build orchestrations after their
// parent builds reach a terminal state.
type Orchestrator struct {
	logger            slog.Logger
	db                database.Store
	pubsub            pubsub.Pubsub
	fileCache         *files.Cache
	buildUsageChecker *atomic.Pointer[wsbuilder.UsageChecker]
	deploymentValues  *codersdk.DeploymentValues
	experiments       codersdk.Experiments
	builderMetrics    *wsbuilder.Metrics
	clock             quartz.Clock

	wakeCh chan struct{}

	// startOnce ensures the background goroutines are launched at most
	// once, even if Start is called more than once.
	startOnce sync.Once
	// cancel cancels the context on all running jobs. If the ctx
	// passed into `Start` is canceled, the jobs will also stop.
	cancel context.CancelFunc
	// wg ensures all job goroutines have exited before Close returns.
	wg sync.WaitGroup
}

type Options struct {
	Logger            slog.Logger
	Database          database.Store
	Pubsub            pubsub.Pubsub
	FileCache         *files.Cache
	BuildUsageChecker *atomic.Pointer[wsbuilder.UsageChecker]
	DeploymentValues  *codersdk.DeploymentValues
	Experiments       codersdk.Experiments
	BuilderMetrics    *wsbuilder.Metrics
	Clock             quartz.Clock
}

// New constructs an Orchestrator. Call Start to begin processing.
func New(opts Options) *Orchestrator {
	clock := opts.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &Orchestrator{
		logger:            opts.Logger.Named("workspace_build_orchestrator"),
		db:                opts.Database,
		pubsub:            opts.Pubsub,
		fileCache:         opts.FileCache,
		buildUsageChecker: opts.BuildUsageChecker,
		deploymentValues:  opts.DeploymentValues,
		experiments:       opts.Experiments,
		builderMetrics:    opts.BuilderMetrics,
		clock:             clock,
		// Keep one pending wake signal while the worker is between
		// runs. One is enough because each run drains all ready
		// orchestration rows.
		wakeCh: make(chan struct{}, 1),
	}
}

// Start launches the orchestrator's background goroutines. It is safe
// to call more than once; only the first call has any effect. Call
// Close to stop the goroutines and wait for their exit.
func (o *Orchestrator) Start(ctx context.Context) {
	o.startOnce.Do(func() {
		ctx, o.cancel = context.WithCancel(ctx)
		o.wg.Add(2)
		pproflabel.Go(ctx, pproflabel.Service(pproflabel.ServiceWorkspaceBuildOrchestrator, "goroutine", "subscribe"), func(ctx context.Context) {
			defer o.wg.Done()
			o.subscribe(ctx)
		})
		pproflabel.Go(ctx, pproflabel.Service(pproflabel.ServiceWorkspaceBuildOrchestrator, "goroutine", "run"), func(ctx context.Context) {
			defer o.wg.Done()
			o.run(ctx)
		})
	})
}

// Close stops the orchestrator and waits for its goroutines to exit.
func (o *Orchestrator) Close() {
	if o.cancel != nil {
		o.cancel()
	}
	o.wg.Wait()
}

func (o *Orchestrator) subscribe(ctx context.Context) {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0
	eb.MaxInterval = subscribeMaxBackoff
	bkoff := backoff.WithContext(eb, ctx)

	var cancelSubscribe func()
	err := backoff.Retry(func() error {
		cancelFn, err := o.pubsub.SubscribeWithErr(
			wspubsub.WorkspaceBuildOrchestrationWakeChannel,
			o.listen,
		)
		if err != nil {
			o.logger.Warn(ctx, "failed to subscribe to wake channel", slog.Error(err))
			return err
		}
		cancelSubscribe = cancelFn
		return nil
	}, bkoff)
	if err != nil {
		if ctx.Err() == nil {
			o.logger.Error(ctx, "code bug: retry failed before context canceled", slog.Error(err))
		}
		return
	}
	defer cancelSubscribe()
	o.logger.Debug(ctx, "subscribed to wake channel")

	// Reconcile rows that may have become ready while the worker was
	// not subscribed.
	o.wake()

	<-ctx.Done()
}

func (o *Orchestrator) listen(ctx context.Context, _ []byte, err error) {
	if xerrors.Is(err, pubsub.ErrDroppedMessages) {
		o.logger.Warn(ctx, "pubsub may have dropped wake signals")
		o.wake()
		return
	}
	if err != nil {
		o.logger.Warn(ctx, "unhandled pubsub error", slog.Error(err))
		return
	}
	o.wake()
}

func (o *Orchestrator) wake() {
	select {
	case o.wakeCh <- struct{}{}:
	default:
	}
}

func (o *Orchestrator) run(ctx context.Context) {
	ticker := o.clock.NewTicker(backupPollInterval)
	defer ticker.Stop()

	for {
		// wakeCh can win the select below even when ctx is canceled,
		// so re-check here. Once canceled, do not begin another
		// processing round.
		if ctx.Err() != nil {
			return
		}

		err := o.processAll(ctx)
		if err != nil && ctx.Err() == nil {
			o.logger.Error(ctx, "failed to process orchestrations", slog.Error(err))
		}

		select {
		case <-o.wakeCh:
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// processAll processes all pending orchestration rows whose parent
// builds have reached a terminal state.
func (o *Orchestrator) processAll(ctx context.Context) error {
	for {
		found, err := o.processNext(ctx)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
	}
}

func (o *Orchestrator) processNext(ctx context.Context) (bool, error) {
	//nolint:gocritic // Inserting the orchestration row required
	// authorization for the parent and child transitions. The worker
	// uses system authority to fulfill that durable intent after the
	// parent build completes.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	var (
		found           bool
		workspace       database.Workspace
		childJob        *database.ProvisionerJob
		orchestrationID uuid.UUID
		childBuildErr   error
	)

	err := o.db.InTx(func(tx database.Store) error {
		orchestration, err := tx.GetNextPendingWorkspaceBuildOrchestrationForUpdate(sysCtx)
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("get next pending workspace build orchestration: %w", err)
		}

		found = true
		orchestrationID = orchestration.ID

		// markFailed resolves the locked orchestration as failed with
		// a message, so a row that cannot make progress does not keep
		// blocking later ones.
		markFailed := func(msg string) error {
			_, err := tx.UpdateWorkspaceBuildOrchestrationFailedByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationFailedByIDParams{
				Error:     sql.NullString{String: msg, Valid: true},
				UpdatedAt: dbtime.Now(),
				ID:        orchestration.ID,
			})
			if err != nil {
				return xerrors.Errorf("mark workspace build orchestration as failed: %w", err)
			}
			return nil
		}

		// parentBuild and parentJob are guaranteed to exist by
		// foreign keys on the locked orchestration row, so an error
		// here is unexpected and likely transient. Return it to
		// retry, rather than resolving the orchestration as failed.
		parentBuild, err := tx.GetWorkspaceBuildByID(sysCtx, orchestration.ParentBuildID)
		if err != nil {
			return xerrors.Errorf("get parent workspace build: %w", err)
		}

		parentJob, err := tx.GetProvisionerJobByID(sysCtx, parentBuild.JobID)
		if err != nil {
			return xerrors.Errorf("get parent provisioner job: %w", err)
		}

		// Resolve terminal parent outcomes that do not create a child
		// build. Successful parents continue below.
		switch parentJob.JobStatus {
		case database.ProvisionerJobStatusSucceeded:
		case database.ProvisionerJobStatusCanceled:
			_, err = tx.UpdateWorkspaceBuildOrchestrationCanceledByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationCanceledByIDParams{
				ID:        orchestration.ID,
				UpdatedAt: dbtime.Now(),
			})
			if err != nil {
				return xerrors.Errorf("mark workspace build orchestration as canceled: %w", err)
			}
			return nil
		case database.ProvisionerJobStatusFailed:
			parentFailure := "parent workspace build failed"
			if parentJob.Error.Valid && parentJob.Error.String != "" {
				parentFailure = fmt.Sprintf("parent workspace build failed: %s", parentJob.Error.String)
			}
			return markFailed(parentFailure)
		default:
			// This should be unreachable because the row-locking query
			// only selects terminal parent jobs. Mark the row as failed
			// because retrying would block later orchestrations.
			return markFailed(fmt.Sprintf("unexpected parent job status %q", parentJob.JobStatus))
		}

		childBuildRequest, err := childBuildRequestFromOrchestration(orchestration)
		if err != nil {
			// Mark the row failed to avoid retrying work that cannot
			// make progress.
			return markFailed(err.Error())
		}

		workspace, err = tx.GetWorkspaceByID(sysCtx, parentBuild.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace: %w", err)
		}

		// GetWorkspaceByID returns soft-deleted rows.
		if workspace.Deleted {
			return markFailed("workspace was deleted")
		}

		// A dormant workspace must be woken before it can start.
		// Starting it while still dormant would leave it running but
		// still subject to deleting_at, which could auto-delete it.
		if workspace.DormantAt.Valid {
			return markFailed("workspace is dormant")
		}

		childBuild, provisionerJob, err := o.createBuild(sysCtx, tx, workspace, parentBuild.InitiatorID, childBuildRequest)
		if err != nil {
			// Carry the builder error out of the transaction; the
			// fail-vs-retry decision runs after the rollback.
			childBuildErr = err
			return xerrors.Errorf("create child workspace build: %w", err)
		}
		childJob = provisionerJob

		_, err = tx.UpdateWorkspaceBuildOrchestrationCompletedByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationCompletedByIDParams{
			ChildBuildID: uuid.NullUUID{
				UUID:  childBuild.ID,
				Valid: true,
			},
			UpdatedAt: dbtime.Now(),
			ID:        orchestration.ID,
		})
		if err != nil {
			return xerrors.Errorf("complete workspace build orchestration: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		if !found {
			// A persistent error here blocks the whole queue, but
			// that is systemic, not a poison row. Surface for retry.
			return false, err
		}

		if ctx.Err() != nil {
			// On shutdown, don't resolve or log it as unexpected
			// error below.
			return false, err
		}

		// A row was locked but processing failed. Resolve so it does
		// not stay pending and block newer orchestrations.
		errMsg := err.Error()
		failNow := false
		if childBuildErr != nil {
			// The child build error carries an HTTP status we can
			// classify into retryable vs permanent.
			errMsg = childBuildErrorMessage(childBuildErr)
			failNow = childBuildErrorShouldFailOrchestration(childBuildErr)
		} else {
			o.logger.Error(ctx, "unexpected error processing orchestration",
				slog.F("workspace_build_orchestration_id", orchestrationID),
				slog.Error(err))
		}

		var markErr error
		if failNow {
			// Mark the orchestration failed so one bad row does not
			// block later orchestrations.
			_, markErr = o.db.UpdateWorkspaceBuildOrchestrationFailedByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationFailedByIDParams{
				Error: sql.NullString{
					String: errMsg,
					Valid:  true,
				},
				UpdatedAt: dbtime.Now(),
				ID:        orchestrationID,
			})
		} else {
			// Back off and retry, eventually failing after maxAttempts
			// so a persistently failing row stops blocking the queue.
			now := dbtime.Now()
			_, markErr = o.db.UpdateWorkspaceBuildOrchestrationRetryByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationRetryByIDParams{
				Error: sql.NullString{
					String: errMsg,
					Valid:  true,
				},
				NextRetryAfter:  now.Add(retryDelay),
				UpdatedAt:       now,
				ID:              orchestrationID,
				MaxAttemptCount: maxAttempts,
			})
		}

		if markErr != nil {
			if xerrors.Is(markErr, sql.ErrNoRows) {
				// This update runs after the transaction has ended, so
				// another worker may have resolved the orchestration
				// first. Treat that race as success because the row no
				// longer needs processing.
				return found, nil
			}
			// Preserve the original error because the orchestration row
			// could not be updated with it.
			return false, errors.Join(
				err,
				xerrors.Errorf("resolve workspace build orchestration: %w", markErr),
			)
		}

		return found, nil
	}

	// These post-commit notifications are best-effort. The child
	// build and provisioner job are already persisted, so missing
	// pubsub does not corrupt state. It can delay workers or
	// subscribers until another wake or refresh.
	if childJob != nil {
		if err := provisionerjobs.PostJob(o.pubsub, *childJob); err != nil {
			o.logger.Error(ctx, "failed to post child provisioner job to pubsub",
				slog.F("workspace_build_orchestration_id", orchestrationID),
				slog.F("workspace_id", workspace.ID),
				slog.Error(err),
			)
		}

		err := wspubsub.PublishWorkspaceEvent(ctx, o.pubsub, workspace.OwnerID, wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindStateChange,
			WorkspaceID: workspace.ID,
		})
		if err != nil {
			o.logger.Warn(ctx, "failed to publish workspace update",
				slog.F("workspace_build_orchestration_id", orchestrationID),
				slog.F("workspace_id", workspace.ID), slog.Error(err))
		}
	}

	return found, nil
}

func childBuildRequestFromOrchestration(orchestration database.WorkspaceBuildOrchestration) (codersdk.CreateWorkspaceBuildRequest, error) {
	var childParameterValues []codersdk.WorkspaceBuildParameter
	if len(orchestration.ChildRichParameterValues) > 0 {
		err := json.Unmarshal(orchestration.ChildRichParameterValues, &childParameterValues)
		if err != nil {
			return codersdk.CreateWorkspaceBuildRequest{}, xerrors.Errorf("unmarshal child rich parameter values: %w", err)
		}
	}
	if childParameterValues == nil {
		childParameterValues = []codersdk.WorkspaceBuildParameter{}
	}

	request := codersdk.CreateWorkspaceBuildRequest{
		Transition:          codersdk.WorkspaceTransition(orchestration.ChildTransition),
		RichParameterValues: childParameterValues,
		LogLevel:            codersdk.ProvisionerLogLevel(orchestration.ChildLogLevel),
	}

	if orchestration.ChildTemplateVersionID.Valid {
		request.TemplateVersionID = orchestration.ChildTemplateVersionID.UUID
	}
	if orchestration.ChildTemplateVersionPresetID.Valid {
		request.TemplateVersionPresetID = orchestration.ChildTemplateVersionPresetID.UUID
	}
	if orchestration.ChildReason.Valid {
		request.Reason = codersdk.CreateWorkspaceBuildReason(orchestration.ChildReason.BuildReason)
	}

	return request, nil
}

func (o *Orchestrator) createBuild(
	ctx context.Context,
	tx database.Store,
	workspace database.Workspace,
	initiatorID uuid.UUID,
	request codersdk.CreateWorkspaceBuildRequest,
) (*database.WorkspaceBuild, *database.ProvisionerJob, error) {
	transition := database.WorkspaceTransition(request.Transition)
	builder := wsbuilder.New(workspace, transition, *o.buildUsageChecker.Load()).
		Initiator(initiatorID).
		RichParameterValues(request.RichParameterValues).
		LogLevel(string(request.LogLevel)).
		DeploymentValues(o.deploymentValues).
		Experiments(o.experiments).
		TemplateVersionPresetID(request.TemplateVersionPresetID).
		BuildMetrics(o.builderMetrics)

	if request.TemplateVersionID != uuid.Nil {
		builder = builder.VersionID(request.TemplateVersionID)
	} else if transition == database.WorkspaceTransitionStart {
		builder = builder.ActiveVersion()
	}
	if request.Reason != "" {
		builder = builder.Reason(database.BuildReason(request.Reason))
	}

	workspaceBuild, provisionerJob, _, err := builder.Build(ctx, tx, o.fileCache,
		// nil authorization function skips the builder's RBAC and
		// config checks. The parent and child transitions were
		// authorized when the orchestration row was inserted, and the
		// child reuses the parent build's already-validated log
		// level.
		nil,
		// The child build is created by a background worker, so there
		// is no request IP to attach. Its initiator is still set from
		// the parent build.
		audit.WorkspaceBuildBaggage{},
	)
	if err != nil {
		return nil, nil, err
	}

	return workspaceBuild, provisionerJob, nil
}

// childBuildErrorShouldFailOrchestration reports whether a child build
// error should be persisted as a failed orchestration instead of retried.
func childBuildErrorShouldFailOrchestration(err error) bool {
	buildErr, ok := errors.AsType[wsbuilder.BuildError](err)
	if !ok {
		return false
	}

	switch buildErr.Status {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound:
		// These statuses indicate invalid stored build input or a
		// permission/resource state that retrying the same request
		// will not fix.
		return true
	default:
		return false
	}
}

// childBuildErrorMessage returns the error text to be stored on the
// orchestration row. Build errors can expose cleaner response
// messages than Error(), which may contain only the wrapped cause.
func childBuildErrorMessage(err error) string {
	buildErr, ok := errors.AsType[wsbuilder.BuildError](err)
	if !ok {
		return err.Error()
	}

	_, response := buildErr.Response()
	if response.Detail != "" && response.Detail != response.Message {
		return fmt.Sprintf("%s: %s", response.Message, response.Detail)
	}
	if response.Message != "" {
		return response.Message
	}
	return buildErr.Error()
}
