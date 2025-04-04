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

func NewStoreReconciler(store database.Store, ps pubsub.Pubsub, cfg codersdk.PrebuildsConfig, logger slog.Logger, clock quartz.Clock) *StoreReconciler {
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

	c.logger.Info(ctx, "starting reconciler", slog.F("interval", reconciliationInterval),
		slog.F("backoff_interval", c.cfg.ReconciliationBackoffInterval.String()), slog.F("backoff_lookback", c.cfg.ReconciliationBackoffLookback.String()))

	ticker := c.clock.NewTicker(reconciliationInterval)
	defer ticker.Stop()
	defer func() {
		c.done <- struct{}{}
	}()

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
			c.logger.Warn(context.Background(), "reconciliation loop exited", slog.Error(ctx.Err()), slog.F("cause", context.Cause(ctx)))
			return
		}
	}
}

func (c *StoreReconciler) Stop(ctx context.Context, cause error) {
	c.logger.Warn(context.Background(), "stopping reconciler", slog.F("cause", cause))

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
		c.logger.Error(context.Background(), "reconciler stop exited prematurely", slog.Error(ctx.Err()), slog.F("cause", context.Cause(ctx)))
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
// TODO: make this unexported?
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
		state, err := c.SnapshotState(ctx, db)
		if err != nil {
			return xerrors.Errorf("determine current state: %w", err)
		}
		if len(state.Presets) == 0 {
			logger.Debug(ctx, "no templates found with prebuilds configured")
			return nil
		}

		// TODO: bounded concurrency? probably not but consider
		var eg errgroup.Group
		for _, preset := range state.Presets {
			ps, err := state.FilterByPreset(preset.ID)
			if err != nil {
				logger.Warn(ctx, "failed to find preset state", slog.Error(err), slog.F("preset_id", preset.ID.String()))
				continue
			}

			if !preset.UsingActiveVersion && len(ps.Running) == 0 && len(ps.InProgress) == 0 {
				logger.Debug(ctx, "skipping reconciliation for preset; inactive, no running prebuilds, and no in-progress operations",
					slog.F("template_id", preset.TemplateID.String()), slog.F("template_name", preset.TemplateName),
					slog.F("template_version_id", preset.TemplateVersionID.String()), slog.F("template_version_name", preset.TemplateVersionName),
					slog.F("preset_id", preset.ID.String()), slog.F("preset_name", preset.Name))
				continue
			}

			eg.Go(func() error {
				actions, err := c.DetermineActions(ctx, *ps)
				if err != nil {
					logger.Error(ctx, "failed to determine actions for preset", slog.Error(err), slog.F("preset_id", preset.ID))
					return nil
				}

				// Pass outer context.
				err = c.Reconcile(ctx, *ps, *actions)
				if err != nil {
					logger.Error(ctx, "failed to reconcile prebuilds for preset", slog.Error(err), slog.F("preset_id", preset.ID))
				}
				// DO NOT return error otherwise the tx will end.
				return nil
			})
		}

		return eg.Wait()
	})
	if err != nil {
		logger.Error(ctx, "failed to reconcile", slog.Error(err))
	}

	return err
}

func (c *StoreReconciler) WithReconciliationLock(ctx context.Context, logger slog.Logger, fn func(ctx context.Context, db database.Store) error) error {
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

		// TODO: use TryAcquireLock here and bail out early.
		err := db.AcquireLock(ctx, database.LockIDReconcileTemplatePrebuilds)
		if err != nil {
			logger.Warn(ctx, "failed to acquire top-level reconciliation lock; likely running on another coderd replica", slog.Error(err))
			return nil
		}

		logger.Debug(ctx, "acquired top-level reconciliation lock", slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", c.clock.Since(start).Seconds())))

		return fn(ctx, db)
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead,
		ReadOnly:     true,
		TxIdentifier: "template_prebuilds",
	})
}

// SnapshotState determines the current state of prebuilds & the presets which define them.
// An application-level lock is used
func (c *StoreReconciler) SnapshotState(ctx context.Context, store database.Store) (*prebuilds.ReconciliationState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var state prebuilds.ReconciliationState

	err := store.InTx(func(db database.Store) error {
		start := c.clock.Now()

		// TODO: per-template ID lock?
		err := db.AcquireLock(ctx, database.LockIDDeterminePrebuildsState)
		if err != nil {
			return xerrors.Errorf("failed to acquire state determination lock: %w", err)
		}

		c.logger.Debug(ctx, "acquired state determination lock", slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", c.clock.Since(start).Seconds())))

		presetsWithPrebuilds, err := db.GetTemplatePresetsWithPrebuilds(ctx, uuid.NullUUID{}) // TODO: implement template-specific reconciliations later
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

		state = prebuilds.NewReconciliationState(presetsWithPrebuilds, allRunningPrebuilds, allPrebuildsInProgress, presetsBackoff)
		return nil
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead, // This mirrors the MVCC snapshotting Postgres does when using CTEs
		ReadOnly:     true,
		TxIdentifier: "prebuilds_state_determination",
	})

	return &state, err
}

func (c *StoreReconciler) DetermineActions(ctx context.Context, state prebuilds.PresetState) (*prebuilds.ReconciliationActions, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return state.CalculateActions(c.clock, c.cfg.ReconciliationBackoffInterval.Value())
}

func (c *StoreReconciler) Reconcile(ctx context.Context, ps prebuilds.PresetState, actions prebuilds.ReconciliationActions) error {
	logger := c.logger.With(slog.F("template_id", ps.Preset.TemplateID.String()), slog.F("template_name", ps.Preset.TemplateName))

	var lastErr multierror.Error
	vlogger := logger.With(slog.F("template_version_id", ps.Preset.TemplateVersionID), slog.F("template_version_name", ps.Preset.TemplateVersionName),
		slog.F("preset_id", ps.Preset.ID), slog.F("preset_name", ps.Preset.Name))

	prebuildsCtx := dbauthz.AsPrebuildsOrchestrator(ctx)

	levelFn := vlogger.Debug
	if actions.Create > 0 || len(actions.DeleteIDs) > 0 {
		// Only log with info level when there's a change that needs to be effected.
		levelFn = vlogger.Info
	} else if c.clock.Now().Before(actions.BackoffUntil) {
		levelFn = vlogger.Warn
	}

	fields := []any{
		slog.F("create_count", actions.Create), slog.F("delete_count", len(actions.DeleteIDs)),
		slog.F("to_delete", actions.DeleteIDs),
		slog.F("desired", actions.Desired), slog.F("actual", actions.Actual),
		slog.F("outdated", actions.Outdated), slog.F("extraneous", actions.Extraneous),
		slog.F("starting", actions.Starting), slog.F("stopping", actions.Stopping),
		slog.F("deleting", actions.Deleting), slog.F("eligible", actions.Eligible),
	}

	// TODO: add quartz

	// If there is anything to backoff for (usually a cycle of failed prebuilds), then log and bail out.
	if actions.BackoffUntil.After(c.clock.Now()) {
		levelFn(ctx, "template prebuild state retrieved, backing off",
			append(fields,
				slog.F("failed", actions.Failed),
				slog.F("backoff_until", actions.BackoffUntil.Format(time.RFC3339)),
				slog.F("backoff_secs", math.Round(actions.BackoffUntil.Sub(c.clock.Now()).Seconds())),
			)...)

		// return ErrBackoff
		return nil
	}

	levelFn(ctx, "template prebuild state retrieved", fields...)

	// Shit happens (i.e. bugs or bitflips); let's defend against disastrous outcomes.
	// See https://blog.robertelder.org/causes-of-bit-flips-in-computer-memory/.
	// This is obviously not comprehensive protection against this sort of problem, but this is one essential check.
	if actions.Create > actions.Desired {
		vlogger.Critical(ctx, "determined excessive count of prebuilds to create; clamping to desired count",
			slog.F("create_count", actions.Create), slog.F("desired_count", actions.Desired))

		actions.Create = actions.Desired
	}

	// TODO: i've removed the surrounding tx, but if we restore it then we need to pass down the store to these funcs.
	for range actions.Create {
		if err := c.createPrebuild(prebuildsCtx, uuid.New(), ps.Preset.TemplateID, ps.Preset.ID); err != nil {
			vlogger.Error(ctx, "failed to create prebuild", slog.Error(err))
			lastErr.Errors = append(lastErr.Errors, err)
		}
	}

	for _, id := range actions.DeleteIDs {
		if err := c.deletePrebuild(prebuildsCtx, id, ps.Preset.TemplateID, ps.Preset.ID); err != nil {
			vlogger.Error(ctx, "failed to delete prebuild", slog.Error(err))
			lastErr.Errors = append(lastErr.Errors, err)
		}
	}

	return lastErr.ErrorOrNil()
}

func (c *StoreReconciler) createPrebuild(ctx context.Context, prebuildID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
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
			ID:               prebuildID,
			CreatedAt:        now,
			UpdatedAt:        now,
			OwnerID:          prebuilds.SystemUserID,
			OrganizationID:   template.OrganizationID,
			TemplateID:       template.ID,
			Name:             name,
			LastUsedAt:       c.clock.Now(),
			AutomaticUpdates: database.AutomaticUpdatesNever,
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
			slog.F("workspace_id", prebuildID.String()), slog.F("preset_id", presetID.String()))

		return c.provision(ctx, db, prebuildID, template, presetID, database.WorkspaceTransitionStart, workspace)
	}, &database.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  false,
	})
}

func (c *StoreReconciler) deletePrebuild(ctx context.Context, prebuildID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
	return c.store.InTx(func(db database.Store) error {
		workspace, err := db.GetWorkspaceByID(ctx, prebuildID)
		if err != nil {
			return xerrors.Errorf("get workspace by ID: %w", err)
		}

		template, err := db.GetTemplateByID(ctx, templateID)
		if err != nil {
			return xerrors.Errorf("failed to get template: %w", err)
		}

		c.logger.Info(ctx, "attempting to delete prebuild",
			slog.F("workspace_id", prebuildID.String()), slog.F("preset_id", presetID.String()))

		return c.provision(ctx, db, prebuildID, template, presetID, database.WorkspaceTransitionDelete, workspace)
	}, &database.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  false,
	})
}

func (c *StoreReconciler) provision(ctx context.Context, db database.Store, prebuildID uuid.UUID, template database.Template, presetID uuid.UUID, transition database.WorkspaceTransition, workspace database.Workspace) error {
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
		ActiveVersion().
		VersionID(template.ActiveVersionID).
		Prebuild().
		TemplateVersionPresetID(presetID)

	// We only inject the required params when the prebuild is being created.
	// This mirrors the behavior of regular workspace deletion (see cli/delete.go).
	if transition != database.WorkspaceTransitionDelete {
		builder = builder.RichParameterValues(params)
	}

	_, provisionerJob, _, err := builder.Build(
		ctx,
		db,
		func(action policy.Action, object rbac.Objecter) bool {
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
