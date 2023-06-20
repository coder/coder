package schedule

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

type UserMaintenanceScheduleOptions struct {
	// Schedule is the cron schedule to use for maintenance windows for all
	// workspaces owned by the user.
	//
	// This value will be set to the parsed custom schedule of the user. If the
	// user doesn't have a custom schedule set, it will be set to the default
	// schedule (and UserSet will be false). If maintenance schedules are not
	// entitled or disabled instance-wide, this value will be nil to denote that
	// maintenance windows should not be used.
	Schedule *Schedule
	UserSet  bool
}

type UserMaintenanceScheduleStore interface {
	// GetUserMaintenanceScheduleOptions retrieves the maintenance schedule for
	// the given user. If the user has not set a custom schedule, the default
	// schedule will be returned. If maintenance schedules are not entitled or
	// disabled instance-wide, this will return a nil schedule.
	GetUserMaintenanceScheduleOptions(ctx context.Context, db database.Store, userID uuid.UUID) (UserMaintenanceScheduleOptions, error)
	// SetUserMaintenanceScheduleOptions sets the maintenance schedule for the
	// given user. If the given schedule is an empty string, the user's custom schedule will
	// be cleared and the default schedule will be used from now on. If
	// maintenance schedules are not entitled or disabled instance-wide, this
	// will do nothing and return a nil schedule.
	SetUserMaintenanceScheduleOptions(ctx context.Context, db database.Store, userID uuid.UUID, rawSchedule string) (UserMaintenanceScheduleOptions, error)
}

type agplUserMaintenanceScheduleStore struct{}

var _ UserMaintenanceScheduleStore = &agplUserMaintenanceScheduleStore{}

func NewAGPLUserMaintenanceScheduleStore() UserMaintenanceScheduleStore {
	return &agplUserMaintenanceScheduleStore{}
}

func (*agplUserMaintenanceScheduleStore) GetUserMaintenanceScheduleOptions(_ context.Context, _ database.Store, _ uuid.UUID) (UserMaintenanceScheduleOptions, error) {
	// User maintenance windows are not supported in AGPL.
	return UserMaintenanceScheduleOptions{
		Schedule: nil,
		UserSet:  false,
	}, nil
}

func (*agplUserMaintenanceScheduleStore) SetUserMaintenanceScheduleOptions(_ context.Context, _ database.Store, _ uuid.UUID, _ string) (UserMaintenanceScheduleOptions, error) {
	// User maintenance windows are not supported in AGPL.
	return UserMaintenanceScheduleOptions{
		Schedule: nil,
		UserSet:  false,
	}, nil
}
