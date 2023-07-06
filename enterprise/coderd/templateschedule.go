package coderd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/schedule"
)

type EnterpriseTemplateScheduleStore struct{}

var _ schedule.TemplateScheduleStore = &EnterpriseTemplateScheduleStore{}

func (*EnterpriseTemplateScheduleStore) GetTemplateScheduleOptions(ctx context.Context, db database.Store, templateID uuid.UUID) (schedule.TemplateScheduleOptions, error) {
	tpl, err := db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return schedule.TemplateScheduleOptions{}, err
	}

	return schedule.TemplateScheduleOptions{
		UserAutostartEnabled: tpl.AllowUserAutostart,
		UserAutostopEnabled:  tpl.AllowUserAutostop,
		DefaultTTL:           time.Duration(tpl.DefaultTTL),
		MaxTTL:               time.Duration(tpl.MaxTTL),
		FailureTTL:           time.Duration(tpl.FailureTTL),
		InactivityTTL:        time.Duration(tpl.InactivityTTL),
		LockedTTL:            time.Duration(tpl.LockedTTL),
	}, nil
}

func (*EnterpriseTemplateScheduleStore) SetTemplateScheduleOptions(ctx context.Context, db database.Store, tpl database.Template, opts schedule.TemplateScheduleOptions) (database.Template, error) {
	if int64(opts.DefaultTTL) == tpl.DefaultTTL &&
		int64(opts.MaxTTL) == tpl.MaxTTL &&
		int64(opts.FailureTTL) == tpl.FailureTTL &&
		int64(opts.InactivityTTL) == tpl.InactivityTTL &&
		int64(opts.LockedTTL) == tpl.LockedTTL &&
		opts.UserAutostartEnabled == tpl.AllowUserAutostart &&
		opts.UserAutostopEnabled == tpl.AllowUserAutostop {
		// Avoid updating the UpdatedAt timestamp if nothing will be changed.
		return tpl, nil
	}

	var template database.Template
	err := db.InTx(func(db database.Store) error {
		err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
			ID:                 tpl.ID,
			UpdatedAt:          database.Now(),
			AllowUserAutostart: opts.UserAutostartEnabled,
			AllowUserAutostop:  opts.UserAutostopEnabled,
			DefaultTTL:         int64(opts.DefaultTTL),
			MaxTTL:             int64(opts.MaxTTL),
			FailureTTL:         int64(opts.FailureTTL),
			InactivityTTL:      int64(opts.InactivityTTL),
			LockedTTL:          int64(opts.LockedTTL),
		})
		if err != nil {
			return xerrors.Errorf("update template schedule: %w", err)
		}

		// Update all workspaces using the template to set the user defined schedule
		// to be within the new bounds. This essentially does the following for each
		// workspace using the template.
		//   if (template.ttl != NULL) {
		//     workspace.ttl = min(workspace.ttl, template.ttl)
		//   }
		//
		// NOTE: this does not apply to currently running workspaces as their
		// schedule information is committed to the workspace_build during start.
		// This limitation is displayed to the user while editing the template.
		if opts.MaxTTL > 0 {
			err = db.UpdateWorkspaceTTLToBeWithinTemplateMax(ctx, database.UpdateWorkspaceTTLToBeWithinTemplateMaxParams{
				TemplateID:     tpl.ID,
				TemplateMaxTTL: int64(opts.MaxTTL),
			})
			if err != nil {
				return xerrors.Errorf("update TTL of all workspaces on template to be within new template max TTL: %w", err)
			}
		}

		// If we updated the locked_ttl we need to update all the workspaces deleting_at
		// to ensure workspaces are being cleaned up correctly. Similarly if we are
		// disabling it (by passing 0), then we want to delete nullify the deleting_at
		// fields of all the template workspaces.
		err = db.UpdateWorkspacesDeletingAtByTemplateID(ctx, database.UpdateWorkspacesDeletingAtByTemplateIDParams{
			TemplateID:  tpl.ID,
			LockedTtlMs: opts.LockedTTL.Milliseconds(),
		})
		if err != nil {
			return xerrors.Errorf("update deleting_at of all workspaces for new locked_ttl %q: %w", opts.LockedTTL, err)
		}

		template, err = db.GetTemplateByID(ctx, tpl.ID)
		if err != nil {
			return xerrors.Errorf("get updated template schedule: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return database.Template{}, xerrors.Errorf("in tx: %w", err)
	}

	return template, nil
}
