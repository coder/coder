package prebuilds

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"fmt"
	"math"
	mrand "math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/hashicorp/go-multierror"

	"golang.org/x/exp/slices"

	"github.com/coder/coder/v2/coderd/audit"
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

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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

		defer logger.Debug(ctx, "acquired top-level prebuilds reconciliation lock", slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", time.Since(start).Seconds())))

		innerCtx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()

		var id uuid.NullUUID
		if templateID != nil {
			id.UUID = *templateID
		}

		presetsWithPrebuilds, err := db.GetTemplatePresetsWithPrebuilds(ctx, id)
		if len(presetsWithPrebuilds) == 0 {
			logger.Debug(innerCtx, "no templates found with prebuilds configured")
			return nil
		}

		runningPrebuilds, err := db.GetRunningPrebuilds(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get running prebuilds: %w", err)
		}

		prebuildsInProgress, err := db.GetPrebuildsInProgress(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get prebuilds in progress: %w", err)
		}

		// TODO: bounded concurrency? probably not but consider
		var eg errgroup.Group
		for _, preset := range presetsWithPrebuilds {
			eg.Go(func() error {
				// Pass outer context.
				// TODO: name these better to avoid the comment.
				err := c.reconcilePrebuildsForPreset(ctx, preset, runningPrebuilds, prebuildsInProgress)
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

type reconciliationActions struct {
	deleteIDs []uuid.UUID
	createIDs []uuid.UUID

	actual                       int32 // Running prebuilds for active version.
	desired                      int32 // Active template version's desired instances as defined in preset.
	eligible                     int32 // Prebuilds which can be claimed.
	outdated                     int32 // Prebuilds which no longer match the active template version.
	extraneous                   int32 // Extra running prebuilds for active version (somehow).
	starting, stopping, deleting int32 // Prebuilds currently being provisioned up or down.
}

// calculateActions MUST be called within the context of a transaction (TODO: isolation)
// with an advisory lock to prevent TOCTOU races.
func (c *Controller) calculateActions(ctx context.Context, preset database.GetTemplatePresetsWithPrebuildsRow, running []database.GetRunningPrebuildsRow, inProgress []database.GetPrebuildsInProgressRow) (*reconciliationActions, error) {
	// TODO: align workspace states with how we represent them on the FE and the CLI
	//	     right now there's some slight differences which can lead to additional prebuilds being created

	// TODO: add mechanism to prevent prebuilds being reconciled from being claimable by users; i.e. if a prebuild is
	// 		 about to be deleted, it should not be deleted if it has been claimed - beware of TOCTOU races!

	var (
		actual                       int32 // Running prebuilds for active version.
		desired                      int32 // Active template version's desired instances as defined in preset.
		eligible                     int32 // Prebuilds which can be claimed.
		outdated                     int32 // Prebuilds which no longer match the active template version.
		extraneous                   int32 // Extra running prebuilds for active version (somehow).
		starting, stopping, deleting int32 // Prebuilds currently being provisioned up or down.
	)

	if preset.UsingActiveVersion {
		actual = int32(len(running))
		desired = preset.DesiredInstances
	}

	for _, prebuild := range running {
		if preset.UsingActiveVersion {
			if prebuild.Eligible {
				eligible++
			}

			extraneous = int32(math.Max(float64(actual-preset.DesiredInstances), 0))
		}

		if prebuild.TemplateVersionID == preset.TemplateVersionID && !preset.UsingActiveVersion {
			outdated++
		}
	}

	for _, progress := range inProgress {
		switch progress.Transition {
		case database.WorkspaceTransitionStart:
			starting++
		case database.WorkspaceTransitionStop:
			stopping++
		case database.WorkspaceTransitionDelete:
			deleting++
		default:
			c.logger.Warn(ctx, "unknown transition found in prebuilds in progress result", slog.F("transition", progress.Transition))
		}
	}

	var (
		toCreate = int(math.Max(0, float64(
			desired- // The number specified in the preset
				(actual+starting)- // The current number of prebuilds (or builds in-flight)
				stopping), // The number of prebuilds currently being stopped (should be 0)
		))
		toDelete = int(math.Max(0, float64(
			outdated- // The number of prebuilds running above the desired count for active version
				deleting), // The number of prebuilds currently being deleted
		))

		actions = &reconciliationActions{
			actual:     actual,
			desired:    desired,
			eligible:   eligible,
			outdated:   outdated,
			extraneous: extraneous,
			starting:   starting,
			stopping:   stopping,
			deleting:   deleting,
		}
	)

	// Bail early to avoid scheduling new prebuilds while operations are in progress.
	if (toCreate+toDelete) > 0 && (starting+stopping+deleting) > 0 {
		c.logger.Warn(ctx, "prebuild operations in progress, skipping reconciliation",
			slog.F("template_id", preset.TemplateID.String()), slog.F("starting", starting),
			slog.F("stopping", stopping), slog.F("deleting", deleting),
			slog.F("wanted_to_create", toCreate), slog.F("wanted_to_delete", toDelete))
		return actions, nil
	}

	// It's possible that an operator could stop/start prebuilds which interfere with the reconciliation loop, so
	// we check if there are somehow more prebuilds than we expect, and then pick random victims to be deleted.
	if extraneous > 0 {
		// Sort running IDs randomly so we can pick random victims.
		slices.SortFunc(running, func(_, _ database.GetRunningPrebuildsRow) int {
			if mrand.Float64() > 0.5 {
				return -1
			}

			return 1
		})

		var victims []uuid.UUID
		for i := 0; i < int(extraneous); i++ {
			if i >= len(running) {
				// This should never happen.
				c.logger.Warn(ctx, "unexpected reconciliation state; extraneous count exceeds running prebuilds count!",
					slog.F("running_count", len(running)),
					slog.F("extraneous", extraneous))
				continue
			}

			victims = append(victims, running[i].WorkspaceID)
		}

		actions.deleteIDs = append(actions.deleteIDs, victims...)

		c.logger.Warn(ctx, "found extra prebuilds running, picking random victim(s)",
			slog.F("template_id", preset.TemplateID.String()), slog.F("desired", desired), slog.F("actual", actual), slog.F("extra", extraneous),
			slog.F("victims", victims))

		// Prevent the rest of the reconciliation from completing
		return actions, nil
	}

	// If the template has become deleted or deprecated since the last reconciliation, we need to ensure we
	// scale those prebuilds down to zero.
	if preset.Deleted || preset.Deprecated {
		toCreate = 0
		toDelete = int(actual + outdated)
	}

	for i := 0; i < toCreate; i++ {
		actions.createIDs = append(actions.createIDs, uuid.New())
	}

	if toDelete > 0 && len(running) != toDelete {
		c.logger.Warn(ctx, "mismatch between running prebuilds and expected deletion count!",
			slog.F("template_id", preset.TemplateID.String()), slog.F("running", len(running)), slog.F("to_delete", toDelete))
	}

	// TODO: implement lookup to not perform same action on workspace multiple times in $period
	// 		 i.e. a workspace cannot be deleted for some reason, which continually makes it eligible for deletion
	for i := 0; i < toDelete; i++ {
		if i >= len(running) {
			// Above warning will have already addressed this.
			continue
		}

		actions.deleteIDs = append(actions.deleteIDs, running[i].WorkspaceID)
	}

	return actions, nil
}

func (c *Controller) reconcilePrebuildsForPreset(ctx context.Context, preset database.GetTemplatePresetsWithPrebuildsRow,
	allRunning []database.GetRunningPrebuildsRow, allInProgress []database.GetPrebuildsInProgressRow,
) error {
	logger := c.logger.With(slog.F("template_id", preset.TemplateID.String()))

	var lastErr multierror.Error
	vlogger := logger.With(slog.F("template_version_id", preset.TemplateVersionID), slog.F("preset_id", preset.PresetID))

	running := slice.Filter(allRunning, func(prebuild database.GetRunningPrebuildsRow) bool {
		if !prebuild.DesiredPresetID.Valid && !prebuild.CurrentPresetID.Valid {
			return false
		}
		return prebuild.CurrentPresetID.UUID == preset.PresetID &&
			prebuild.TemplateVersionID == preset.TemplateVersionID // Not strictly necessary since presets are 1:1 with template versions, but no harm in being extra safe.
	})

	inProgress := slice.Filter(allInProgress, func(prebuild database.GetPrebuildsInProgressRow) bool {
		return prebuild.TemplateVersionID == preset.TemplateVersionID
	})

	actions, err := c.calculateActions(ctx, preset, running, inProgress)
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
	if len(actions.createIDs) > 0 || len(actions.deleteIDs) > 0 {
		// Only log with info level when there's a change that needs to be effected.
		levelFn = vlogger.Info
	}
	levelFn(ctx, "template prebuild state retrieved",
		slog.F("to_create", len(actions.createIDs)), slog.F("to_delete", len(actions.deleteIDs)),
		slog.F("desired", actions.desired), slog.F("actual", actions.actual),
		slog.F("outdated", actions.outdated), slog.F("extraneous", actions.extraneous),
		slog.F("starting", actions.starting), slog.F("stopping", actions.stopping),
		slog.F("deleting", actions.deleting), slog.F("eligible", actions.eligible))

	// Provision workspaces within the same tx so we don't get any timing issues here.
	// i.e. we hold the advisory lock until all reconciliatory actions have been taken.
	// TODO: max per reconciliation iteration?

	// TODO: i've removed the surrounding tx, but if we restore it then we need to pass down the store to these funcs.
	for _, id := range actions.createIDs {
		if err := c.createPrebuild(ownerCtx, id, preset.TemplateID, preset.PresetID); err != nil {
			vlogger.Error(ctx, "failed to create prebuild", slog.Error(err))
			lastErr.Errors = append(lastErr.Errors, err)
		}
	}

	for _, id := range actions.deleteIDs {
		if err := c.deletePrebuild(ownerCtx, id, preset.TemplateID, preset.PresetID); err != nil {
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
