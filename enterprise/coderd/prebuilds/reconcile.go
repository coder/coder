package prebuilds

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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

var ErrBackoff = xerrors.New("reconciliation in backoff")

type storeReconciler struct {
	store  database.Store
	cfg    codersdk.PrebuildsConfig
	pubsub pubsub.Pubsub

	logger   slog.Logger
	cancelFn context.CancelCauseFunc
	stopped  atomic.Bool
	done     chan struct{}
}

func NewStoreReconciler(store database.Store, pubsub pubsub.Pubsub, cfg codersdk.PrebuildsConfig, logger slog.Logger) prebuilds.Reconciler {
	return &storeReconciler{
		store:  store,
		pubsub: pubsub,
		logger: logger,
		cfg:    cfg,
		done:   make(chan struct{}, 1),
	}
}

func (c *storeReconciler) RunLoop(ctx context.Context) {
	reconciliationInterval := c.cfg.ReconciliationInterval.Value()
	if reconciliationInterval <= 0 { // avoids a panic
		reconciliationInterval = 5 * time.Minute
	}

	c.logger.Info(ctx, "starting reconciler", slog.F("interval", reconciliationInterval))

	ticker := time.NewTicker(reconciliationInterval)
	defer ticker.Stop()
	defer func() {
		c.done <- struct{}{}
	}()

	// TODO: create new authz role
	ctx, cancel := context.WithCancelCause(dbauthz.AsSystemRestricted(ctx))
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

func (c *storeReconciler) Stop(ctx context.Context, cause error) {
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

func (c *storeReconciler) isStopped() bool {
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
func (c *storeReconciler) ReconcileAll(ctx context.Context) error {
	logger := c.logger.With(slog.F("reconcile_context", "all"))

	select {
	case <-ctx.Done():
		logger.Warn(context.Background(), "reconcile exiting prematurely; context done", slog.Error(ctx.Err()))
		return nil
	default:
	}

	logger.Debug(ctx, "starting reconciliation")

	// This tx holds a global lock, which prevents any other coderd replica from starting a reconciliation and
	// possibly getting an inconsistent view of the state.
	//
	// The lock MUST be held until ALL modifications have been effected.
	//
	// It is run with RepeatableRead isolation, so it's effectively snapshotting the data at the start of the tx.
	//
	// This is a read-only tx, so returning an error (i.e. causing a rollback) has no impact.
	err := c.store.InTx(func(db database.Store) error {
		start := time.Now()

		// TODO: use TryAcquireLock here and bail out early.
		err := db.AcquireLock(ctx, database.LockIDReconcileTemplatePrebuilds)
		if err != nil {
			logger.Warn(ctx, "failed to acquire top-level reconciliation lock; likely running on another coderd replica", slog.Error(err))
			return nil
		}

		logger.Debug(ctx, "acquired top-level reconciliation lock", slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", time.Since(start).Seconds())))

		state, err := c.determineState(ctx, db)
		if err != nil {
			return xerrors.Errorf("determine current state: %w", err)
		}
		if len(state.presets) == 0 {
			logger.Debug(ctx, "no templates found with prebuilds configured")
			return nil
		}

		// TODO: bounded concurrency? probably not but consider
		var eg errgroup.Group
		for _, preset := range state.presets {
			ps, err := state.filterByPreset(preset.PresetID)
			if err != nil {
				logger.Warn(ctx, "failed to find preset state", slog.Error(err), slog.F("preset_id", preset.PresetID.String()))
				continue
			}

			if !preset.UsingActiveVersion && len(ps.running) == 0 && len(ps.inProgress) == 0 {
				logger.Debug(ctx, "skipping reconciliation for preset; inactive, no running prebuilds, and no in-progress operations",
					slog.F("preset_id", preset.PresetID.String()))
				continue
			}

			eg.Go(func() error {
				// Pass outer context.
				err := c.reconcilePrebuildsForPreset(ctx, ps)
				if err != nil {
					logger.Error(ctx, "failed to reconcile prebuilds for preset", slog.Error(err), slog.F("preset_id", preset.PresetID))
				}
				// DO NOT return error otherwise the tx will end.
				return nil
			})
		}

		return eg.Wait()
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead,
		ReadOnly:     true,
		TxIdentifier: "template_prebuilds",
	})
	if err != nil {
		logger.Error(ctx, "failed to reconcile", slog.Error(err))
	}

	return err
}

// determineState determines the current state of prebuilds & the presets which define them.
// An application-level lock is used
func (c *storeReconciler) determineState(ctx context.Context, store database.Store) (*reconciliationState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var state reconciliationState

	err := store.InTx(func(db database.Store) error {
		start := time.Now()

		// TODO: per-template ID lock?
		err := db.AcquireLock(ctx, database.LockIDDeterminePrebuildsState)
		if err != nil {
			return xerrors.Errorf("failed to acquire state determination lock: %w", err)
		}

		c.logger.Debug(ctx, "acquired state determination lock", slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", time.Since(start).Seconds())))

		presetsWithPrebuilds, err := db.GetTemplatePresetsWithPrebuilds(ctx, uuid.NullUUID{}) // TODO: implement template-specific reconciliations later
		if err != nil {
			return xerrors.Errorf("failed to get template presets with prebuilds: %w", err)
		}
		if len(presetsWithPrebuilds) == 0 {
			return nil
		}

		allRunningPrebuilds, err := db.GetRunningPrebuilds(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get running prebuilds: %w", err)
		}

		allPrebuildsInProgress, err := db.GetPrebuildsInProgress(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get prebuilds in progress: %w", err)
		}

		presetsBackoff, err := db.GetPresetsBackoff(ctx, durationToInterval(c.cfg.ReconciliationBackoffLookback.Value()))
		if err != nil {
			return xerrors.Errorf("failed to get backoffs for presets: %w", err)
		}

		state = newReconciliationState(presetsWithPrebuilds, allRunningPrebuilds, allPrebuildsInProgress, presetsBackoff)
		return nil
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead, // This mirrors the MVCC snapshotting Postgres does when using CTEs
		ReadOnly:     true,
		TxIdentifier: "prebuilds_state_determination",
	})

	return &state, err
}

func (c *storeReconciler) reconcilePrebuildsForPreset(ctx context.Context, ps *presetState) error {
	if ps == nil {
		return xerrors.Errorf("unexpected nil preset state")
	}

	logger := c.logger.With(slog.F("template_id", ps.preset.TemplateID.String()))

	var lastErr multierror.Error
	vlogger := logger.With(slog.F("template_version_id", ps.preset.TemplateVersionID), slog.F("preset_id", ps.preset.PresetID))

	// TODO: move log lines up from calculateActions.
	actions, err := ps.calculateActions(c.cfg.ReconciliationBackoffInterval.Value())
	if err != nil {
		vlogger.Error(ctx, "failed to calculate reconciliation actions", slog.Error(err))
		return xerrors.Errorf("failed to calculate reconciliation actions: %w", err)
	}

	// TODO: authz // Can't use existing profiles (i.e. AsSystemRestricted) because of dbauthz rules
	ownerCtx := dbauthz.As(ctx, rbac.Subject{
		ID:     "owner",
		Roles:  rbac.RoleIdentifiers{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ExpandableScope(rbac.ScopeAll),
	})

	levelFn := vlogger.Debug
	if actions.create > 0 || len(actions.deleteIDs) > 0 {
		// Only log with info level when there's a change that needs to be effected.
		levelFn = vlogger.Info
	} else if dbtime.Now().Before(actions.backoffUntil) {
		levelFn = vlogger.Warn
	}

	fields := []any{
		slog.F("create_count", actions.create), slog.F("delete_count", len(actions.deleteIDs)),
		slog.F("to_delete", actions.deleteIDs),
		slog.F("desired", actions.desired), slog.F("actual", actions.actual),
		slog.F("outdated", actions.outdated), slog.F("extraneous", actions.extraneous),
		slog.F("starting", actions.starting), slog.F("stopping", actions.stopping),
		slog.F("deleting", actions.deleting), slog.F("eligible", actions.eligible),
	}

	// TODO: add quartz

	// If there is anything to backoff for (usually a cycle of failed prebuilds), then log and bail out.
	if actions.backoffUntil.After(dbtime.Now()) {
		levelFn(ctx, "template prebuild state retrieved, backing off",
			append(fields,
				slog.F("backoff_until", actions.backoffUntil.Format(time.RFC3339)),
				slog.F("backoff_secs", math.Round(actions.backoffUntil.Sub(dbtime.Now()).Seconds())),
			)...)

		// return ErrBackoff
		return nil
	} else {
		levelFn(ctx, "template prebuild state retrieved", fields...)
	}

	// Provision workspaces within the same tx so we don't get any timing issues here.
	// i.e. we hold the advisory lock until all "reconciliatory" actions have been taken.
	// TODO: max per reconciliation iteration?

	// TODO: i've removed the surrounding tx, but if we restore it then we need to pass down the store to these funcs.
	for range actions.create {
		if err := c.createPrebuild(ownerCtx, uuid.New(), ps.preset.TemplateID, ps.preset.PresetID); err != nil {
			vlogger.Error(ctx, "failed to create prebuild", slog.Error(err))
			lastErr.Errors = append(lastErr.Errors, err)
		}
	}

	for _, id := range actions.deleteIDs {
		if err := c.deletePrebuild(ownerCtx, id, ps.preset.TemplateID, ps.preset.PresetID); err != nil {
			vlogger.Error(ctx, "failed to delete prebuild", slog.Error(err))
			lastErr.Errors = append(lastErr.Errors, err)
		}
	}

	return lastErr.ErrorOrNil()
}

func (c *storeReconciler) createPrebuild(ctx context.Context, prebuildID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
	name, err := generateName()
	if err != nil {
		return xerrors.Errorf("failed to generate unique prebuild ID: %w", err)
	}

	return c.store.InTx(func(db database.Store) error {
		template, err := db.GetTemplateByID(ctx, templateID)
		if err != nil {
			return xerrors.Errorf("failed to get template: %w", err)
		}

		now := dbtime.Now()

		minimumWorkspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:               prebuildID,
			CreatedAt:        now,
			UpdatedAt:        now,
			OwnerID:          OwnerID,
			OrganizationID:   template.OrganizationID,
			TemplateID:       template.ID,
			Name:             name,
			LastUsedAt:       dbtime.Now(),
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

func (c *storeReconciler) deletePrebuild(ctx context.Context, prebuildID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
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

func (c *storeReconciler) provision(ctx context.Context, db database.Store, prebuildID uuid.UUID, template database.Template, presetID uuid.UUID, transition database.WorkspaceTransition, workspace database.Workspace) error {
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
		Initiator(OwnerID).
		ActiveVersion().
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

// generateName generates a 20-byte prebuild name which should safe to use without truncation in most situations.
// UUIDs may be too long for a resource name in cloud providers (since this ID will be used in the prebuild's name).
//
// We're generating a 9-byte suffix (72 bits of entry):
// 1 - e^(-1e9^2 / (2 * 2^72)) = ~0.01% likelihood of collision in 1 billion IDs.
// See https://en.wikipedia.org/wiki/Birthday_attack.
func generateName() (string, error) {
	b := make([]byte, 9)

	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	// Encode the bytes to Base32 (A-Z2-7), strip any '=' padding
	return fmt.Sprintf("prebuild-%s", strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))), nil
}

// durationToInterval converts a given duration to microseconds, which is the unit PG represents intervals in.
func durationToInterval(d time.Duration) int32 {
	// Convert duration to seconds (as an example)
	return int32(d.Microseconds())
}
