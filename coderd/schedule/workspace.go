package schedule

import (
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule/cron"
)

// GetWorkspaceAutostartSchedule will return the workspace's autostart schedule
// if it is eligible for autostart. If the workspace is not eligible for
// autostart it will return nil.
func GetWorkspaceAutostartSchedule(templateSchedule TemplateScheduleOptions, workspace database.Workspace) (*cron.Schedule, error) {
	if !templateSchedule.UserAutostartEnabled || !workspace.AutostartSchedule.Valid || workspace.AutostartSchedule.String == "" {
		return nil, nil
	}

	sched, err := cron.Weekly(workspace.AutostartSchedule.String)
	if err != nil {
		return nil, xerrors.Errorf("parse workspace autostart schedule %q: %w", workspace.AutostartSchedule.String, err)
	}

	return sched, nil
}

// WorkspaceTTL returns the workspace's TTL or the template's default TTL if the
// template forbids the user from setting their own TTL.
func WorkspaceTTL(templateSchedule TemplateScheduleOptions, ws database.Workspace) time.Duration {
	var ttl time.Duration
	if ws.Ttl.Valid && ws.Ttl.Int64 > 0 {
		ttl = time.Duration(ws.Ttl.Int64)
	}
	if !templateSchedule.UserAutostopEnabled {
		ttl = 0
		if templateSchedule.DefaultTTL > 0 {
			ttl = templateSchedule.DefaultTTL
		}
	}
	return ttl
}

// MaybeBumpDeadline may bump the deadline of the workspace if the deadline
// exceeds the next autostart time. The returned time if non-zero if it should
// be used as the new deadline.
//
// The MaxDeadline is not taken into account.
func MaybeBumpDeadline(autostartSchedule *cron.Schedule, deadline time.Time, ttl time.Duration) time.Time {
	// Calculate the next autostart after (build.deadline - ttl). If this value
	// is before the current deadline + 1h, we should bump the deadline.
	autostartTime := autostartSchedule.Next(deadline.Add(-ttl))
	if !autostartTime.Before(deadline.Add(time.Hour)) {
		return time.Time{}
	}

	// Calculate the expected new deadline.
	newDeadline := autostartTime.Add(ttl)

	// Calculate the next autostart time after the one we just calculated. If
	// it's before the new deadline, we should not bump the deadline.
	//
	// If we didn't have this check, we could end up in a situation where we
	// would be bumping the deadline every day and keeping it alive permanently.
	// E.g. ttl is 1 week but the autostart is daily. We would bump the deadline
	// by 1 week every day, keeping the workspace alive forever.
	//
	// This essentially means we do nothing* if the TTL is longer than a day.
	// *Depends on what days the autostart schedule is set to and the TTL
	// duration
	nextAutostartTime := autostartSchedule.Next(autostartTime)
	if nextAutostartTime.Before(newDeadline) {
		return time.Time{}
	}

	return newDeadline
}
