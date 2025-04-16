package prebuilds

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"

	"cdr.dev/slog"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

type StoreReconciler struct {
	store  database.Store
	cfg    codersdk.PrebuildsConfig
	pubsub pubsub.Pubsub
	logger slog.Logger
	clock  quartz.Clock

	cancelFn context.CancelCauseFunc
	stopped  atomic.Bool
	done     chan struct{}
}

var _ prebuilds.ReconciliationOrchestrator = &StoreReconciler{}

func NewStoreReconciler(
	store database.Store,
	ps pubsub.Pubsub,
	cfg codersdk.PrebuildsConfig,
	logger slog.Logger,
	clock quartz.Clock,
) *StoreReconciler {
	return &StoreReconciler{
		store:  store,
		pubsub: ps,
		logger: logger,
		cfg:    cfg,
		clock:  clock,
		done:   make(chan struct{}, 1),
	}
}

func (c *StoreReconciler) RunLoop(ctx context.Context) {
	reconciliationInterval := c.cfg.ReconciliationInterval.Value()
	if reconciliationInterval <= 0 { // avoids a panic
		reconciliationInterval = 5 * time.Minute
	}

	c.logger.Info(ctx, "starting reconciler",
		slog.F("interval", reconciliationInterval),
		slog.F("backoff_interval", c.cfg.ReconciliationBackoffInterval.String()),
		slog.F("backoff_lookback", c.cfg.ReconciliationBackoffLookback.String()))

	ticker := c.clock.NewTicker(reconciliationInterval)
	defer ticker.Stop()
	defer func() {
		c.done <- struct{}{}
	}()

	// nolint:gocritic // Reconciliation Loop needs Prebuilds Orchestrator permissions.
	ctx, cancel := context.WithCancelCause(dbauthz.AsPrebuildsOrchestrator(ctx))
	c.cancelFn = cancel

	for {
		select {
		// TODO: implement pubsub listener to allow reconciling a specific template imperatively once it has been changed,
		//		 instead of waiting for the next reconciliation interval
		case <-ticker.C:
			// Trigger a new iteration on each tick.
			err := c.ReconcileAll(ctx)
			if err != nil {
				c.logger.Error(context.Background(), "reconciliation failed", slog.Error(err))
			}
		case <-ctx.Done():
			// nolint:gocritic // it's okay to use slog.F() for an error in this case
			// because we want to differentiate two different types of errors: ctx.Err() and context.Cause()
			c.logger.Warn(
				context.Background(),
				"reconciliation loop exited",
				slog.Error(ctx.Err()),
				slog.F("cause", context.Cause(ctx)),
			)
			return
		}
	}
}

func (c *StoreReconciler) Stop(ctx context.Context, cause error) {
	if cause != nil {
		c.logger.Error(context.Background(), "stopping reconciler due to an error", slog.Error(cause))
	} else {
		c.logger.Info(context.Background(), "gracefully stopping reconciler")
	}

	if c.isStopped() {
		return
	}
	c.stopped.Store(true)
	if c.cancelFn != nil {
		c.cancelFn(cause)
	}

	select {
	// Give up waiting for control loop to exit.
	case <-ctx.Done():
		// nolint:gocritic // it's okay to use slog.F() for an error in this case
		// because we want to differentiate two different types of errors: ctx.Err() and context.Cause()
		c.logger.Error(
			context.Background(),
			"reconciler stop exited prematurely",
			slog.Error(ctx.Err()),
			slog.F("cause", context.Cause(ctx)),
		)
	// Wait for the control loop to exit.
	case <-c.done:
		c.logger.Info(context.Background(), "reconciler stopped")
	}
}

func (c *StoreReconciler) isStopped() bool {
	return c.stopped.Load()
}

// ReconcileAll will attempt to resolve the desired vs actual state of all templates which have presets with prebuilds configured.
//
// NOTE:
//
// This function will kick of n provisioner jobs, based on the calculated state modifications.
//
// These provisioning jobs are fire-and-forget. We DO NOT wait for the prebuilt workspaces to complete their
// provisioning. As a consequence, it's possible that another reconciliation run will occur, which will mean that
// multiple preset versions could be reconciling at once. This may mean some temporary over-provisioning, but the
// reconciliation loop will bring these resources back into their desired numbers in an EVENTUALLY-consistent way.
//
// For example: we could decide to provision 1 new instance in this reconciliation.
// While that workspace is being provisioned, another template version is created which means this same preset will
// be reconciled again, leading to another workspace being provisioned. Two workspace builds will be occurring
// simultaneously for the same preset, but once both jobs have completed the reconciliation loop will notice the
// extraneous instance and delete it.
func (c *StoreReconciler) ReconcileAll(ctx context.Context) error {
	logger := c.logger.With(slog.F("reconcile_context", "all"))

	select {
	case <-ctx.Done():
		logger.Warn(context.Background(), "reconcile exiting prematurely; context done", slog.Error(ctx.Err()))
		return nil
	default:
	}

	logger.Debug(ctx, "starting reconciliation")

	err := c.WithReconciliationLock(ctx, logger, func(ctx context.Context, db database.Store) error {
		snapshot, err := c.SnapshotState(ctx, db)
		if err != nil {
			return xerrors.Errorf("determine current snapshot: %w", err)
		}
		if len(snapshot.Presets) == 0 {
			logger.Debug(ctx, "no templates found with prebuilds configured")
			return nil
		}

		var eg errgroup.Group
		// Reconcile presets in parallel. Each preset in its own goroutine.
		for _, preset := range snapshot.Presets {
			ps, err := snapshot.FilterByPreset(preset.ID)
			if err != nil {
				logger.Warn(ctx, "failed to find preset snapshot", slog.Error(err), slog.F("preset_id", preset.ID.String()))
				continue
			}

			eg.Go(func() error {
				// Pass outer context.
				err = c.ReconcilePreset(ctx, *ps)
				if err != nil {
					logger.Error(
						ctx,
						"failed to reconcile prebuilds for preset",
						slog.Error(err),
						slog.F("preset_id", preset.ID),
					)
				}
				// DO NOT return error otherwise the tx will end.
				return nil
			})
		}

		// Release lock only when all preset reconciliation goroutines are finished.
		return eg.Wait()
	})
	if err != nil {
		logger.Error(ctx, "failed to reconcile", slog.Error(err))
	}

	return err
}

// SnapshotState captures the current state of all prebuilds across templates.
func (c *StoreReconciler) SnapshotState(ctx context.Context, store database.Store) (*prebuilds.GlobalSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var state prebuilds.GlobalSnapshot

	err := store.InTx(func(db database.Store) error {
		// TODO: implement template-specific reconciliations later
		presetsWithPrebuilds, err := db.GetTemplatePresetsWithPrebuilds(ctx, uuid.NullUUID{})
		if err != nil {
			return xerrors.Errorf("failed to get template presets with prebuilds: %w", err)
		}
		if len(presetsWithPrebuilds) == 0 {
			return nil
		}
		allRunningPrebuilds, err := db.GetRunningPrebuiltWorkspaces(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get running prebuilds: %w", err)
		}

		allPrebuildsInProgress, err := db.CountInProgressPrebuilds(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get prebuilds in progress: %w", err)
		}

		presetsBackoff, err := db.GetPresetsBackoff(ctx, c.clock.Now().Add(-c.cfg.ReconciliationBackoffLookback.Value()))
		if err != nil {
			return xerrors.Errorf("failed to get backoffs for presets: %w", err)
		}

		state = prebuilds.NewGlobalSnapshot(presetsWithPrebuilds, allRunningPrebuilds, allPrebuildsInProgress, presetsBackoff)
		return nil
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead, // This mirrors the MVCC snapshotting Postgres does when using CTEs
		ReadOnly:     true,
		TxIdentifier: "prebuilds_state_determination",
	})

	return &state, err
}

func (c *StoreReconciler) ReconcilePreset(ctx context.Context, ps prebuilds.PresetSnapshot) error {
	logger := c.logger.With(
		slog.F("template_id", ps.Preset.TemplateID.String()),
		slog.F("template_name", ps.Preset.TemplateName),
		slog.F("template_version_id", ps.Preset.TemplateVersionID),
		slog.F("template_version_name", ps.Preset.TemplateVersionName),
		slog.F("preset_id", ps.Preset.ID),
		slog.F("preset_name", ps.Preset.Name),
	)

	state := ps.CalculateState()
	actions, err := c.CalculateActions(ctx, ps)
	if err != nil {
		logger.Error(ctx, "failed to calculate actions for preset", slog.Error(err), slog.F("preset_id", ps.Preset.ID))
		return nil
	}

	// nolint:gocritic // ReconcilePreset needs Prebuilds Orchestrator permissions.
	prebuildsCtx := dbauthz.AsPrebuildsOrchestrator(ctx)

	levelFn := logger.Debug
	switch {
	case actions.ActionType == prebuilds.ActionTypeBackoff:
		levelFn = logger.Warn
	// Log at info level when there's a change to be effected.
	case actions.ActionType == prebuilds.ActionTypeCreate && actions.Create > 0:
		levelFn = logger.Info
	case actions.ActionType == prebuilds.ActionTypeDelete && len(actions.DeleteIDs) > 0:
		levelFn = logger.Info
	}

	fields := []any{
		slog.F("action_type", actions.ActionType),
		slog.F("create_count", actions.Create), slog.F("delete_count", len(actions.DeleteIDs)),
		slog.F("to_delete", actions.DeleteIDs),
		slog.F("desired", state.Desired), slog.F("actual", state.Actual),
		slog.F("extraneous", state.Extraneous), slog.F("starting", state.Starting),
		slog.F("stopping", state.Stopping), slog.F("deleting", state.Deleting),
		slog.F("eligible", state.Eligible),
	}

	levelFn(ctx, "reconciliation actions for preset are calculated", fields...)

	switch actions.ActionType {
	case prebuilds.ActionTypeBackoff:
		// If there is anything to backoff for (usually a cycle of failed prebuilds), then log and bail out.
		levelFn(ctx, "template prebuild state retrieved, backing off",
			append(fields,
				slog.F("backoff_until", actions.BackoffUntil.Format(time.RFC3339)),
				slog.F("backoff_secs", math.Round(actions.BackoffUntil.Sub(c.clock.Now()).Seconds())),
			)...)

		// return ErrBackoff
		return nil

	case prebuilds.ActionTypeCreate:
		// Unexpected things happen (i.e. bugs or bitflips); let's defend against disastrous outcomes.
		// See https://blog.robertelder.org/causes-of-bit-flips-in-computer-memory/.
		// This is obviously not comprehensive protection against this sort of problem, but this is one essential check.
		desired := ps.Preset.DesiredInstances.Int32
		if actions.Create > desired {
			logger.Critical(ctx, "determined excessive count of prebuilds to create; clamping to desired count",
				slog.F("create_count", actions.Create), slog.F("desired_count", desired))

			actions.Create = desired
		}

		var multiErr multierror.Error

		for range actions.Create {
			if err := c.createPrebuiltWorkspace(prebuildsCtx, uuid.New(), ps.Preset.TemplateID, ps.Preset.ID); err != nil {
				logger.Error(ctx, "failed to create prebuild", slog.Error(err))
				multiErr.Errors = append(multiErr.Errors, err)
			}
		}

		return multiErr.ErrorOrNil()

	case prebuilds.ActionTypeDelete:
		var multiErr multierror.Error

		for _, id := range actions.DeleteIDs {
			if err := c.deletePrebuiltWorkspace(prebuildsCtx, id, ps.Preset.TemplateID, ps.Preset.ID); err != nil {
				logger.Error(ctx, "failed to delete prebuild", slog.Error(err))
				multiErr.Errors = append(multiErr.Errors, err)
			}
		}

		return multiErr.ErrorOrNil()

	default:
		return xerrors.Errorf("unknown action type: %v", actions.ActionType)
	}
}

func (c *StoreReconciler) CalculateActions(ctx context.Context, snapshot prebuilds.PresetSnapshot) (*prebuilds.ReconciliationActions, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return snapshot.CalculateActions(c.clock, c.cfg.ReconciliationBackoffInterval.Value())
}

func (c *StoreReconciler) WithReconciliationLock(
	ctx context.Context,
	logger slog.Logger,
	fn func(ctx context.Context, db database.Store) error,
) error {
	// This tx holds a global lock, which prevents any other coderd replica from starting a reconciliation and
	// possibly getting an inconsistent view of the state.
	//
	// The lock MUST be held until ALL modifications have been effected.
	//
	// It is run with RepeatableRead isolation, so it's effectively snapshotting the data at the start of the tx.
	//
	// This is a read-only tx, so returning an error (i.e. causing a rollback) has no impact.
	return c.store.InTx(func(db database.Store) error {
		start := c.clock.Now()

		// Try to acquire the lock. If we can't get it, another replica is handling reconciliation.
		acquired, err := db.TryAcquireLock(ctx, database.LockIDReconcilePrebuilds)
		if err != nil {
			// This is a real database error, not just lock contention
			logger.Error(ctx, "failed to acquire reconciliation lock due to database error", slog.Error(err))
			return err
		}
		if !acquired {
			// Normal case: another replica has the lock
			return nil
		}

		logger.Debug(ctx,
			"acquired top-level reconciliation lock",
			slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", c.clock.Since(start).Seconds())),
		)

		return fn(ctx, db)
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead,
		ReadOnly:     true,
		TxIdentifier: "prebuilds",
	})
}

func (c *StoreReconciler) createPrebuiltWorkspace(ctx context.Context, prebuiltWorkspaceID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
	name, err := prebuilds.GenerateName()
	if err != nil {
		return xerrors.Errorf("failed to generate unique prebuild ID: %w", err)
	}

	return c.store.InTx(func(db database.Store) error {
		template, err := db.GetTemplateByID(ctx, templateID)
		if err != nil {
			return xerrors.Errorf("failed to get template: %w", err)
		}

		now := c.clock.Now()

		minimumWorkspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:                prebuiltWorkspaceID,
			CreatedAt:         now,
			UpdatedAt:         now,
			OwnerID:           prebuilds.SystemUserID,
			OrganizationID:    template.OrganizationID,
			TemplateID:        template.ID,
			Name:              name,
			LastUsedAt:        c.clock.Now(),
			AutomaticUpdates:  database.AutomaticUpdatesNever,
			AutostartSchedule: sql.NullString{},
			Ttl:               sql.NullInt64{},
			NextStartAt:       sql.NullTime{},
		})
		if err != nil {
			return xerrors.Errorf("insert workspace: %w", err)
		}

		// We have to refetch the workspace for the joined in fields.
		workspace, err := db.GetWorkspaceByID(ctx, minimumWorkspace.ID)
		if err != nil {
			return xerrors.Errorf("get workspace by ID: %w", err)
		}

		c.logger.Info(ctx, "attempting to create prebuild", slog.F("name", name),
			slog.F("workspace_id", prebuiltWorkspaceID.String()), slog.F("preset_id", presetID.String()))

		return c.provision(ctx, db, prebuiltWorkspaceID, template, presetID, database.WorkspaceTransitionStart, workspace)
	}, &database.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  false,
	})
}

func (c *StoreReconciler) deletePrebuiltWorkspace(ctx context.Context, prebuiltWorkspaceID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
	return c.store.InTx(func(db database.Store) error {
		workspace, err := db.GetWorkspaceByID(ctx, prebuiltWorkspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace by ID: %w", err)
		}

		template, err := db.GetTemplateByID(ctx, templateID)
		if err != nil {
			return xerrors.Errorf("failed to get template: %w", err)
		}

		if workspace.OwnerID != prebuilds.SystemUserID {
			return xerrors.Errorf("prebuilt workspace is not owned by prebuild user anymore, probably it was claimed")
		}

		c.logger.Info(ctx, "attempting to delete prebuild",
			slog.F("workspace_id", prebuiltWorkspaceID.String()), slog.F("preset_id", presetID.String()))

		return c.provision(ctx, db, prebuiltWorkspaceID, template, presetID, database.WorkspaceTransitionDelete, workspace)
	}, &database.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  false,
	})
}

func (c *StoreReconciler) provision(
	ctx context.Context,
	db database.Store,
	prebuildID uuid.UUID,
	template database.Template,
	presetID uuid.UUID,
	transition database.WorkspaceTransition,
	workspace database.Workspace,
) error {
	tvp, err := db.GetPresetParametersByTemplateVersionID(ctx, template.ActiveVersionID)
	if err != nil {
		return xerrors.Errorf("fetch preset details: %w", err)
	}

	var params []codersdk.WorkspaceBuildParameter
	for _, param := range tvp {
		// TODO: don't fetch in the first place.
		if param.TemplateVersionPresetID != presetID {
			continue
		}

		params = append(params, codersdk.WorkspaceBuildParameter{
			Name:  param.Name,
			Value: param.Value,
		})
	}

	builder := wsbuilder.New(workspace, transition).
		Reason(database.BuildReasonInitiator).
		Initiator(prebuilds.SystemUserID).
		VersionID(template.ActiveVersionID).
		MarkPrebuild().
		TemplateVersionPresetID(presetID)

	// We only inject the required params when the prebuild is being created.
	// This mirrors the behavior of regular workspace deletion (see cli/delete.go).
	if transition != database.WorkspaceTransitionDelete {
		builder = builder.RichParameterValues(params)
	}

	_, provisionerJob, _, err := builder.Build(
		ctx,
		db,
		func(_ policy.Action, _ rbac.Objecter) bool {
			return true // TODO: harden?
		},
		audit.WorkspaceBuildBaggage{},
	)
	if err != nil {
		return xerrors.Errorf("provision workspace: %w", err)
	}

	err = provisionerjobs.PostJob(c.pubsub, *provisionerJob)
	if err != nil {
		// Client probably doesn't care about this error, so just log it.
		c.logger.Error(ctx, "failed to post provisioner job to pubsub", slog.Error(err))
	}

	c.logger.Info(ctx, "prebuild job scheduled", slog.F("transition", transition),
		slog.F("prebuild_id", prebuildID.String()), slog.F("preset_id", presetID.String()),
		slog.F("job_id", provisionerJob.ID))

	return nil
}
