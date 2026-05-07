package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
)

const (
	workspaceBuildOrchestratorSubscribeMaxBackoff = 10 * time.Second
	// Pubsub should wake the worker promptly, while occasional
	// polling prevents missed wakes from leaving rows pending
	// indefinitely.
	workspaceBuildOrchestratorBackupPollInterval = 30 * time.Second
	workspaceBuildOrchestrationMaxAttempts       = 3
	workspaceBuildOrchestrationRetryDelay        = 30 * time.Second
)

type workspaceBuildOrchestrator struct {
	api    *API
	logger slog.Logger
	wakeCh chan struct{}
}

func newWorkspaceBuildOrchestrator(api *API) *workspaceBuildOrchestrator {
	return &workspaceBuildOrchestrator{
		api:    api,
		logger: api.Logger.Named("workspace_build_orchestrator"),
		// Keep one pending wake signal while the worker is between
		// runs. One is enough because each run drains all ready
		// orchestration rows.
		wakeCh: make(chan struct{}, 1),
	}
}

func (o *workspaceBuildOrchestrator) start(ctx context.Context) {
	go o.subscribe(ctx)
	go o.run(ctx)
}

func (o *workspaceBuildOrchestrator) subscribe(ctx context.Context) {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	eb.MaxInterval = workspaceBuildOrchestratorSubscribeMaxBackoff
	bkoff := backoff.WithContext(eb, ctx)

	var cancelSubscribe func()
	err := backoff.Retry(func() error {
		cancelFn, err := o.api.Pubsub.SubscribeWithErr(
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

func (o *workspaceBuildOrchestrator) listen(ctx context.Context, _ []byte, err error) {
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

func (o *workspaceBuildOrchestrator) wake() {
	select {
	case o.wakeCh <- struct{}{}:
	default:
	}
}

func (o *workspaceBuildOrchestrator) run(ctx context.Context) {
	ticker := time.NewTicker(workspaceBuildOrchestratorBackupPollInterval)
	defer ticker.Stop()

	for {
		err := o.api.processWorkspaceBuildOrchestrations(ctx)
		if err != nil && ctx.Err() == nil {
			o.logger.Error(ctx, "process orchestrations", slog.Error(err))
		}

		select {
		case <-o.wakeCh:
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// processWorkspaceBuildOrchestrations processes all pending orchestration rows
// whose parent builds have reached a terminal state.
func (api *API) processWorkspaceBuildOrchestrations(ctx context.Context) error {
	for {
		found, err := api.processNextWorkspaceBuildOrchestration(ctx)
		if err != nil {
			return err
		}
		if !found {
			// No pending orchestration rows with terminal parent jobs
			// remain. The caller can wait for the next wake signal.
			return nil
		}
	}
}

func (api *API) processNextWorkspaceBuildOrchestration(ctx context.Context) (bool, error) {
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

	err := api.Database.InTx(func(tx database.Store) error {
		orchestration, err := tx.GetNextPendingWorkspaceBuildOrchestrationForUpdate(sysCtx)
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("get next pending workspace build orchestration: %w", err)
		}

		found = true
		orchestrationID = orchestration.ID

		// The dependent row lookups in this transaction rely on rows
		// selected by the row-locking query and rows protected by
		// foreign keys. Missing rows would indicate an invariant
		// violation; other errors are treated as retryable database
		// or runtime errors instead of resolving the orchestration
		// row as failed.
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
			_, err = tx.UpdateWorkspaceBuildOrchestrationFailedByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationFailedByIDParams{
				Error: sql.NullString{
					String: parentFailure,
					Valid:  true,
				},
				UpdatedAt: dbtime.Now(),
				ID:        orchestration.ID,
			})
			if err != nil {
				return xerrors.Errorf("mark workspace build orchestration as failed: %w", err)
			}
			return nil
		default:
			// This should be unreachable because the row-locking query
			// only selects terminal parent jobs. Mark the row as failed
			// because retrying would block later orchestrations.
			_, err = tx.UpdateWorkspaceBuildOrchestrationFailedByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationFailedByIDParams{
				Error: sql.NullString{
					String: fmt.Sprintf("unexpected parent job status %q", parentJob.JobStatus),
					Valid:  true,
				},
				UpdatedAt: dbtime.Now(),
				ID:        orchestration.ID,
			})
			if err != nil {
				return xerrors.Errorf("mark workspace build orchestration as failed: %w", err)
			}
			return nil
		}

		childBuildRequest, err := childBuildRequestFromOrchestration(orchestration)
		if err != nil {
			// The stored child build request cannot be reconstructed.
			// Mark the row failed to avoid retrying work that cannot
			// make progress.
			_, err = tx.UpdateWorkspaceBuildOrchestrationFailedByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationFailedByIDParams{
				Error: sql.NullString{
					String: err.Error(),
					Valid:  true,
				},
				UpdatedAt: dbtime.Now(),
				ID:        orchestration.ID,
			})
			if err != nil {
				return xerrors.Errorf("mark workspace build orchestration as failed: %w", err)
			}
			return nil
		}

		workspace, err = tx.GetWorkspaceByID(sysCtx, parentBuild.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace: %w", err)
		}

		childBuild, provisionerJob, err := api.createWorkspaceBuildFromOrchestration(sysCtx, tx, workspace, parentBuild.InitiatorID, childBuildRequest)
		if err != nil {
			// Decide whether to mark the orchestration failed based on
			// the builder error after the transaction rolls back.
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
		if childBuildErr == nil {
			return false, err
		}

		shouldFail := childBuildErrorShouldFailOrchestration(childBuildErr)
		var markErr error
		if shouldFail {
			// Mark the orchestration failed so one bad row does not
			// block later orchestrations.
			_, markErr = api.Database.UpdateWorkspaceBuildOrchestrationFailedByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationFailedByIDParams{
				Error: sql.NullString{
					String: childBuildErrorMessage(childBuildErr),
					Valid:  true,
				},
				UpdatedAt: dbtime.Now(),
				ID:        orchestrationID,
			})
		} else {
			now := dbtime.Now()
			_, markErr = api.Database.UpdateWorkspaceBuildOrchestrationRetryByID(sysCtx, database.UpdateWorkspaceBuildOrchestrationRetryByIDParams{
				Error: sql.NullString{
					String: childBuildErrorMessage(childBuildErr),
					Valid:  true,
				},
				NextRetryAfter:  now.Add(workspaceBuildOrchestrationRetryDelay),
				UpdatedAt:       now,
				ID:              orchestrationID,
				MaxAttemptCount: workspaceBuildOrchestrationMaxAttempts,
			})
		}

		if markErr != nil {
			if xerrors.Is(markErr, sql.ErrNoRows) {
				// This update runs after the child build transaction
				// has ended, so another worker may have resolved the
				// orchestration first. Treat that race as success
				// because the row no longer needs processing.
				return found, nil
			}
			// Preserve the original child build error because the
			// orchestration row could not be updated with it.
			return false, errors.Join(
				err,
				xerrors.Errorf("update workspace build orchestration after child build failure: %w", markErr),
			)
		}

		return found, nil
	}

	// These post-commit notifications are best-effort. The child
	// build and provisioner job are already persisted, so missing
	// pubsub does not corrupt state. It can delay workers or
	// subscribers until another wake or refresh.
	if childJob != nil {
		if err := provisionerjobs.PostJob(api.Pubsub, *childJob); err != nil {
			api.Logger.Error(ctx, "failed to post child provisioner job to pubsub",
				slog.F("workspace_build_orchestration_id", orchestrationID),
				slog.Error(err),
			)
		}

		api.publishWorkspaceUpdate(ctx, workspace.OwnerID, wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindStateChange,
			WorkspaceID: workspace.ID,
		})
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

func (api *API) createWorkspaceBuildFromOrchestration(
	ctx context.Context,
	tx database.Store,
	workspace database.Workspace,
	initiatorID uuid.UUID,
	request codersdk.CreateWorkspaceBuildRequest,
) (*database.WorkspaceBuild, *database.ProvisionerJob, error) {
	transition := database.WorkspaceTransition(request.Transition)
	builder := wsbuilder.New(workspace, transition, *api.BuildUsageChecker.Load()).
		Initiator(initiatorID).
		RichParameterValues(request.RichParameterValues).
		LogLevel(string(request.LogLevel)).
		DeploymentValues(api.Options.DeploymentValues).
		Experiments(api.Experiments).
		TemplateVersionPresetID(request.TemplateVersionPresetID).
		BuildMetrics(api.WorkspaceBuilderMetrics)

	if request.TemplateVersionID != uuid.Nil {
		builder = builder.VersionID(request.TemplateVersionID)
	} else if transition == database.WorkspaceTransitionStart {
		builder = builder.ActiveVersion()
	}
	if request.Reason != "" {
		builder = builder.Reason(database.BuildReason(request.Reason))
	}

	workspaceBuild, provisionerJob, _, err := builder.Build(ctx, tx, api.FileCache,
		func(policy.Action, rbac.Objecter) bool {
			// Inserting the orchestration row required authorization
			// for the parent and child transitions. The worker uses
			// system authority to fulfill that durable intent after
			// the parent build completes.
			return true
		},
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
	var buildErr wsbuilder.BuildError
	if !errors.As(err, &buildErr) {
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

// childBuildErrorMessage returns the error text stored on the
// orchestration row. Build errors can expose cleaner response
// messages than Error(), which may contain only the wrapped cause.
func childBuildErrorMessage(err error) string {
	var buildErr wsbuilder.BuildError
	if errors.As(err, &buildErr) {
		_, response := buildErr.Response()
		if response.Detail != "" && response.Detail != response.Message {
			return fmt.Sprintf("%s: %s", response.Message, response.Detail)
		}
		if response.Message != "" {
			return response.Message
		}
		return buildErr.Error()
	}
	return err.Error()
}
