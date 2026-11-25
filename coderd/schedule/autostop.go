package schedule

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/tracing"
)

const (
	// AutostopRequirementLeeway is the minimum time between build
	// completion or a template schedule save and a newly scheduled required stop.
	AutostopRequirementLeeway = 2 * time.Hour
)

type CalculateAutostopParams struct {
	Database                    database.Store
	TemplateScheduleStore       TemplateScheduleStore
	UserQuietHoursScheduleStore UserQuietHoursScheduleStore
	// WorkspaceAutostart can be the empty string if no workspace autostart
	// is configured.
	// If configured, this is expected to be a cron weekly event parsable
	// by autobuild.NextAutostart
	WorkspaceAutostart string

	// WorkspaceBuildCompletedAt is the time when the workspace build was
	// completed.
	//
	// We always want to calculate startup leeway using the build completion
	// time to avoid forcing a workspace build's max_deadline to be pushed to
	// the next potential cron instance.
	//
	// E.g. if this function is called for an existing workspace build, which
	//      currently has a max_deadline within the next 2 hours (see leeway
	//      above), and the current time is passed in as the completion time,
	//      the max_deadline will be updated to be much later than expected.
	WorkspaceBuildCompletedAt time.Time
	Workspace                 database.WorkspaceTable

	// AutostopRequirementNotBefore is the earliest required stop when
	// recalculating an existing build. Build startup leeway still applies.
	//
	// E.g. if a build completes on Friday and an admin adds a weekly Sunday
	//      requirement the following Wednesday, pass the template save time
	//      to select the upcoming Sunday's quiet hours. A zero value starts
	//      from build completion plus startup leeway and selects the previous
	//      Sunday's quiet hours. That past deadline can cause an unexpected
	//      stop on Wednesday, outside quiet hours.
	AutostopRequirementNotBefore time.Time
}

type AutostopTime struct {
	// Deadline is the time when the workspace will be stopped. The value can be
	// bumped by user activity or manually by the user via the UI.
	Deadline time.Time
	// MaxDeadline is the maximum value for deadline.
	MaxDeadline time.Time
}

// CalculateAutostop calculates the deadline and max_deadline for a workspace
// build.
//
// Deadline is the time when the workspace will be stopped, as long as it
// doesn't see any new activity (such as SSH, app requests, etc.). When activity
// is detected the deadline is bumped by the workspace's TTL (this only happens
// when activity is detected and more than 5% of the TTL has passed to save
// database queries, see the ActivityBumpWorkspace query).
//
// MaxDeadline is the maximum value for deadline. The deadline cannot be bumped
// past this value, so it denotes the absolute deadline that the workspace build
// must be stopped by. MaxDeadline is calculated using the template's "autostop
// requirement" settings and the user's "quiet hours" settings to pick a time
// outside of working hours.
//
// Note that the deadline is checked at the database level:
//
//	(deadline IS NOT zero AND deadline <= max_deadline) UNLESS max_deadline is zero.
//
// Deadline is intended as a cost saving measure, not as a hard policy. It is
// derived from either the workspace's TTL or the template's TTL, depending on
// the template's policy, to ensure workspaces are stopped when they are idle.
//
// MaxDeadline is intended as a compliance policy. It is derived from the
// template's autostop requirement to cap workspace uptime and effectively force
// people to update often.
//
// Note that only the build's CURRENT deadline property influences automation in
// the autobuild package. As stated above, the MaxDeadline property is only used
// to cap the value of a build's deadline.
func CalculateAutostop(ctx context.Context, params CalculateAutostopParams) (AutostopTime, error) {
	ctx, span := tracing.StartSpan(ctx,
		trace.WithAttributes(attribute.String("coder.workspace_id", params.Workspace.ID.String())),
		trace.WithAttributes(attribute.String("coder.template_id", params.Workspace.TemplateID.String())),
	)
	defer span.End()

	var (
		db               = params.Database
		workspace        = params.Workspace
		buildCompletedAt = params.WorkspaceBuildCompletedAt

		autostop AutostopTime
	)

	templateSchedule, err := params.TemplateScheduleStore.Get(ctx, db, workspace.TemplateID)
	if err != nil {
		return autostop, xerrors.Errorf("get template schedule options: %w", err)
	}

	ttl := workspaceTTL(workspace, templateSchedule)
	if ttl > 0 {
		// Only apply non-zero TTLs.
		autostop.Deadline = buildCompletedAt.Add(ttl)
		if params.WorkspaceAutostart != "" {
			// If the deadline passes the next autostart, we need to extend the deadline to
			// autostart + deadline. ActivityBumpWorkspace already covers this case
			// when extending the deadline.
			//
			// Situation this is solving.
			// 1. User has workspace with auto-start at 9:00am, 12 hour auto-stop.
			// 2. Coder stops workspace at 9pm
			// 3. User starts workspace at 9:45pm.
			//	- The initial deadline is calculated to be 9:45am
			//	- This crosses the autostart deadline, so the deadline is extended to 9pm
			nextAutostart, ok := NextAutostart(params.WorkspaceBuildCompletedAt, params.WorkspaceAutostart, templateSchedule)
			if ok && autostop.Deadline.After(nextAutostart) {
				autostop.Deadline = nextAutostart.Add(ttl)
			}
		}
	}

	autostop.MaxDeadline, err = calculateMaxDeadline(ctx, params, templateSchedule.AutostopRequirement)
	if err != nil {
		return autostop, err
	}

	// If the workspace doesn't have a deadline or the max deadline is sooner
	// than the workspace deadline, use the max deadline as the actual deadline.
	if !autostop.MaxDeadline.IsZero() && (autostop.Deadline.IsZero() || autostop.MaxDeadline.Before(autostop.Deadline)) {
		autostop.Deadline = autostop.MaxDeadline
	}

	if (!autostop.Deadline.IsZero() && autostop.Deadline.Before(buildCompletedAt)) || (!autostop.MaxDeadline.IsZero() && autostop.MaxDeadline.Before(buildCompletedAt)) {
		// Something went wrong with the deadline calculation, so we should
		// bail.
		return autostop, xerrors.Errorf("deadline calculation error, computed deadline or max deadline is in the past for workspace build: deadline=%q maxDeadline=%q now=%q", autostop.Deadline, autostop.MaxDeadline, buildCompletedAt)
	}

	return autostop, nil
}

func calculateMaxDeadline(ctx context.Context, params CalculateAutostopParams, requirement TemplateAutostopRequirement) (time.Time, error) {
	if requirement.DaysOfWeek == 0 {
		return time.Time{}, nil
	}

	// Get the user's quiet hours schedule (this will return the default if
	// the user has not set their own schedule).
	userQuietHoursSchedule, err := params.UserQuietHoursScheduleStore.Get(ctx, params.Database, params.Workspace.OwnerID)
	if err != nil {
		return time.Time{}, xerrors.Errorf("get user quiet hours schedule options: %w", err)
	}

	// If the schedule is nil, that means the deployment isn't entitled to
	// use quiet hours. In this case, do not set a max deadline on the
	// workspace.
	if userQuietHoursSchedule.Schedule == nil {
		return time.Time{}, nil
	}

	// A bitmap with only the unused bit set would never match a date.
	if requirement.DaysOfWeek&0b01111111 == 0 {
		return time.Time{}, xerrors.New("autostop requirement has no valid weekdays")
	}

	loc := userQuietHoursSchedule.Schedule.Location()
	buildCompletedAtInLoc := params.WorkspaceBuildCompletedAt.In(loc)
	// Add the leeway here so we avoid checking today's quiet hours if
	// the workspace was started shortly before midnight.
	notBefore := buildCompletedAtInLoc.Add(AutostopRequirementLeeway)
	if params.AutostopRequirementNotBefore.After(notBefore) {
		notBefore = params.AutostopRequirementNotBefore.In(loc)
	}
	requirementDays := requirement.DaysMap()
	searchStart := notBefore

	// If the clock rolls back across midnight, quiet hours on the
	// previous date can occur again after the cutoff.
	prevDate := notBefore.AddDate(0, 0, -1)
	canCheckPrevDate := requirement.Weeks <= 1 ||
		// WeeksSinceEpoch returns an error for pre-epoch dates.
		!prevDate.Before(TemplateAutostopRequirementEpoch(loc))
	if canCheckPrevDate && requirementDays[prevDate.Weekday()] {
		quietStart, _ := userQuietHoursSchedule.Schedule.FirstOnDate(prevDate, notBefore)
		if !quietStart.IsZero() {
			searchStart = prevDate
		}
	}

	weeks := max(1, requirement.Weeks)
	// Include the second full cadence so one skipped required date still
	// leaves another chance to schedule the stop.
	lastDayOffset := 2 * 7 * int(min(weeks, MaxTemplateAutostopRequirementWeeks))
	year, month, day := searchStart.Date()
	useNextQuietHours := false
	for offset := 0; offset <= lastDayOffset; offset++ {
		// Count dates in UTC so skipped local dates cannot stall the search.
		y, m, d := time.Date(year, month, day+offset, 0, 0, 0, 0, time.UTC).Date()
		stopDay := time.Date(y, m, d, 12, 0, 0, 0, loc)
		if stopDay.Year() != y || stopDay.Month() != m || stopDay.Day() != d {
			continue
		}
		if !useNextQuietHours && weeks > 1 {
			week, err := WeeksSinceEpoch(stopDay)
			if err != nil {
				return time.Time{}, xerrors.Errorf("determine stop week: %w", err)
			}
			if week%weeks != 0 {
				continue
			}
		}
		if !useNextQuietHours && !requirementDays[stopDay.Weekday()] {
			continue
		}

		quietStart, skippedQuietHours := userQuietHoursSchedule.Schedule.FirstOnDate(stopDay, notBefore)
		if !quietStart.IsZero() {
			return quietStart, nil
		}

		// Use the next quiet-hours start on any date when a clock jump
		// at or after the cutoff skips the selected date's quiet hours.
		if skippedQuietHours {
			useNextQuietHours = true
		}
	}
	return time.Time{}, xerrors.Errorf("could not find a required stop within %d days of %q", lastDayOffset, searchStart)
}

// workspaceTTL returns the TTL to use for a workspace.
//
// If the template forbids custom workspace TTLs, then we always use the
// template's configured TTL (or 0 if the template has no TTL configured).
func workspaceTTL(workspace database.WorkspaceTable, templateSchedule TemplateScheduleOptions) time.Duration {
	// If the template forbids custom workspace TTLs, then we always use the
	// template's configured TTL (or 0 if the template has no TTL configured).
	if !templateSchedule.UserAutostopEnabled {
		// This is intentionally a nested if statement because of the else if.
		if templateSchedule.DefaultTTL > 0 {
			return templateSchedule.DefaultTTL
		}
		return 0
	}
	if workspace.Ttl.Valid {
		return time.Duration(workspace.Ttl.Int64)
	}
	return 0
}

// WeeksSinceEpoch gets the weeks since the epoch for a given time. This is a
// 0-indexed number of weeks since the epoch (Monday).
//
// The timezone embedded in the time object is used to determine the epoch.
func WeeksSinceEpoch(now time.Time) (int64, error) {
	epoch := TemplateAutostopRequirementEpoch(now.Location())
	if now.Before(epoch) {
		return 0, xerrors.New("coder server system clock is incorrect, cannot calculate template autostop requirement")
	}

	// This calculation needs to be done using YearDay, as dividing by the
	// amount of hours is impacted by daylight savings. Even though daylight
	// savings is usually only an hour difference, this calculation is used to
	// get the current week number and could result in an entire week getting
	// skipped if the calculation is off by an hour.
	//
	// Old naive algorithm: weeksSinceEpoch := int64(since.Hours() / (24 * 7))

	// Get days since epoch. Start with a negative number of days, as we want to
	// subtract the YearDay() of the epoch itself.
	days := -epoch.YearDay()
	for i := epoch.Year(); i < now.Year(); i++ {
		startOfNextYear := time.Date(i+1, 1, 1, 0, 0, 0, 0, now.Location())
		if startOfNextYear.Year() != i+1 {
			return 0, xerrors.New("overflow calculating weeks since epoch")
		}
		endOfThisYear := startOfNextYear.AddDate(0, 0, -1)
		if endOfThisYear.Year() != i {
			return 0, xerrors.New("overflow calculating weeks since epoch")
		}

		days += endOfThisYear.YearDay()
	}
	// Add this year's days.
	days += now.YearDay()

	// Ensure that the number of days is positive.
	if days < 0 {
		return 0, xerrors.New("overflow calculating weeks since epoch")
	}

	// Divide by 7 to get the number of weeks.
	weeksSinceEpoch := int64(days / 7)
	return weeksSinceEpoch, nil
}
