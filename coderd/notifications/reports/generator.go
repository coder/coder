package reports

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"slices"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/google/uuid"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
)

const (
	delay = 15 * time.Minute
)

func NewReportGenerator(ctx context.Context, logger slog.Logger, db database.Store, enqueur notifications.Enqueuer, clk quartz.Clock) io.Closer {
	closed := make(chan struct{})

	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system generates periodic reports without direct user input.
	ctx = dbauthz.AsSystemRestricted(ctx)

	// Start the ticker with the initial delay.
	ticker := clk.NewTicker(delay)
	doTick := func(start time.Time) {
		defer ticker.Reset(delay)
		// Start a transaction to grab advisory lock, we don't want to run generator jobs at the same time (multiple replicas).
		if err := db.InTx(func(tx database.Store) error {
			// Acquire a lock to ensure that only one instance of the generator is running at a time.
			ok, err := tx.TryAcquireLock(ctx, database.LockIDReportGenerator)
			if err != nil {
				return err
			}
			if !ok {
				logger.Debug(ctx, "unable to acquire lock for generating periodic reports, skipping")
				return nil
			}

			err = reportFailedWorkspaceBuilds(ctx, logger, db, enqueur, clk)
			if err != nil {
				logger.Debug(ctx, "unable to report failed workspace builds")
				return err
			}

			logger.Info(ctx, "report generator finished", slog.F("duration", clk.Since(start)))

			return nil
		}, nil); err != nil {
			logger.Error(ctx, "failed to generate reports", slog.Error(err))
			return
		}
	}

	go func() {
		defer close(closed)
		defer ticker.Stop()
		// Force an initial tick.
		doTick(dbtime.Time(clk.Now()).UTC())
		for {
			select {
			case <-ctx.Done():
				return
			case tick := <-ticker.C:
				ticker.Stop()
				doTick(dbtime.Time(tick).UTC())
			}
		}
	}()
	return &reportGenerator{
		cancel: cancelFunc,
		closed: closed,
	}
}

type reportGenerator struct {
	cancel context.CancelFunc
	closed chan struct{}
}

func (i *reportGenerator) Close() error {
	i.cancel()
	<-i.closed
	return nil
}

func reportFailedWorkspaceBuilds(ctx context.Context, logger slog.Logger, db database.Store, enqueuer notifications.Enqueuer, clk quartz.Clock) error {
	const frequencyDays = 7

	statsRows, err := db.GetWorkspaceBuildStatsByTemplates(ctx, dbtime.Time(clk.Now()).UTC())
	if err != nil {
		return xerrors.Errorf("unable to fetch failed workspace builds: %w", err)
	}

	for _, stats := range statsRows {
		var failedBuilds []database.GetFailedWorkspaceBuildsByTemplateIDRow
		reportData := map[string]any{}

		if stats.FailedBuilds > 0 {
			failedBuilds, err = db.GetFailedWorkspaceBuildsByTemplateID(ctx, database.GetFailedWorkspaceBuildsByTemplateIDParams{
				TemplateID: stats.TemplateID,
				Since:      dbtime.Time(clk.Now()).UTC(),
			})
			if err != nil {
				logger.Error(ctx, "unable to fetch failed workspace builds", slog.F("template_id", stats.TemplateID), slog.Error(err))
				continue
			}

			// There are some failed builds, so we have to prepare input data for the report.
			reportData = buildDataForReportFailedWorkspaceBuilds(frequencyDays, stats, failedBuilds)
		}

		templateAdmins, err := findTemplateAdmins(ctx, db, stats)
		if err != nil {
			logger.Error(ctx, "unable to find template admins", slog.F("template_id", stats.TemplateID), slog.Error(err))
			continue
		}

		for _, templateAdmin := range templateAdmins {
			reportLog, err := db.GetReportGeneratorLogByUserAndTemplate(ctx, database.GetReportGeneratorLogByUserAndTemplateParams{
				UserID:                 templateAdmin.ID,
				NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
			})
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) { // sql.ErrNoRows: report not generated yet
				return xerrors.Errorf("unable to get recent report generator log for user: %w", err)
			}

			if !reportLog.LastGeneratedAt.IsZero() && reportLog.LastGeneratedAt.Add(frequencyDays*24*time.Hour).After(clk.Now()) {
				// report generated recently, no need to send it now
				err = db.UpsertReportGeneratorLog(ctx, database.UpsertReportGeneratorLogParams{
					UserID:                 templateAdmin.ID,
					NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
					LastGeneratedAt:        dbtime.Time(clk.Now()).UTC(),
				})
				if err != nil {
					logger.Error(ctx, "unable to update report generator logs", slog.F("template_id", stats.TemplateID), slog.F("user_id", templateAdmin.ID), slog.F("failed_builds", len(failedBuilds)), slog.Error(err))
					continue
				}
			}

			if len(failedBuilds) == 0 {
				// no failed workspace builds, no need to send the report
				err = db.UpsertReportGeneratorLog(ctx, database.UpsertReportGeneratorLogParams{
					UserID:                 templateAdmin.ID,
					NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
					LastGeneratedAt:        dbtime.Time(clk.Now()).UTC(),
				})
				if err != nil {
					logger.Error(ctx, "unable to update report generator logs", slog.F("template_id", stats.TemplateID), slog.F("user_id", templateAdmin.ID), slog.F("failed_builds", len(failedBuilds)), slog.Error(err))
					continue
				}
			}

			templateDisplayName := stats.TemplateDisplayName
			if templateDisplayName == "" {
				templateDisplayName = stats.TemplateName
			}

			if _, err := enqueuer.EnqueueData(ctx, templateAdmin.ID, notifications.TemplateWorkspaceBuildsFailedReport,
				map[string]string{
					"template_name":         stats.TemplateName,
					"template_display_name": templateDisplayName,
				},
				reportData,
				"report_generator",
				stats.TemplateID, stats.TemplateOrganizationID,
			); err != nil {
				logger.Warn(ctx, "failed to send a report with failed workspace builds", slog.Error(err))
			}

			err = db.UpsertReportGeneratorLog(ctx, database.UpsertReportGeneratorLogParams{
				UserID:                 templateAdmin.ID,
				NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
				LastGeneratedAt:        dbtime.Time(clk.Now()).UTC(),
			})
			if err != nil {
				logger.Error(ctx, "unable to update report generator logs", slog.F("template_id", stats.TemplateID), slog.F("user_id", templateAdmin.ID), slog.F("failed_builds", len(failedBuilds)), slog.Error(err))
				continue
			}
		}
	}

	err = db.DeleteOldReportGeneratorLogs(ctx, frequencyDays)
	if err != nil {
		return xerrors.Errorf("unable to delete old report generator logs: %w", err)
	}
	return nil
}

func buildDataForReportFailedWorkspaceBuilds(frequencyDays int, stats database.GetWorkspaceBuildStatsByTemplatesRow, failedBuilds []database.GetFailedWorkspaceBuildsByTemplateIDRow) map[string]any {
	// Format frequency label
	var frequencyLabel string
	if frequencyDays == 7 {
		frequencyLabel = "week"
	} else {
		var plural string
		if frequencyDays > 1 {
			plural = "s"
		}
		frequencyLabel = fmt.Sprintf("%d day%s", frequencyDays, plural)
	}

	// Sorting order: template_version_name ASC, workspace build number DESC
	sort.Slice(failedBuilds, func(i, j int) bool {
		if failedBuilds[i].TemplateVersionName != failedBuilds[j].TemplateVersionName {
			return failedBuilds[i].TemplateVersionName < failedBuilds[j].TemplateVersionName
		}
		return failedBuilds[i].WorkspaceBuildNumber > failedBuilds[j].WorkspaceBuildNumber
	})

	// Build notification model for template versions and failed workspace builds
	templateVersions := []map[string]any{}
	for _, failedBuild := range failedBuilds {
		c := len(templateVersions)

		if len(templateVersions) == 0 || templateVersions[c-1]["template_version_name"] != failedBuild.TemplateVersionName {
			templateVersions = append(templateVersions, map[string]any{
				"template_version_name": failedBuild.TemplateVersionName,
				"failed_count":          1,
				"failed_builds": map[string]any{
					"workspace_owner_username": failedBuild.WorkspaceOwnerUsername,
					"workspace_name":           failedBuild.WorkspaceName,
					"build_number":             failedBuild.WorkspaceBuildNumber,
				},
			})
			continue
		}

		//nolint:errorlint,forcetypeassert // only this function prepares the notification model
		builds := templateVersions[c-1]["failed_builds"].([]map[string]any)
		builds = append(builds, map[string]any{
			"workspace_owner_username": failedBuild.WorkspaceOwnerUsername,
			"workspace_name":           failedBuild.WorkspaceName,
			"build_number":             failedBuild.WorkspaceBuildNumber,
		})
		templateVersions[c-1]["failed_builds"] = builds
	}

	return map[string]any{
		"failed_builds":     stats.FailedBuilds,
		"total_builds":      stats.TotalBuilds,
		"report_frequency":  frequencyLabel,
		"template_versions": templateVersions,
	}
}

func findTemplateAdmins(ctx context.Context, db database.Store, stats database.GetWorkspaceBuildStatsByTemplatesRow) ([]database.GetUsersRow, error) {
	users, err := db.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleTemplateAdmin},
	})
	if err != nil {
		return nil, xerrors.Errorf("unable to fetch template admins: %w", err)
	}

	usersByIDs := map[uuid.UUID]database.GetUsersRow{}
	var userIDs []uuid.UUID
	for _, user := range users {
		usersByIDs[user.ID] = user
		userIDs = append(userIDs, user.ID)
	}

	var templateAdmins []database.GetUsersRow
	if len(userIDs) > 0 {
		orgIDsByMemberIDs, err := db.GetOrganizationIDsByMemberIDs(ctx, userIDs)
		if err != nil {
			return nil, xerrors.Errorf("unable to fetch organization IDs by member IDs: %w", err)
		}

		for _, entry := range orgIDsByMemberIDs {
			if slices.Contains(entry.OrganizationIDs, stats.TemplateOrganizationID) {
				templateAdmins = append(templateAdmins, usersByIDs[entry.UserID])
			}
		}
	}
	sort.Slice(templateAdmins, func(i, j int) bool {
		return templateAdmins[i].Username < templateAdmins[j].Username
	})

	return templateAdmins, nil
}
