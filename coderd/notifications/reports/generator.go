package reports

import (
	"context"
	"database/sql"
	"io"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	delay = 15 * time.Minute
)

// runReport executes one report in its own transaction, guarded by its own
// advisory lock so that only one replica generates it. A replica that cannot
// take the lock skips this tick and tries again on the next one; how often a
// report is actually sent is enforced by the report itself, against the
// timestamp it persists.
func runReport(ctx context.Context, logger slog.Logger, db database.Store, lockID int64, name string, report func(tx database.Store) error) {
	err := db.InTx(func(tx database.Store) error {
		ok, err := tx.TryAcquireLock(ctx, lockID)
		if err != nil {
			return xerrors.Errorf("failed to acquire report lock: %w", err)
		}
		if !ok {
			logger.Debug(ctx, "unable to acquire lock for generating periodic report, skipping", slog.F("report", name))
			return nil
		}
		return report(tx)
	}, nil)
	if err != nil {
		logger.Error(ctx, "failed to generate report", slog.F("report", name), slog.Error(err))
	}
}

func NewReportGenerator(ctx context.Context, logger slog.Logger, db database.Store, enqueuer notifications.Enqueuer, clk quartz.Clock) io.Closer {
	closed := make(chan struct{})

	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system generates periodic reports without direct user input.
	ctx = dbauthz.AsSystemRestricted(ctx)

	// Start the ticker with the initial delay.
	ticker := clk.NewTicker(delay)
	ticker.Stop()
	doTick := func(start time.Time) {
		defer ticker.Reset(delay)

		// Reports are independent, so each runs in its own transaction under
		// its own advisory lock. Sharing either would couple them.
		runReport(ctx, logger, db, database.LockIDNotificationsReportGenerator, "failed workspace builds",
			func(tx database.Store) error {
				return reportFailedWorkspaceBuilds(ctx, logger, tx, enqueuer, clk)
			})
		runReport(ctx, logger, db, database.LockIDNotifyUnpricedAIModels, "unpriced AI models",
			func(tx database.Store) error {
				return reportUnpricedAIModels(ctx, logger, tx, enqueuer, clk)
			})

		logger.Info(ctx, "report generator finished", slog.F("duration", clk.Since(start)))
	}

	go func() {
		defer close(closed)
		defer ticker.Stop()
		// Force an initial tick.
		doTick(dbtime.Time(clk.Now()).UTC())
		for {
			select {
			case <-ctx.Done():
				logger.Debug(ctx, "closing report generator")
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

const (
	failedWorkspaceBuildsReportFrequency      = 7 * 24 * time.Hour
	failedWorkspaceBuildsReportFrequencyLabel = "week"
)

type adminReport struct {
	stats        database.GetWorkspaceBuildStatsByTemplatesRow
	failedBuilds []database.GetFailedWorkspaceBuildsByTemplateIDRow
}

func reportFailedWorkspaceBuilds(ctx context.Context, logger slog.Logger, db database.Store, enqueuer notifications.Enqueuer, clk quartz.Clock) error {
	now := clk.Now()
	since := now.Add(-failedWorkspaceBuildsReportFrequency)

	// Firstly, check if this is the first run of the job ever
	reportLog, err := db.GetNotificationReportGeneratorLogByTemplate(ctx, notifications.TemplateWorkspaceBuildsFailedReport)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("unable to read report generator log: %w", err)
	}
	if xerrors.Is(err, sql.ErrNoRows) {
		// First run? Check-in the job, and get back after one week.
		logger.Info(ctx, "report generator is executing the job for the first time", slog.F("notification_template_id", notifications.TemplateWorkspaceBuildsFailedReport))

		err = db.UpsertNotificationReportGeneratorLog(ctx, database.UpsertNotificationReportGeneratorLogParams{
			NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
			LastGeneratedAt:        dbtime.Time(now).UTC(),
		})
		if err != nil {
			return xerrors.Errorf("unable to update report generator logs (first time execution): %w", err)
		}
		return nil
	}

	// Secondly, check if the job has not been running recently
	if !reportLog.LastGeneratedAt.IsZero() && reportLog.LastGeneratedAt.Add(failedWorkspaceBuildsReportFrequency).After(now) {
		return nil // reports sent recently, no need to send them now
	}

	// Thirdly, fetch workspace build stats by templates
	templateStatsRows, err := db.GetWorkspaceBuildStatsByTemplates(ctx, dbtime.Time(since).UTC())
	if err != nil {
		return xerrors.Errorf("unable to fetch failed workspace builds: %w", err)
	}

	reports := make(map[uuid.UUID][]adminReport)

	for _, stats := range templateStatsRows {
		select {
		case <-ctx.Done():
			logger.Debug(ctx, "context is canceled, quitting", slog.Error(ctx.Err()))
			break
		default:
		}

		if stats.FailedBuilds == 0 {
			logger.Info(ctx, "no failed workspace builds found for template", slog.F("template_id", stats.TemplateID), slog.Error(err))
			continue
		}

		// Fetch template admins with org access to the templates
		templateAdmins, err := findTemplateAdmins(ctx, db, stats)
		if err != nil {
			logger.Error(ctx, "unable to find template admins for template", slog.F("template_id", stats.TemplateID), slog.Error(err))
			continue
		}

		// Fetch failed builds by the template
		failedBuilds, err := db.GetFailedWorkspaceBuildsByTemplateID(ctx, database.GetFailedWorkspaceBuildsByTemplateIDParams{
			TemplateID: stats.TemplateID,
			Since:      dbtime.Time(since).UTC(),
		})
		if err != nil {
			logger.Error(ctx, "unable to fetch failed workspace builds", slog.F("template_id", stats.TemplateID), slog.Error(err))
			continue
		}

		for _, templateAdmin := range templateAdmins {
			adminReports := reports[templateAdmin.ID]
			adminReports = append(adminReports, adminReport{
				failedBuilds: failedBuilds,
				stats:        stats,
			})

			reports[templateAdmin.ID] = adminReports
		}
	}

	for templateAdmin, reports := range reports {
		select {
		case <-ctx.Done():
			logger.Debug(ctx, "context is canceled, quitting", slog.Error(ctx.Err()))
			break
		default:
		}

		reportData := buildDataForReportFailedWorkspaceBuilds(reports)

		targets := []uuid.UUID{}
		for _, report := range reports {
			targets = append(targets, report.stats.TemplateID, report.stats.TemplateOrganizationID)
		}

		if _, err := enqueuer.EnqueueWithData(ctx, templateAdmin, notifications.TemplateWorkspaceBuildsFailedReport,
			map[string]string{},
			reportData,
			"report_generator",
			slice.Unique(targets)...,
		); err != nil {
			logger.Warn(ctx, "failed to send a report with failed workspace builds", slog.Error(err))
		}
	}

	if xerrors.Is(ctx.Err(), context.Canceled) {
		logger.Error(ctx, "report generator job is canceled")
		return ctx.Err()
	}

	// Lastly, update the timestamp in the generator log.
	err = db.UpsertNotificationReportGeneratorLog(ctx, database.UpsertNotificationReportGeneratorLogParams{
		NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
		LastGeneratedAt:        dbtime.Time(now).UTC(),
	})
	if err != nil {
		return xerrors.Errorf("unable to update report generator logs: %w", err)
	}
	return nil
}

const workspaceBuildsLimitPerTemplateVersion = 10

func buildDataForReportFailedWorkspaceBuilds(reports []adminReport) map[string]any {
	templates := []map[string]any{}

	for _, report := range reports {
		// Build notification model for template versions and failed workspace builds.
		//
		// Failed builds are sorted by template version ascending, workspace build number descending.
		// Review builds, group them by template versions, and assign to builds to template versions.
		// The map requires `[]map[string]any{}` to be compatible with data passed to `NotificationEnqueuer`.
		templateVersions := []map[string]any{}
		for _, failedBuild := range report.failedBuilds {
			c := len(templateVersions)

			if c == 0 || templateVersions[c-1]["template_version_name"] != failedBuild.TemplateVersionName {
				templateVersions = append(templateVersions, map[string]any{
					"template_version_name": failedBuild.TemplateVersionName,
					"failed_count":          1,
					"failed_builds": []map[string]any{
						{
							"workspace_owner_username": failedBuild.WorkspaceOwnerUsername,
							"workspace_name":           failedBuild.WorkspaceName,
							"workspace_id":             failedBuild.WorkspaceID,
							"build_number":             failedBuild.WorkspaceBuildNumber,
						},
					},
				})
				continue
			}

			tv := templateVersions[c-1]
			//nolint:errorlint,forcetypeassert // only this function prepares the notification model
			tv["failed_count"] = tv["failed_count"].(int) + 1

			//nolint:errorlint,forcetypeassert // only this function prepares the notification model
			builds := tv["failed_builds"].([]map[string]any)
			if len(builds) < workspaceBuildsLimitPerTemplateVersion {
				// return N last builds to prevent long email reports
				builds = append(builds, map[string]any{
					"workspace_owner_username": failedBuild.WorkspaceOwnerUsername,
					"workspace_name":           failedBuild.WorkspaceName,
					"workspace_id":             failedBuild.WorkspaceID,
					"build_number":             failedBuild.WorkspaceBuildNumber,
				})
				tv["failed_builds"] = builds
			}
			templateVersions[c-1] = tv
		}

		templateDisplayName := report.stats.TemplateDisplayName
		if templateDisplayName == "" {
			templateDisplayName = report.stats.TemplateName
		}

		templates = append(templates, map[string]any{
			"failed_builds": report.stats.FailedBuilds,
			"total_builds":  report.stats.TotalBuilds,
			"versions":      templateVersions,
			"name":          report.stats.TemplateName,
			"display_name":  templateDisplayName,
		})
	}

	return map[string]any{
		"report_frequency": failedWorkspaceBuildsReportFrequencyLabel,
		"templates":        templates,
	}
}

func findTemplateAdmins(ctx context.Context, db database.Store, stats database.GetWorkspaceBuildStatsByTemplatesRow) ([]database.GetUsersRow, error) {
	users, err := db.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleTemplateAdmin},
	})
	if err != nil {
		return nil, xerrors.Errorf("unable to fetch template admins: %w", err)
	}

	var templateAdmins []database.GetUsersRow
	if len(users) == 0 {
		return templateAdmins, nil
	}

	usersByIDs := map[uuid.UUID]database.GetUsersRow{}
	var userIDs []uuid.UUID
	for _, user := range users {
		usersByIDs[user.ID] = user
		userIDs = append(userIDs, user.ID)
	}

	orgIDsByMemberIDs, err := db.GetOrganizationIDsByMemberIDs(ctx, userIDs)
	if err != nil {
		return nil, xerrors.Errorf("unable to fetch organization IDs by member IDs: %w", err)
	}

	for _, entry := range orgIDsByMemberIDs {
		if slices.Contains(entry.OrganizationIDs, stats.TemplateOrganizationID) {
			templateAdmins = append(templateAdmins, usersByIDs[entry.UserID])
		}
	}
	sort.Slice(templateAdmins, func(i, j int) bool {
		return templateAdmins[i].Username < templateAdmins[j].Username
	})
	return templateAdmins, nil
}

const (
	unpricedAIModelsReportFrequency      = 7 * 24 * time.Hour
	unpricedAIModelsReportFrequencyLabel = "week"
	// unpricedAIModelsLimit caps how many models a single report lists.
	// A deployment can accumulate more unpriced models than we want to display
	// in a single notification, so the remaining models are reported as a count.
	unpricedAIModelsLimit = 100
)

// reportUnpricedAIModels notifies owners about models used without a price
// in the preceding week. Unpriced usage is recorded but contributes nothing to
// spend, so it is neither reported nor enforced against a budget.
//
// The set of unpriced models is derived at report time from interceptions and
// the price table, so setting a price removes the model from the next report.
func reportUnpricedAIModels(ctx context.Context, logger slog.Logger, db database.Store, enqueuer notifications.Enqueuer, clk quartz.Clock) error {
	now := clk.Now()
	since := now.Add(-unpricedAIModelsReportFrequency)

	reportLog, err := db.GetNotificationReportGeneratorLogByTemplate(ctx, notifications.TemplateAIModelsUnpricedReport)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("unable to read report generator log: %w", err)
	}

	// Check if the job has not been running recently. The ticker alone cannot
	// enforce the frequency: it restarts with the process and each replica
	// runs on its own phase.
	if !reportLog.LastGeneratedAt.IsZero() && reportLog.LastGeneratedAt.Add(unpricedAIModelsReportFrequency).After(now) {
		return nil // reports sent recently, no need to send them now
	}

	// Fetch the models used without a price.
	unpricedModels, err := db.GetUnpricedAIModelsSince(ctx, dbtime.Time(since).UTC())
	if err != nil {
		return xerrors.Errorf("unable to fetch unpriced AI models: %w", err)
	}

	if len(unpricedModels) > 0 {
		owners, err := db.GetUsers(ctx, database.GetUsersParams{
			RbacRole: []string{codersdk.RoleOwner},
		})
		if err != nil {
			return xerrors.Errorf("unable to fetch owners: %w", err)
		}

		reportData := buildDataForReportUnpricedAIModels(unpricedModels)
		for _, owner := range owners {
			if _, err := enqueuer.EnqueueWithData(ctx, owner.ID, notifications.TemplateAIModelsUnpricedReport,
				map[string]string{},
				reportData,
				"report_generator",
			); err != nil {
				logger.Warn(ctx, "failed to send a report with unpriced AI models", slog.F("user_id", owner.ID), slog.Error(err))
			}
		}
	}

	// Update the timestamp in the generator log. This happens even
	// when nothing was reported, so the next report covers one week rather
	// than every week since usage was last seen.
	err = db.UpsertNotificationReportGeneratorLog(ctx, database.UpsertNotificationReportGeneratorLogParams{
		NotificationTemplateID: notifications.TemplateAIModelsUnpricedReport,
		LastGeneratedAt:        dbtime.Time(now).UTC(),
	})
	if err != nil {
		return xerrors.Errorf("unable to update report generator logs: %w", err)
	}
	return nil
}

// buildDataForReportUnpricedAIModels renders the models most used first, so
// the models dropped by the limit are the ones with the least unreported
// usage.
func buildDataForReportUnpricedAIModels(unpricedModels []database.GetUnpricedAIModelsSinceRow) map[string]any {
	reportedCount := min(len(unpricedModels), unpricedAIModelsLimit)
	reportedModels := make([]map[string]any, 0, reportedCount)
	for _, row := range unpricedModels[:reportedCount] {
		reportedModels = append(reportedModels, map[string]any{
			"provider": row.ProviderType,
			"model":    row.Model,
		})
	}

	return map[string]any{
		"report_frequency": unpricedAIModelsReportFrequencyLabel,
		"models":           reportedModels,
		"total_count":      len(unpricedModels),
		"truncated":        len(unpricedModels) > reportedCount,
	}
}
