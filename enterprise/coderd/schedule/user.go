package schedule

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	agpl "github.com/coder/coder/coderd/schedule"
)

// enterpriseUserQuietHoursScheduleStore provides an
// agpl.UserQuietHoursScheduleStore that has all fields implemented for
// enterprise customers.
type enterpriseUserQuietHoursScheduleStore struct {
	defaultSchedule string
}

var _ agpl.UserQuietHoursScheduleStore = &enterpriseUserQuietHoursScheduleStore{}

func NewEnterpriseUserQuietHoursScheduleStore(defaultSchedule string) (agpl.UserQuietHoursScheduleStore, error) {
	if defaultSchedule == "" {
		return nil, xerrors.Errorf("default schedule must be set")
	}

	s := &enterpriseUserQuietHoursScheduleStore{
		defaultSchedule: defaultSchedule,
	}

	_, err := s.parseSchedule(defaultSchedule)
	if err != nil {
		return nil, xerrors.Errorf("parse default schedule: %w", err)
	}

	return s, nil
}

func (s *enterpriseUserQuietHoursScheduleStore) parseSchedule(rawSchedule string) (agpl.UserQuietHoursScheduleOptions, error) {
	userSet := true
	if strings.TrimSpace(rawSchedule) == "" {
		userSet = false
		rawSchedule = s.defaultSchedule
	}

	sched, err := agpl.Daily(rawSchedule)
	if err != nil {
		// This shouldn't get hit during Gets, only Sets.
		return agpl.UserQuietHoursScheduleOptions{}, xerrors.Errorf("parse daily schedule %q: %w", rawSchedule, err)
	}
	if strings.HasPrefix(sched.Time(), "cron(") {
		// Times starting with "cron(" mean it isn't a single time and probably
		// a range or a list of times as a cron expression. We only support
		// single times for user quiet hours schedules.
		// This shouldn't get hit during Gets, only Sets.
		return agpl.UserQuietHoursScheduleOptions{}, xerrors.Errorf("daily schedule %q has more than one time: %v", rawSchedule, sched.Time())
	}

	return agpl.UserQuietHoursScheduleOptions{
		Schedule: sched,
		UserSet:  userSet,
	}, nil
}

func (s *enterpriseUserQuietHoursScheduleStore) Get(ctx context.Context, db database.Store, userID uuid.UUID) (agpl.UserQuietHoursScheduleOptions, error) {
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return agpl.UserQuietHoursScheduleOptions{}, xerrors.Errorf("get user by ID: %w", err)
	}

	return s.parseSchedule(user.QuietHoursSchedule)
}

func (s *enterpriseUserQuietHoursScheduleStore) Set(ctx context.Context, db database.Store, userID uuid.UUID, rawSchedule string) (agpl.UserQuietHoursScheduleOptions, error) {
	opts, err := s.parseSchedule(rawSchedule)
	if err != nil {
		return opts, err
	}

	// Use the tidy version when storing in the database.
	rawSchedule = ""
	if opts.UserSet {
		rawSchedule = opts.Schedule.String()
	}
	_, err = db.UpdateUserQuietHoursSchedule(ctx, database.UpdateUserQuietHoursScheduleParams{
		ID:                 userID,
		QuietHoursSchedule: rawSchedule,
	})
	if err != nil {
		return agpl.UserQuietHoursScheduleOptions{}, xerrors.Errorf("update user quiet hours schedule: %w", err)
	}

	// TODO(@dean): update max_deadline for all active builds for this user to clamp to
	// the new schedule.

	return opts, nil
}
