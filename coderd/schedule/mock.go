package schedule

import (
	"context"

	"github.com/google/uuid"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications"
)

type MockTemplateScheduleStore struct {
	GetFn func(ctx context.Context, db database.Store, templateID uuid.UUID) (TemplateScheduleOptions, error)
	SetFn func(ctx context.Context, db database.Store, template database.Template, options TemplateScheduleOptions, enqueuer notifications.Enqueuer, logger slog.Logger) (database.Template, error)
}

var _ TemplateScheduleStore = MockTemplateScheduleStore{}

func (m MockTemplateScheduleStore) Get(ctx context.Context, db database.Store, templateID uuid.UUID) (TemplateScheduleOptions, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, db, templateID)
	}

	return NewAGPLTemplateScheduleStore().Get(ctx, db, templateID)
}

func (m MockTemplateScheduleStore) Set(ctx context.Context, db database.Store, template database.Template, options TemplateScheduleOptions, enqueuer notifications.Enqueuer, logger slog.Logger) (database.Template, error) {
	if m.SetFn != nil {
		return m.SetFn(ctx, db, template, options, enqueuer, logger)
	}

	return NewAGPLTemplateScheduleStore().Set(ctx, db, template, options, enqueuer, logger)
}

type MockUserQuietHoursScheduleStore struct {
	GetFn func(ctx context.Context, db database.Store, userID uuid.UUID) (UserQuietHoursScheduleOptions, error)
	SetFn func(ctx context.Context, db database.Store, userID uuid.UUID, schedule string) (UserQuietHoursScheduleOptions, error)
}

var _ UserQuietHoursScheduleStore = MockUserQuietHoursScheduleStore{}

func (m MockUserQuietHoursScheduleStore) Get(ctx context.Context, db database.Store, userID uuid.UUID) (UserQuietHoursScheduleOptions, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, db, userID)
	}

	return NewAGPLUserQuietHoursScheduleStore().Get(ctx, db, userID)
}

func (m MockUserQuietHoursScheduleStore) Set(ctx context.Context, db database.Store, userID uuid.UUID, schedule string) (UserQuietHoursScheduleOptions, error) {
	if m.SetFn != nil {
		return m.SetFn(ctx, db, userID, schedule)
	}

	return NewAGPLUserQuietHoursScheduleStore().Set(ctx, db, userID, schedule)
}
