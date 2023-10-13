package schedule

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/tracing"
)

const MaxTemplateAutostopRequirementWeeks = 16

func TemplateAutostopRequirementEpoch(loc *time.Location) time.Time {
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

type TemplateAutostartRequirement struct {
	// DaysOfWeek is a bitmap of which days of the week the workspace is allowed
	// to be auto started. If fully zero, the workspace is not allowed to be auto started.
	//
	// First bit is Monday, ..., seventh bit is Sunday, eighth bit is unused.
	DaysOfWeek uint8
}

func (r TemplateAutostartRequirement) DaysMap() map[time.Weekday]bool {
	return daysMap(r.DaysOfWeek)
}

type TemplateAutostopRequirement struct {
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
func (r TemplateAutostopRequirement) DaysMap() map[time.Weekday]bool {
	return daysMap(r.DaysOfWeek)
}

// daysMap returns a map of the days of the week that are specified in the
// bitmap.
func daysMap(daysOfWeek uint8) map[time.Weekday]bool {
	days := make(map[time.Weekday]bool)
	for i, day := range DaysOfWeek {
		days[day] = daysOfWeek&(1<<uint(i)) != 0
	}
	return days
}

// VerifyTemplateAutostopRequirement returns an error if the autostop
// requirement is invalid.
func VerifyTemplateAutostopRequirement(days uint8, weeks int64) error {
	if days&0b10000000 != 0 {
		return xerrors.New("invalid autostop requirement days, last bit is set")
	}
	if days > 0b11111111 {
		return xerrors.New("invalid autostop requirement days, too large")
	}
	if weeks < 1 {
		return xerrors.New("invalid autostop requirement weeks, less than 1")
	}
	if weeks > MaxTemplateAutostopRequirementWeeks {
		return xerrors.New("invalid autostop requirement weeks, too large")
	}
	return nil
}

// VerifyTemplateAutostartRequirement returns an error if the autostart
// requirement is invalid.
func VerifyTemplateAutostartRequirement(days uint8) error {
	if days&0b10000000 != 0 {
		return xerrors.New("invalid autostart requirement days, last bit is set")
	}
	if days > 0b11111111 {
		return xerrors.New("invalid autostart requirement days, too large")
	}

	return nil
}

type TemplateScheduleOptions struct {
	UserAutostartEnabled bool          `json:"user_autostart_enabled"`
	UserAutostopEnabled  bool          `json:"user_autostop_enabled"`
	DefaultTTL           time.Duration `json:"default_ttl"`
	// TODO(@dean): remove MaxTTL once autostop_requirement is matured and the
	// default
	MaxTTL time.Duration `json:"max_ttl"`
	// UseAutostopRequirement dictates whether the autostop requirement should
	// be used instead of MaxTTL. This is governed by the feature flag and
	// licensing.
	// TODO(@dean): remove this when we remove max_tll
	UseAutostopRequirement bool
	// AutostopRequirement dictates when the workspace must be restarted. This
	// used to be handled by MaxTTL.
	AutostopRequirement TemplateAutostopRequirement `json:"autostop_requirement"`
	// AutostartRequirement dictates when the workspace can be auto started.
	AutostartRequirement TemplateAutostartRequirement `json:"autostart_requirement"`
	// FailureTTL dictates the duration after which failed workspaces will be
	// stopped automatically.
	FailureTTL time.Duration `json:"failure_ttl"`
	// TimeTilDormant dictates the duration after which inactive workspaces will
	// go dormant.
	TimeTilDormant time.Duration `json:"time_til_dormant"`
	// TimeTilDormantAutoDelete dictates the duration after which dormant workspaces will be
	// permanently deleted.
	TimeTilDormantAutoDelete time.Duration `json:"time_til_dormant_autodelete"`
	// UpdateWorkspaceLastUsedAt updates the template's workspaces'
	// last_used_at field. This is useful for preventing updates to the
	// templates inactivity_ttl immediately triggering a dormant action against
	// workspaces whose last_used_at field violates the new template
	// inactivity_ttl threshold.
	UpdateWorkspaceLastUsedAt bool `json:"update_workspace_last_used_at"`
	// UpdateWorkspaceDormantAt updates the template's workspaces'
	// dormant_at field. This is useful for preventing updates to the
	// templates locked_ttl immediately triggering a delete action against
	// workspaces whose dormant_at field violates the new template time_til_dormant_autodelete
	// threshold.
	UpdateWorkspaceDormantAt bool `json:"update_workspace_dormant_at"`
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
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

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
		// Disregard the values in the database, since AutostopRequirement,
		// FailureTTL, TimeTilDormant, and TimeTilDormantAutoDelete are enterprise features.
		UseAutostopRequirement: false,
		MaxTTL:                 0,
		AutostartRequirement: TemplateAutostartRequirement{
			// Default to allowing all days for AGPL
			DaysOfWeek: 0b01111111,
		},
		AutostopRequirement: TemplateAutostopRequirement{
			// No days means never. The weeks value should always be greater
			// than zero though.
			DaysOfWeek: 0,
			Weeks:      1,
		},
		FailureTTL:               0,
		TimeTilDormant:           0,
		TimeTilDormantAutoDelete: 0,
	}, nil
}

func (*agplTemplateScheduleStore) Set(ctx context.Context, db database.Store, tpl database.Template, opts TemplateScheduleOptions) (database.Template, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if int64(opts.DefaultTTL) == tpl.DefaultTTL {
		// Avoid updating the UpdatedAt timestamp if nothing will be changed.
		return tpl, nil
	}

	var template database.Template
	err := db.InTx(func(db database.Store) error {
		err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
			ID:         tpl.ID,
			UpdatedAt:  dbtime.Now(),
			DefaultTTL: int64(opts.DefaultTTL),
			// Don't allow changing these settings, but keep the value in the DB (to
			// avoid clearing settings if the license has an issue).
			MaxTTL:                        tpl.MaxTTL,
			AutostopRequirementDaysOfWeek: tpl.AutostopRequirementDaysOfWeek,
			AutostopRequirementWeeks:      tpl.AutostopRequirementWeeks,
			AutostartBlockDaysOfWeek:      tpl.AutostartBlockDaysOfWeek,
			AllowUserAutostart:            tpl.AllowUserAutostart,
			AllowUserAutostop:             tpl.AllowUserAutostop,
			FailureTTL:                    tpl.FailureTTL,
			TimeTilDormant:                tpl.TimeTilDormant,
			TimeTilDormantAutoDelete:      tpl.TimeTilDormantAutoDelete,
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
