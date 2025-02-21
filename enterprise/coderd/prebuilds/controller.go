package prebuilds

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"fmt"
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
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"

	"cdr.dev/slog"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

type Controller struct {
	store  database.Store
	cfg    codersdk.PrebuildsConfig
	pubsub pubsub.Pubsub

	logger   slog.Logger
	nudgeCh  chan *uuid.UUID
	cancelFn context.CancelCauseFunc
	closed   atomic.Bool
}

func NewController(store database.Store, pubsub pubsub.Pubsub, cfg codersdk.PrebuildsConfig, logger slog.Logger) *Controller {
	return &Controller{
		store:   store,
		pubsub:  pubsub,
		logger:  logger,
		cfg:     cfg,
		nudgeCh: make(chan *uuid.UUID, 1),
	}
}

func (c *Controller) Loop(ctx context.Context) error {
	ticker := time.NewTicker(c.cfg.ReconciliationInterval.Value())
	defer ticker.Stop()

	// TODO: create new authz role
	ctx, cancel := context.WithCancelCause(dbauthz.AsSystemRestricted(ctx))
	c.cancelFn = cancel

	for {
		select {
		// Accept nudges from outside the control loop to trigger a new iteration.
		case template := <-c.nudgeCh:
			c.reconcile(ctx, template)
		// Trigger a new iteration on each tick.
		case <-ticker.C:
			c.reconcile(ctx, nil)
		case <-ctx.Done():
			c.logger.Error(context.Background(), "prebuilds reconciliation loop exited", slog.Error(ctx.Err()), slog.F("cause", context.Cause(ctx)))
			return ctx.Err()
		}
	}
}

func (c *Controller) Close(cause error) {
	if c.isClosed() {
		return
	}
	c.closed.Store(true)
	if c.cancelFn != nil {
		c.cancelFn(cause)
	}
}

func (c *Controller) isClosed() bool {
	return c.closed.Load()
}

func (c *Controller) ReconcileTemplate(templateID uuid.UUID) {
	// TODO: replace this with pubsub listening
	c.nudgeCh <- &templateID
}

func (c *Controller) reconcile(ctx context.Context, templateID *uuid.UUID) {
	var logger slog.Logger
	if templateID == nil {
		logger = c.logger.With(slog.F("reconcile_context", "all"))
	} else {
		logger = c.logger.With(slog.F("reconcile_context", "specific"), slog.F("template_id", templateID.String()))
	}

	select {
	case <-ctx.Done():
		logger.Warn(context.Background(), "reconcile exiting prematurely; context done", slog.Error(ctx.Err()))
		return
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

		// TODO: give up after some time waiting on this?
		err := db.AcquireLock(ctx, database.LockIDReconcileTemplatePrebuilds)
		if err != nil {
			logger.Warn(ctx, "failed to acquire top-level prebuilds reconciliation lock; likely running on another coderd replica", slog.Error(err))
			return nil
		}

		logger.Debug(ctx, "acquired top-level prebuilds reconciliation lock", slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", time.Since(start).Seconds())))

		var id uuid.NullUUID
		if templateID != nil {
			id.UUID = *templateID
			id.Valid = true
		}

		state, err := c.determineState(ctx, db, id)
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
				logger.Debug(ctx, "skipping reconciliation for preset; inactive, no running prebuilds, and no in-progress operationss",
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
}

// determineState determines the current state of prebuilds & the presets which define them.
// This function MUST be called within
func (c *Controller) determineState(ctx context.Context, store database.Store, id uuid.NullUUID) (*reconciliationState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var state reconciliationState

	err := store.InTx(func(db database.Store) error {
		start := time.Now()

		// TODO: give up after some time waiting on this?
		err := db.AcquireLock(ctx, database.LockIDDeterminePrebuildsState)
		if err != nil {
			return xerrors.Errorf("failed to acquire state determination lock: %w", err)
		}

		c.logger.Debug(ctx, "acquired state determination lock", slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", time.Since(start).Seconds())))

		presetsWithPrebuilds, err := db.GetTemplatePresetsWithPrebuilds(ctx, id)
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

		state = newReconciliationState(presetsWithPrebuilds, allRunningPrebuilds, allPrebuildsInProgress)
		return nil
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead, // This mirrors the MVCC snapshotting Postgres does when using CTEs
		ReadOnly:     true,
		TxIdentifier: "prebuilds_state_determination",
	})

	return &state, err
}

func (c *Controller) reconcilePrebuildsForPreset(ctx context.Context, ps *presetState) error {
	if ps == nil {
		return xerrors.Errorf("unexpected nil preset state")
	}

	logger := c.logger.With(slog.F("template_id", ps.preset.TemplateID.String()))

	var lastErr multierror.Error
	vlogger := logger.With(slog.F("template_version_id", ps.preset.TemplateVersionID), slog.F("preset_id", ps.preset.PresetID))

	// TODO: move log lines up from calculateActions.
	actions, err := ps.calculateActions()
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
	}
	levelFn(ctx, "template prebuild state retrieved",
		slog.F("to_create", actions.create), slog.F("to_delete", len(actions.deleteIDs)),
		slog.F("desired", actions.desired), slog.F("actual", actions.actual),
		slog.F("outdated", actions.outdated), slog.F("extraneous", actions.extraneous),
		slog.F("starting", actions.starting), slog.F("stopping", actions.stopping),
		slog.F("deleting", actions.deleting), slog.F("eligible", actions.eligible))

	// Provision workspaces within the same tx so we don't get any timing issues here.
	// i.e. we hold the advisory lock until all reconciliatory actions have been taken.
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

func (c *Controller) createPrebuild(ctx context.Context, prebuildID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
	name, err := generateName()
	if err != nil {
		return xerrors.Errorf("failed to generate unique prebuild ID: %w", err)
	}

	template, err := c.store.GetTemplateByID(ctx, templateID)
	if err != nil {
		return xerrors.Errorf("failed to get template: %w", err)
	}

	now := dbtime.Now()
	// Workspaces are created without any versions.
	minimumWorkspace, err := c.store.InsertWorkspace(ctx, database.InsertWorkspaceParams{
		ID:               prebuildID,
		CreatedAt:        now,
		UpdatedAt:        now,
		OwnerID:          ownerID,
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
	workspace, err := c.store.GetWorkspaceByID(ctx, minimumWorkspace.ID)
	if err != nil {
		return xerrors.Errorf("get workspace by ID: %w", err)
	}

	c.logger.Info(ctx, "attempting to create prebuild", slog.F("name", name),
		slog.F("workspace_id", prebuildID.String()), slog.F("preset_id", presetID.String()))

	return c.provision(ctx, prebuildID, template, presetID, database.WorkspaceTransitionStart, workspace)
}

func (c *Controller) deletePrebuild(ctx context.Context, prebuildID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
	workspace, err := c.store.GetWorkspaceByID(ctx, prebuildID)
	if err != nil {
		return xerrors.Errorf("get workspace by ID: %w", err)
	}

	template, err := c.store.GetTemplateByID(ctx, templateID)
	if err != nil {
		return xerrors.Errorf("failed to get template: %w", err)
	}

	c.logger.Info(ctx, "attempting to delete prebuild",
		slog.F("workspace_id", prebuildID.String()), slog.F("preset_id", presetID.String()))

	return c.provision(ctx, prebuildID, template, presetID, database.WorkspaceTransitionDelete, workspace)
}

func (c *Controller) provision(ctx context.Context, prebuildID uuid.UUID, template database.Template, presetID uuid.UUID, transition database.WorkspaceTransition, workspace database.Workspace) error {
	tvp, err := c.store.GetPresetParametersByTemplateVersionID(ctx, template.ActiveVersionID)
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
		Initiator(ownerID).
		ActiveVersion().
		VersionID(template.ActiveVersionID).
		MarkPrebuild().
		TemplateVersionPresetID(presetID)

	// We only inject the required params when the prebuild is being created.
	// This mirrors the behaviour of regular workspace deletion (see cli/delete.go).
	if transition != database.WorkspaceTransitionDelete {
		builder = builder.RichParameterValues(params)
	}

	_, provisionerJob, _, err := builder.Build(
		ctx,
		c.store,
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
