package schedule

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

var DaysOfWeek = []time.Weekday{
	time.Sunday,
	time.Monday,
	time.Tuesday,
	time.Wednesday,
	time.Thursday,
	time.Friday,
	time.Saturday,
}

type TemplateRestartRequirement struct {
	// DaysOfWeek is a bitmap of which days of the week the workspace must be
	// restarted. If fully zero, the workspace is not required to be restarted
	// ever.
	//
	// First bit is Sunday, second bit is Monday, ..., seventh bit is Saturday,
	// eighth bit is unused.
	DaysOfWeek uint8
}

// Days returns the days of the week that the workspace must be restarted.
func (r TemplateRestartRequirement) Days() []time.Weekday {
	days := make([]time.Weekday, 0, 7)
	for i, day := range DaysOfWeek {
		if r.DaysOfWeek&(1<<uint(i)) != 0 {
			days = append(days, day)
		}
	}
	return days
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

type TemplateScheduleOptions struct {
	UserAutostartEnabled bool          `json:"user_autostart_enabled"`
	UserAutostopEnabled  bool          `json:"user_autostop_enabled"`
	DefaultTTL           time.Duration `json:"default_ttl"`
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
}

// TemplateScheduleStore provides an interface for retrieving template
// scheduling options set by the template/site admin.
type TemplateScheduleStore interface {
	GetTemplateScheduleOptions(ctx context.Context, db database.Store, templateID uuid.UUID) (TemplateScheduleOptions, error)
	SetTemplateScheduleOptions(ctx context.Context, db database.Store, template database.Template, opts TemplateScheduleOptions) (database.Template, error)
}

type agplTemplateScheduleStore struct{}

var _ TemplateScheduleStore = &agplTemplateScheduleStore{}

func NewAGPLTemplateScheduleStore() TemplateScheduleStore {
	return &agplTemplateScheduleStore{}
}

func (*agplTemplateScheduleStore) GetTemplateScheduleOptions(ctx context.Context, db database.Store, templateID uuid.UUID) (TemplateScheduleOptions, error) {
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
		RestartRequirement: TemplateRestartRequirement{
			DaysOfWeek: 0,
		},
		FailureTTL:    0,
		InactivityTTL: 0,
		LockedTTL:     0,
	}, nil
}

func (*agplTemplateScheduleStore) SetTemplateScheduleOptions(ctx context.Context, db database.Store, tpl database.Template, opts TemplateScheduleOptions) (database.Template, error) {
	if int64(opts.DefaultTTL) == tpl.DefaultTTL {
		// Avoid updating the UpdatedAt timestamp if nothing will be changed.
		return tpl, nil
	}

	// TODO: fix storage to use new restart requirement
	return db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
		ID:         tpl.ID,
		UpdatedAt:  database.Now(),
		DefaultTTL: int64(opts.DefaultTTL),
		// Don't allow changing these settings, but keep the value in the DB (to
		// avoid clearing settings if the license has an issue).
		AllowUserAutostart: tpl.AllowUserAutostart,
		AllowUserAutostop:  tpl.AllowUserAutostop,
		MaxTTL:             tpl.MaxTTL,
		FailureTTL:         tpl.FailureTTL,
		InactivityTTL:      tpl.InactivityTTL,
		LockedTTL:          tpl.LockedTTL,
	})
}
