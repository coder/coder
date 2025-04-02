package reports

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/rbac"
)

const dayDuration = 24 * time.Hour

var (
	jobError     = sql.NullString{String: "badness", Valid: true}
	jobErrorCode = sql.NullString{String: "ERR-42", Valid: true}
)

func TestReportFailedWorkspaceBuilds(t *testing.T) {
	t.Parallel()

	t.Run("EmptyState_NoBuilds_NoReport", func(t *testing.T) {
		t.Parallel()

		// Setup
		ctx, logger, db, _, notifEnq, clk := setup(t)

		// Database is ready, so we can clear notifications queue
		notifEnq.Clear()

		// When: first run
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then: no report should be generated
		require.NoError(t, err)
		require.Empty(t, notifEnq.Sent())

		// Given: one week later and no jobs were executed
		clk.Advance(failedWorkspaceBuildsReportFrequency + time.Minute)

		// When
		notifEnq.Clear()
		err = reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then: report is still empty
		require.NoError(t, err)
		require.Empty(t, notifEnq.Sent())
	})

	t.Run("InitialState_NoBuilds_NoReport", func(t *testing.T) {
		t.Parallel()

		// Setup
		ctx, logger, db, ps, notifEnq, clk := setup(t)
		now := clk.Now()

		// Organization
		org := dbgen.Organization(t, db, database.Organization{})

		// Template admins
		templateAdmin1 := dbgen.User(t, db, database.User{Username: "template-admin-1", RBACRoles: []string{rbac.RoleTemplateAdmin().Name}})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: templateAdmin1.ID, OrganizationID: org.ID})

		// Regular users
		user1 := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user1.ID, OrganizationID: org.ID})
		user2 := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user2.ID, OrganizationID: org.ID})

		// Templates
		t1 := dbgen.Template(t, db, database.Template{Name: "template-1", DisplayName: "First Template", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID})

		// Template versions
		t1v1 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-1", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, JobID: uuid.New()})

		// Workspaces
		w1 := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: t1.ID, OwnerID: user1.ID, OrganizationID: org.ID})

		w1wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-6 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: 1, TemplateVersionID: t1v1.ID, JobID: w1wb1pj.ID, CreatedAt: now.Add(-2 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		// When: first run
		notifEnq.Clear()
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then: failed builds should not be reported
		require.NoError(t, err)
		require.Empty(t, notifEnq.Sent())

		// Given: one week later, but still no jobs
		clk.Advance(failedWorkspaceBuildsReportFrequency + time.Minute)

		// When
		notifEnq.Clear()
		err = reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then: report is still empty
		require.NoError(t, err)
		require.Empty(t, notifEnq.Sent())
	})

	t.Run("FailedBuilds_SecondRun_Report_ThirdRunTooEarly_NoReport_FourthRun_Report", func(t *testing.T) {
		t.Parallel()

		verifyNotification := func(t *testing.T, recipient database.User, notif *notificationstest.FakeNotification, tmpl database.Template, failedBuilds, totalBuilds int64, templateVersions []map[string]interface{}) {
			t.Helper()

			require.Equal(t, recipient.ID, notif.UserID)
			require.Equal(t, notifications.TemplateWorkspaceBuildsFailedReport, notif.TemplateID)
			require.Equal(t, tmpl.Name, notif.Labels["template_name"])
			require.Equal(t, tmpl.DisplayName, notif.Labels["template_display_name"])
			require.Equal(t, failedBuilds, notif.Data["failed_builds"])
			require.Equal(t, totalBuilds, notif.Data["total_builds"])
			require.Equal(t, "week", notif.Data["report_frequency"])
			require.Equal(t, templateVersions, notif.Data["template_versions"])
		}

		// Setup
		ctx, logger, db, ps, notifEnq, clk := setup(t)

		// Given

		// Organization
		org := dbgen.Organization(t, db, database.Organization{})

		// Template admins
		templateAdmin1 := dbgen.User(t, db, database.User{Username: "template-admin-1", RBACRoles: []string{rbac.RoleTemplateAdmin().Name}})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: templateAdmin1.ID, OrganizationID: org.ID})
		templateAdmin2 := dbgen.User(t, db, database.User{Username: "template-admin-2", RBACRoles: []string{rbac.RoleTemplateAdmin().Name}})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: templateAdmin2.ID, OrganizationID: org.ID})
		_ = dbgen.User(t, db, database.User{Name: "template-admin-3", RBACRoles: []string{rbac.RoleTemplateAdmin().Name}})
		// template admin in some other org, they should not receive any notification

		// Regular users
		user1 := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user1.ID, OrganizationID: org.ID})
		user2 := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user2.ID, OrganizationID: org.ID})

		// Templates
		t1 := dbgen.Template(t, db, database.Template{Name: "template-1", DisplayName: "First Template", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID})
		t2 := dbgen.Template(t, db, database.Template{Name: "template-2", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID})

		// Template versions
		t1v1 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-1", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, JobID: uuid.New()})
		t1v2 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-2", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, JobID: uuid.New()})
		t2v1 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-2-version-1", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true}, JobID: uuid.New()})
		t2v2 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-2-version-2", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true}, JobID: uuid.New()})

		// Workspaces
		w1 := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: t1.ID, OwnerID: user1.ID, OrganizationID: org.ID})
		w2 := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: t2.ID, OwnerID: user2.ID, OrganizationID: org.ID})
		w3 := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: t1.ID, OwnerID: user1.ID, OrganizationID: org.ID})
		w4 := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: t2.ID, OwnerID: user2.ID, OrganizationID: org.ID})

		// When: first run
		notifEnq.Clear()
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then
		require.NoError(t, err)
		require.Empty(t, notifEnq.Sent()) // no notifications

		// One week later...
		clk.Advance(failedWorkspaceBuildsReportFrequency + time.Minute)
		now := clk.Now()

		// Workspace builds
		w1wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-6 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: 1, TemplateVersionID: t1v1.ID, JobID: w1wb1pj.ID, CreatedAt: now.Add(-6 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w1wb2pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-5 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: 2, TemplateVersionID: t1v2.ID, JobID: w1wb2pj.ID, CreatedAt: now.Add(-5 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w1wb3pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-4 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: 3, TemplateVersionID: t1v2.ID, JobID: w1wb3pj.ID, CreatedAt: now.Add(-4 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		w2wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-5 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w2.ID, BuildNumber: 4, TemplateVersionID: t2v1.ID, JobID: w2wb1pj.ID, CreatedAt: now.Add(-5 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w2wb2pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-4 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w2.ID, BuildNumber: 5, TemplateVersionID: t2v2.ID, JobID: w2wb2pj.ID, CreatedAt: now.Add(-4 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w2wb3pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-3 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w2.ID, BuildNumber: 6, TemplateVersionID: t2v2.ID, JobID: w2wb3pj.ID, CreatedAt: now.Add(-3 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		w3wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-3 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w3.ID, BuildNumber: 7, TemplateVersionID: t1v1.ID, JobID: w3wb1pj.ID, CreatedAt: now.Add(-3 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		w4wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-6 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w4.ID, BuildNumber: 8, TemplateVersionID: t2v1.ID, JobID: w4wb1pj.ID, CreatedAt: now.Add(-6 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w4wb2pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w4.ID, BuildNumber: 9, TemplateVersionID: t2v2.ID, JobID: w4wb2pj.ID, CreatedAt: now.Add(-dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		// When
		notifEnq.Clear()
		err = reportFailedWorkspaceBuilds(ctx, logger, authedDB(t, db, logger), notifEnq, clk)

		// Then
		require.NoError(t, err)

		sent := notifEnq.Sent()
		require.Len(t, sent, 4) // 2 templates, 2 template admins
		for i, templateAdmin := range []database.User{templateAdmin1, templateAdmin2} {
			verifyNotification(t, templateAdmin, sent[i], t1, 3, 4, []map[string]interface{}{
				{
					"failed_builds": []map[string]interface{}{
						{"build_number": int32(7), "workspace_name": w3.Name, "workspace_owner_username": user1.Username},
						{"build_number": int32(1), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					},
					"failed_count":          2,
					"template_version_name": t1v1.Name,
				},
				{
					"failed_builds": []map[string]interface{}{
						{"build_number": int32(3), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					},
					"failed_count":          1,
					"template_version_name": t1v2.Name,
				},
			})
		}

		for i, templateAdmin := range []database.User{templateAdmin1, templateAdmin2} {
			verifyNotification(t, templateAdmin, sent[i+2], t2, 3, 5, []map[string]interface{}{
				{
					"failed_builds": []map[string]interface{}{
						{"build_number": int32(8), "workspace_name": w4.Name, "workspace_owner_username": user2.Username},
					},
					"failed_count":          1,
					"template_version_name": t2v1.Name,
				},
				{
					"failed_builds": []map[string]interface{}{
						{"build_number": int32(6), "workspace_name": w2.Name, "workspace_owner_username": user2.Username},
						{"build_number": int32(5), "workspace_name": w2.Name, "workspace_owner_username": user2.Username},
					},
					"failed_count":          2,
					"template_version_name": t2v2.Name,
				},
			})
		}

		// Given: 6 days later (less than report frequency), and failed build
		clk.Advance(6 * dayDuration).MustWait(context.Background())
		now = clk.Now()

		w1wb4pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: 77, TemplateVersionID: t1v2.ID, JobID: w1wb4pj.ID, CreatedAt: now.Add(-dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		// When
		notifEnq.Clear()
		err = reportFailedWorkspaceBuilds(ctx, logger, authedDB(t, db, logger), notifEnq, clk)
		require.NoError(t, err)

		// Then: no notifications as it is too early
		require.Empty(t, notifEnq.Sent())

		// Given: 1 day 1 hour later
		clk.Advance(dayDuration + time.Hour).MustWait(context.Background())

		// When
		notifEnq.Clear()
		err = reportFailedWorkspaceBuilds(ctx, logger, authedDB(t, db, logger), notifEnq, clk)
		require.NoError(t, err)

		// Then: we should see the failed job in the report
		sent = notifEnq.Sent()
		require.Len(t, sent, 2) // a new failed job should be reported
		for i, templateAdmin := range []database.User{templateAdmin1, templateAdmin2} {
			verifyNotification(t, templateAdmin, sent[i], t1, 1, 1, []map[string]interface{}{
				{
					"failed_builds": []map[string]interface{}{
						{"build_number": int32(77), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					},
					"failed_count":          1,
					"template_version_name": t1v2.Name,
				},
			})
		}
	})

	t.Run("TooManyFailedBuilds_SecondRun_Report", func(t *testing.T) {
		t.Parallel()

		verifyNotification := func(t *testing.T, recipient database.User, notif *notificationstest.FakeNotification, tmpl database.Template, failedBuilds, totalBuilds int64, templateVersions []map[string]interface{}) {
			t.Helper()

			require.Equal(t, recipient.ID, notif.UserID)
			require.Equal(t, notifications.TemplateWorkspaceBuildsFailedReport, notif.TemplateID)
			require.Equal(t, tmpl.Name, notif.Labels["template_name"])
			require.Equal(t, tmpl.DisplayName, notif.Labels["template_display_name"])
			require.Equal(t, failedBuilds, notif.Data["failed_builds"])
			require.Equal(t, totalBuilds, notif.Data["total_builds"])
			require.Equal(t, "week", notif.Data["report_frequency"])
			require.Equal(t, templateVersions, notif.Data["template_versions"])
		}

		// Setup
		ctx, logger, db, ps, notifEnq, clk := setup(t)

		// Given

		// Organization
		org := dbgen.Organization(t, db, database.Organization{})

		// Template admins
		templateAdmin1 := dbgen.User(t, db, database.User{Username: "template-admin-1", RBACRoles: []string{rbac.RoleTemplateAdmin().Name}})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: templateAdmin1.ID, OrganizationID: org.ID})

		// Regular users
		user1 := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user1.ID, OrganizationID: org.ID})

		// Templates
		t1 := dbgen.Template(t, db, database.Template{Name: "template-1", DisplayName: "First Template", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID})

		// Template versions
		t1v1 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-1", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, JobID: uuid.New()})
		t1v2 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-2", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, JobID: uuid.New()})

		// Workspaces
		w1 := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: t1.ID, OwnerID: user1.ID, OrganizationID: org.ID})

		// When: first run
		notifEnq.Clear()
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then
		require.NoError(t, err)
		require.Empty(t, notifEnq.Sent()) // no notifications

		// One week later...
		clk.Advance(failedWorkspaceBuildsReportFrequency + time.Minute)
		now := clk.Now()

		// Workspace builds
		pj0 := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-24 * time.Hour), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: 777, TemplateVersionID: t1v1.ID, JobID: pj0.ID, CreatedAt: now.Add(-24 * time.Hour), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		for i := 1; i <= 23; i++ {
			at := now.Add(-time.Duration(i) * time.Hour)

			pj1 := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: at, Valid: true}})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: int32(i), TemplateVersionID: t1v1.ID, JobID: pj1.ID, CreatedAt: at, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator}) // nolint:gosec

			pj2 := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: at, Valid: true}})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: int32(i) + 100, TemplateVersionID: t1v2.ID, JobID: pj2.ID, CreatedAt: at, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator}) // nolint:gosec
		}

		// When
		notifEnq.Clear()
		err = reportFailedWorkspaceBuilds(ctx, logger, authedDB(t, db, logger), notifEnq, clk)

		// Then
		require.NoError(t, err)

		sent := notifEnq.Sent()
		require.Len(t, sent, 1) // 1 template, 1 template admin
		verifyNotification(t, templateAdmin1, sent[0], t1, 46, 47, []map[string]interface{}{
			{
				"failed_builds": []map[string]interface{}{
					{"build_number": int32(23), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(22), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(21), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(20), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(19), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(18), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(17), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(16), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(15), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(14), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
				},
				"failed_count":          23,
				"template_version_name": t1v1.Name,
			},
			{
				"failed_builds": []map[string]interface{}{
					{"build_number": int32(123), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(122), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(121), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(120), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(119), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(118), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(117), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(116), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(115), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
					{"build_number": int32(114), "workspace_name": w1.Name, "workspace_owner_username": user1.Username},
				},
				"failed_count":          23,
				"template_version_name": t1v2.Name,
			},
		})
	})

	t.Run("NoFailedBuilds_NoReport", func(t *testing.T) {
		t.Parallel()

		// Setup
		ctx, logger, db, ps, notifEnq, clk := setup(t)

		// Given
		// Organization
		org := dbgen.Organization(t, db, database.Organization{})

		// Template admins
		templateAdmin1 := dbgen.User(t, db, database.User{Username: "template-admin-1", RBACRoles: []string{rbac.RoleTemplateAdmin().Name}})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: templateAdmin1.ID, OrganizationID: org.ID})

		// Regular users
		user1 := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user1.ID, OrganizationID: org.ID})

		// Templates
		t1 := dbgen.Template(t, db, database.Template{Name: "template-1", DisplayName: "First Template", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID})

		// Template versions
		t1v1 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-1", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, JobID: uuid.New()})

		// Workspaces
		w1 := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: t1.ID, OwnerID: user1.ID, OrganizationID: org.ID})

		// When: first run
		notifEnq.Clear()
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then: no notifications
		require.NoError(t, err)
		require.Empty(t, notifEnq.Sent())

		// Given: one week later, and a successful few jobs being executed
		clk.Advance(failedWorkspaceBuildsReportFrequency + time.Minute)
		now := clk.Now()

		// Workspace builds
		w1wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-6 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: 1, TemplateVersionID: t1v1.ID, JobID: w1wb1pj.ID, CreatedAt: now.Add(-2 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w1wb2pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-5 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, BuildNumber: 2, TemplateVersionID: t1v1.ID, JobID: w1wb2pj.ID, CreatedAt: now.Add(-1 * dayDuration), Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		// When
		notifEnq.Clear()
		err = reportFailedWorkspaceBuilds(ctx, logger, authedDB(t, db, logger), notifEnq, clk)

		// Then: no failures? nothing to report
		require.NoError(t, err)
		require.Len(t, notifEnq.Sent(), 0) // all jobs succeeded so nothing to report
	})
}

func setup(t *testing.T) (context.Context, slog.Logger, database.Store, pubsub.Pubsub, *notificationstest.FakeEnqueuer, *quartz.Mock) {
	t.Helper()

	// nolint:gocritic // reportFailedWorkspaceBuilds is called by system.
	ctx := dbauthz.AsSystemRestricted(context.Background())
	logger := slogtest.Make(t, &slogtest.Options{})
	db, ps := dbtestutil.NewDB(t)
	notifyEnq := &notificationstest.FakeEnqueuer{}
	clk := quartz.NewMock(t)
	return ctx, logger, db, ps, notifyEnq, clk
}

func authedDB(t *testing.T, db database.Store, logger slog.Logger) database.Store {
	t.Helper()
	return dbauthz.New(db, rbac.NewAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())
}
