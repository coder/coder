package schedule

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

type MockTemplateScheduleStore struct {
	GetFn func(ctx context.Context, db database.Store, templateID uuid.UUID) (TemplateScheduleOptions, error)
	SetFn func(ctx context.Context, db database.Store, template database.Template, options TemplateScheduleOptions) (database.Template, error)
}

var _ TemplateScheduleStore = MockTemplateScheduleStore{}

func (m MockTemplateScheduleStore) GetTemplateScheduleOptions(ctx context.Context, db database.Store, templateID uuid.UUID) (TemplateScheduleOptions, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, db, templateID)
	}

	return NewAGPLTemplateScheduleStore().GetTemplateScheduleOptions(ctx, db, templateID)
}

func (m MockTemplateScheduleStore) SetTemplateScheduleOptions(ctx context.Context, db database.Store, template database.Template, options TemplateScheduleOptions) (database.Template, error) {
	if m.SetFn != nil {
		return m.SetFn(ctx, db, template, options)
	}

	return NewAGPLTemplateScheduleStore().SetTemplateScheduleOptions(ctx, db, template, options)
}

type MockUserQuietHoursScheduleStore struct {
	GetFn func(ctx context.Context, db database.Store, userID uuid.UUID) (UserQuietHoursScheduleOptions, error)
	SetFn func(ctx context.Context, db database.Store, userID uuid.UUID, schedule string) (UserQuietHoursScheduleOptions, error)
}

var _ UserQuietHoursScheduleStore = MockUserQuietHoursScheduleStore{}

func (m MockUserQuietHoursScheduleStore) GetUserQuietHoursScheduleOptions(ctx context.Context, db database.Store, userID uuid.UUID) (UserQuietHoursScheduleOptions, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, db, userID)
	}

	return NewAGPLUserQuietHoursScheduleStore().GetUserQuietHoursScheduleOptions(ctx, db, userID)
}

func (m MockUserQuietHoursScheduleStore) SetUserQuietHoursScheduleOptions(ctx context.Context, db database.Store, userID uuid.UUID, schedule string) (UserQuietHoursScheduleOptions, error) {
	if m.SetFn != nil {
		return m.SetFn(ctx, db, userID, schedule)
	}

	return NewAGPLUserQuietHoursScheduleStore().SetUserQuietHoursScheduleOptions(ctx, db, userID, schedule)
}
