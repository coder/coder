package schedule

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	agpl "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
)

// EnterpriseTemplateScheduleStore provides an agpl.TemplateScheduleStore that
// has all fields implemented for enterprise customers.
type EnterpriseTemplateScheduleStore struct {
	// UserQuietHoursScheduleStore is used when recalculating build deadlines on
	// update.
	UserQuietHoursScheduleStore *atomic.Pointer[agpl.UserQuietHoursScheduleStore]

	// Custom time.Now() function to use in tests. Defaults to dbtime.Now().
	TimeNowFn func() time.Time
}

var _ agpl.TemplateScheduleStore = &EnterpriseTemplateScheduleStore{}

func NewEnterpriseTemplateScheduleStore(userQuietHoursStore *atomic.Pointer[agpl.UserQuietHoursScheduleStore]) *EnterpriseTemplateScheduleStore {
	return &EnterpriseTemplateScheduleStore{
		UserQuietHoursScheduleStore: userQuietHoursStore,
	}
}

func (s *EnterpriseTemplateScheduleStore) now() time.Time {
	if s.TimeNowFn != nil {
		return s.TimeNowFn()
	}
	return dbtime.Now()
}

// Get implements agpl.TemplateScheduleStore.
func (*EnterpriseTemplateScheduleStore) Get(ctx context.Context, db database.Store, templateID uuid.UUID) (agpl.TemplateScheduleOptions, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	tpl, err := db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return agpl.TemplateScheduleOptions{}, err
	}

	// These extra checks have to be done before the conversion because we lose
	// precision and signs when converting to the agpl types from the database.
	if tpl.AutostopRequirementDaysOfWeek < 0 {
		return agpl.TemplateScheduleOptions{}, xerrors.New("invalid autostop requirement days, negative")
	}
	if tpl.AutostopRequirementDaysOfWeek > 0b11111111 {
		return agpl.TemplateScheduleOptions{}, xerrors.New("invalid autostop requirement days, too large")
	}
	if tpl.AutostopRequirementWeeks == 0 {
		tpl.AutostopRequirementWeeks = 1
	}
	err = agpl.VerifyTemplateAutostopRequirement(uint8(tpl.AutostopRequirementDaysOfWeek), tpl.AutostopRequirementWeeks)
	if err != nil {
		return agpl.TemplateScheduleOptions{}, err
	}

	return agpl.TemplateScheduleOptions{
		UserAutostartEnabled: tpl.AllowUserAutostart,
		UserAutostopEnabled:  tpl.AllowUserAutostop,
		DefaultTTL:           time.Duration(tpl.DefaultTTL),
		ActivityBump:         time.Duration(tpl.ActivityBump),
		AutostopRequirement: agpl.TemplateAutostopRequirement{
			DaysOfWeek: uint8(tpl.AutostopRequirementDaysOfWeek),
			Weeks:      tpl.AutostopRequirementWeeks,
		},
		AutostartRequirement: agpl.TemplateAutostartRequirement{
			DaysOfWeek: tpl.AutostartAllowedDays(),
		},
		FailureTTL:               time.Duration(tpl.FailureTTL),
		TimeTilDormant:           time.Duration(tpl.TimeTilDormant),
		TimeTilDormantAutoDelete: time.Duration(tpl.TimeTilDormantAutoDelete),
	}, nil
}

// Set implements agpl.TemplateScheduleStore.
func (s *EnterpriseTemplateScheduleStore) Set(ctx context.Context, db database.Store, tpl database.Template, opts agpl.TemplateScheduleOptions) (database.Template, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	if opts.AutostopRequirement.Weeks <= 0 {
		opts.AutostopRequirement.Weeks = 1
	}
	if tpl.AutostopRequirementWeeks <= 0 {
		tpl.AutostopRequirementWeeks = 1
	}

	if int64(opts.DefaultTTL) == tpl.DefaultTTL &&
		int64(opts.ActivityBump) == tpl.ActivityBump &&
		int16(opts.AutostopRequirement.DaysOfWeek) == tpl.AutostopRequirementDaysOfWeek &&
		opts.AutostartRequirement.DaysOfWeek == tpl.AutostartAllowedDays() &&
		opts.AutostopRequirement.Weeks == tpl.AutostopRequirementWeeks &&
		int64(opts.FailureTTL) == tpl.FailureTTL &&
		int64(opts.TimeTilDormant) == tpl.TimeTilDormant &&
		int64(opts.TimeTilDormantAutoDelete) == tpl.TimeTilDormantAutoDelete &&
		opts.UserAutostartEnabled == tpl.AllowUserAutostart &&
		opts.UserAutostopEnabled == tpl.AllowUserAutostop {
		// Avoid updating the UpdatedAt timestamp if nothing will be changed.
		return tpl, nil
	}

	err := agpl.VerifyTemplateAutostopRequirement(opts.AutostopRequirement.DaysOfWeek, opts.AutostopRequirement.Weeks)
	if err != nil {
		return database.Template{}, xerrors.Errorf("verify autostop requirement: %w", err)
	}

	err = agpl.VerifyTemplateAutostartRequirement(opts.AutostartRequirement.DaysOfWeek)
	if err != nil {
		return database.Template{}, xerrors.Errorf("verify autostart requirement: %w", err)
	}

	var template database.Template
	err = db.InTx(func(tx database.Store) error {
		ctx, span := tracing.StartSpanWithName(ctx, "(*schedule.EnterpriseTemplateScheduleStore).Set()-InTx()")
		defer span.End()

		err := tx.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
			ID:                            tpl.ID,
			UpdatedAt:                     s.now(),
			AllowUserAutostart:            opts.UserAutostartEnabled,
			AllowUserAutostop:             opts.UserAutostopEnabled,
			DefaultTTL:                    int64(opts.DefaultTTL),
			ActivityBump:                  int64(opts.ActivityBump),
			AutostopRequirementDaysOfWeek: int16(opts.AutostopRequirement.DaysOfWeek),
			AutostopRequirementWeeks:      opts.AutostopRequirement.Weeks,
			// Database stores the inverse of the allowed days of the week.
			// Make sure the 8th bit is always zeroed out, as there is no 8th day of the week.
			AutostartBlockDaysOfWeek: int16(^opts.AutostartRequirement.DaysOfWeek & 0b01111111),
			FailureTTL:               int64(opts.FailureTTL),
			TimeTilDormant:           int64(opts.TimeTilDormant),
			TimeTilDormantAutoDelete: int64(opts.TimeTilDormantAutoDelete),
		})
		if err != nil {
			return xerrors.Errorf("update template schedule: %w", err)
		}

		var dormantAt time.Time
		if opts.UpdateWorkspaceDormantAt {
			dormantAt = dbtime.Now()
		}

		// If we updated the time_til_dormant_autodelete we need to update all the workspaces deleting_at
		// to ensure workspaces are being cleaned up correctly. Similarly if we are
		// disabling it (by passing 0), then we want to delete nullify the deleting_at
		// fields of all the template workspaces.
		err = tx.UpdateWorkspacesDormantDeletingAtByTemplateID(ctx, database.UpdateWorkspacesDormantDeletingAtByTemplateIDParams{
			TemplateID:                 tpl.ID,
			TimeTilDormantAutodeleteMs: opts.TimeTilDormantAutoDelete.Milliseconds(),
			DormantAt:                  dormantAt,
		})
		if err != nil {
			return xerrors.Errorf("update deleting_at of all workspaces for new time_til_dormant_autodelete %q: %w", opts.TimeTilDormantAutoDelete, err)
		}

		if opts.UpdateWorkspaceLastUsedAt {
			err = tx.UpdateTemplateWorkspacesLastUsedAt(ctx, database.UpdateTemplateWorkspacesLastUsedAtParams{
				TemplateID: tpl.ID,
				LastUsedAt: dbtime.Now(),
			})
			if err != nil {
				return xerrors.Errorf("update template workspaces last_used_at: %w", err)
			}
		}

		template, err = tx.GetTemplateByID(ctx, tpl.ID)
		if err != nil {
			return xerrors.Errorf("get updated template schedule: %w", err)
		}

		// Recalculate max_deadline and deadline for all running workspace
		// builds on this template.
		err = s.updateWorkspaceBuilds(ctx, tx, template)
		if err != nil {
			return xerrors.Errorf("update workspace builds: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		return database.Template{}, err
	}

	return template, nil
}

func (s *EnterpriseTemplateScheduleStore) updateWorkspaceBuilds(ctx context.Context, db database.Store, template database.Template) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	//nolint:gocritic // This function will retrieve all workspace builds on
	// the template and update their max deadline to be within the new
	// policy parameters.
	ctx = dbauthz.AsSystemRestricted(ctx)

	builds, err := db.GetActiveWorkspaceBuildsByTemplateID(ctx, template.ID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get active workspace builds: %w", err)
	}

	for _, build := range builds {
		err := s.updateWorkspaceBuild(ctx, db, build)
		if err != nil {
			return xerrors.Errorf("update workspace build %q: %w", build.ID, err)
		}
	}

	return nil
}

func (s *EnterpriseTemplateScheduleStore) updateWorkspaceBuild(ctx context.Context, db database.Store, build database.WorkspaceBuild) error {
	ctx, span := tracing.StartSpan(ctx,
		trace.WithAttributes(attribute.String("coder.workspace_id", build.WorkspaceID.String())),
		trace.WithAttributes(attribute.String("coder.workspace_build_id", build.ID.String())),
	)
	defer span.End()

	if !build.MaxDeadline.IsZero() && build.MaxDeadline.Before(s.now().Add(2*time.Hour)) {
		// Skip this since it's already too close to the max_deadline.
		return nil
	}

	workspace, err := db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return xerrors.Errorf("get workspace %q: %w", build.WorkspaceID, err)
	}

	job, err := db.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return xerrors.Errorf("get provisioner job %q: %w", build.JobID, err)
	}
	if codersdk.ProvisionerJobStatus(job.JobStatus) != codersdk.ProvisionerJobSucceeded {
		// Only touch builds that are completed.
		return nil
	}

	// If the job completed before the autostop epoch, then it must be skipped
	// to avoid failures below. Add a week to account for timezones.
	if job.CompletedAt.Time.Before(agpl.TemplateAutostopRequirementEpoch(time.UTC).Add(time.Hour * 7 * 24)) {
		return nil
	}

	autostop, err := agpl.CalculateAutostop(ctx, agpl.CalculateAutostopParams{
		Database:                    db,
		TemplateScheduleStore:       s,
		UserQuietHoursScheduleStore: *s.UserQuietHoursScheduleStore.Load(),
		// Use the job completion time as the time we calculate autostop from.
		Now:                job.CompletedAt.Time,
		Workspace:          workspace,
		WorkspaceAutostart: workspace.AutostartSchedule.String,
	})
	if err != nil {
		return xerrors.Errorf("calculate new autostop for workspace %q: %w", workspace.ID, err)
	}

	// If max deadline is before now()+2h, then set it to that.
	// This is intended to give ample warning to this workspace about an upcoming auto-stop.
	// If we were to omit this "grace" period, then this workspace could be set to be stopped "now".
	// The "2 hours" was an arbitrary decision for this window.
	now := s.now()
	if !autostop.MaxDeadline.IsZero() && autostop.MaxDeadline.Before(now.Add(2*time.Hour)) {
		autostop.MaxDeadline = now.Add(time.Hour * 2)
	}

	// If the current deadline on the build is after the new max_deadline, then
	// set it to the max_deadline.
	autostop.Deadline = build.Deadline
	if !autostop.MaxDeadline.IsZero() && autostop.Deadline.After(autostop.MaxDeadline) {
		autostop.Deadline = autostop.MaxDeadline
	}

	// If there's a max_deadline but the deadline is 0, then set the deadline to
	// the max_deadline.
	if !autostop.MaxDeadline.IsZero() && autostop.Deadline.IsZero() {
		autostop.Deadline = autostop.MaxDeadline
	}

	// Update the workspace build deadline.
	err = db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
		ID:          build.ID,
		UpdatedAt:   now,
		Deadline:    autostop.Deadline,
		MaxDeadline: autostop.MaxDeadline,
	})
	if err != nil {
		return xerrors.Errorf("update workspace build %q: %w", build.ID, err)
	}

	return nil
}
