package schedule

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

type TemplateScheduleOptions struct {
	UserSchedulingEnabled bool          `json:"user_scheduling_enabled"`
	DefaultTTL            time.Duration `json:"default_ttl"`
	// If MaxTTL is set, the workspace must be stopped before this time or it
	// will be stopped automatically.
	//
	// If set, users cannot disable automatic workspace shutdown.
	MaxTTL time.Duration `json:"max_ttl"`
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
		UserSchedulingEnabled: true,
		DefaultTTL:            time.Duration(tpl.DefaultTTL),
		// Disregard the value in the database, since MaxTTL is an enterprise
		// feature.
		MaxTTL: 0,
	}, nil
}

func (*agplTemplateScheduleStore) SetTemplateScheduleOptions(ctx context.Context, db database.Store, tpl database.Template, opts TemplateScheduleOptions) (database.Template, error) {
	return db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
		ID:         tpl.ID,
		UpdatedAt:  database.Now(),
		DefaultTTL: int64(opts.DefaultTTL),
		// Don't allow changing it, but keep the value in the DB (to avoid
		// clearing settings if the license has an issue).
		MaxTTL: tpl.MaxTTL,
	})
}
