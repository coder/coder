package schedule

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/db2sdk"
	agpl "github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/codersdk"
)

// EnterpriseTemplateScheduleStore provides an agpl.TemplateScheduleStore that
// has all fields implemented for enterprise customers.
type EnterpriseTemplateScheduleStore struct {
	// UseRestartRequirement decides whether the RestartRequirement field should
	// be used instead of the MaxTTL field for determining the max deadline of a
	// workspace build. This value is determined by a feature flag, licensing,
	// and whether a default user quiet hours schedule is set.
	UseRestartRequirement atomic.Bool

	// UserQuietHoursScheduleStore is used when recalculating build deadlines on
	// update.
	UserQuietHoursScheduleStore *atomic.Pointer[agpl.UserQuietHoursScheduleStore]
}

var _ agpl.TemplateScheduleStore = &EnterpriseTemplateScheduleStore{}

func NewEnterpriseTemplateScheduleStore(userQuietHoursStore *atomic.Pointer[agpl.UserQuietHoursScheduleStore]) *EnterpriseTemplateScheduleStore {
	return &EnterpriseTemplateScheduleStore{
		UserQuietHoursScheduleStore: userQuietHoursStore,
	}
}

// Get implements agpl.TemplateScheduleStore.
func (s *EnterpriseTemplateScheduleStore) Get(ctx context.Context, db database.Store, templateID uuid.UUID) (agpl.TemplateScheduleOptions, error) {
	tpl, err := db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return agpl.TemplateScheduleOptions{}, err
	}

	// These extra checks have to be done before the conversion because we lose
	// precision and signs when converting to the agpl types from the database.
	if tpl.RestartRequirementDaysOfWeek < 0 {
		return agpl.TemplateScheduleOptions{}, xerrors.New("invalid restart requirement days, negative")
	}
	if tpl.RestartRequirementDaysOfWeek > 0b11111111 {
		return agpl.TemplateScheduleOptions{}, xerrors.New("invalid restart requirement days, too large")
	}
	err = agpl.VerifyTemplateRestartRequirement(uint8(tpl.RestartRequirementDaysOfWeek), tpl.RestartRequirementWeeks)
	if err != nil {
		return agpl.TemplateScheduleOptions{}, err
	}

	return agpl.TemplateScheduleOptions{
		UserAutostartEnabled:  tpl.AllowUserAutostart,
		UserAutostopEnabled:   tpl.AllowUserAutostop,
		DefaultTTL:            time.Duration(tpl.DefaultTTL),
		MaxTTL:                time.Duration(tpl.MaxTTL),
		UseRestartRequirement: s.UseRestartRequirement.Load(),
		RestartRequirement: agpl.TemplateRestartRequirement{
			DaysOfWeek: uint8(tpl.RestartRequirementDaysOfWeek),
			Weeks:      tpl.RestartRequirementWeeks,
		},
		FailureTTL:    time.Duration(tpl.FailureTTL),
		InactivityTTL: time.Duration(tpl.InactivityTTL),
		LockedTTL:     time.Duration(tpl.LockedTTL),
	}, nil
}

// Set implements agpl.TemplateScheduleStore.
func (s *EnterpriseTemplateScheduleStore) Set(ctx context.Context, db database.Store, tpl database.Template, opts agpl.TemplateScheduleOptions) (database.Template, error) {
	if int64(opts.DefaultTTL) == tpl.DefaultTTL &&
		int64(opts.MaxTTL) == tpl.MaxTTL &&
		int16(opts.RestartRequirement.DaysOfWeek) == tpl.RestartRequirementDaysOfWeek &&
		opts.RestartRequirement.Weeks == tpl.RestartRequirementWeeks &&
		int64(opts.FailureTTL) == tpl.FailureTTL &&
		int64(opts.InactivityTTL) == tpl.InactivityTTL &&
		int64(opts.LockedTTL) == tpl.LockedTTL &&
		opts.UserAutostartEnabled == tpl.AllowUserAutostart &&
		opts.UserAutostopEnabled == tpl.AllowUserAutostop {
		// Avoid updating the UpdatedAt timestamp if nothing will be changed.
		return tpl, nil
	}

	err := agpl.VerifyTemplateRestartRequirement(opts.RestartRequirement.DaysOfWeek, opts.RestartRequirement.Weeks)
	if err != nil {
		return database.Template{}, err
	}

	var template database.Template
	err = db.InTx(func(db database.Store) error {
		err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
			ID:                           tpl.ID,
			UpdatedAt:                    database.Now(),
			AllowUserAutostart:           opts.UserAutostartEnabled,
			AllowUserAutostop:            opts.UserAutostopEnabled,
			DefaultTTL:                   int64(opts.DefaultTTL),
			MaxTTL:                       int64(opts.MaxTTL),
			RestartRequirementDaysOfWeek: int16(opts.RestartRequirement.DaysOfWeek),
			RestartRequirementWeeks:      opts.RestartRequirement.Weeks,
			FailureTTL:                   int64(opts.FailureTTL),
			InactivityTTL:                int64(opts.InactivityTTL),
			LockedTTL:                    int64(opts.LockedTTL),
		})
		if err != nil {
			return xerrors.Errorf("update template schedule: %w", err)
		}

		// If we updated the locked_ttl we need to update all the workspaces deleting_at
		// to ensure workspaces are being cleaned up correctly. Similarly if we are
		// disabling it (by passing 0), then we want to delete nullify the deleting_at
		// fields of all the template workspaces.
		err = db.UpdateWorkspacesDeletingAtByTemplateID(ctx, database.UpdateWorkspacesDeletingAtByTemplateIDParams{
			TemplateID:  tpl.ID,
			LockedTtlMs: opts.LockedTTL.Milliseconds(),
		})
		if err != nil {
			return xerrors.Errorf("update deleting_at of all workspaces for new locked_ttl %q: %w", opts.LockedTTL, err)
		}

		// Recalculate max_deadline and deadline for all running workspace
		// builds on this template.
		builds, err := db.GetActiveWorkspaceBuildsByTemplateID(ctx, template.ID)
		if err != nil {
			return xerrors.Errorf("get active workspace builds: %w", err)
		}
		for _, build := range builds {
			if !build.MaxDeadline.IsZero() && build.MaxDeadline.Before(time.Now().Add(time.Hour)) {
				// Skip this since it's already too close to the max_deadline.
				continue
			}

			workspace, err := db.GetWorkspaceByID(ctx, build.WorkspaceID)
			if err != nil {
				return xerrors.Errorf("get workspace %q: %w", build.WorkspaceID, err)
			}

			job, err := db.GetProvisionerJobByID(ctx, build.JobID)
			if err != nil {
				return xerrors.Errorf("get provisioner job %q: %w", build.JobID, err)
			}
			if db2sdk.ProvisionerJobStatus(job) != codersdk.ProvisionerJobSucceeded {
				continue
			}

			// If the job completed before the autostop epoch, then it must be
			// skipped to avoid failures below.
			if job.CompletedAt.Time.Before(agpl.TemplateRestartRequirementEpoch(time.UTC).Add(time.Hour * 7 * 24)) {
				continue
			}

			autostop, err := agpl.CalculateAutostop(ctx, agpl.CalculateAutostopParams{
				Database:                    db,
				TemplateScheduleStore:       s,
				UserQuietHoursScheduleStore: *s.UserQuietHoursScheduleStore.Load(),
				// Use the job completion time as the time we calculate autostop
				// from.
				Now:       job.CompletedAt.Time,
				Workspace: workspace,
			})
			if err != nil {
				return xerrors.Errorf("calculate new autostop for workspace %q: %w", workspace.ID, err)
			}

			// If max deadline is before now, then set it to two hours from now.
			now := database.Now()
			if autostop.MaxDeadline.Before(now) {
				autostop.MaxDeadline = now.Add(time.Hour * 2)
			}

			// If the current deadline on the build is after the new
			// max_deadline, then set it to the max_deadline.
			autostop.Deadline = build.Deadline
			if build.Deadline.After(build.MaxDeadline) {
				autostop.Deadline = build.MaxDeadline
			}

			// Update the workspace build.
			err = db.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
				ID:          build.ID,
				UpdatedAt:   database.Now(),
				Deadline:    autostop.Deadline,
				MaxDeadline: autostop.MaxDeadline,
			})
			if err != nil {
				return xerrors.Errorf("update workspace build %q: %w", build.ID, err)
			}
		}

		template, err = db.GetTemplateByID(ctx, tpl.ID)
		if err != nil {
			return xerrors.Errorf("get updated template schedule: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		return database.Template{}, err
	}

	return template, nil
}
