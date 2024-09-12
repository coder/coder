package reports

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

const dayDuration = 24 * time.Hour

var (
	jobError     = sql.NullString{String: "badness", Valid: true}
	jobErrorCode = sql.NullString{String: "ERR-42", Valid: true}
)

func TestReportFailedWorkspaceBuilds(t *testing.T) {
	t.Parallel()

	t.Run("InitialState_NoBuilds_NoReport", func(t *testing.T) {
		t.Parallel()

		// Setup
		ctx, logger, db, _, notifEnq, clk := setup(t)

		// When
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then
		require.NoError(t, err)
	})

	t.Run("FailedBuilds_TemplateAdminOptIn_FirstRun_Report_SecondRunTooEarly_NoReport_ThirdRun_Report", func(t *testing.T) {
		t.Parallel()

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
		// template admin in some other org

		// Regular users
		user1 := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user1.ID, OrganizationID: org.ID})
		user2 := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user2.ID, OrganizationID: org.ID})
		user3 := dbgen.User(t, db, database.User{})
		// user in some other org

		// Templates
		t1 := dbgen.Template(t, db, database.Template{Name: "template-1", DisplayName: "First Template", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID})
		t2 := dbgen.Template(t, db, database.Template{Name: "template-2", CreatedBy: templateAdmin1.ID, OrganizationID: org.ID})

		// Template versions
		t1v1 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-1", OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, JobID: uuid.New()})
		t1v2 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-2", OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, JobID: uuid.New()})
		t2v1 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-2-version-1", OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true}, JobID: uuid.New()})
		t2v2 := dbgen.TemplateVersion(t, db, database.TemplateVersion{Name: "template-1-version-1", OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true}, JobID: uuid.New()})

		// Workspaces
		w1 := dbgen.Workspace(t, db, database.Workspace{TemplateID: t1.ID, OwnerID: user1.ID, OrganizationID: org.ID})
		w2 := dbgen.Workspace(t, db, database.Workspace{TemplateID: t2.ID, OwnerID: user2.ID, OrganizationID: org.ID})
		w3 := dbgen.Workspace(t, db, database.Workspace{TemplateID: t1.ID, OwnerID: user3.ID, OrganizationID: org.ID})
		w4 := dbgen.Workspace(t, db, database.Workspace{TemplateID: t2.ID, OwnerID: user2.ID, OrganizationID: org.ID})

		now := clk.Now()

		// Workspace builds
		w1wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-6 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, TemplateVersionID: t1v1.ID, JobID: w1wb1pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w1wb2pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-5 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, TemplateVersionID: t1v2.ID, JobID: w1wb2pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w1wb3pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-4 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w1.ID, TemplateVersionID: t1v2.ID, JobID: w1wb3pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		w2wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-5 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w2.ID, TemplateVersionID: t2v1.ID, JobID: w2wb1pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w2wb2pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-4 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w2.ID, TemplateVersionID: t2v2.ID, JobID: w2wb2pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w2wb3pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-3 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w2.ID, TemplateVersionID: t2v2.ID, JobID: w2wb3pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		w3wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-3 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w3.ID, TemplateVersionID: t1v1.ID, JobID: w3wb1pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		w4wb1pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, Error: jobError, ErrorCode: jobErrorCode, CompletedAt: sql.NullTime{Time: now.Add(-6 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w4.ID, TemplateVersionID: t2v1.ID, JobID: w4wb1pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})
		w4wb2pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID, CompletedAt: sql.NullTime{Time: now.Add(-1 * dayDuration), Valid: true}})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: w4.ID, TemplateVersionID: t2v2.ID, JobID: w4wb2pj.ID, Transition: database.WorkspaceTransitionStart, Reason: database.BuildReasonInitiator})

		// Database is ready, so we can clear notifications queue
		notifEnq.Clear()

		// When
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then
		require.NoError(t, err)

		require.Len(t, notifEnq.Sent, 4) // 2 templates, 2 template admins
		require.Equal(t, notifEnq.Sent[0].UserID, templateAdmin1.ID)
		require.Equal(t, notifEnq.Sent[0].TemplateID, notifications.TemplateWorkspaceBuildsFailedReport)
		require.Equal(t, notifEnq.Sent[0].Labels["template_name"], t1.Name)
		require.Equal(t, notifEnq.Sent[0].Labels["template_display_name"], t1.DisplayName)
		require.Equal(t, notifEnq.Sent[0].Data["failed_builds"], int64(3))
		require.Equal(t, notifEnq.Sent[0].Data["total_builds"], int64(4))
		require.Equal(t, notifEnq.Sent[0].Data["report_frequency"], "week")
		// require.Contains(t, notifEnq.Sent[0].Data["template_versions"], "?")

		require.Equal(t, notifEnq.Sent[1].UserID, templateAdmin2.ID)
		require.Equal(t, notifEnq.Sent[1].TemplateID, notifications.TemplateWorkspaceBuildsFailedReport)
		require.Equal(t, notifEnq.Sent[1].Labels["template_name"], t1.Name)
		require.Equal(t, notifEnq.Sent[1].Labels["template_display_name"], t1.DisplayName)
		require.Equal(t, notifEnq.Sent[1].Data["failed_builds"], int64(3))
		require.Equal(t, notifEnq.Sent[1].Data["total_builds"], int64(4))
		require.Equal(t, notifEnq.Sent[1].Data["report_frequency"], "week")
		// require.Contains(t, notifEnq.Sent[1].Data["template_versions"], "?")

		require.Equal(t, notifEnq.Sent[2].UserID, templateAdmin1.ID)
		require.Equal(t, notifEnq.Sent[2].TemplateID, notifications.TemplateWorkspaceBuildsFailedReport)
		require.Equal(t, notifEnq.Sent[2].Labels["template_name"], t2.Name)
		require.Equal(t, notifEnq.Sent[2].Labels["template_display_name"], t2.DisplayName)
		require.Equal(t, notifEnq.Sent[2].Data["failed_builds"], int64(3))
		require.Equal(t, notifEnq.Sent[2].Data["total_builds"], int64(5))
		require.Equal(t, notifEnq.Sent[2].Data["report_frequency"], "week")
		// require.Contains(t, notifEnq.Sent[0].Data["template_versions"], "?")

		require.Equal(t, notifEnq.Sent[3].UserID, templateAdmin2.ID)
		require.Equal(t, notifEnq.Sent[3].TemplateID, notifications.TemplateWorkspaceBuildsFailedReport)
		require.Equal(t, notifEnq.Sent[3].Labels["template_name"], t2.Name)
		require.Equal(t, notifEnq.Sent[3].Labels["template_display_name"], t2.DisplayName)
		require.Equal(t, notifEnq.Sent[3].Data["failed_builds"], int64(3))
		require.Equal(t, notifEnq.Sent[3].Data["total_builds"], int64(5))
		require.Equal(t, notifEnq.Sent[3].Data["report_frequency"], "week")
		// require.Contains(t, notifEnq.Sent[0].Data["template_versions"], "?")
	})

	t.Run("NoFailedBuilds_TemplateAdminIn_NoReport", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("FailedBuilds_TemplateAdminOptOut_NoReport", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("StaleFailedBuilds_TemplateAdminOptIn_NoReport_Cleanup", func(t *testing.T) {
		t.Parallel()
		// TODO
	})
}

func setup(t *testing.T) (context.Context, slog.Logger, database.Store, pubsub.Pubsub, *testutil.FakeNotificationsEnqueuer, quartz.Clock) {
	t.Helper()

	// nolint:gocritic // reportFailedWorkspaceBuilds is called by system.
	ctx := dbauthz.AsSystemRestricted(context.Background())
	logger := slogtest.Make(t, &slogtest.Options{})
	db, ps := dbtestutil.NewDB(t)
	// does not work with works
	// db := dbauthz.New(rdb, rbac.NewAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())
	notifyEnq := &testutil.FakeNotificationsEnqueuer{}
	clk := quartz.NewMock(t)
	return ctx, logger, db, ps, notifyEnq, clk
}
