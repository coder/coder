package schedule

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	agpl "github.com/coder/coder/coderd/schedule"
)

// EnterpriseTemplateScheduleStore provides an agpl.TemplateScheduleStore that
// has all fields implemented for enterprise customers.
type EnterpriseTemplateScheduleStore struct{}

var _ agpl.TemplateScheduleStore = &EnterpriseTemplateScheduleStore{}

func NewEnterpriseTemplateScheduleStore() agpl.TemplateScheduleStore {
	return &EnterpriseTemplateScheduleStore{}
}

// GetTemplateScheduleOptions implements agpl.TemplateScheduleStore.
func (*EnterpriseTemplateScheduleStore) GetTemplateScheduleOptions(ctx context.Context, db database.Store, templateID uuid.UUID) (agpl.TemplateScheduleOptions, error) {
	tpl, err := db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return agpl.TemplateScheduleOptions{}, err
	}

	return agpl.TemplateScheduleOptions{
		UserAutostartEnabled: tpl.AllowUserAutostart,
		UserAutostopEnabled:  tpl.AllowUserAutostop,
		DefaultTTL:           time.Duration(tpl.DefaultTTL),
		// TODO: fix storage to use new restart requirement
		RestartRequirement: agpl.TemplateRestartRequirement{
			DaysOfWeek: 0b01111111,
		},
		FailureTTL:    time.Duration(tpl.FailureTTL),
		InactivityTTL: time.Duration(tpl.InactivityTTL),
		LockedTTL:     time.Duration(tpl.LockedTTL),
	}, nil
}

// SetTemplateScheduleOptions implements agpl.TemplateScheduleStore.
func (*EnterpriseTemplateScheduleStore) SetTemplateScheduleOptions(ctx context.Context, db database.Store, tpl database.Template, opts agpl.TemplateScheduleOptions) (database.Template, error) {
	if int64(opts.DefaultTTL) == tpl.DefaultTTL &&
		// TODO: fix storage to use new restart requirement
		int64(opts.RestartRequirement.DaysOfWeek) == tpl.MaxTTL &&
		int64(opts.FailureTTL) == tpl.FailureTTL &&
		int64(opts.InactivityTTL) == tpl.InactivityTTL &&
		int64(opts.LockedTTL) == tpl.LockedTTL &&
		opts.UserAutostartEnabled == tpl.AllowUserAutostart &&
		opts.UserAutostopEnabled == tpl.AllowUserAutostop {
		// Avoid updating the UpdatedAt timestamp if nothing will be changed.
		return tpl, nil
	}

	template, err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
		ID:                 tpl.ID,
		UpdatedAt:          database.Now(),
		AllowUserAutostart: opts.UserAutostartEnabled,
		AllowUserAutostop:  opts.UserAutostopEnabled,
		DefaultTTL:         int64(opts.DefaultTTL),
		// TODO: fix storage to use new restart requirement
		MaxTTL:        0,
		FailureTTL:    int64(opts.FailureTTL),
		InactivityTTL: int64(opts.InactivityTTL),
		LockedTTL:     int64(opts.LockedTTL),
	})
	if err != nil {
		return database.Template{}, xerrors.Errorf("update template schedule: %w", err)
	}

	return template, nil
}
