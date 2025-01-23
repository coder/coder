package prebuilds

import (
	"context"
	"fmt"
	"math"
	"time"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

type Controller struct {
	store  database.Store
	logger slog.Logger

	nudgeCh chan *uuid.UUID
	closeCh chan struct{}
}

func NewController(logger slog.Logger, store database.Store) *Controller {
	return &Controller{
		store:   store,
		logger:  logger,
		nudgeCh: make(chan *uuid.UUID, 1),
		closeCh: make(chan struct{}, 1),
	}
}

func (c Controller) Loop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 15) // TODO: configurable? 1m probably lowest valid value
	defer ticker.Stop()

	// TODO: create new authz role
	ctx = dbauthz.AsSystemRestricted(ctx)

	for {
		select {
		// Accept nudges from outside the control loop to trigger a new iteration.
		case template := <-c.nudgeCh:
			c.reconcile(ctx, template)
		// Trigger a new iteration on each tick.
		case <-ticker.C:
			c.reconcile(ctx, nil)
		case <-c.closeCh:
			c.logger.Info(ctx, "control loop stopped")
			return
		case <-ctx.Done():
			c.logger.Error(context.Background(), "control loop exited: %w", ctx.Err())
			return
		}
	}
}

func (c Controller) ReconcileTemplate(templateID uuid.UUID) {
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
			c.logger.Debug(innerCtx, "could not fetch template(s)", slog.F("template_id", templateID), slog.F("all", templateID == nil))
			return xerrors.Errorf("fetch template(s): %w", err)
		}

		if len(templates) == 0 {
			c.logger.Debug(innerCtx, "no templates found", slog.F("template_id", templateID), slog.F("all", templateID == nil))
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

		results, err := db.GetTemplatePrebuildState(innerCtx, template.ID)
		if err != nil {
			return xerrors.Errorf("failed to retrieve template's prebuild state: %w", err)
		}

		for _, result := range results {
			desired, actual, extraneous, inProgress := result.Desired, result.Actual, result.Extraneous, result.InProgress

			// If the template has become deleted or deprecated since the last reconciliation, we need to ensure we
			// scale those prebuilds down to zero.
			if result.Deleted || result.Deprecated {
				desired = 0
			}

			toCreate := math.Max(0, float64(desired-(actual+inProgress)))
			// TODO: we might need to get inProgress here by job type (i.e. create or destroy), then we wouldn't have this ambiguity
			toDestroy := math.Max(0, float64(extraneous-inProgress))

			c.logger.Info(innerCtx, "template prebuild state retrieved",
				slog.F("template_id", template.ID), slog.F("to_create", toCreate), slog.F("to_destroy", toDestroy),
				slog.F("desired", desired), slog.F("actual", actual),
				slog.F("extraneous", extraneous), slog.F("in_progress", inProgress))
		}

		return nil
	}, &database.TxOptions{
		// TODO: isolation
		ReadOnly:     true,
		TxIdentifier: "template_prebuilds",
	})
	if err != nil {
		logger.Error(ctx, "failed to acquire database transaction", slog.Error(err))
	}

	// trigger n InsertProvisionerJob calls to scale up or down instances
	return nil
}

func (c Controller) Stop() {
	c.closeCh <- struct{}{}
}
