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
		UserAutostartEnabled: tpl.AllowUserAutostart,
		UserAutostopEnabled:  tpl.AllowUserAutostop,
		DefaultTTL:           time.Duration(tpl.DefaultTTL),
		RestartRequirement: agpl.TemplateRestartRequirement{
			DaysOfWeek: uint8(tpl.RestartRequirementDaysOfWeek),
			Weeks:      tpl.RestartRequirementWeeks,
		},
		FailureTTL:    time.Duration(tpl.FailureTTL),
		InactivityTTL: time.Duration(tpl.InactivityTTL),
		LockedTTL:     time.Duration(tpl.LockedTTL),
	}, nil
}

// SetTemplateScheduleOptions implements agpl.TemplateScheduleStore.
func (*EnterpriseTemplateScheduleStore) SetTemplateScheduleOptions(ctx context.Context, db database.Store, tpl database.Template, opts agpl.TemplateScheduleOptions) (database.Template, error) {
	if int64(opts.DefaultTTL) == tpl.DefaultTTL &&
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

	template, err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
		ID:                           tpl.ID,
		UpdatedAt:                    database.Now(),
		AllowUserAutostart:           opts.UserAutostartEnabled,
		AllowUserAutostop:            opts.UserAutostopEnabled,
		DefaultTTL:                   int64(opts.DefaultTTL),
		RestartRequirementDaysOfWeek: int16(opts.RestartRequirement.DaysOfWeek),
		RestartRequirementWeeks:      opts.RestartRequirement.Weeks,
		FailureTTL:                   int64(opts.FailureTTL),
		InactivityTTL:                int64(opts.InactivityTTL),
		LockedTTL:                    int64(opts.LockedTTL),
	})
	if err != nil {
		return database.Template{}, xerrors.Errorf("update template schedule: %w", err)
	}

	return template, nil
}
