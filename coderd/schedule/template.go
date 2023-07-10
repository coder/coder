package schedule

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

type TemplateScheduleOptions struct {
	UserAutostartEnabled bool          `json:"user_autostart_enabled"`
	UserAutostopEnabled  bool          `json:"user_autostop_enabled"`
	DefaultTTL           time.Duration `json:"default_ttl"`
	// If MaxTTL is set, the workspace must be stopped before this time or it
	// will be stopped automatically.
	//
	// If set, users cannot disable automatic workspace shutdown.
	MaxTTL time.Duration `json:"max_ttl"`
	// FailureTTL dictates the duration after which failed workspaces will be stopped automatically.
	FailureTTL time.Duration `json:"failure_ttl"`
	// InactivityTTL dictates the duration after which inactive workspaces will be locked.
	InactivityTTL time.Duration `json:"inactivity_ttl"`
	// LockedTTL dictates the duration after which locked workspaces will be permanently deleted.
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
		// Disregard the values in the database, since MaxTTL, FailureTTL, InactivityTTL, and LockedTTL are enterprise
		// features.
		MaxTTL:        0,
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

	return db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
		ID:         tpl.ID,
		UpdatedAt:  database.Now(),
		DefaultTTL: int64(opts.DefaultTTL),
		// Don't allow changing it, but keep the value in the DB (to avoid
		// clearing settings if the license has an issue).
		AllowUserAutostart: tpl.AllowUserAutostart,
		AllowUserAutostop:  tpl.AllowUserAutostop,
		MaxTTL:             tpl.MaxTTL,
		FailureTTL:         tpl.FailureTTL,
		InactivityTTL:      tpl.InactivityTTL,
		LockedTTL:          tpl.LockedTTL,
	})
}
