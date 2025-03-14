package reports
import (
	"fmt"
	"errors"
	"context"
	"database/sql"
	"io"
	"slices"
	"sort"
	"time"
	"github.com/google/uuid"
	"cdr.dev/slog"
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
		// Start a transaction to grab advisory lock, we don't want to run generator jobs at the same time (multiple replicas).
		if err := db.InTx(func(tx database.Store) error {
			// Acquire a lock to ensure that only one instance of the generator is running at a time.
			ok, err := tx.TryAcquireLock(ctx, database.LockIDNotificationsReportGenerator)
			if err != nil {
				return fmt.Errorf("failed to acquire report generator lock: %w", err)
			}
			if !ok {
				logger.Debug(ctx, "unable to acquire lock for generating periodic reports, skipping")
				return nil
			}
			err = reportFailedWorkspaceBuilds(ctx, logger, tx, enqueuer, clk)
			if err != nil {
				return fmt.Errorf("unable to generate reports with failed workspace builds: %w", err)
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
func reportFailedWorkspaceBuilds(ctx context.Context, logger slog.Logger, db database.Store, enqueuer notifications.Enqueuer, clk quartz.Clock) error {
	now := clk.Now()
	since := now.Add(-failedWorkspaceBuildsReportFrequency)
	// Firstly, check if this is the first run of the job ever
	reportLog, err := db.GetNotificationReportGeneratorLogByTemplate(ctx, notifications.TemplateWorkspaceBuildsFailedReport)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unable to read report generator log: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		// First run? Check-in the job, and get back after one week.
		logger.Info(ctx, "report generator is executing the job for the first time", slog.F("notification_template_id", notifications.TemplateWorkspaceBuildsFailedReport))
		err = db.UpsertNotificationReportGeneratorLog(ctx, database.UpsertNotificationReportGeneratorLogParams{
			NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
			LastGeneratedAt:        dbtime.Time(now).UTC(),
		})
		if err != nil {
			return fmt.Errorf("unable to update report generator logs (first time execution): %w", err)
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
		return fmt.Errorf("unable to fetch failed workspace builds: %w", err)
	}
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
		reportData := buildDataForReportFailedWorkspaceBuilds(stats, failedBuilds)
		// Send reports to template admins
		templateDisplayName := stats.TemplateDisplayName
		if templateDisplayName == "" {
			templateDisplayName = stats.TemplateName
		}
		for _, templateAdmin := range templateAdmins {
			select {
			case <-ctx.Done():
				logger.Debug(ctx, "context is canceled, quitting", slog.Error(ctx.Err()))
				break
			default:
			}
			if _, err := enqueuer.EnqueueWithData(ctx, templateAdmin.ID, notifications.TemplateWorkspaceBuildsFailedReport,
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
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		logger.Error(ctx, "report generator job is canceled")
		return ctx.Err()
	}
	// Lastly, update the timestamp in the generator log.
	err = db.UpsertNotificationReportGeneratorLog(ctx, database.UpsertNotificationReportGeneratorLogParams{
		NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
		LastGeneratedAt:        dbtime.Time(now).UTC(),
	})
	if err != nil {
		return fmt.Errorf("unable to update report generator logs: %w", err)
	}
	return nil
}
const workspaceBuildsLimitPerTemplateVersion = 10
func buildDataForReportFailedWorkspaceBuilds(stats database.GetWorkspaceBuildStatsByTemplatesRow, failedBuilds []database.GetFailedWorkspaceBuildsByTemplateIDRow) map[string]any {
	// Build notification model for template versions and failed workspace builds.
	//
	// Failed builds are sorted by template version ascending, workspace build number descending.
	// Review builds, group them by template versions, and assign to builds to template versions.
	// The map requires `[]map[string]any{}` to be compatible with data passed to `NotificationEnqueuer`.
	templateVersions := []map[string]any{}
	for _, failedBuild := range failedBuilds {
		c := len(templateVersions)
		if c == 0 || templateVersions[c-1]["template_version_name"] != failedBuild.TemplateVersionName {
			templateVersions = append(templateVersions, map[string]any{
				"template_version_name": failedBuild.TemplateVersionName,
				"failed_count":          1,
				"failed_builds": []map[string]any{
					{
						"workspace_owner_username": failedBuild.WorkspaceOwnerUsername,
						"workspace_name":           failedBuild.WorkspaceName,
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
				"build_number":             failedBuild.WorkspaceBuildNumber,
			})
			tv["failed_builds"] = builds
		}
		templateVersions[c-1] = tv
	}
	return map[string]any{
		"failed_builds":     stats.FailedBuilds,
		"total_builds":      stats.TotalBuilds,
		"report_frequency":  failedWorkspaceBuildsReportFrequencyLabel,
		"template_versions": templateVersions,
	}
}
func findTemplateAdmins(ctx context.Context, db database.Store, stats database.GetWorkspaceBuildStatsByTemplatesRow) ([]database.GetUsersRow, error) {
	users, err := db.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleTemplateAdmin},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to fetch template admins: %w", err)
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
		return nil, fmt.Errorf("unable to fetch organization IDs by member IDs: %w", err)
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
