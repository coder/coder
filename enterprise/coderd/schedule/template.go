package schedule

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	agpl "github.com/coder/coder/coderd/schedule"
)

// EnterpriseTemplateScheduleStore provides an agpl.TemplateScheduleStore that
// has all fields implemented for enterprise customers.
type EnterpriseTemplateScheduleStore struct {
	// UseRestartRequirement decides whether the RestartRequirement field should
	// be used instead of the MaxTTL field for determining the max deadline of a
	// workspace build. This value is determined by a feature flag, licensing,
	// and whether a default user quiet hours schedule is set.
	UseRestartRequirement atomic.Bool
}

var _ agpl.TemplateScheduleStore = &EnterpriseTemplateScheduleStore{}

func NewEnterpriseTemplateScheduleStore() *EnterpriseTemplateScheduleStore {
	return &EnterpriseTemplateScheduleStore{}
}

// Get implements agpl.TemplateScheduleStore.
func (s *EnterpriseTemplateScheduleStore) Get(ctx context.Context, db database.Store, templateID uuid.UUID) (agpl.TemplateScheduleOptions, error) {
	tpl, err := db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return agpl.TemplateScheduleOptions{}, err
	}

	// These extra checks have to be done before the conversion because we lose
	// precision and signs when converting to the agpl types from the database.
	if tpl.RestartRequirementDaysOfWeek < 0 {
		return agpl.TemplateScheduleOptions{}, xerrors.New("invalid restart requirement days, negative")
	}
	if tpl.RestartRequirementDaysOfWeek > 0b11111111 {
		return agpl.TemplateScheduleOptions{}, xerrors.New("invalid restart requirement days, too large")
	}
	err = agpl.VerifyTemplateRestartRequirement(uint8(tpl.RestartRequirementDaysOfWeek), tpl.RestartRequirementWeeks)
	if err != nil {
		return agpl.TemplateScheduleOptions{}, err
	}

	return agpl.TemplateScheduleOptions{
		UserAutostartEnabled:  tpl.AllowUserAutostart,
		UserAutostopEnabled:   tpl.AllowUserAutostop,
		DefaultTTL:            time.Duration(tpl.DefaultTTL),
		MaxTTL:                time.Duration(tpl.MaxTTL),
		UseRestartRequirement: s.UseRestartRequirement.Load(),
		RestartRequirement: agpl.TemplateRestartRequirement{
			DaysOfWeek: uint8(tpl.RestartRequirementDaysOfWeek),
			Weeks:      tpl.RestartRequirementWeeks,
		},
		FailureTTL:    time.Duration(tpl.FailureTTL),
		InactivityTTL: time.Duration(tpl.InactivityTTL),
		LockedTTL:     time.Duration(tpl.LockedTTL),
	}, nil
}

// Set implements agpl.TemplateScheduleStore.
func (*EnterpriseTemplateScheduleStore) Set(ctx context.Context, db database.Store, tpl database.Template, opts agpl.TemplateScheduleOptions) (database.Template, error) {
	if int64(opts.DefaultTTL) == tpl.DefaultTTL &&
		int64(opts.MaxTTL) == tpl.MaxTTL &&
		int16(opts.RestartRequirement.DaysOfWeek) == tpl.RestartRequirementDaysOfWeek &&
		opts.RestartRequirement.Weeks == tpl.RestartRequirementWeeks &&
		int64(opts.FailureTTL) == tpl.FailureTTL &&
		int64(opts.InactivityTTL) == tpl.InactivityTTL &&
		int64(opts.LockedTTL) == tpl.LockedTTL &&
		opts.UserAutostartEnabled == tpl.AllowUserAutostart &&
		opts.UserAutostopEnabled == tpl.AllowUserAutostop {
		// Avoid updating the UpdatedAt timestamp if nothing will be changed.
		return tpl, nil
	}

	err := agpl.VerifyTemplateRestartRequirement(opts.RestartRequirement.DaysOfWeek, opts.RestartRequirement.Weeks)
	if err != nil {
		return database.Template{}, err
	}

	var template database.Template
	err = db.InTx(func(tx database.Store) error {
		err := tx.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
			ID:                           tpl.ID,
			UpdatedAt:                    database.Now(),
			AllowUserAutostart:           opts.UserAutostartEnabled,
			AllowUserAutostop:            opts.UserAutostopEnabled,
			DefaultTTL:                   int64(opts.DefaultTTL),
			MaxTTL:                       int64(opts.MaxTTL),
			RestartRequirementDaysOfWeek: int16(opts.RestartRequirement.DaysOfWeek),
			RestartRequirementWeeks:      opts.RestartRequirement.Weeks,
			FailureTTL:                   int64(opts.FailureTTL),
			InactivityTTL:                int64(opts.InactivityTTL),
			LockedTTL:                    int64(opts.LockedTTL),
		})
		if err != nil {
			return xerrors.Errorf("update template schedule: %w", err)
		}

		var lockedAt time.Time
		if opts.UpdateWorkspaceLockedAt {
			lockedAt = database.Now()
		}

		// If we updated the locked_ttl we need to update all the workspaces deleting_at
		// to ensure workspaces are being cleaned up correctly. Similarly if we are
		// disabling it (by passing 0), then we want to delete nullify the deleting_at
		// fields of all the template workspaces.
		err = tx.UpdateWorkspacesLockedDeletingAtByTemplateID(ctx, database.UpdateWorkspacesLockedDeletingAtByTemplateIDParams{
			TemplateID:  tpl.ID,
			LockedTtlMs: opts.LockedTTL.Milliseconds(),
			LockedAt:    lockedAt,
		})
		if err != nil {
			return xerrors.Errorf("update deleting_at of all workspaces for new locked_ttl %q: %w", opts.LockedTTL, err)
		}

		if opts.UpdateWorkspaceLastUsedAt {
			err = tx.UpdateTemplateWorkspacesLastUsedAt(ctx, database.UpdateTemplateWorkspacesLastUsedAtParams{
				TemplateID: tpl.ID,
				LastUsedAt: database.Now(),
			})
			if err != nil {
				return xerrors.Errorf("update template workspaces last_used_at: %w", err)
			}
		}

		// TODO: update all workspace max_deadlines to be within new bounds
		template, err = tx.GetTemplateByID(ctx, tpl.ID)
		if err != nil {
			return xerrors.Errorf("get updated template schedule: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		return database.Template{}, err
	}

	return template, nil
}
