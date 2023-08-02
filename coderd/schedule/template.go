package schedule

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

const MaxTemplateRestartRequirementWeeks = 16

func TemplateRestartRequirementEpoch(loc *time.Location) time.Time {
	// The "first week" starts on January 2nd, 2023, which is the first Monday
	// of 2023. All other weeks are counted using modulo arithmetic from that
	// date.
	return time.Date(2023, time.January, 2, 0, 0, 0, 0, loc)
}

// DaysOfWeek intentionally starts on Monday as opposed to Sunday so the weekend
// days are contiguous in the bitmap. This matters greatly when doing restarts
// every second week or more to avoid workspaces restarting "at the start" of
// the week rather than "at the end" of the week.
var DaysOfWeek = []time.Weekday{
	time.Monday,
	time.Tuesday,
	time.Wednesday,
	time.Thursday,
	time.Friday,
	time.Saturday,
	time.Sunday,
}

type TemplateRestartRequirement struct {
	// DaysOfWeek is a bitmap of which days of the week the workspace must be
	// restarted. If fully zero, the workspace is not required to be restarted
	// ever.
	//
	// First bit is Monday, ..., seventh bit is Sunday, eighth bit is unused.
	DaysOfWeek uint8
	// Weeks is the amount of weeks between restarts. If 0 or 1, the workspace
	// is restarted weekly in accordance with DaysOfWeek. If 2, the workspace is
	// restarted every other week. And so forth.
	//
	// The limit for this value is 16, which is roughly 4 months.
	//
	// The "first week" starts on January 2nd, 2023, which is the first Monday
	// of 2023. All other weeks are counted using modulo arithmetic from that
	// date.
	Weeks int64
}

// DaysMap returns a map of the days of the week that the workspace must be
// restarted.
func (r TemplateRestartRequirement) DaysMap() map[time.Weekday]bool {
	days := make(map[time.Weekday]bool)
	for i, day := range DaysOfWeek {
		days[day] = r.DaysOfWeek&(1<<uint(i)) != 0
	}
	return days
}

// VerifyTemplateRestartRequirement returns an error if the restart requirement
// is invalid.
func VerifyTemplateRestartRequirement(days uint8, weeks int64) error {
	if days&0b10000000 != 0 {
		return xerrors.New("invalid restart requirement days, last bit is set")
	}
	if days > 0b11111111 {
		return xerrors.New("invalid restart requirement days, too large")
	}
	if weeks < 0 {
		return xerrors.New("invalid restart requirement weeks, negative")
	}
	if weeks > MaxTemplateRestartRequirementWeeks {
		return xerrors.New("invalid restart requirement weeks, too large")
	}
	return nil
}

type TemplateScheduleOptions struct {
	UserAutostartEnabled bool          `json:"user_autostart_enabled"`
	UserAutostopEnabled  bool          `json:"user_autostop_enabled"`
	DefaultTTL           time.Duration `json:"default_ttl"`
	// TODO(@dean): remove MaxTTL once restart_requirement is matured and the
	// default
	MaxTTL time.Duration `json:"max_ttl"`
	// UseRestartRequirement dictates whether the restart requirement should be
	// used instead of MaxTTL. This is governed by the feature flag and
	// licensing.
	// TODO(@dean): remove this when we remove max_tll
	UseRestartRequirement bool
	// RestartRequirement dictates when the workspace must be restarted. This
	// used to be handled by MaxTTL.
	RestartRequirement TemplateRestartRequirement `json:"restart_requirement"`
	// FailureTTL dictates the duration after which failed workspaces will be
	// stopped automatically.
	FailureTTL time.Duration `json:"failure_ttl"`
	// InactivityTTL dictates the duration after which inactive workspaces will
	// be locked.
	InactivityTTL time.Duration `json:"inactivity_ttl"`
	// LockedTTL dictates the duration after which locked workspaces will be
	// permanently deleted.
	LockedTTL time.Duration `json:"locked_ttl"`
	// UpdateWorkspaceLastUsedAt updates the template's workspaces'
	// last_used_at field. This is useful for preventing updates to the
	// templates inactivity_ttl immediately triggering a lock action against
	// workspaces whose last_used_at field violates the new template
	// inactivity_ttl threshold.
	UpdateWorkspaceLastUsedAt bool `json:"update_workspace_last_used_at"`
	// UpdateWorkspaceLockedAt updates the template's workspaces'
	// locked_at field. This is useful for preventing updates to the
	// templates locked_ttl immediately triggering a delete action against
	// workspaces whose locked_at field violates the new template locked_ttl
	// threshold.
	UpdateWorkspaceLockedAt bool `json:"update_workspace_locked_at"`
}

// TemplateScheduleStore provides an interface for retrieving template
// scheduling options set by the template/site admin.
type TemplateScheduleStore interface {
	Get(ctx context.Context, db database.Store, templateID uuid.UUID) (TemplateScheduleOptions, error)
	Set(ctx context.Context, db database.Store, template database.Template, opts TemplateScheduleOptions) (database.Template, error)
}

type agplTemplateScheduleStore struct{}

var _ TemplateScheduleStore = &agplTemplateScheduleStore{}

func NewAGPLTemplateScheduleStore() TemplateScheduleStore {
	return &agplTemplateScheduleStore{}
}

func (*agplTemplateScheduleStore) Get(ctx context.Context, db database.Store, templateID uuid.UUID) (TemplateScheduleOptions, error) {
	tpl, err := db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return TemplateScheduleOptions{}, err
	}

	return TemplateScheduleOptions{
		// Disregard the values in the database, since user scheduling is an
		// enterprise feature.
		UserAutostartEnabled: true,
		UserAutostopEnabled:  true,
		DefaultTTL:           time.Duration(tpl.DefaultTTL),
		// Disregard the values in the database, since RestartRequirement,
		// FailureTTL, InactivityTTL, and LockedTTL are enterprise features.
		UseRestartRequirement: false,
		MaxTTL:                0,
		RestartRequirement: TemplateRestartRequirement{
			DaysOfWeek: 0,
			Weeks:      0,
		},
		FailureTTL:    0,
		InactivityTTL: 0,
		LockedTTL:     0,
	}, nil
}

func (*agplTemplateScheduleStore) Set(ctx context.Context, db database.Store, tpl database.Template, opts TemplateScheduleOptions) (database.Template, error) {
	if int64(opts.DefaultTTL) == tpl.DefaultTTL {
		// Avoid updating the UpdatedAt timestamp if nothing will be changed.
		return tpl, nil
	}

	var template database.Template
	err := db.InTx(func(db database.Store) error {
		err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
			ID:         tpl.ID,
			UpdatedAt:  database.Now(),
			DefaultTTL: int64(opts.DefaultTTL),
			// Don't allow changing these settings, but keep the value in the DB (to
			// avoid clearing settings if the license has an issue).
			MaxTTL:                       tpl.MaxTTL,
			RestartRequirementDaysOfWeek: tpl.RestartRequirementDaysOfWeek,
			RestartRequirementWeeks:      tpl.RestartRequirementWeeks,
			AllowUserAutostart:           tpl.AllowUserAutostart,
			AllowUserAutostop:            tpl.AllowUserAutostop,
			FailureTTL:                   tpl.FailureTTL,
			InactivityTTL:                tpl.InactivityTTL,
			LockedTTL:                    tpl.LockedTTL,
		})
		if err != nil {
			return xerrors.Errorf("update template schedule: %w", err)
		}

		template, err = db.GetTemplateByID(ctx, tpl.ID)
		if err != nil {
			return xerrors.Errorf("fetch updated template: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		return database.Template{}, err
	}

	return template, err
}
