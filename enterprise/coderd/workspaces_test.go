package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/rbac"
	agplschedule "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/schedule"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// agplUserQuietHoursScheduleStore is passed to
// NewEnterpriseTemplateScheduleStore as we don't care about updating the
// schedule and having it recalculate the build deadline in these tests.
func agplUserQuietHoursScheduleStore() *atomic.Pointer[agplschedule.UserQuietHoursScheduleStore] {
	store := agplschedule.NewAGPLUserQuietHoursScheduleStore()
	p := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
	p.Store(&store)
	return p
}

func TestCreateWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("NoTemplateAccess", func(t *testing.T) {
		t.Parallel()

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleMember(), rbac.RoleOwner())

		ctx := testutil.Context(t, testutil.WaitLong)

		org, err := other.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)
		version := coderdtest.CreateTemplateVersion(t, other, org.ID, nil)
		template := coderdtest.CreateTemplate(t, other, org.ID, version.ID)

		ctx = testutil.Context(t, testutil.WaitLong) // Reset the context to avoid timeouts.

		_, err = client.CreateWorkspace(ctx, first.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotAcceptable, apiErr.StatusCode())
	})

	// Test that a user cannot indirectly access
	// a template they do not have access to.
	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		acl, err := templateAdminClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)

		err = templateAdminClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				acl.Groups[0].ID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		client1, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())

		req := codersdk.CreateWorkspaceRequest{
			TemplateID:        template.ID,
			Name:              "testme",
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
			TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
		}

		_, err = client1.CreateWorkspace(ctx, user.OrganizationID, user1.ID.String(), req)
		require.Error(t, err)
	})

	t.Run("NoTemplateAccess", func(t *testing.T) {
		t.Parallel()
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})

		templateAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
		user, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleMember())

		version := coderdtest.CreateTemplateVersion(t, templateAdmin, owner.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, version.ID)
		template := coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		// Remove everyone access
		err := templateAdmin.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{},
			GroupPerms: map[string]codersdk.TemplateRole{
				owner.OrganizationID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		// Test "everyone" access is revoked to the regular user
		_, err = user.Template(ctx, template.ID)
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())

		_, err = user.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID:        template.ID,
			Name:              "random",
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
			TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
			AutomaticUpdates:  codersdk.AutomaticUpdatesNever,
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "doesn't exist")
	})

	// Auditors cannot "use" templates, they can only read them.
	t.Run("Auditor", func(t *testing.T) {
		t.Parallel()

		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		// A member of the org as an auditor
		auditor, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleAuditor())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Given: a template with a version without the "use" permission on everyone
		version := coderdtest.CreateTemplateVersion(t, owner, first.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, owner, version.ID)
		template := coderdtest.CreateTemplate(t, owner, first.OrganizationID, version.ID)

		//nolint:gocritic // This should be run as the owner user.
		err := owner.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: nil,
			GroupPerms: map[string]codersdk.TemplateRole{
				first.OrganizationID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		_, err = auditor.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Unauthorized access to use the template")
	})
}

func TestCreateUserWorkspace(t *testing.T) {
	t.Parallel()

	// Create a custom role that can create workspaces for another user.
	t.Run("ForAnotherUser", func(t *testing.T) {
		t.Parallel()

		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:  1,
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitShort)
		r, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
			Name:           "creator",
			OrganizationID: first.OrganizationID.String(),
			DisplayName:    "Creator",
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionCreate},
			}),
		})
		require.NoError(t, err)

		// use admin for setting up test
		admin, adminID := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleTemplateAdmin())

		// try the test action with this user & custom role
		creator, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleMember(), rbac.RoleIdentifier{
			Name:           r.Name,
			OrganizationID: first.OrganizationID,
		})

		version := coderdtest.CreateTemplateVersion(t, admin, first.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, admin, version.ID)
		template := coderdtest.CreateTemplate(t, admin, first.OrganizationID, version.ID)

		ctx = testutil.Context(t, testutil.WaitLong*1000) // Reset the context to avoid timeouts.

		var _ = creator
		_, err = owner.CreateUserWorkspace(ctx, adminID.ID.String(), codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.NoError(t, err)
	})

	t.Run("NoTemplateAccess", func(t *testing.T) {
		t.Parallel()

		client, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleMember(), rbac.RoleOwner())

		ctx := testutil.Context(t, testutil.WaitLong)

		org, err := other.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)
		version := coderdtest.CreateTemplateVersion(t, other, org.ID, nil)
		template := coderdtest.CreateTemplate(t, other, org.ID, version.ID)

		ctx = testutil.Context(t, testutil.WaitLong) // Reset the context to avoid timeouts.

		_, err = client.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotAcceptable, apiErr.StatusCode())
	})

	// Test that a user cannot indirectly access
	// a template they do not have access to.
	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		acl, err := templateAdminClient.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)

		err = templateAdminClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				acl.Groups[0].ID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		client1, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())

		req := codersdk.CreateWorkspaceRequest{
			TemplateID:        template.ID,
			Name:              "testme",
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
			TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
		}

		_, err = client1.CreateUserWorkspace(ctx, user1.ID.String(), req)
		require.Error(t, err)
	})
}

func TestWorkspaceAutobuild(t *testing.T) {
	t.Parallel()

	t.Run("FailureTTLOK", func(t *testing.T) {
		t.Parallel()

		var (
			ticker = make(chan time.Time)
			statCh = make(chan autobuild.Stats)
			logger = slogtest.Make(t, &slogtest.Options{
				// We ignore errors here since we expect to fail
				// builds.
				IgnoreErrors: true,
			})
			failureTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Logger:                   &logger,
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyFailed,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.FailureTTLMillis = ptr.Ref[int64](failureTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		ticker <- build.Job.CompletedAt.Add(failureTTL * 2)
		stats := <-statCh
		// Expect workspace to transition to stopped state for breaching
		// failure TTL.
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionStop)
	})

	t.Run("FailureTTLTooEarly", func(t *testing.T) {
		t.Parallel()

		var (
			ticker = make(chan time.Time)
			statCh = make(chan autobuild.Stats)
			logger = slogtest.Make(t, &slogtest.Options{
				// We ignore errors here since we expect to fail
				// builds.
				IgnoreErrors: true,
			})
			failureTTL = time.Minute
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Logger:                   &logger,
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyFailed,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.FailureTTLMillis = ptr.Ref[int64](failureTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		// Make it impossible to trigger the failure TTL.
		ticker <- build.Job.CompletedAt.Add(-failureTTL * 2)
		stats := <-statCh
		// Expect no transitions since not enough time has elapsed.
		require.Len(t, stats.Transitions, 0)
	})

	// This just provides a baseline that no actions are being taken
	// against a workspace when none of the TTL fields are set.
	t.Run("TemplateTTLsUnset", func(t *testing.T) {
		t.Parallel()

		var (
			ticker = make(chan time.Time)
			statCh = make(chan autobuild.Stats)
			logger = slogtest.Make(t, &slogtest.Options{
				// We ignore errors here since we expect to fail
				// builds.
				IgnoreErrors: true,
			})
		)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Logger:                   &logger,
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		// Create a template without setting a failure_ttl.
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		require.Zero(t, template.TimeTilDormantMillis)
		require.Zero(t, template.FailureTTLMillis)
		require.Zero(t, template.TimeTilDormantAutoDeleteMillis)

		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		ticker <- time.Now()
		stats := <-statCh
		// Expect no transitions since the fields are unset on the template.
		require.Len(t, stats.Transitions, 0)
	})

	t.Run("DormancyThresholdOK", func(t *testing.T) {
		t.Parallel()

		var (
			ticker        = make(chan time.Time)
			statCh        = make(chan autobuild.Stats)
			inactiveTTL   = time.Minute
			auditRecorder = audit.NewMock()
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:       ticker,
				AutobuildStats:        statCh,
				TemplateScheduleStore: schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
				Auditor:               auditRecorder,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		tpl := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
			OrganizationID: user.OrganizationID,
			CreatedBy:      user.UserID,
		}).Do().Template

		template := coderdtest.UpdateTemplateMeta(t, client, tpl.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantMillis: inactiveTTL.Milliseconds(),
		})

		resp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
			TemplateID:     template.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Do()
		require.Equal(t, database.WorkspaceTransitionStart, resp.Build.Transition)
		workspace := resp.Workspace

		auditRecorder.ResetLogs()
		// Simulate being inactive.
		ticker <- workspace.LastUsedAt.Add(inactiveTTL * 2)
		stats := <-statCh

		// Expect workspace to transition to stopped state for breaching
		// failure TTL.
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, stats.Transitions[workspace.ID], database.WorkspaceTransitionStop)

		ws := coderdtest.MustWorkspace(t, client, workspace.ID)
		// Should be dormant now.
		require.NotNil(t, ws.DormantAt)
		// Should be transitioned to stop.
		require.Equal(t, codersdk.WorkspaceTransitionStop, ws.LatestBuild.Transition)
		require.Len(t, auditRecorder.AuditLogs(), 1)
		alog := auditRecorder.AuditLogs()[0]
		require.Equal(t, int32(http.StatusOK), alog.StatusCode)
		require.Equal(t, database.AuditActionWrite, alog.Action)
		require.Equal(t, workspace.Name, alog.ResourceTarget)

		ctx := testutil.Context(t, testutil.WaitMedium)

		dormantLastUsedAt := ws.LastUsedAt
		// nolint:gocritic // this test is not testing RBAC.
		err := client.UpdateWorkspaceDormancy(ctx, ws.ID, codersdk.UpdateWorkspaceDormancy{Dormant: false})
		require.NoError(t, err)

		// Assert that we updated our last_used_at so that we don't immediately
		// retrigger another lock action.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.True(t, ws.LastUsedAt.After(dormantLastUsedAt))
	})

	// This test serves as a regression prevention for generating
	// audit logs in the same transaction the transition workspaces to
	// the dormant state. The auditor that is passed to autobuild does
	// not use the transaction when inserting an audit log which can
	// cause a deadlock.
	t.Run("NoDeadlock", func(t *testing.T) {
		t.Parallel()

		if !dbtestutil.WillUsePostgres() {
			t.Skipf("Skipping non-postgres run")
		}

		var (
			ticker      = make(chan time.Time)
			statCh      = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
		)

		const (
			maxConns      = 3
			numWorkspaces = maxConns * 5
		)
		// This is a bit bizarre but necessary so that we can
		// initialize our coderd with a real auditor and limit DB connections
		// to simulate deadlock conditions.
		db, pubsub, sdb := dbtestutil.NewDBWithSQLDB(t)
		// Set MaxOpenConns so we can ensure we aren't inadvertently acquiring
		// another connection from within a transaction.
		sdb.SetMaxOpenConns(maxConns)
		auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true))
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
				Database:                 db,
				Pubsub:                   pubsub,
				Auditor:                  auditor,
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		workspaces := make([]codersdk.Workspace, 0, numWorkspaces)
		for i := 0; i < numWorkspaces; i++ {
			ws := coderdtest.CreateWorkspace(t, client, template.ID)
			build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
			require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
			workspaces = append(workspaces, ws)
		}

		// Simulate being inactive.
		ticker <- time.Now().Add(time.Hour)
		stats := <-statCh

		// Expect workspace to transition to stopped state for breaching
		// failure TTL.
		require.Len(t, stats.Transitions, numWorkspaces)
		for _, ws := range workspaces {
			// The workspace should be dormant.
			ws = coderdtest.MustWorkspace(t, client, ws.ID)
			require.NotNil(t, ws.DormantAt)
		}
	})

	t.Run("DormancyThresholdTooEarly", func(t *testing.T) {
		t.Parallel()

		var (
			ticker      = make(chan time.Time)
			statCh      = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		// Make it impossible to trigger the inactive ttl.
		ticker <- ws.LastUsedAt.Add(-inactiveTTL)
		stats := <-statCh
		// Expect no transitions since not enough time has elapsed.
		require.Len(t, stats.Transitions, 0)
	})

	// This is kind of a dumb test but it exists to offer some marginal
	// confidence that a bug in the auto-deletion logic doesn't delete running
	// workspaces.
	t.Run("ActiveWorkspacesNotDeleted", func(t *testing.T) {
		t.Parallel()

		var (
			ticker        = make(chan time.Time)
			statCh        = make(chan autobuild.Stats)
			autoDeleteTTL = time.Minute
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](autoDeleteTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Nil(t, ws.DormantAt)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		ticker <- ws.LastUsedAt.Add(autoDeleteTTL * 2)
		stats := <-statCh
		// Expect no transitions since workspace is active.
		require.Len(t, stats.Transitions, 0)
	})

	// Assert that a stopped workspace that breaches the inactivity threshold
	// does not trigger a build transition but is still placed in the
	// dormant state.
	t.Run("InactiveStoppedWorkspaceNoTransition", func(t *testing.T) {
		t.Parallel()

		var (
			ticker      = make(chan time.Time)
			statCh      = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ws := coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = nil
		})
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		// Stop the workspace so we can assert autobuild does nothing
		// if we breach our inactivity threshold.
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

		// Simulate not having accessed the workspace in a while.
		ticker <- ws.LastUsedAt.Add(2 * inactiveTTL)
		stats := <-statCh
		// Expect no transitions since workspace is stopped.
		require.Len(t, stats.Transitions, 0)
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		// The workspace should still be dormant even though we didn't
		// transition the workspace.
		require.NotNil(t, ws.DormantAt)
	})

	// Test the flow of a workspace transitioning from
	// inactive -> dormant -> deleted.
	t.Run("WorkspaceInactiveDeleteTransition", func(t *testing.T) {
		t.Parallel()

		var (
			ticker        = make(chan time.Time)
			statCh        = make(chan autobuild.Stats)
			transitionTTL = time.Minute
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](transitionTTL.Milliseconds())
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](transitionTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		// Simulate not having accessed the workspace in a while.
		ticker <- ws.LastUsedAt.Add(2 * transitionTTL)
		stats := <-statCh
		// Expect workspace to transition to stopped state for breaching
		// inactive TTL.
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionStop)

		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		// The workspace should be dormant.
		require.NotNil(t, ws.DormantAt)

		// Wait for the autobuilder to stop the workspace.
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		// Simulate the workspace being dormant beyond the threshold.
		ticker <- ws.DormantAt.Add(2 * transitionTTL)
		stats = <-statCh
		require.Len(t, stats.Transitions, 1)
		// The workspace should be scheduled for deletion.
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionDelete)

		// Wait for the workspace to be deleted.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		// Assert that the workspace is actually deleted.
		//nolint:gocritic // ensuring workspace is deleted and not just invisible to us due to RBAC
		_, err := client.Workspace(testutil.Context(t, testutil.WaitShort), ws.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusGone, cerr.StatusCode())
	})

	t.Run("DormantTTLTooEarly", func(t *testing.T) {
		t.Parallel()

		var (
			ticker     = make(chan time.Time)
			statCh     = make(chan autobuild.Stats)
			dormantTTL = time.Minute
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](dormantTTL.Milliseconds())
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, anotherClient, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, anotherClient, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := anotherClient.UpdateWorkspaceDormancy(ctx, ws.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.NotNil(t, ws.DormantAt)

		// Ensure we haven't breached our threshold.
		ticker <- ws.DormantAt.Add(-dormantTTL * 2)
		stats := <-statCh
		// Expect no transitions since not enough time has elapsed.
		require.Len(t, stats.Transitions, 0)

		_, err = anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantAutoDeleteMillis: dormantTTL.Milliseconds(),
		})
		require.NoError(t, err)

		// Simlute the workspace breaching the threshold.
		ticker <- ws.DormantAt.Add(dormantTTL * 2)
		stats = <-statCh
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, database.WorkspaceTransitionDelete, stats.Transitions[ws.ID])
	})

	// Assert that a dormant workspace does not autostart.
	t.Run("DormantNoAutostart", func(t *testing.T) {
		t.Parallel()

		var (
			tickCh      = make(chan time.Time)
			statsCh     = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		sched, err := cron.Weekly("CRON_TZ=UTC 0 * * * *")
		require.NoError(t, err)

		ws := coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

		// Assert that autostart works when the workspace isn't dormant..
		tickCh <- sched.Next(ws.LatestBuild.CreatedAt)
		stats := <-statsCh
		require.Len(t, stats.Errors, 0)
		require.Len(t, stats.Transitions, 1)
		require.Contains(t, stats.Transitions, ws.ID)
		require.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[ws.ID])

		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Now that we've validated that the workspace is eligible for autostart
		// lets cause it to become dormant.
		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantMillis: inactiveTTL.Milliseconds(),
		})
		require.NoError(t, err)

		// We should see the workspace get stopped now.
		tickCh <- ws.LastUsedAt.Add(inactiveTTL * 2)
		stats = <-statsCh
		require.Len(t, stats.Errors, 0)
		require.Len(t, stats.Transitions, 1)
		require.Contains(t, stats.Transitions, ws.ID)
		require.Equal(t, database.WorkspaceTransitionStop, stats.Transitions[ws.ID])

		// The workspace should be dormant now.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.NotNil(t, ws.DormantAt)

		// Assert that autostart is no longer triggered since workspace is dormant.
		tickCh <- sched.Next(ws.LatestBuild.CreatedAt)
		stats = <-statsCh
		require.Len(t, stats.Transitions, 0)
	})

	// Test that failing to auto-delete a workspace will only retry
	// once a day.
	t.Run("FailedDeleteRetryDaily", func(t *testing.T) {
		t.Parallel()

		var (
			ticker        = make(chan time.Time)
			statCh        = make(chan autobuild.Stats)
			transitionTTL = time.Minute
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		// Create a template version that passes to get a functioning workspace.
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ws := coderdtest.CreateWorkspace(t, templateAdmin, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, ws.LatestBuild.ID)

		// Create a new version that will fail when we try to delete a workspace.
		version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyFailed,
		}, func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Try to delete the workspace. This simulates a "failed" autodelete.
		build, err := templateAdmin.CreateWorkspaceBuild(ctx, ws.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition:        codersdk.WorkspaceTransitionDelete,
			TemplateVersionID: version.ID,
		})
		require.NoError(t, err)

		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
		require.NotEmpty(t, build.Job.Error)

		ctx = testutil.Context(t, testutil.WaitLong) // Reset the context to avoid timeouts.

		// Update our workspace to be dormant so that it qualifies for auto-deletion.
		err = templateAdmin.UpdateWorkspaceDormancy(ctx, ws.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		// Enable auto-deletion for the template.
		_, err = templateAdmin.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantAutoDeleteMillis: transitionTTL.Milliseconds(),
		})
		require.NoError(t, err)

		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.NotNil(t, ws.DeletingAt)

		// Simulate ticking an hour after the workspace is expected to be deleted.
		// Under normal circumstances this should result in a transition but
		// since our last build resulted in failure it should be skipped.
		ticker <- build.Job.CompletedAt.Add(time.Hour)
		stats := <-statCh
		require.Len(t, stats.Transitions, 0)

		// Simulate ticking a day after the workspace was last attempted to
		// be deleted. This should result in an attempt.
		ticker <- build.Job.CompletedAt.Add(time.Hour * 25)
		stats = <-statCh
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, database.WorkspaceTransitionDelete, stats.Transitions[ws.ID])
	})

	t.Run("RequireActiveVersion", func(t *testing.T) {
		t.Parallel()

		var (
			tickCh  = make(chan time.Time)
			statsCh = make(chan autobuild.Stats)
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAccessControl: 1},
			},
		})

		sched, err := cron.Weekly("CRON_TZ=UTC 0 * * * *")
		require.NoError(t, err)

		// Create a template version1 that passes to get a functioning workspace.
		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)
		require.Equal(t, version1.ID, template.ActiveVersionID)

		ws := coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

		// Create a new version so that we can assert we don't update
		// to the latest by default.
		version2 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Make sure to promote it.
		err = client.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version2.ID,
		})
		require.NoError(t, err)

		// Kick of an autostart build.
		tickCh <- sched.Next(ws.LatestBuild.CreatedAt)
		stats := <-statsCh
		require.Len(t, stats.Errors, 0)
		require.Len(t, stats.Transitions, 1)
		require.Contains(t, stats.Transitions, ws.ID)
		require.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[ws.ID])

		// Validate that we didn't update to the promoted version.
		started := coderdtest.MustWorkspace(t, client, ws.ID)
		firstBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, started.LatestBuild.ID)
		require.Equal(t, version1.ID, firstBuild.TemplateVersionID)

		ctx = testutil.Context(t, testutil.WaitMedium) // Reset the context after workspace operations.

		// Update the template to require the promoted version.
		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			RequireActiveVersion: true,
			AllowUserAutostart:   true,
		})
		require.NoError(t, err)

		// Reset the workspace to the stopped state so we can try
		// to autostart again.
		coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop, func(req *codersdk.CreateWorkspaceBuildRequest) {
			req.TemplateVersionID = ws.LatestBuild.TemplateVersionID
		})

		// Force an autostart transition again.
		tickCh <- sched.Next(firstBuild.CreatedAt)
		stats = <-statsCh
		require.Len(t, stats.Errors, 0)
		require.Len(t, stats.Transitions, 1)
		require.Contains(t, stats.Transitions, ws.ID)
		require.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[ws.ID])

		// Validate that we are using the promoted version.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.Equal(t, version2.ID, ws.LatestBuild.TemplateVersionID)
	})

	t.Run("NextStartAtIsValid", func(t *testing.T) {
		t.Parallel()

		var (
			tickCh  = make(chan time.Time)
			statsCh = make(chan autobuild.Stats)
			clock   = quartz.NewMock(t)
		)

		clock.Set(dbtime.Now())

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				Logger:                   &logger,
				Clock:                    clock,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, clock),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

		// First create a template that only supports Monday-Friday
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.AutostartRequirement = &codersdk.TemplateAutostartRequirement{DaysOfWeek: codersdk.BitmapToWeekdays(0b00011111)}
		})
		require.Equal(t, version1.ID, template.ActiveVersionID)

		// Then create a workspace with a schedule Sunday-Saturday
		sched, err := cron.Weekly("CRON_TZ=UTC 0 9 * * 0-6")
		require.NoError(t, err)
		ws := coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)
		next := ws.LatestBuild.CreatedAt

		// For each day of the week (Monday-Sunday)
		// We iterate through each day of the week to ensure the behavior of each
		// day of the week is as expected.
		for range 7 {
			next = sched.Next(next)

			clock.Set(next)
			tickCh <- next
			stats := <-statsCh
			ws = coderdtest.MustWorkspace(t, client, ws.ID)

			// Our cron schedule specifies Sunday-Saturday but the template only allows
			// Monday-Friday so we expect there to be no transitions on the weekend.
			if next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
				assert.Len(t, stats.Errors, 0)
				assert.Len(t, stats.Transitions, 0)

				ws = coderdtest.MustWorkspace(t, client, ws.ID)
			} else {
				assert.Len(t, stats.Errors, 0)
				assert.Len(t, stats.Transitions, 1)
				assert.Contains(t, stats.Transitions, ws.ID)
				assert.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[ws.ID])

				coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
				ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)
			}

			// Ensure that there is a valid next start at and that is is after
			// the previous start.
			require.NotNil(t, ws.NextStartAt)
			require.Greater(t, *ws.NextStartAt, next)

			// Our autostart requirement disallows sundays and saturdays so
			// the next start at should never land on these days.
			require.NotEqual(t, time.Saturday, ws.NextStartAt.Weekday())
			require.NotEqual(t, time.Sunday, ws.NextStartAt.Weekday())
		}
	})

	t.Run("NextStartAtIsUpdatedWhenTemplateAutostartRequirementsChange", func(t *testing.T) {
		t.Parallel()

		var (
			tickCh  = make(chan time.Time)
			statsCh = make(chan autobuild.Stats)
			clock   = quartz.NewMock(t)
		)

		// Set the clock to 8AM Monday, 1st January, 2024 to keep
		// this test deterministic.
		clock.Set(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil)
		templateScheduleStore.Clock = clock
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				Logger:                   &logger,
				Clock:                    clock,
				TemplateScheduleStore:    templateScheduleStore,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

		// First create a template that only supports Monday-Friday
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.AllowUserAutostart = ptr.Ref(true)
			ctr.AutostartRequirement = &codersdk.TemplateAutostartRequirement{DaysOfWeek: codersdk.BitmapToWeekdays(0b00011111)}
		})
		require.Equal(t, version1.ID, template.ActiveVersionID)

		// Then create a workspace with a schedule Monday-Friday
		sched, err := cron.Weekly("CRON_TZ=UTC 0 9 * * 1-5")
		require.NoError(t, err)
		ws := coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

		// Our next start at should be Monday
		require.NotNil(t, ws.NextStartAt)
		require.Equal(t, time.Monday, ws.NextStartAt.Weekday())

		// Now update the template to only allow Tuesday-Friday
		coderdtest.UpdateTemplateMeta(t, client, template.ID, codersdk.UpdateTemplateMeta{
			AutostartRequirement: &codersdk.TemplateAutostartRequirement{
				DaysOfWeek: codersdk.BitmapToWeekdays(0b00011110),
			},
		})

		// Verify that our next start at has been updated to Tuesday
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.NotNil(t, ws.NextStartAt)
		require.Equal(t, time.Tuesday, ws.NextStartAt.Weekday())
	})

	t.Run("NextStartAtIsNullifiedOnScheduleChange", func(t *testing.T) {
		t.Parallel()

		if !dbtestutil.WillUsePostgres() {
			t.Skip("this test uses triggers so does not work with dbmem.go")
		}

		var (
			tickCh  = make(chan time.Time)
			statsCh = make(chan autobuild.Stats)
		)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				Logger:                   &logger,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		// Create a template that allows autostart Monday-Sunday
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.AutostartRequirement = &codersdk.TemplateAutostartRequirement{DaysOfWeek: codersdk.AllDaysOfWeek}
		})
		require.Equal(t, version.ID, template.ActiveVersionID)

		// Create a workspace with a schedule Sunday-Saturday
		sched, err := cron.Weekly("CRON_TZ=UTC 0 9 * * 0-6")
		require.NoError(t, err)
		ws := coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

		// Check we have a 'NextStartAt'
		require.NotNil(t, ws.NextStartAt)

		// Create a new slightly different cron schedule that could
		// potentially make NextStartAt invalid.
		sched, err = cron.Weekly("CRON_TZ=UTC 0 9 * * 1-6")
		require.NoError(t, err)
		ctx := testutil.Context(t, testutil.WaitShort)

		// We want to test the database nullifies the NextStartAt so we
		// make a raw DB call here. We pass in NextStartAt here so we
		// can test the database will nullify it and not us.
		//nolint: gocritic // We need system context to modify this.
		err = db.UpdateWorkspaceAutostart(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAutostartParams{
			ID:                ws.ID,
			AutostartSchedule: sql.NullString{Valid: true, String: sched.String()},
			NextStartAt:       sql.NullTime{Valid: true, Time: *ws.NextStartAt},
		})
		require.NoError(t, err)

		ws = coderdtest.MustWorkspace(t, client, ws.ID)

		// Check 'NextStartAt' has been nullified
		require.Nil(t, ws.NextStartAt)

		// Now we let the lifecycle executor run. This should spot that the
		// NextStartAt is null and update it for us.
		next := dbtime.Now()
		tickCh <- next
		stats := <-statsCh
		assert.Len(t, stats.Errors, 0)
		assert.Len(t, stats.Transitions, 0)

		// Ensure NextStartAt has been set, and is the expected value
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.NotNil(t, ws.NextStartAt)
		require.Equal(t, sched.Next(next), ws.NextStartAt.UTC())
	})
}

func TestTemplateDoesNotAllowUserAutostop(t *testing.T) {
	t.Parallel()

	t.Run("TTLSetByTemplate", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		templateTTL := 24 * time.Hour.Milliseconds()
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DefaultTTLMillis = ptr.Ref(templateTTL)
			ctr.AllowUserAutostop = ptr.Ref(false)
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.TTLMillis = nil // ensure that no default TTL is set
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// TTL should be set by the template
		require.Equal(t, false, template.AllowUserAutostop)
		require.Equal(t, templateTTL, template.DefaultTTLMillis)
		require.Equal(t, templateTTL, *workspace.TTLMillis)

		// Change the template's default TTL and refetch the workspace
		templateTTL = 72 * time.Hour.Milliseconds()
		ctx := testutil.Context(t, testutil.WaitShort)
		template = coderdtest.UpdateTemplateMeta(t, client, template.ID, codersdk.UpdateTemplateMeta{
			DefaultTTLMillis: templateTTL,
		})
		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)

		// Ensure that the new value is reflected in the template and workspace
		require.Equal(t, templateTTL, template.DefaultTTLMillis)
		require.Equal(t, templateTTL, *workspace.TTLMillis)
	})
}

// TestWorkspaceTagsTerraform tests that a workspace can be created with tags.
// This is an end-to-end-style test, meaning that we actually run the
// real Terraform provisioner and validate that the workspace is created
// successfully. The workspace itself does not specify any resources, and
// this is fine.
// To improve speed, we pre-download the providers and set a custom Terraform
// config file so that we only reference those
// nolint:paralleltest // t.Setenv
func TestWorkspaceTagsTerraform(t *testing.T) {
	mainTfTemplate := `
		terraform {
			required_providers {
				coder = {
					source = "coder/coder"
				}
			}
		}
		provider "coder" {}
		data "coder_workspace" "me" {}
		data "coder_workspace_owner" "me" {}
		data "coder_parameter" "unrelated" {
			name    = "unrelated"
			type    = "list(string)"
			default = jsonencode(["a", "b"])
		}
		%s
	`
	tfCliConfigPath := downloadProviders(t, fmt.Sprintf(mainTfTemplate, ""))
	t.Setenv("TF_CLI_CONFIG_FILE", tfCliConfigPath)

	for _, tc := range []struct {
		name string
		// tags to apply to the external provisioner
		provisionerTags map[string]string
		// tags to apply to the create template version request
		createTemplateVersionRequestTags map[string]string
		// the coder_workspace_tags bit of main.tf.
		// you can add more stuff here if you need
		tfWorkspaceTags                  string
		templateImportUserVariableValues []codersdk.VariableValue
		// if we need to set parameters on workspace build
		workspaceBuildParameters []codersdk.WorkspaceBuildParameter
		skipCreateWorkspace      bool
	}{
		{
			name:            "no tags",
			tfWorkspaceTags: ``,
		},
		{
			name: "empty tags",
			tfWorkspaceTags: `
				data "coder_workspace_tags" "tags" {
					tags = {}
				}
			`,
		},
		{
			name:            "static tag",
			provisionerTags: map[string]string{"foo": "bar"},
			tfWorkspaceTags: `
				data "coder_workspace_tags" "tags" {
					tags = {
						"foo" = "bar"
					}
				}`,
		},
		{
			name:            "tag variable",
			provisionerTags: map[string]string{"foo": "bar"},
			tfWorkspaceTags: `
				variable "foo" {
					default = "bar"
				}
				data "coder_workspace_tags" "tags" {
					tags = {
						"foo" = var.foo
					}
				}`,
		},
		{
			name:            "tag param",
			provisionerTags: map[string]string{"foo": "bar"},
			tfWorkspaceTags: `
				data "coder_parameter" "foo" {
					name = "foo"
					type = "string"
					default = "bar"
				}
				data "coder_workspace_tags" "tags" {
					tags = {
						"foo" = data.coder_parameter.foo.value
					}
				}`,
		},
		{
			name:            "tag param with default from var",
			provisionerTags: map[string]string{"foo": "bar"},
			tfWorkspaceTags: `
				variable "foo" {
					type = string
					default = "bar"
				}
				data "coder_parameter" "foo" {
					name = "foo"
					type = "string"
					default = var.foo
				}
				data "coder_workspace_tags" "tags" {
					tags = {
						"foo" = data.coder_parameter.foo.value
					}
				}`,
		},
		{
			name:                             "override no tags",
			provisionerTags:                  map[string]string{"foo": "baz"},
			createTemplateVersionRequestTags: map[string]string{"foo": "baz"},
			tfWorkspaceTags:                  ``,
		},
		{
			name:                             "override empty tags",
			provisionerTags:                  map[string]string{"foo": "baz"},
			createTemplateVersionRequestTags: map[string]string{"foo": "baz"},
			tfWorkspaceTags: `
				data "coder_workspace_tags" "tags" {
					tags = {}
				}`,
		},
		{
			name:                             "overrides static tag from request",
			provisionerTags:                  map[string]string{"foo": "baz"},
			createTemplateVersionRequestTags: map[string]string{"foo": "baz"},
			tfWorkspaceTags: `
				data "coder_workspace_tags" "tags" {
					tags = {
						"foo" = "bar"
					}
				}`,
			// When we go to create the workspace, there won't be any provisioner
			// matching tag foo=bar.
			skipCreateWorkspace: true,
		},
		{
			name:                             "overrides with dynamic option from var",
			provisionerTags:                  map[string]string{"foo": "bar"},
			createTemplateVersionRequestTags: map[string]string{"foo": "bar"},
			templateImportUserVariableValues: []codersdk.VariableValue{{Name: "default_foo", Value: "baz"}, {Name: "foo", Value: "bar,baz"}},
			workspaceBuildParameters:         []codersdk.WorkspaceBuildParameter{{Name: "foo", Value: "bar"}},
			tfWorkspaceTags: `
				variable "default_foo" {
					type = string
				}
				variable "foo" {
					type = string
				}
				data "coder_parameter" "foo" {
					name = "foo"
					type = "string"
					default = var.default_foo
					mutable = false
					dynamic "option" {
						for_each = toset(split(",", var.foo))
						content {
							name  = option.value
							value = option.value
						}
					}
				}
				data "coder_workspace_tags" "tags" {
					tags = {
						"foo" = data.coder_parameter.foo.value
					}
				}`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			client, owner := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					// We intentionally do not run a built-in provisioner daemon here.
					IncludeProvisionerDaemon: false,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureExternalProvisionerDaemons: 1,
					},
				},
			})
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

			_ = coderdenttest.NewExternalProvisionerDaemonTerraform(t, client, owner.OrganizationID, tc.provisionerTags)

			// This can take a while, so set a relatively long timeout.
			ctx := testutil.Context(t, 2*testutil.WaitSuperLong)

			// Creating a template as a template admin must succeed
			templateFiles := map[string]string{"main.tf": fmt.Sprintf(mainTfTemplate, tc.tfWorkspaceTags)}
			tarBytes := testutil.CreateTar(t, templateFiles)
			fi, err := templateAdmin.Upload(ctx, "application/x-tar", bytes.NewReader(tarBytes))
			require.NoError(t, err, "failed to upload file")
			tv, err := templateAdmin.CreateTemplateVersion(ctx, owner.OrganizationID, codersdk.CreateTemplateVersionRequest{
				Name:               testutil.GetRandomName(t),
				FileID:             fi.ID,
				StorageMethod:      codersdk.ProvisionerStorageMethodFile,
				Provisioner:        codersdk.ProvisionerTypeTerraform,
				ProvisionerTags:    tc.createTemplateVersionRequestTags,
				UserVariableValues: tc.templateImportUserVariableValues,
			})
			require.NoError(t, err, "failed to create template version")
			coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, tv.ID)
			tpl := coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, tv.ID)

			if !tc.skipCreateWorkspace {
				// Creating a workspace as a non-privileged user must succeed
				ws, err := member.CreateUserWorkspace(ctx, memberUser.Username, codersdk.CreateWorkspaceRequest{
					TemplateID:          tpl.ID,
					Name:                coderdtest.RandomUsername(t),
					RichParameterValues: tc.workspaceBuildParameters,
				})
				require.NoError(t, err, "failed to create workspace")
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, member, ws.LatestBuild.ID)
			}
		})
	}
}

// downloadProviders is a test helper that creates a temporary file and writes a
// terraform CLI config file with a provider_installation stanza for coder/coder
// using dev_overrides. It also fetches the latest provider release from GitHub
// and extracts the binary to the temporary dir. It is the responsibility of the
// caller to set TF_CLI_CONFIG_FILE.
func downloadProviders(t *testing.T, providersTf string) string {
	t.Helper()
	// We firstly write a Terraform CLI config file to a temporary directory:
	var (
		tempDir         = t.TempDir()
		cacheDir        = filepath.Join(tempDir, ".cache")
		providersTfPath = filepath.Join(tempDir, "providers.tf")
		cliConfigPath   = filepath.Join(tempDir, "local.tfrc")
	)

	// Write files to disk
	require.NoError(t, os.MkdirAll(cacheDir, os.ModePerm|os.ModeDir))
	require.NoError(t, os.WriteFile(providersTfPath, []byte(providersTf), os.ModePerm)) // nolint:gosec
	cliConfigTemplate := `
	provider_installation {
		filesystem_mirror {
			path = %q
			include = ["*/*/*"]
		}
		direct {
			exclude = ["*/*/*"]
		}
	}`
	err := os.WriteFile(cliConfigPath, []byte(fmt.Sprintf(cliConfigTemplate, cacheDir)), os.ModePerm) // nolint:gosec
	require.NoError(t, err, "failed to write %s", cliConfigPath)

	ctx := testutil.Context(t, testutil.WaitLong)

	// Run terraform providers mirror to mirror required providers to cacheDir
	cmd := exec.CommandContext(ctx, "terraform", "providers", "mirror", cacheDir)
	cmd.Env = os.Environ() // without this terraform may complain about path
	cmd.Env = append(cmd.Env, "TF_CLI_CONFIG_FILE="+cliConfigPath)
	cmd.Dir = tempDir
	out, err := cmd.CombinedOutput()
	if !assert.NoError(t, err) {
		t.Log("failed to download providers:")
		t.Log(string(out))
		t.FailNow()
	}

	t.Logf("Set TF_CLI_CONFIG_FILE=%s", cliConfigPath)
	return cliConfigPath
}

// Blocked by autostart requirements
func TestExecutorAutostartBlocked(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var allowed []string
	for _, day := range agplschedule.DaysOfWeek {
		// Skip the day the workspace was created on and if the next day is within 2
		// hours, skip that too. The cron scheduler will start the workspace every hour,
		// so it can span into the next day.
		if day != now.UTC().Weekday() &&
			day != now.UTC().Add(time.Hour*2).Weekday() {
			allowed = append(allowed, day.String())
		}
	}

	var (
		sched   = must(cron.Weekly("CRON_TZ=UTC 0 * * * *"))
		tickCh  = make(chan time.Time)
		statsCh = make(chan autobuild.Stats)

		logger        = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, owner = coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		version  = coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		template = coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(request *codersdk.CreateTemplateRequest) {
			request.AutostartRequirement = &codersdk.TemplateAutostartRequirement{
				DaysOfWeek: allowed,
			}
		})
		_         = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace = coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutostartSchedule = ptr.Ref(sched.String())
		})
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	)

	// Given: workspace is stopped
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)

	// When: the autobuild executor ticks into the future
	go func() {
		tickCh <- workspace.LatestBuild.CreatedAt.Add(2 * time.Hour)
		close(tickCh)
	}()

	// Then: the workspace should not be started.
	stats := <-statsCh
	require.Len(t, stats.Errors, 0)
	require.Len(t, stats.Transitions, 0)
}

func TestWorkspacesFiltering(t *testing.T) {
	t.Parallel()

	t.Run("Dormant", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				TemplateScheduleStore: schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})
		templateAdminClient, templateAdmin := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

		resp := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      owner.UserID,
		}).Do()

		dormantWS1 := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        templateAdmin.ID,
			OrganizationID: owner.OrganizationID,
		}).Do().Workspace

		dormantWS2 := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        templateAdmin.ID,
			OrganizationID: owner.OrganizationID,
			TemplateID:     resp.Template.ID,
		}).Do().Workspace

		_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        templateAdmin.ID,
			OrganizationID: owner.OrganizationID,
			TemplateID:     resp.Template.ID,
		}).Do().Workspace

		ctx := testutil.Context(t, testutil.WaitMedium)

		err := templateAdminClient.UpdateWorkspaceDormancy(ctx, dormantWS1.ID, codersdk.UpdateWorkspaceDormancy{Dormant: true})
		require.NoError(t, err)

		err = templateAdminClient.UpdateWorkspaceDormancy(ctx, dormantWS2.ID, codersdk.UpdateWorkspaceDormancy{Dormant: true})
		require.NoError(t, err)

		workspaces, err := templateAdminClient.Workspaces(ctx, codersdk.WorkspaceFilter{
			FilterQuery: "dormant:true",
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 2)

		for _, ws := range workspaces.Workspaces {
			if ws.ID != dormantWS1.ID && ws.ID != dormantWS2.ID {
				t.Fatalf("Unexpected workspace %+v", ws)
			}
		}
	})
}

// TestWorkspacesWithoutTemplatePerms creates a workspace for a user, then drops
// the user's perms to the underlying template.
func TestWorkspacesWithoutTemplatePerms(t *testing.T) {
	t.Parallel()

	client, first := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		},
	})

	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)

	user, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
	workspace := coderdtest.CreateWorkspace(t, user, template.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Remove everyone access
	//nolint:gocritic // creating a separate user just for this is overkill
	err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
		GroupPerms: map[string]codersdk.TemplateRole{
			first.OrganizationID.String(): codersdk.TemplateRoleDeleted,
		},
	})
	require.NoError(t, err, "remove everyone access")

	// This should fail as the user cannot read the template
	_, err = user.Workspace(ctx, workspace.ID)
	require.Error(t, err, "fetch workspace")
	var sdkError *codersdk.Error
	require.ErrorAs(t, err, &sdkError)
	require.Equal(t, http.StatusForbidden, sdkError.StatusCode())

	_, err = user.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "fetch workspaces should not fail")

	// Now create another workspace the user can read.
	version2 := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)
	template2 := coderdtest.CreateTemplate(t, client, first.OrganizationID, version2.ID)
	_ = coderdtest.CreateWorkspace(t, user, template2.ID)

	workspaces, err := user.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "fetch workspaces should not fail")
	require.Len(t, workspaces.Workspaces, 1)
}

func TestWorkspaceLock(t *testing.T) {
	t.Parallel()

	t.Run("TemplateTimeTilDormantAutoDelete", func(t *testing.T) {
		t.Parallel()
		var (
			client, user = coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					IncludeProvisionerDaemon: true,
					TemplateScheduleStore:    &schedule.EnterpriseTemplateScheduleStore{Clock: quartz.NewReal()},
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureAdvancedTemplateScheduling: 1,
					},
				},
			})

			version    = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_          = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			dormantTTL = time.Minute
		)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](dormantTTL.Milliseconds())
		})

		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		lastUsedAt := workspace.LastUsedAt
		err := client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.NoError(t, err, "fetch provisioned workspace")
		require.NotNil(t, workspace.DeletingAt)
		require.NotNil(t, workspace.DormantAt)
		require.Equal(t, workspace.DormantAt.Add(dormantTTL), *workspace.DeletingAt)
		require.WithinRange(t, *workspace.DormantAt, time.Now().Add(-time.Second*10), time.Now())
		// Locking a workspace shouldn't update the last_used_at.
		require.Equal(t, lastUsedAt, workspace.LastUsedAt)

		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		lastUsedAt = workspace.LastUsedAt
		err = client.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: false,
		})
		require.NoError(t, err)

		workspace, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch provisioned workspace")
		require.Nil(t, workspace.DormantAt)
		// Unlocking a workspace should cause the deleting_at to be unset.
		require.Nil(t, workspace.DeletingAt)
		// The last_used_at should get updated when we unlock the workspace.
		require.True(t, workspace.LastUsedAt.After(lastUsedAt))
	})
}

func TestResolveAutostart(t *testing.T) {
	t.Parallel()

	ownerClient, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			TemplateScheduleStore: &schedule.EnterpriseTemplateScheduleStore{Clock: quartz.NewReal()},
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAccessControl: 1,
			},
		},
	})

	version1 := dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			CreatedBy:      owner.UserID,
			OrganizationID: owner.OrganizationID,
		}).Do()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	_, err := ownerClient.UpdateTemplateMeta(ctx, version1.Template.ID, codersdk.UpdateTemplateMeta{
		RequireActiveVersion: true,
	})
	require.NoError(t, err)

	client, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        member.ID,
		OrganizationID: owner.OrganizationID,
		TemplateID:     version1.Template.ID,
	}).Seed(database.WorkspaceBuild{
		TemplateVersionID: version1.TemplateVersion.ID,
	}).Do().Workspace

	_ = dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
		CreatedBy:      owner.UserID,
		OrganizationID: owner.OrganizationID,
		TemplateID:     version1.TemplateVersion.TemplateID,
	}).Params(database.TemplateVersionParameter{
		Name:     "param",
		Required: true,
	}).Do()

	// Autostart shouldn't be possible if parameters do not match.
	resp, err := client.ResolveAutostart(ctx, workspace.ID.String())
	require.NoError(t, err)
	require.True(t, resp.ParameterMismatch)
}

func TestAdminViewAllWorkspaces(t *testing.T) {
	t.Parallel()

	client, user := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations:      1,
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		},
	})

	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	//nolint:gocritic // intentionally using owner
	_, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)

	otherOrg, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name: "default-test",
	})
	require.NoError(t, err, "create other org")

	// This other user is not in the first user's org. Since other is an admin, they can
	// still see the "first" user's workspace.
	otherOwner, _ := coderdtest.CreateAnotherUser(t, client, otherOrg.ID, rbac.RoleOwner())
	otherWorkspaces, err := otherOwner.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(other) fetch workspaces")

	firstWorkspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(first) fetch workspaces")

	require.ElementsMatch(t, otherWorkspaces.Workspaces, firstWorkspaces.Workspaces)
	require.Equal(t, len(firstWorkspaces.Workspaces), 1, "should be 1 workspace present")

	memberView, _ := coderdtest.CreateAnotherUser(t, client, otherOrg.ID)
	memberViewWorkspaces, err := memberView.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err, "(member) fetch workspaces")
	require.Equal(t, 0, len(memberViewWorkspaces.Workspaces), "member in other org should see 0 workspaces")
}

func TestWorkspaceByOwnerAndName(t *testing.T) {
	t.Parallel()

	t.Run("Matching Provisioner", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, db, userResponse := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		userSubject, _, err := httpmw.UserRBACSubject(ctx, db, userResponse.UserID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		user, err := client.User(ctx, userSubject.ID)
		require.NoError(t, err)
		username := user.Username

		_ = coderdenttest.NewExternalProvisionerDaemon(t, client, userResponse.OrganizationID, map[string]string{
			provisionersdk.TagScope: provisionersdk.ScopeOrganization,
		})

		version := coderdtest.CreateTemplateVersion(t, client, userResponse.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, userResponse.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

		// Pending builds should show matching provisioners
		require.Equal(t, workspace.LatestBuild.Status, codersdk.WorkspaceStatusPending)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Count, 1)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Available, 1)

		// Completed builds should not show matching provisioners, because no provisioner daemon can
		// be eligible to process a job that is already completed.
		completedBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		require.Equal(t, completedBuild.Status, codersdk.WorkspaceStatusRunning)
		require.Equal(t, completedBuild.MatchedProvisioners.Count, 0)
		require.Equal(t, completedBuild.MatchedProvisioners.Available, 0)

		ws, err := client.WorkspaceByOwnerAndName(ctx, username, workspace.Name, codersdk.WorkspaceOptions{})
		require.NoError(t, err)

		// Verify the workspace details
		require.Equal(t, workspace.ID, ws.ID)
		require.Equal(t, workspace.Name, ws.Name)
		require.Equal(t, workspace.TemplateID, ws.TemplateID)
		require.Equal(t, completedBuild.Status, ws.LatestBuild.Status)
		require.Equal(t, ws.LatestBuild.MatchedProvisioners.Count, 0)
		require.Equal(t, ws.LatestBuild.MatchedProvisioners.Available, 0)

		// Verify that the provisioner daemon is registered in the database
		//nolint:gocritic // unit testing
		daemons, err := db.GetProvisionerDaemons(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)
		require.Equal(t, 1, len(daemons))
		require.Equal(t, provisionersdk.ScopeOrganization, daemons[0].Tags[provisionersdk.TagScope])
	})

	t.Run("No Matching Provisioner", func(t *testing.T) {
		t.Parallel()

		client, db, userResponse := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)

		userSubject, _, err := httpmw.UserRBACSubject(ctx, db, userResponse.UserID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		user, err := client.User(ctx, userSubject.ID)
		require.NoError(t, err)
		username := user.Username

		closer := coderdenttest.NewExternalProvisionerDaemon(t, client, userResponse.OrganizationID, map[string]string{
			provisionersdk.TagScope: provisionersdk.ScopeOrganization,
		})

		version := coderdtest.CreateTemplateVersion(t, client, userResponse.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, userResponse.OrganizationID, version.ID)

		ctx = testutil.Context(t, testutil.WaitLong) // Reset the context to avoid timeouts.

		// nolint:gocritic // unit testing
		daemons, err := db.GetProvisionerDaemons(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)
		require.Equal(t, len(daemons), 1)

		// Simulate a provisioner daemon failure:
		err = closer.Close()
		require.NoError(t, err)

		// Simulate it's subsequent deletion from the database:

		// nolint:gocritic // unit testing
		_, err = db.UpsertProvisionerDaemon(dbauthz.AsSystemRestricted(ctx), database.UpsertProvisionerDaemonParams{
			Name:           daemons[0].Name,
			OrganizationID: daemons[0].OrganizationID,
			Tags:           daemons[0].Tags,
			Provisioners:   daemons[0].Provisioners,
			Version:        daemons[0].Version,
			APIVersion:     daemons[0].APIVersion,
			KeyID:          daemons[0].KeyID,
			// Simulate the passing of time such that the provisioner daemon is considered stale
			// and will be deleted:
			CreatedAt: time.Now().Add(-time.Hour * 24 * 8),
			LastSeenAt: sql.NullTime{
				Time:  time.Now().Add(-time.Hour * 24 * 8),
				Valid: true,
			},
		})
		require.NoError(t, err)
		// nolint:gocritic // unit testing
		err = db.DeleteOldProvisionerDaemons(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)

		// Create a workspace that will not be able to provision due to a lack of provisioner daemons:
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

		require.Equal(t, workspace.LatestBuild.Status, codersdk.WorkspaceStatusPending)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Count, 0)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Available, 0)

		// nolint:gocritic // unit testing
		_, err = client.WorkspaceByOwnerAndName(dbauthz.As(ctx, userSubject), username, workspace.Name, codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.Status, codersdk.WorkspaceStatusPending)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Count, 0)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Available, 0)
	})

	t.Run("Unavailable Provisioner", func(t *testing.T) {
		t.Parallel()

		client, db, userResponse := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)

		userSubject, _, err := httpmw.UserRBACSubject(ctx, db, userResponse.UserID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		user, err := client.User(ctx, userSubject.ID)
		require.NoError(t, err)
		username := user.Username

		closer := coderdenttest.NewExternalProvisionerDaemon(t, client, userResponse.OrganizationID, map[string]string{
			provisionersdk.TagScope: provisionersdk.ScopeOrganization,
		})

		version := coderdtest.CreateTemplateVersion(t, client, userResponse.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, userResponse.OrganizationID, version.ID)

		ctx = testutil.Context(t, testutil.WaitLong) // Reset the context to avoid timeouts.

		// nolint:gocritic // unit testing
		daemons, err := db.GetProvisionerDaemons(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)
		require.Equal(t, len(daemons), 1)

		// Simulate a provisioner daemon failure:
		err = closer.Close()
		require.NoError(t, err)

		// nolint:gocritic // unit testing
		_, err = db.UpsertProvisionerDaemon(dbauthz.AsSystemRestricted(ctx), database.UpsertProvisionerDaemonParams{
			Name:           daemons[0].Name,
			OrganizationID: daemons[0].OrganizationID,
			Tags:           daemons[0].Tags,
			Provisioners:   daemons[0].Provisioners,
			Version:        daemons[0].Version,
			APIVersion:     daemons[0].APIVersion,
			KeyID:          daemons[0].KeyID,
			// Simulate the passing of time such that the provisioner daemon, though not stale, has been
			// has been inactive for a while:
			CreatedAt: time.Now().Add(-time.Hour * 24 * 2),
			LastSeenAt: sql.NullTime{
				Time:  time.Now().Add(-time.Hour * 24 * 2),
				Valid: true,
			},
		})
		require.NoError(t, err)

		// Create a workspace that will not be able to provision due to a lack of provisioner daemons:
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

		require.Equal(t, workspace.LatestBuild.Status, codersdk.WorkspaceStatusPending)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Count, 1)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Available, 0)

		// nolint:gocritic // unit testing
		_, err = client.WorkspaceByOwnerAndName(dbauthz.As(ctx, userSubject), username, workspace.Name, codersdk.WorkspaceOptions{})
		require.NoError(t, err)
		require.Equal(t, workspace.LatestBuild.Status, codersdk.WorkspaceStatusPending)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Count, 1)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Available, 0)
	})
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
