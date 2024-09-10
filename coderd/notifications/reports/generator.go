package reports

import (
	"context"
	"database/sql"
	"io"
	"slices"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	delay = 5 * time.Minute
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

	templates, err := db.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
		Deleted:    false,
		Deprecated: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return xerrors.Errorf("unable to fetch active templates: %w", err)
	}

	for _, template := range templates {
		failedBuilds, err := db.GetFailedWorkspaceBuildsByTemplateID(ctx, database.GetFailedWorkspaceBuildsByTemplateIDParams{
			TemplateID: template.ID,
			Since:      dbtime.Time(clk.Now()).UTC(),
		})
		if err != nil {
			logger.Error(ctx, "unable to fetch failed workspace builds", slog.F("template_id", template.ID), slog.Error(err))
			continue
		}

		// TODO Lazy-render the report.
		reportData := map[string]any{}

		templateAdmins, err := findTemplateAdmins(ctx, db, template)
		if err != nil {
			logger.Error(ctx, "unable to find template admins", slog.F("template_id", template.ID), slog.Error(err))
			continue
		}

		for _, templateAdmin := range templateAdmins {
			// TODO Check if report is enabled for the person.

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
					logger.Error(ctx, "unable to update report generator logs", slog.F("template_id", template.ID), slog.F("user_id", templateAdmin.ID), slog.F("failed_builds", len(failedBuilds)), slog.Error(err))
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
					logger.Error(ctx, "unable to update report generator logs", slog.F("template_id", template.ID), slog.F("user_id", templateAdmin.ID), slog.F("failed_builds", len(failedBuilds)), slog.Error(err))
					continue
				}
			}

			templateDisplayName := template.DisplayName
			if templateDisplayName == "" {
				templateDisplayName = template.Name
			}

			if _, err := enqueuer.EnqueueData(ctx, templateAdmin.ID, notifications.TemplateWorkspaceBuildsFailedReport,
				map[string]string{
					"template_name":         template.Name,
					"template_display_name": templateDisplayName,
				},
				reportData,
				"report_generator",
				template.ID, template.OrganizationID,
			); err != nil {
				logger.Warn(ctx, "failed to send a report with failed workspace builds", slog.Error(err))
			}

			err = db.UpsertReportGeneratorLog(ctx, database.UpsertReportGeneratorLogParams{
				UserID:                 templateAdmin.ID,
				NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
				LastGeneratedAt:        dbtime.Time(clk.Now()).UTC(),
			})
			if err != nil {
				logger.Error(ctx, "unable to update report generator logs", slog.F("template_id", template.ID), slog.F("user_id", templateAdmin.ID), slog.F("failed_builds", len(failedBuilds)), slog.Error(err))
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

func buildDataForReportFailedWorkspaceBuilds() map[string]any {
	// TODO Lazy-render the report.
	reportData := map[string]any{}

	return reportData
}

func findTemplateAdmins(ctx context.Context, db database.Store, template database.Template) ([]database.GetUsersRow, error) {
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
			if slices.Contains(entry.OrganizationIDs, template.OrganizationID) {
				templateAdmins = append(templateAdmins, usersByIDs[entry.UserID])
			}
		}
	}
	sort.Slice(templateAdmins, func(i, j int) bool {
		return templateAdmins[i].Username < templateAdmins[j].Username
	})

	return templateAdmins, nil
}
