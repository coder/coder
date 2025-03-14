package schedule
import (
	"fmt"
	"errors"
	"context"
	"database/sql"
	"sync/atomic"
	"time"
	"cdr.dev/slog"
	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	agpl "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)
// EnterpriseTemplateScheduleStore provides an agpl.TemplateScheduleStore that
// has all fields implemented for enterprise customers.
type EnterpriseTemplateScheduleStore struct {
	// UserQuietHoursScheduleStore is used when recalculating build deadlines on
	// update.
	UserQuietHoursScheduleStore *atomic.Pointer[agpl.UserQuietHoursScheduleStore]
	// Clock for testing
	Clock quartz.Clock
	enqueuer notifications.Enqueuer
	logger   slog.Logger
}
var _ agpl.TemplateScheduleStore = &EnterpriseTemplateScheduleStore{}
func NewEnterpriseTemplateScheduleStore(userQuietHoursStore *atomic.Pointer[agpl.UserQuietHoursScheduleStore], enqueuer notifications.Enqueuer, logger slog.Logger, clock quartz.Clock) *EnterpriseTemplateScheduleStore {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &EnterpriseTemplateScheduleStore{
		UserQuietHoursScheduleStore: userQuietHoursStore,
		Clock:                       clock,
		enqueuer:                    enqueuer,
		logger:                      logger,
	}
}
func (s *EnterpriseTemplateScheduleStore) now() time.Time {
	return dbtime.Time(s.Clock.Now())
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
		return agpl.TemplateScheduleOptions{}, errors.New("invalid autostop requirement days, negative")
	}
	if tpl.AutostopRequirementDaysOfWeek > 0b11111111 {
		return agpl.TemplateScheduleOptions{}, errors.New("invalid autostop requirement days, too large")
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
		return database.Template{}, fmt.Errorf("verify autostop requirement: %w", err)
	}
	err = agpl.VerifyTemplateAutostartRequirement(opts.AutostartRequirement.DaysOfWeek)
	if err != nil {
		return database.Template{}, fmt.Errorf("verify autostart requirement: %w", err)
	}
	var (
		template          database.Template
		markedForDeletion []database.WorkspaceTable
	)
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
			return fmt.Errorf("update template schedule: %w", err)
		}
		var dormantAt time.Time
		if opts.UpdateWorkspaceDormantAt {
			dormantAt = s.now()
		}
		// If we updated the time_til_dormant_autodelete we need to update all the workspaces deleting_at
		// to ensure workspaces are being cleaned up correctly. Similarly if we are
		// disabling it (by passing 0), then we want to delete nullify the deleting_at
		// fields of all the template workspaces.
		markedForDeletion, err = tx.UpdateWorkspacesDormantDeletingAtByTemplateID(ctx, database.UpdateWorkspacesDormantDeletingAtByTemplateIDParams{
			TemplateID:                 tpl.ID,
			TimeTilDormantAutodeleteMs: opts.TimeTilDormantAutoDelete.Milliseconds(),
			DormantAt:                  dormantAt,
		})
		if err != nil {
			return fmt.Errorf("update deleting_at of all workspaces for new time_til_dormant_autodelete %q: %w", opts.TimeTilDormantAutoDelete, err)
		}
		if opts.UpdateWorkspaceLastUsedAt != nil {
			err = opts.UpdateWorkspaceLastUsedAt(ctx, tx, tpl.ID, s.now())
			if err != nil {
				return fmt.Errorf("update workspace last used at: %w", err)
			}
		}
		template, err = tx.GetTemplateByID(ctx, tpl.ID)
		if err != nil {
			return fmt.Errorf("get updated template schedule: %w", err)
		}
		// Update all workspace's TTL using this template if either of the following:
		//   - The template's AllowUserAutostop has just been disabled
		//   - The template's TTL has been modified and AllowUserAutostop is disabled
		if !opts.UserAutostopEnabled && (tpl.AllowUserAutostop || int64(opts.DefaultTTL) != tpl.DefaultTTL) {
			var ttl sql.NullInt64
			if opts.DefaultTTL != 0 {
				ttl = sql.NullInt64{Valid: true, Int64: int64(opts.DefaultTTL)}
			}
			if err = tx.UpdateWorkspacesTTLByTemplateID(ctx, database.UpdateWorkspacesTTLByTemplateIDParams{
				TemplateID: template.ID,
				Ttl:        ttl,
			}); err != nil {
				return fmt.Errorf("update workspaces ttl by template id %q: %w", template.ID, err)
			}
		}
		// Recalculate max_deadline and deadline for all running workspace
		// builds on this template.
		err = s.updateWorkspaceBuilds(ctx, tx, template)
		if err != nil {
			return fmt.Errorf("update workspace builds: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return database.Template{}, err
	}
	if opts.AutostartRequirement.DaysOfWeek != tpl.AutostartAllowedDays() {
		templateSchedule, err := s.Get(ctx, db, tpl.ID)
		if err != nil {
			return database.Template{}, fmt.Errorf("get template schedule: %w", err)
		}
		//nolint:gocritic // We need to be able to read information about all workspaces.
		workspaces, err := db.GetWorkspacesByTemplateID(dbauthz.AsSystemRestricted(ctx), tpl.ID)
		if err != nil {
			return database.Template{}, fmt.Errorf("get workspaces by template id: %w", err)
		}
		workspaceIDs := []uuid.UUID{}
		nextStartAts := []time.Time{}
		for _, workspace := range workspaces {
			nextStartAt := time.Time{}
			if workspace.AutostartSchedule.Valid {
				next, err := agpl.NextAllowedAutostart(s.now(), workspace.AutostartSchedule.String, templateSchedule)
				if err == nil {
					nextStartAt = dbtime.Time(next.UTC())
				}
			}
			workspaceIDs = append(workspaceIDs, workspace.ID)
			nextStartAts = append(nextStartAts, nextStartAt)
		}
		//nolint:gocritic // We need to be able to update information about all workspaces.
		if err := db.BatchUpdateWorkspaceNextStartAt(dbauthz.AsSystemRestricted(ctx), database.BatchUpdateWorkspaceNextStartAtParams{
			IDs:          workspaceIDs,
			NextStartAts: nextStartAts,
		}); err != nil {
			return database.Template{}, fmt.Errorf("update workspace next start at: %w", err)
		}
	}
	for _, ws := range markedForDeletion {
		dormantTime := s.now().Add(opts.TimeTilDormantAutoDelete)
		_, err = s.enqueuer.Enqueue(
			// nolint:gocritic // Need actor to enqueue notification
			dbauthz.AsNotifier(ctx),
			ws.OwnerID,
			notifications.TemplateWorkspaceMarkedForDeletion,
			map[string]string{
				"name":           ws.Name,
				"reason":         "an update to the template's dormancy",
				"timeTilDormant": humanize.Time(dormantTime),
			},
			"scheduletemplate",
			// Associate this notification with all the related entities.
			ws.ID,
			ws.OwnerID,
			ws.TemplateID,
			ws.OrganizationID,
		)
		if err != nil {
			s.logger.Warn(ctx, "failed to notify of workspace marked for deletion", slog.Error(err), slog.F("workspace_id", ws.ID))
		}
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get active workspace builds: %w", err)
	}
	for _, build := range builds {
		err := s.updateWorkspaceBuild(ctx, db, build)
		if err != nil {
			return fmt.Errorf("update workspace build %q: %w", build.ID, err)
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
		return fmt.Errorf("get workspace %q: %w", build.WorkspaceID, err)
	}
	job, err := db.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return fmt.Errorf("get provisioner job %q: %w", build.JobID, err)
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
		Workspace:          workspace.WorkspaceTable(),
		WorkspaceAutostart: workspace.AutostartSchedule.String,
	})
	if err != nil {
		return fmt.Errorf("calculate new autostop for workspace %q: %w", workspace.ID, err)
	}
	if workspace.AutostartSchedule.Valid {
		templateScheduleOptions, err := s.Get(ctx, db, workspace.TemplateID)
		if err != nil {
			return fmt.Errorf("get template schedule options: %w", err)
		}
		nextStartAt, _ := agpl.NextAutostart(s.now(), workspace.AutostartSchedule.String, templateScheduleOptions)
		err = db.UpdateWorkspaceNextStartAt(ctx, database.UpdateWorkspaceNextStartAtParams{
			ID:          workspace.ID,
			NextStartAt: sql.NullTime{Valid: true, Time: nextStartAt},
		})
		if err != nil {
			return fmt.Errorf("update workspace next start at: %w", err)
		}
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
		return fmt.Errorf("update workspace build %q: %w", build.ID, err)
	}
	return nil
}
