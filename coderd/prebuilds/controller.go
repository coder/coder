package prebuilds

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"math"
	mrand "math/rand"
	"strings"
	"time"

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
	pubsub pubsub.Pubsub
	logger slog.Logger

	nudgeCh chan *uuid.UUID
	closeCh chan struct{}
}

func NewController(store database.Store, pubsub pubsub.Pubsub, logger slog.Logger) *Controller {
	return &Controller{
		store:   store,
		pubsub:  pubsub,
		logger:  logger,
		nudgeCh: make(chan *uuid.UUID, 1),
		closeCh: make(chan struct{}, 1),
	}
}

func (c Controller) Loop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 5) // TODO: configurable? 1m probably lowest valid value
	defer ticker.Stop()

	// TODO: create new authz role
	ctx = dbauthz.AsSystemRestricted(ctx)

	// TODO: bounded concurrency?
	var eg errgroup.Group
	for {
		select {
		// Accept nudges from outside the control loop to trigger a new iteration.
		case template := <-c.nudgeCh:
			eg.Go(func() error {
				c.reconcile(ctx, template)
				return nil
			})
		// Trigger a new iteration on each tick.
		case <-ticker.C:
			eg.Go(func() error {
				c.reconcile(ctx, nil)
				return nil
			})
		case <-c.closeCh:
			c.logger.Info(ctx, "control loop stopped")
			goto wait
		case <-ctx.Done():
			c.logger.Error(context.Background(), "control loop exited: %w", ctx.Err())
			goto wait
		}
	}

	// TODO: no gotos
wait:
	_ = eg.Wait()
}

func (c Controller) ReconcileTemplate(templateID uuid.UUID) {
	// TODO: replace this with pubsub listening
	c.nudgeCh <- &templateID
}

func (c Controller) reconcile(ctx context.Context, templateID *uuid.UUID) {
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

	// get all templates or specific one requested
	err := c.store.InTx(func(db database.Store) error {
		err := db.AcquireLock(ctx, database.LockIDReconcileTemplatePrebuilds)
		if err != nil {
			logger.Warn(ctx, "failed to acquire top-level prebuilds lock; likely running on another coderd replica", slog.Error(err))
			return nil
		}

		innerCtx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()

		var ids []uuid.UUID
		if templateID != nil {
			ids = append(ids, *templateID)
		}

		templates, err := db.GetTemplatesWithFilter(innerCtx, database.GetTemplatesWithFilterParams{
			IDs: ids,
		})
		if err != nil {
			c.logger.Debug(innerCtx, "could not fetch template(s)")
			return xerrors.Errorf("fetch template(s): %w", err)
		}

		if len(templates) == 0 {
			c.logger.Debug(innerCtx, "no templates found")
			return nil
		}

		// TODO: bounded concurrency? probably not but consider
		var eg errgroup.Group
		for _, template := range templates {
			eg.Go(func() error {
				// Pass outer context.
				// TODO: name these better to avoid the comment.
				return c.reconcileTemplate(ctx, template)
			})
		}

		return eg.Wait()
	}, &database.TxOptions{
		// TODO: isolation
		ReadOnly:     true,
		TxIdentifier: "template_prebuilds",
	})
	if err != nil {
		logger.Error(ctx, "failed to acquire database transaction", slog.Error(err))
	}
}

type reconciliationActions struct {
	deleteIDs []uuid.UUID
	createIDs []uuid.UUID

	meta database.GetTemplatePrebuildStateRow
}

// calculateActions MUST be called within the context of a transaction (TODO: isolation)
// with an advisory lock to prevent TOCTOU races.
func (c Controller) calculateActions(ctx context.Context, template database.Template, state database.GetTemplatePrebuildStateRow) (*reconciliationActions, error) {
	// TODO: align workspace states with how we represent them on the FE and the CLI
	//	     right now there's some slight differences which can lead to additional prebuilds being created

	// TODO: add mechanism to prevent prebuilds being reconciled from being claimable by users; i.e. if a prebuild is
	// 		 about to be deleted, it should not be deleted if it has been claimed - beware of TOCTOU races!

	var (
		toCreate = int(math.Max(0, float64(
			state.Desired- // The number specified in the preset
				(state.Actual+state.Starting)- // The current number of prebuilds (or builds in-flight)
				state.Stopping), // The number of prebuilds currently being stopped (should be 0)
		))
		toDelete = int(math.Max(0, float64(
			state.Outdated- // The number of prebuilds running above the desired count for active version
				state.Deleting), // The number of prebuilds currently being deleted
		))

		actions    = &reconciliationActions{meta: state}
		runningIDs = strings.Split(state.RunningPrebuildIds, ",")
	)

	// Bail early to avoid scheduling new prebuilds while operations are in progress.
	if (toCreate+toDelete) > 0 && (state.Starting+state.Stopping+state.Deleting) > 0 {
		c.logger.Warn(ctx, "prebuild operations in progress, skipping reconciliation",
			slog.F("template_id", template.ID), slog.F("starting", state.Starting),
			slog.F("stopping", state.Stopping), slog.F("deleting", state.Deleting),
			slog.F("wanted_to_create", toCreate), slog.F("wanted_to_delete", toDelete))
		return actions, nil
	}

	// It's possible that an operator could stop/start prebuilds which interfere with the reconciliation loop, so
	// we check if there are somehow more prebuilds than we expect, and then pick random victims to be deleted.
	if len(runningIDs) > 0 && state.Extraneous > 0 {
		// Sort running IDs randomly so we can pick random victims.
		slices.SortFunc(runningIDs, func(_, _ string) int {
			if mrand.Float64() > 0.5 {
				return -1
			}

			return 1
		})

		var victims []uuid.UUID
		for i := 0; i < int(state.Extraneous); i++ {
			if i >= len(runningIDs) {
				// This should never happen.
				c.logger.Warn(ctx, "unexpected reconciliation state; extraneous count exceeds running prebuilds count!",
					slog.F("running_count", len(runningIDs)),
					slog.F("extraneous", state.Extraneous))
				continue
			}

			victim := runningIDs[i]

			id, err := uuid.Parse(victim)
			if err != nil {
				c.logger.Warn(ctx, "invalid prebuild ID", slog.F("template_id", template.ID),
					slog.F("id", string(victim)), slog.Error(err))
			} else {
				victims = append(victims, id)
			}
		}

		actions.deleteIDs = append(actions.deleteIDs, victims...)

		c.logger.Warn(ctx, "found extra prebuilds running, picking random victim(s)",
			slog.F("template_id", template.ID), slog.F("desired", state.Desired), slog.F("actual", state.Actual), slog.F("extra", state.Extraneous),
			slog.F("victims", victims))

		// Prevent the rest of the reconciliation from completing
		return actions, nil
	}

	// If the template has become deleted or deprecated since the last reconciliation, we need to ensure we
	// scale those prebuilds down to zero.
	if state.TemplateDeleted || state.TemplateDeprecated {
		toCreate = 0
		toDelete = int(state.Actual + state.Outdated)
	}

	for i := 0; i < toCreate; i++ {
		actions.createIDs = append(actions.createIDs, uuid.New())
	}

	if toDelete > 0 && len(runningIDs) != toDelete {
		c.logger.Warn(ctx, "mismatch between running prebuilds and expected deletion count!",
			slog.F("template_id", template.ID), slog.F("running", len(runningIDs)), slog.F("to_delete", toDelete))
	}

	// TODO: implement lookup to not perform same action on workspace multiple times in $period
	// 		 i.e. a workspace cannot be deleted for some reason, which continually makes it eligible for deletion
	for i := 0; i < toDelete; i++ {
		if i >= len(runningIDs) {
			// Above warning will have already addressed this.
			continue
		}

		running := runningIDs[i]
		id, err := uuid.Parse(running)
		if err != nil {
			c.logger.Warn(ctx, "invalid prebuild ID", slog.F("template_id", template.ID),
				slog.F("id", string(running)), slog.Error(err))
			continue
		}

		actions.deleteIDs = append(actions.deleteIDs, id)
	}

	return actions, nil
}

func (c Controller) reconcileTemplate(ctx context.Context, template database.Template) error {
	logger := c.logger.With(slog.F("template_id", template.ID.String()))

	// get number of desired vs actual prebuild instances
	err := c.store.InTx(func(db database.Store) error {
		err := db.AcquireLock(ctx, database.GenLockID(fmt.Sprintf("template:%s", template.ID.String())))
		if err != nil {
			logger.Warn(ctx, "failed to acquire template prebuilds lock; likely running on another coderd replica", slog.Error(err))
			return nil
		}

		innerCtx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()

		versionStates, err := db.GetTemplatePrebuildState(ctx, template.ID)
		if err != nil {
			return xerrors.Errorf("failed to retrieve template's prebuild states: %w", err)
		}

		for _, state := range versionStates {
			vlogger := logger.With(slog.F("template_version_id", state.TemplateVersionID))

			actions, err := c.calculateActions(innerCtx, template, state)
			if err != nil {
				vlogger.Error(ctx, "failed to calculate reconciliation actions", slog.Error(err))
				continue
			}

			// TODO: authz // Can't use existing profiles (i.e. AsSystemRestricted) because of dbauthz rules
			var ownerCtx = dbauthz.As(ctx, rbac.Subject{
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
			levelFn(innerCtx, "template prebuild state retrieved",
				slog.F("to_create", len(actions.createIDs)), slog.F("to_delete", len(actions.deleteIDs)),
				slog.F("desired", actions.meta.Desired), slog.F("actual", actions.meta.Actual),
				slog.F("outdated", actions.meta.Outdated), slog.F("extraneous", actions.meta.Extraneous),
				slog.F("starting", actions.meta.Starting), slog.F("stopping", actions.meta.Stopping), slog.F("deleting", actions.meta.Deleting))

			// Provision workspaces within the same tx so we don't get any timing issues here.
			// i.e. we hold the advisory lock until all reconciliatory actions have been taken.
			// TODO: max per reconciliation iteration?

			for _, id := range actions.createIDs {
				if err := c.createPrebuild(ownerCtx, db, id, template); err != nil {
					vlogger.Error(ctx, "failed to create prebuild", slog.Error(err))
				}
			}

			for _, id := range actions.deleteIDs {
				if err := c.deletePrebuild(ownerCtx, db, id, template); err != nil {
					vlogger.Error(ctx, "failed to delete prebuild", slog.Error(err))
				}
			}
		}

		return nil
	}, &database.TxOptions{
		// TODO: isolation
		TxIdentifier: "template_prebuilds",
	})
	if err != nil {
		logger.Error(ctx, "failed to acquire database transaction", slog.Error(err))
	}

	return nil
}

func (c Controller) createPrebuild(ctx context.Context, db database.Store, prebuildID uuid.UUID, template database.Template) error {
	name, err := generateName()
	if err != nil {
		return xerrors.Errorf("failed to generate unique prebuild ID: %w", err)
	}

	now := dbtime.Now()
	// Workspaces are created without any versions.
	minimumWorkspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
		ID:               prebuildID,
		CreatedAt:        now,
		UpdatedAt:        now,
		OwnerID:          PrebuildOwnerUUID,
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

	c.logger.Info(ctx, "attempting to create prebuild", slog.F("name", name), slog.F("workspace_id", prebuildID.String()))

	return c.provision(ctx, db, prebuildID, template, database.WorkspaceTransitionStart, workspace)
}
func (c Controller) deletePrebuild(ctx context.Context, db database.Store, prebuildID uuid.UUID, template database.Template) error {
	workspace, err := db.GetWorkspaceByID(ctx, prebuildID)
	if err != nil {
		return xerrors.Errorf("get workspace by ID: %w", err)
	}

	c.logger.Info(ctx, "attempting to delete prebuild", slog.F("workspace_id", prebuildID.String()))

	return c.provision(ctx, db, prebuildID, template, database.WorkspaceTransitionDelete, workspace)
}

func (c Controller) provision(ctx context.Context, db database.Store, prebuildID uuid.UUID, template database.Template, transition database.WorkspaceTransition, workspace database.Workspace) error {
	tvp, err := db.GetPresetParametersByTemplateVersionID(ctx, template.ActiveVersionID)
	if err != nil {
		return xerrors.Errorf("fetch preset details: %w", err)
	}

	var params []codersdk.WorkspaceBuildParameter
	for _, param := range tvp {
		params = append(params, codersdk.WorkspaceBuildParameter{
			Name:  param.Name,
			Value: param.Value,
		})
	}

	builder := wsbuilder.New(workspace, transition).
		Reason(database.BuildReasonInitiator).
		Initiator(PrebuildOwnerUUID).
		ActiveVersion().
		VersionID(template.ActiveVersionID).
		MarkPrebuild()

	// We only inject the required params when the prebuild is being created.
	// This mirrors the behaviour of regular workspace deletion (see cli/delete.go).
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
		slog.F("prebuild_id", prebuildID.String()), slog.F("job_id", provisionerJob.ID))

	return nil
}

func (c Controller) Stop() {
	c.closeCh <- struct{}{}
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
