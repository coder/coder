package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/notifications"
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	agplschedule "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/enterprise/coderd/schedule"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
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
		//nolint:gocritic // using owner to setup roles
		r, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
			Name:           "creator",
			OrganizationID: first.OrganizationID.String(),
			DisplayName:    "Creator",
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace:          {codersdk.ActionCreate, codersdk.ActionWorkspaceStart, codersdk.ActionUpdate, codersdk.ActionRead},
				codersdk.ResourceOrganizationMember: {codersdk.ActionRead},
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

		template, _ := coderdtest.DynamicParameterTemplate(t, admin, first.OrganizationID, coderdtest.DynamicParameterTemplateParams{
			Zip: true,
		})

		ctx = testutil.Context(t, testutil.WaitLong)

		wrk, err := creator.CreateUserWorkspace(ctx, adminID.ID.String(), codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, admin, wrk.LatestBuild.ID)

		_, err = creator.WorkspaceByOwnerAndName(ctx, adminID.Username, wrk.Name, codersdk.WorkspaceOptions{
			IncludeDeleted: false,
		})
		require.NoError(t, err)
	})

	t.Run("ForANonOrgMember", func(t *testing.T) {
		t.Parallel()

		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // using owner to setup roles
		r, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
			Name:           "creator",
			OrganizationID: first.OrganizationID.String(),
			DisplayName:    "Creator",
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace:          {codersdk.ActionCreate, codersdk.ActionWorkspaceStart, codersdk.ActionUpdate, codersdk.ActionRead},
				codersdk.ResourceOrganizationMember: {codersdk.ActionRead},
			}),
		})
		require.NoError(t, err)

		// user to make the workspace for, **note** the user is not a member of the first org.
		// This is strange, but technically valid. The creator can create a workspace for
		// this user in this org, even though the user cannot access the workspace.
		secondOrg := coderdenttest.CreateOrganization(t, owner, coderdenttest.CreateOrganizationOptions{})
		_, forUser := coderdtest.CreateAnotherUser(t, owner, secondOrg.ID)

		// try the test action with this user & custom role
		creator, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleMember(),
			rbac.RoleTemplateAdmin(), // Need site wide access to make workspace for non-org
			rbac.RoleIdentifier{
				Name:           r.Name,
				OrganizationID: first.OrganizationID,
			},
		)

		template, _ := coderdtest.DynamicParameterTemplate(t, creator, first.OrganizationID, coderdtest.DynamicParameterTemplateParams{})

		ctx = testutil.Context(t, testutil.WaitLong)

		wrk, err := creator.CreateUserWorkspace(ctx, forUser.ID.String(), codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, creator, wrk.LatestBuild.ID)

		_, err = creator.WorkspaceByOwnerAndName(ctx, forUser.Username, wrk.Name, codersdk.WorkspaceOptions{
			IncludeDeleted: false,
		})
		require.NoError(t, err)
	})

	// Asserting some authz calls when creating a workspace.
	t.Run("AuthzStory", func(t *testing.T) {
		t.Parallel()
		owner, _, api, first := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong*2000)
		defer cancel()

		//nolint:gocritic // using owner to setup roles
		creatorRole, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
			Name:           "creator",
			OrganizationID: first.OrganizationID.String(),
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace:          {codersdk.ActionCreate, codersdk.ActionWorkspaceStart, codersdk.ActionUpdate, codersdk.ActionRead},
				codersdk.ResourceOrganizationMember: {codersdk.ActionRead},
			}),
		})
		require.NoError(t, err)

		version := coderdtest.CreateTemplateVersion(t, owner, first.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, owner, version.ID)
		template := coderdtest.CreateTemplate(t, owner, first.OrganizationID, version.ID)
		_, userID := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)
		creator, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleIdentifier{
			Name:           creatorRole.Name,
			OrganizationID: first.OrganizationID,
		})

		// Create a workspace with the current api using an org admin.
		authz := coderdtest.AssertRBAC(t, api.AGPL, creator)
		authz.Reset() // Reset all previous checks done in setup.
		_, err = creator.CreateUserWorkspace(ctx, userID.ID.String(), codersdk.CreateWorkspaceRequest{
			TemplateID: template.ID,
			Name:       "test-user",
		})
		require.NoError(t, err)

		// Assert all authz properties
		t.Run("OnlyOrganizationAuthzCalls", func(t *testing.T) {
			// Creating workspaces is an organization action. So organization
			// permissions should be sufficient to complete the action.
			for _, call := range authz.AllCalls() {
				if call.Action == policy.ActionRead &&
					call.Object.Equal(rbac.ResourceUser.WithOwner(userID.ID.String()).WithID(userID.ID)) {
					// User read checks are called. If they fail, ignore them.
					if call.Err != nil {
						continue
					}
				}

				if call.Object.Type == rbac.ResourceDeploymentConfig.Type {
					continue // Ignore
				}

				assert.Falsef(t, call.Object.OrgID == "",
					"call %q for object %q has no organization set. Site authz calls not expected here",
					call.Action, call.Object.String(),
				)
			}
		})
	})

	t.Run("NoTemplateAccess", func(t *testing.T) {
		// NoTemplateAccess intentionally does not use provisioners. The template
		// version will be stuck in 'pending' forever.
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

	t.Run("ClaimPrebuild", func(t *testing.T) {
		t.Parallel()

		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: coderdtest.DeploymentValues(t),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// GIVEN a template, template version, preset and a prebuilt workspace that uses them all
		presetID := uuid.New()
		tv := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
			OrganizationID: user.OrganizationID,
			CreatedBy:      user.UserID,
		}).Preset(database.TemplateVersionPreset{
			ID: presetID,
		}).Do()

		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:    database.PrebuildsSystemUserID,
			TemplateID: tv.Template.ID,
		}).Seed(database.WorkspaceBuild{
			TemplateVersionID: tv.TemplateVersion.ID,
			TemplateVersionPresetID: uuid.NullUUID{
				UUID:  presetID,
				Valid: true,
			},
		}).WithAgent(func(a []*proto.Agent) []*proto.Agent {
			return a
		}).Do()

		ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong))
		agent, err := db.GetWorkspaceAgentAndLatestBuildByAuthToken(ctx, uuid.MustParse(r.AgentToken))
		require.NoError(t, err)

		err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agent.WorkspaceAgent.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		})
		require.NoError(t, err)

		// WHEN a workspace is created that matches the available prebuilt workspace
		_, err = client.CreateUserWorkspace(ctx, user.UserID.String(), codersdk.CreateWorkspaceRequest{
			TemplateVersionID:       tv.TemplateVersion.ID,
			TemplateVersionPresetID: presetID,
			Name:                    "claimed-workspace",
		})
		require.NoError(t, err)

		// THEN a new build is scheduled with the build stage specified
		build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, r.Workspace.ID)
		require.NoError(t, err)
		require.NotEqual(t, build.ID, r.Build.ID)
		job, err := db.GetProvisionerJobByID(ctx, build.JobID)
		require.NoError(t, err)
		var metadata provisionerdserver.WorkspaceProvisionJob
		require.NoError(t, json.Unmarshal(job.Input, &metadata))
		require.Equal(t, metadata.PrebuiltWorkspaceBuildStage, proto.PrebuiltWorkspaceBuildStage_CLAIM)
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

		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		tickTime := build.Job.CompletedAt.Add(failureTTL * 2)

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
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

		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		tickTime := build.Job.CompletedAt.Add(-failureTTL * 2)

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
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
				AutobuildTicker:          ticker,
				AutobuildStats:           statCh,
				IncludeProvisionerDaemon: true,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
				Auditor:                  auditRecorder,
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
		tickTime := workspace.LastUsedAt.Add(inactiveTTL * 2)

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
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
		err = client.UpdateWorkspaceDormancy(ctx, ws.ID, codersdk.UpdateWorkspaceDormancy{Dormant: false})
		require.NoError(t, err)

		// Assert that we updated our last_used_at so that we don't immediately
		// retrigger another lock action.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.True(t, ws.LastUsedAt.After(dormantLastUsedAt))
	})

	// This test has been added to ensure we don't introduce a regression
	// to this issue https://github.com/coder/coder/issues/20711.
	t.Run("DormantAutostop", func(t *testing.T) {
		t.Parallel()

		var (
			ticker      = make(chan time.Time)
			statCh      = make(chan autobuild.Stats)
			inactiveTTL = time.Minute
			logger      = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		)

		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AutobuildTicker:          ticker,
				AutobuildStats:           statCh,
				IncludeProvisionerDaemon: true,
				TemplateScheduleStore:    schedule.NewEnterpriseTemplateScheduleStore(agplUserQuietHoursScheduleStore(), notifications.NewNoopEnqueuer(), logger, nil),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		// Create a template version that includes agents on both start AND stop builds.
		// This simulates a template without `count = data.coder_workspace.me.start_count`.
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](inactiveTTL.Milliseconds())
		})

		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)

		// Simulate the workspace becoming inactive and transitioning to dormant.
		tickTime := ws.LastUsedAt.Add(inactiveTTL * 2)

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
		stats := <-statCh

		// Expect workspace to transition to stopped state.
		require.Len(t, stats.Transitions, 1)
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionStop)

		// The autostop build should succeed even though the template includes
		// agents without `count = data.coder_workspace.me.start_count`.
		// This verifies that provisionerd has permission to create agents on
		// dormant workspaces during stop builds.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		require.NotNil(t, ws.DormantAt, "workspace should be marked as dormant")
		require.Equal(t, codersdk.WorkspaceTransitionStop, ws.LatestBuild.Transition)

		latestBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusStopped, latestBuild.Status)
	})

	// This test serves as a regression prevention for generating
	// audit logs in the same transaction the transition workspaces to
	// the dormant state. The auditor that is passed to autobuild does
	// not use the transaction when inserting an audit log which can
	// cause a deadlock.
	t.Run("NoDeadlock", func(t *testing.T) {
		t.Parallel()

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
		// Fix provisioner stale issue by updating LastSeenAt to the tick time
		tickTime := time.Now().Add(time.Hour)
		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspaces[0].OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
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
		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Simulate not having accessed the workspace in a while.
		tickTime := ws.LastUsedAt.Add(2 * inactiveTTL)
		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
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
		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		tickTime := ws.LastUsedAt.Add(2 * transitionTTL)
		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
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
		tickTime2 := ws.DormantAt.Add(2 * transitionTTL)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime2)
		ticker <- tickTime2
		stats = <-statCh
		require.Len(t, stats.Transitions, 1)
		// The workspace should be scheduled for deletion.
		require.Equal(t, stats.Transitions[ws.ID], database.WorkspaceTransitionDelete)

		// Wait for the workspace to be deleted.
		ws = coderdtest.MustWorkspace(t, client, ws.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		// Assert that the workspace is actually deleted.
		//nolint:gocritic // ensuring workspace is deleted and not just invisible to us due to RBAC
		_, err = client.Workspace(testutil.Context(t, testutil.WaitShort), ws.ID)
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
		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		tickTime := ws.DormantAt.Add(-dormantTTL * 2)
		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
		stats := <-statCh
		// Expect no transitions since not enough time has elapsed.
		require.Len(t, stats.Transitions, 0)

		_, err = anotherClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			TimeTilDormantAutoDeleteMillis: dormantTTL.Milliseconds(),
		})
		require.NoError(t, err)

		// Simlute the workspace breaching the threshold.
		tickTime2 := ws.DormantAt.Add(dormantTTL * 2)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime2)
		ticker <- tickTime2
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
		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Assert that autostart works when the workspace isn't dormant..
		tickTime := sched.Next(ws.LatestBuild.CreatedAt)
		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
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
		tickTime2 := ws.LastUsedAt.Add(inactiveTTL * 2)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime2)
		tickCh <- tickTime2
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
		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		tickTime := build.Job.CompletedAt.Add(time.Hour)
		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		ticker <- tickTime
		stats := <-statCh
		require.Len(t, stats.Transitions, 0)

		// Simulate ticking a day after the workspace was last attempted to
		// be deleted. This should result in an attempt.
		tickTime2 := build.Job.CompletedAt.Add(time.Hour * 25)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime2)
		ticker <- tickTime2
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
		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

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
		tickTime := sched.Next(ws.LatestBuild.CreatedAt)
		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)
		tickCh <- tickTime
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
		coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop, func(req *codersdk.CreateWorkspaceBuildRequest) {
			req.TemplateVersionID = ws.LatestBuild.TemplateVersionID
		})

		// Force an autostart transition again.
		tickTime2 := sched.Next(firstBuild.CreatedAt)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime2)
		tickCh <- tickTime2
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
		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
		next := ws.LatestBuild.CreatedAt

		// For each day of the week (Monday-Sunday)
		// We iterate through each day of the week to ensure the behavior of each
		// day of the week is as expected.
		for range 7 {
			next = sched.Next(next)

			clock.Set(next)
			p, err := coderdtest.GetProvisionerForTags(db, time.Now(), ws.OrganizationID, nil)
			require.NoError(t, err)
			coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, next)
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
				ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
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
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

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
		ws = coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

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

func TestPrebuildsAutobuild(t *testing.T) {
	t.Parallel()

	getRunningPrebuilds := func(
		t *testing.T,
		ctx context.Context,
		db database.Store,
		prebuildInstances int,
	) []database.GetRunningPrebuiltWorkspacesRow {
		t.Helper()

		var runningPrebuilds []database.GetRunningPrebuiltWorkspacesRow
		testutil.Eventually(ctx, t, func(context.Context) bool {
			rows, err := db.GetRunningPrebuiltWorkspaces(ctx)
			if err != nil {
				return false
			}

			for _, row := range rows {
				runningPrebuilds = append(runningPrebuilds, row)

				agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, row.ID)
				if err != nil {
					return false
				}

				for _, agent := range agents {
					err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
						ID:             agent.ID,
						LifecycleState: database.WorkspaceAgentLifecycleStateReady,
						StartedAt:      sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
						ReadyAt:        sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
					})
					if err != nil {
						return false
					}
				}
			}

			t.Logf("found %d running prebuilds so far, want %d", len(runningPrebuilds), prebuildInstances)
			return len(runningPrebuilds) == prebuildInstances
		}, testutil.IntervalSlow, "prebuilds not running")

		return runningPrebuilds
	}

	runReconciliationLoop := func(
		t *testing.T,
		ctx context.Context,
		db database.Store,
		reconciler *prebuilds.StoreReconciler,
		presets []codersdk.Preset,
	) {
		t.Helper()

		state, err := reconciler.SnapshotState(ctx, db)
		require.NoError(t, err)
		ps, err := state.FilterByPreset(presets[0].ID)
		require.NoError(t, err)
		require.NotNil(t, ps)
		actions, err := reconciler.CalculateActions(ctx, *ps)
		require.NoError(t, err)
		require.NotNil(t, actions)
		require.NoError(t, reconciler.ReconcilePreset(ctx, *ps))
	}

	claimPrebuild := func(
		t *testing.T,
		ctx context.Context,
		client *codersdk.Client,
		userClient *codersdk.Client,
		username string,
		version codersdk.TemplateVersion,
		presetID uuid.UUID,
		autostartSchedule ...string,
	) codersdk.Workspace {
		t.Helper()

		var startSchedule string
		if len(autostartSchedule) > 0 {
			startSchedule = autostartSchedule[0]
		}

		workspaceName := strings.ReplaceAll(testutil.GetRandomName(t), "_", "-")
		userWorkspace, err := userClient.CreateUserWorkspace(ctx, username, codersdk.CreateWorkspaceRequest{
			TemplateVersionID:       version.ID,
			Name:                    workspaceName,
			TemplateVersionPresetID: presetID,
			AutostartSchedule:       ptr.Ref(startSchedule),
		})
		require.NoError(t, err)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, userWorkspace.LatestBuild.ID)
		require.Equal(t, build.Job.Status, codersdk.ProvisionerJobSucceeded)
		workspace := coderdtest.MustWorkspace(t, client, userWorkspace.ID)
		assert.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)

		return workspace
	}

	// Prebuilt workspaces should not be autostopped based on the default TTL.
	// This test ensures that DefaultTTLMillis is ignored while the workspace is in a prebuild state.
	// Once the workspace is claimed, the default TTL should take effect.
	t.Run("DefaultTTLOnlyTriggersAfterClaim", func(t *testing.T) {
		t.Parallel()

		// Set the clock to Monday, January 1st, 2024 at 8:00 AM UTC to keep the test deterministic
		clock := quartz.NewMock(t)
		clock.Set(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))

		// Setup
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		logger := testutil.Logger(t)
		tickCh := make(chan time.Time)
		statsCh := make(chan autobuild.Stats)
		notificationsNoop := notifications.NewNoopEnqueuer()
		client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				Clock:                    clock,
				TemplateScheduleStore: schedule.NewEnterpriseTemplateScheduleStore(
					agplUserQuietHoursScheduleStore(),
					notificationsNoop,
					logger,
					clock,
				),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		// Setup Prebuild reconciler
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		reconciler := prebuilds.NewStoreReconciler(
			db, pb, cache,
			codersdk.PrebuildsConfig{},
			logger,
			clock,
			prometheus.NewRegistry(),
			notificationsNoop,
			api.AGPL.BuildUsageChecker,
		)
		var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(db)
		api.AGPL.PrebuildsClaimer.Store(&claimer)

		// Setup user, template and template version with a preset with 1 prebuild instance
		prebuildInstances := int32(1)
		ttlTime := 2 * time.Hour
		userClient, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(prebuildInstances))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		// Set a template level TTL to trigger the autostop
		// Template level TTL can only be set if autostop is disabled for users
		coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.AllowUserAutostop = ptr.Ref[bool](false)
			ctr.DefaultTTLMillis = ptr.Ref[int64](ttlTime.Milliseconds())
		})
		presets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, presets, 1)

		// Given: Reconciliation loop runs and starts prebuilt workspace
		runReconciliationLoop(t, ctx, db, reconciler, presets)
		runningPrebuilds := getRunningPrebuilds(t, ctx, db, int(prebuildInstances))
		require.Len(t, runningPrebuilds, int(prebuildInstances))

		// Given: a running prebuilt workspace, ready to be claimed
		prebuild := coderdtest.MustWorkspace(t, client, runningPrebuilds[0].ID)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		// Prebuilt workspaces should have an empty Deadline and MaxDeadline
		// which is equivalent to 0001-01-01 00:00:00 +0000
		require.Zero(t, prebuild.LatestBuild.Deadline)
		require.Zero(t, prebuild.LatestBuild.MaxDeadline)

		// When: the autobuild executor ticks *after* the TTL time (10:00 AM UTC)
		next := clock.Now().Add(ttlTime).Add(time.Minute)
		clock.Set(next) // 10:01 AM UTC
		go func() {
			tickCh <- next
		}()

		// Then: the prebuilt workspace should remain in a start transition
		prebuildStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, prebuildStats.Errors, 0)
		require.Len(t, prebuildStats.Transitions, 0)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		prebuild = coderdtest.MustWorkspace(t, client, prebuild.ID)
		require.Equal(t, codersdk.BuildReasonInitiator, prebuild.LatestBuild.Reason)
		require.Zero(t, prebuild.LatestBuild.Deadline)
		require.Zero(t, prebuild.LatestBuild.MaxDeadline)

		// Given: a user claims the prebuilt workspace sometime later
		clock.Set(clock.Now().Add(1 * time.Hour)) // 11:01 AM UTC
		workspace := claimPrebuild(t, ctx, client, userClient, user.Username, version, presets[0].ID)
		require.Equal(t, prebuild.ID, workspace.ID)
		// Workspace deadline must be ttlTime from the time it is claimed (1:01 PM UTC)
		require.True(t, workspace.LatestBuild.Deadline.Time.Equal(clock.Now().Add(ttlTime)))

		// When: the autobuild executor ticks *after* the TTL time (1:01 PM UTC)
		next = workspace.LatestBuild.Deadline.Time.Add(time.Minute)
		clock.Set(next) // 1:02 PM UTC
		go func() {
			tickCh <- next
			close(tickCh)
		}()

		// Then: the workspace should be stopped
		workspaceStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, workspaceStats.Errors, 0)
		require.Len(t, workspaceStats.Transitions, 1)
		require.Contains(t, workspaceStats.Transitions, workspace.ID)
		require.Equal(t, database.WorkspaceTransitionStop, workspaceStats.Transitions[workspace.ID])
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.Equal(t, codersdk.BuildReasonAutostop, workspace.LatestBuild.Reason)
	})

	// Prebuild workspaces should not follow the autostop schedule.
	// This test verifies that AutostopRequirement (autostop schedule) is ignored while the workspace is a prebuild.
	// After being claimed, the workspace should be stopped according to the autostop schedule.
	t.Run("AutostopScheduleOnlyTriggersAfterClaim", func(t *testing.T) {
		t.Parallel()

		// Set the clock to Monday, January 1st, 2024 at 8:00 AM UTC to keep the test deterministic
		clock := quartz.NewMock(t)
		clock.Set(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))

		// Setup
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		logger := testutil.Logger(t)
		tickCh := make(chan time.Time)
		statsCh := make(chan autobuild.Stats)
		notificationsNoop := notifications.NewNoopEnqueuer()
		client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				Clock:                    clock,
				TemplateScheduleStore: schedule.NewEnterpriseTemplateScheduleStore(
					agplUserQuietHoursScheduleStore(),
					notificationsNoop,
					logger,
					clock,
				),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		// Setup Prebuild reconciler
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		reconciler := prebuilds.NewStoreReconciler(
			db, pb, cache,
			codersdk.PrebuildsConfig{},
			logger,
			clock,
			prometheus.NewRegistry(),
			notificationsNoop,
			api.AGPL.BuildUsageChecker,
		)
		var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(db)
		api.AGPL.PrebuildsClaimer.Store(&claimer)

		// Setup user, template and template version with a preset with 1 prebuild instance
		prebuildInstances := int32(1)
		userClient, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(prebuildInstances))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		// Set a template level Autostop schedule to trigger the autostop daily
		coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.AutostopRequirement = ptr.Ref[codersdk.TemplateAutostopRequirement](
				codersdk.TemplateAutostopRequirement{
					DaysOfWeek: []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
					Weeks:      1,
				})
		})
		presets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, presets, 1)

		// Given: Reconciliation loop runs and starts prebuilt workspace
		runReconciliationLoop(t, ctx, db, reconciler, presets)
		runningPrebuilds := getRunningPrebuilds(t, ctx, db, int(prebuildInstances))
		require.Len(t, runningPrebuilds, int(prebuildInstances))

		// Given: a running prebuilt workspace, ready to be claimed
		prebuild := coderdtest.MustWorkspace(t, client, runningPrebuilds[0].ID)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		// Prebuilt workspaces should have an empty Deadline and MaxDeadline
		// which is equivalent to 0001-01-01 00:00:00 +0000
		require.Zero(t, prebuild.LatestBuild.Deadline)
		require.Zero(t, prebuild.LatestBuild.MaxDeadline)

		// When: the autobuild executor ticks *after* the deadline (2024-01-02 0:00 UTC)
		next := clock.Now().Truncate(24 * time.Hour).Add(24 * time.Hour).Add(time.Minute)
		clock.Set(next) // 2024-01-02 0:01 UTC
		go func() {
			tickCh <- next
		}()

		// Then: the prebuilt workspace should remain in a start transition
		prebuildStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, prebuildStats.Errors, 0)
		require.Len(t, prebuildStats.Transitions, 0)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		prebuild = coderdtest.MustWorkspace(t, client, prebuild.ID)
		require.Equal(t, codersdk.BuildReasonInitiator, prebuild.LatestBuild.Reason)
		require.Zero(t, prebuild.LatestBuild.Deadline)
		require.Zero(t, prebuild.LatestBuild.MaxDeadline)

		// Given: a user claims the prebuilt workspace
		workspace := claimPrebuild(t, ctx, client, userClient, user.Username, version, presets[0].ID)
		require.Equal(t, prebuild.ID, workspace.ID)
		// Then: the claimed workspace should respect the next valid scheduled deadline (2024-01-03 0:00 UTC)
		require.True(t, workspace.LatestBuild.Deadline.Time.Equal(clock.Now().Truncate(24*time.Hour).Add(24*time.Hour)))

		// When: the autobuild executor ticks *after* the deadline (2024-01-03 0:00 UTC)
		next = workspace.LatestBuild.Deadline.Time.Add(time.Minute)
		clock.Set(next) // 2024-01-03 0:01 UTC
		go func() {
			tickCh <- next
			close(tickCh)
		}()

		// Then: the workspace should be stopped
		workspaceStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, workspaceStats.Errors, 0)
		require.Len(t, workspaceStats.Transitions, 1)
		require.Contains(t, workspaceStats.Transitions, workspace.ID)
		require.Equal(t, database.WorkspaceTransitionStop, workspaceStats.Transitions[workspace.ID])
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.Equal(t, codersdk.BuildReasonAutostop, workspace.LatestBuild.Reason)
	})

	// Prebuild workspaces should not follow the autostart schedule.
	// This test verifies that AutostartRequirement (autostart schedule) is ignored while the workspace is a prebuild.
	// After being claimed, the workspace should be started according to the autostart schedule.
	t.Run("AutostartScheduleOnlyTriggersAfterClaim", func(t *testing.T) {
		t.Parallel()

		// Set the clock to dbtime.Now() to match the workspace build's CreatedAt
		clock := quartz.NewMock(t)
		clock.Set(dbtime.Now())

		// Setup
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		logger := testutil.Logger(t)
		tickCh := make(chan time.Time)
		statsCh := make(chan autobuild.Stats)
		notificationsNoop := notifications.NewNoopEnqueuer()
		client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				Clock:                    clock,
				TemplateScheduleStore: schedule.NewEnterpriseTemplateScheduleStore(
					agplUserQuietHoursScheduleStore(),
					notificationsNoop,
					logger,
					clock,
				),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		// Setup Prebuild reconciler
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		reconciler := prebuilds.NewStoreReconciler(
			db, pb, cache,
			codersdk.PrebuildsConfig{},
			logger,
			clock,
			prometheus.NewRegistry(),
			notificationsNoop,
			api.AGPL.BuildUsageChecker,
		)
		var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(db)
		api.AGPL.PrebuildsClaimer.Store(&claimer)

		// Setup user, template and template version with a preset with 1 prebuild instance
		prebuildInstances := int32(1)
		userClient, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(prebuildInstances))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		// Template-level autostart config only defines allowed days for workspaces to autostart
		// The actual autostart schedule is set at the workspace level
		sched, err := cron.Weekly("CRON_TZ=UTC 0 0 * * *")
		require.NoError(t, err)
		coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.AllowUserAutostart = ptr.Ref[bool](true)
			ctr.AutostartRequirement = &codersdk.TemplateAutostartRequirement{DaysOfWeek: codersdk.AllDaysOfWeek}
		})
		presets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, presets, 1)

		// Given: Reconciliation loop runs and starts prebuilt workspace
		runReconciliationLoop(t, ctx, db, reconciler, presets)
		runningPrebuilds := getRunningPrebuilds(t, ctx, db, int(prebuildInstances))
		require.Len(t, runningPrebuilds, int(prebuildInstances))

		// Given: a running prebuilt workspace
		prebuild := coderdtest.MustWorkspace(t, client, runningPrebuilds[0].ID)
		// Prebuilt workspaces should have an empty Autostart Schedule
		require.Nil(t, prebuild.AutostartSchedule)
		require.Nil(t, prebuild.NextStartAt)

		// Given: prebuilt workspace is stopped
		prebuild = coderdtest.MustTransitionWorkspace(t, client, prebuild.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, prebuild.LatestBuild.ID)

		// Tick at the next scheduled time after the prebuild’s LatestBuild.CreatedAt,
		// since the next allowed autostart is calculated starting from that point.
		// When: the autobuild executor ticks after the scheduled time
		go func() {
			tickCh <- sched.Next(prebuild.LatestBuild.CreatedAt).Add(time.Minute)
		}()

		// Then: the prebuilt workspace should remain in a stop transition
		prebuildStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, prebuildStats.Errors, 0)
		require.Len(t, prebuildStats.Transitions, 0)
		require.Equal(t, codersdk.WorkspaceTransitionStop, prebuild.LatestBuild.Transition)
		prebuild = coderdtest.MustWorkspace(t, client, prebuild.ID)
		require.Equal(t, codersdk.BuildReasonInitiator, prebuild.LatestBuild.Reason)
		require.Nil(t, prebuild.AutostartSchedule)
		require.Nil(t, prebuild.NextStartAt)

		// Given: a prebuilt workspace that is running and ready to be claimed
		prebuild = coderdtest.MustTransitionWorkspace(t, client, prebuild.ID, codersdk.WorkspaceTransitionStop, codersdk.WorkspaceTransitionStart)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, prebuild.LatestBuild.ID)
		// Make sure the workspace's agent is again ready
		getRunningPrebuilds(t, ctx, db, int(prebuildInstances))

		// Given: a user claims the prebuilt workspace with an Autostart schedule request
		workspace := claimPrebuild(t, ctx, client, userClient, user.Username, version, presets[0].ID, sched.String())
		require.Equal(t, prebuild.ID, workspace.ID)
		// Then: newly claimed workspace's AutostartSchedule and NextStartAt should be set
		require.NotNil(t, workspace.AutostartSchedule)
		require.NotNil(t, workspace.NextStartAt)

		// Given: workspace is stopped
		workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, sched.Next(prebuild.LatestBuild.CreatedAt))

		// Wait for provisioner to be available for this specific workspace
		coderdtest.MustWaitForProvisionersAvailable(t, db, prebuild, sched.Next(prebuild.LatestBuild.CreatedAt))

		tickTime := sched.Next(prebuild.LatestBuild.CreatedAt).Add(time.Minute)
		require.NoError(t, err)

		// Tick at the next scheduled time after the prebuild’s LatestBuild.CreatedAt,
		// since the next allowed autostart is calculated starting from that point.
		// When: the autobuild executor ticks after the scheduled time
		go func() {
			tickCh <- tickTime
		}()

		// Then: the workspace should have a NextStartAt equal to the next autostart schedule
		workspaceStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, workspaceStats.Errors, 0)
		require.Len(t, workspaceStats.Transitions, 1)
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.NotNil(t, workspace.AutostartSchedule)
		require.NotNil(t, workspace.NextStartAt)
		require.Equal(t, sched.Next(clock.Now()), workspace.NextStartAt.UTC())
	})

	// Prebuild workspaces should not transition to dormant or be deleted due to inactivity.
	// This test verifies that both TimeTilDormantMillis and TimeTilDormantAutoDeleteMillis
	// are ignored while the workspace is a prebuild. After the workspace is claimed,
	// it should respect these inactivity thresholds accordingly.
	t.Run("DormantOnlyAfterClaimed", func(t *testing.T) {
		t.Parallel()

		// Set the clock to Monday, January 1st, 2024 at 8:00 AM UTC to keep the test deterministic
		clock := quartz.NewMock(t)
		clock.Set(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))

		// Setup
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		logger := testutil.Logger(t)
		tickCh := make(chan time.Time)
		statsCh := make(chan autobuild.Stats)
		notificationsNoop := notifications.NewNoopEnqueuer()
		client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				Clock:                    clock,
				TemplateScheduleStore: schedule.NewEnterpriseTemplateScheduleStore(
					agplUserQuietHoursScheduleStore(),
					notificationsNoop,
					logger,
					clock,
				),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureAdvancedTemplateScheduling: 1},
			},
		})

		// Setup Prebuild reconciler
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		reconciler := prebuilds.NewStoreReconciler(
			db, pb, cache,
			codersdk.PrebuildsConfig{},
			logger,
			clock,
			prometheus.NewRegistry(),
			notificationsNoop,
			api.AGPL.BuildUsageChecker,
		)
		var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(db)
		api.AGPL.PrebuildsClaimer.Store(&claimer)

		// Setup user, template and template version with a preset with 1 prebuild instance
		prebuildInstances := int32(1)
		dormantTTL := 2 * time.Hour
		deletionTTL := 2 * time.Hour
		userClient, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(prebuildInstances))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		// Set a template level dormant TTL to trigger dormancy
		coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.TimeTilDormantMillis = ptr.Ref[int64](dormantTTL.Milliseconds())
			ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref[int64](deletionTTL.Milliseconds())
		})
		presets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, presets, 1)

		// Given: reconciliation loop runs and starts prebuilt workspace
		runReconciliationLoop(t, ctx, db, reconciler, presets)
		runningPrebuilds := getRunningPrebuilds(t, ctx, db, int(prebuildInstances))
		require.Len(t, runningPrebuilds, int(prebuildInstances))

		// Given: a running prebuilt workspace, ready to be claimed
		prebuild := coderdtest.MustWorkspace(t, client, runningPrebuilds[0].ID)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		require.Nil(t, prebuild.DormantAt)
		require.Nil(t, prebuild.DeletingAt)

		// When: the autobuild executor ticks *after* the dormant TTL (10:00 AM UTC)
		next := clock.Now().Add(dormantTTL).Add(time.Minute)
		clock.Set(next) // 10:01 AM UTC
		go func() {
			tickCh <- next
		}()

		// Then: the prebuilt workspace should remain in a start transition
		prebuildStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, prebuildStats.Errors, 0)
		require.Len(t, prebuildStats.Transitions, 0)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		prebuild = coderdtest.MustWorkspace(t, client, prebuild.ID)
		require.Equal(t, codersdk.BuildReasonInitiator, prebuild.LatestBuild.Reason)
		require.Nil(t, prebuild.DormantAt)
		require.Nil(t, prebuild.DeletingAt)

		// Given: a user claims the prebuilt workspace sometime later
		clock.Set(clock.Now().Add(1 * time.Hour)) // 11:01 AM UTC
		workspace := claimPrebuild(t, ctx, client, userClient, user.Username, version, presets[0].ID)
		require.Equal(t, prebuild.ID, workspace.ID)
		// Then: the claimed workspace should have DormantAt and DeletingAt unset (nil),
		// and LastUsedAt updated
		require.Nil(t, workspace.DormantAt)
		require.Nil(t, workspace.DeletingAt)
		require.True(t, workspace.LastUsedAt.After(prebuild.LastUsedAt))

		// When: the autobuild executor ticks *after* the dormant TTL (1:01 PM UTC)
		next = clock.Now().Add(dormantTTL).Add(time.Minute)
		clock.Set(next) // 1:02 PM UTC
		go func() {
			tickCh <- next
		}()

		// Then: the workspace should transition to stopped state for breaching dormant TTL
		workspaceStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, workspaceStats.Errors, 0)
		require.Len(t, workspaceStats.Transitions, 1)
		require.Contains(t, workspaceStats.Transitions, workspace.ID)
		require.Equal(t, database.WorkspaceTransitionStop, workspaceStats.Transitions[workspace.ID])
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
		require.Equal(t, codersdk.BuildReasonDormancy, workspace.LatestBuild.Reason)
		require.Equal(t, codersdk.WorkspaceStatusStopped, workspace.LatestBuild.Status)
		require.NotNil(t, workspace.DormantAt)
		require.NotNil(t, workspace.DeletingAt)

		tickTime := workspace.DeletingAt.Add(time.Minute)
		p, err := coderdtest.GetProvisionerForTags(db, time.Now(), workspace.OrganizationID, nil)
		require.NoError(t, err)
		coderdtest.UpdateProvisionerLastSeenAt(t, db, p.ID, tickTime)

		// When: the autobuild executor ticks *after* the deletion TTL
		go func() {
			tickCh <- tickTime
		}()

		// Then: the workspace should be deleted
		dormantWorkspaceStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, dormantWorkspaceStats.Errors, 0)
		require.Len(t, dormantWorkspaceStats.Transitions, 1)
		require.Contains(t, dormantWorkspaceStats.Transitions, workspace.ID)
		require.Equal(t, database.WorkspaceTransitionDelete, dormantWorkspaceStats.Transitions[workspace.ID])
	})

	// Prebuild workspaces should not be deleted when the failure TTL is reached.
	// This test verifies that FailureTTLMillis is ignored while the workspace is a prebuild.
	t.Run("FailureTTLOnlyAfterClaimed", func(t *testing.T) {
		t.Parallel()

		// Set the clock to Monday, January 1st, 2024 at 8:00 AM UTC to keep the test deterministic
		clock := quartz.NewMock(t)
		clock.Set(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))

		// Setup
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		logger := testutil.Logger(t)
		tickCh := make(chan time.Time)
		statsCh := make(chan autobuild.Stats)
		notificationsNoop := notifications.NewNoopEnqueuer()
		client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:                 db,
				Pubsub:                   pb,
				AutobuildTicker:          tickCh,
				IncludeProvisionerDaemon: true,
				AutobuildStats:           statsCh,
				Clock:                    clock,
				TemplateScheduleStore: schedule.NewEnterpriseTemplateScheduleStore(
					agplUserQuietHoursScheduleStore(),
					notificationsNoop,
					logger,
					clock,
				),
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})

		// Setup Prebuild reconciler
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		reconciler := prebuilds.NewStoreReconciler(
			db, pb, cache,
			codersdk.PrebuildsConfig{},
			logger,
			clock,
			prometheus.NewRegistry(),
			notificationsNoop,
			api.AGPL.BuildUsageChecker,
		)
		var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(db)
		api.AGPL.PrebuildsClaimer.Store(&claimer)

		// Setup user, template and template version with a preset with 1 prebuild instance
		prebuildInstances := int32(1)
		failureTTL := 2 * time.Hour
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithFailedResponseAndPresetsWithPrebuilds(prebuildInstances))
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		// Set a template level Failure TTL to trigger workspace deletion
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.FailureTTLMillis = ptr.Ref[int64](failureTTL.Milliseconds())
		})
		presets, err := client.TemplateVersionPresets(ctx, version.ID)
		require.NoError(t, err)
		require.Len(t, presets, 1)

		// Given: reconciliation loop runs and starts prebuilt workspace in failed state
		runReconciliationLoop(t, ctx, db, reconciler, presets)
		var failedWorkspaceBuilds []database.GetFailedWorkspaceBuildsByTemplateIDRow
		require.Eventually(t, func() bool {
			rows, err := db.GetFailedWorkspaceBuildsByTemplateID(ctx, database.GetFailedWorkspaceBuildsByTemplateIDParams{
				TemplateID: template.ID,
			})
			if err != nil {
				return false
			}

			failedWorkspaceBuilds = append(failedWorkspaceBuilds, rows...)

			t.Logf("found %d failed prebuilds so far, want %d", len(failedWorkspaceBuilds), prebuildInstances)
			return len(failedWorkspaceBuilds) == int(prebuildInstances)
		}, testutil.WaitSuperLong, testutil.IntervalSlow)
		require.Len(t, failedWorkspaceBuilds, int(prebuildInstances))

		// Given: a failed prebuilt workspace
		prebuild := coderdtest.MustWorkspace(t, client, failedWorkspaceBuilds[0].WorkspaceID)
		require.Equal(t, codersdk.WorkspaceStatusFailed, prebuild.LatestBuild.Status)

		// When: the autobuild executor ticks *after* the failure TTL
		go func() {
			tickCh <- prebuild.LatestBuild.Job.CompletedAt.Add(failureTTL * 2)
		}()

		// Then: the prebuilt workspace should remain in a start transition
		prebuildStats := testutil.RequireReceive(ctx, t, statsCh)
		require.Len(t, prebuildStats.Errors, 0)
		require.Len(t, prebuildStats.Transitions, 0)
		require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
		prebuild = coderdtest.MustWorkspace(t, client, prebuild.ID)
		require.Equal(t, codersdk.BuildReasonInitiator, prebuild.LatestBuild.Reason)
	})
}

func templateWithAgentAndPresetsWithPrebuilds(desiredInstances int32) *echo.Responses {
	agent := &proto.Agent{
		Name:            "smith",
		OperatingSystem: "linux",
		Architecture:    "i386",
	}

	resource := func(withAgent bool) *proto.Resource {
		r := &proto.Resource{Type: "compute", Name: "main"}
		if withAgent {
			r.Agents = []*proto.Agent{agent}
		}
		return r
	}

	applyResponse := func(withAgent bool) *proto.Response {
		return &proto.Response{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{resource(withAgent)},
				},
			},
		}
	}

	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{{
			Type: &proto.Response_Plan{
				Plan: &proto.PlanComplete{
					Presets: []*proto.Preset{{
						Name:       "preset-test",
						Parameters: []*proto.PresetParameter{{Name: "k1", Value: "v1"}},
						Prebuild:   &proto.Prebuild{Instances: desiredInstances},
					}},
				},
			},
		}},
		ProvisionApplyMap: map[proto.WorkspaceTransition][]*proto.Response{
			proto.WorkspaceTransition_START: {applyResponse(true)},
			proto.WorkspaceTransition_STOP:  {applyResponse(false)},
		},
	}
}

func templateWithFailedResponseAndPresetsWithPrebuilds(desiredInstances int32) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Presets: []*proto.Preset{
							{
								Name: "preset-test",
								Parameters: []*proto.PresetParameter{
									{
										Name:  "k1",
										Value: "v1",
									},
								},
								Prebuild: &proto.Prebuild{
									Instances: desiredInstances,
								},
							},
						},
					},
				},
			},
		},
		ProvisionApply: echo.ApplyFailed,
	}
}

func TestPrebuildUpdateLifecycleParams(t *testing.T) {
	t.Parallel()

	// Autostart schedule configuration set to weekly at 9:30 AM UTC
	autostartSchedule, err := cron.Weekly("CRON_TZ=UTC 30 9 * * 1-5")
	require.NoError(t, err)

	// TTL configuration set to 8 hours
	ttlMillis := ptr.Ref((8 * time.Hour).Milliseconds())

	// Deadline configuration set to January 1st, 2024 at 10:00 AM UTC
	deadline := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		endpoint     func(*testing.T, context.Context, *codersdk.Client, uuid.UUID) error
		apiErrorMsg  string
		assertUpdate func(*testing.T, *quartz.Mock, *codersdk.Client, uuid.UUID)
	}{
		{
			name: "AutostartUpdatePrebuildAfterClaim",
			endpoint: func(t *testing.T, ctx context.Context, client *codersdk.Client, workspaceID uuid.UUID) error {
				err = client.UpdateWorkspaceAutostart(ctx, workspaceID, codersdk.UpdateWorkspaceAutostartRequest{
					Schedule: ptr.Ref(autostartSchedule.String()),
				})
				return err
			},
			apiErrorMsg: "Autostart is not supported for prebuilt workspaces",
			assertUpdate: func(t *testing.T, clock *quartz.Mock, client *codersdk.Client, workspaceID uuid.UUID) {
				// The workspace's autostart schedule should be updated to the given schedule,
				// and its next start time should be set to 2024-01-01 09:30 AM UTC
				updatedWorkspace := coderdtest.MustWorkspace(t, client, workspaceID)
				require.Equal(t, autostartSchedule.String(), *updatedWorkspace.AutostartSchedule)
				require.Equal(t, autostartSchedule.Next(clock.Now()), updatedWorkspace.NextStartAt.UTC())
				expectedNext := time.Date(2024, 1, 1, 9, 30, 0, 0, time.UTC)
				require.Equal(t, expectedNext, updatedWorkspace.NextStartAt.UTC())
			},
		},
		{
			name: "TTLUpdatePrebuildAfterClaim",
			endpoint: func(t *testing.T, ctx context.Context, client *codersdk.Client, workspaceID uuid.UUID) error {
				err := client.UpdateWorkspaceTTL(ctx, workspaceID, codersdk.UpdateWorkspaceTTLRequest{
					TTLMillis: ttlMillis,
				})
				return err
			},
			apiErrorMsg: "TTL updates are not supported for prebuilt workspaces",
			assertUpdate: func(t *testing.T, clock *quartz.Mock, client *codersdk.Client, workspaceID uuid.UUID) {
				// The workspace's TTL should be updated accordingly
				updatedWorkspace := coderdtest.MustWorkspace(t, client, workspaceID)
				require.Equal(t, ttlMillis, updatedWorkspace.TTLMillis)
			},
		},
		{
			name: "DormantUpdatePrebuildAfterClaim",
			endpoint: func(t *testing.T, ctx context.Context, client *codersdk.Client, workspaceID uuid.UUID) error {
				err := client.UpdateWorkspaceDormancy(ctx, workspaceID, codersdk.UpdateWorkspaceDormancy{
					Dormant: true,
				})
				return err
			},
			apiErrorMsg: "Dormancy updates are not supported for prebuilt workspaces",
			assertUpdate: func(t *testing.T, clock *quartz.Mock, client *codersdk.Client, workspaceID uuid.UUID) {
				// The workspace's dormantAt should be updated accordingly
				updatedWorkspace := coderdtest.MustWorkspace(t, client, workspaceID)
				require.Equal(t, clock.Now(), updatedWorkspace.DormantAt.UTC())
			},
		},
		{
			name: "DeadlineUpdatePrebuildAfterClaim",
			endpoint: func(t *testing.T, ctx context.Context, client *codersdk.Client, workspaceID uuid.UUID) error {
				err := client.PutExtendWorkspace(ctx, workspaceID, codersdk.PutExtendWorkspaceRequest{
					Deadline: deadline,
				})
				return err
			},
			apiErrorMsg: "Deadline extension is not supported for prebuilt workspaces",
			assertUpdate: func(t *testing.T, clock *quartz.Mock, client *codersdk.Client, workspaceID uuid.UUID) {
				// The workspace build's deadline should be updated accordingly
				updatedWorkspace := coderdtest.MustWorkspace(t, client, workspaceID)
				require.Equal(t, deadline, updatedWorkspace.LatestBuild.Deadline.Time.UTC())
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Set the clock to Monday, January 1st, 2024 at 8:00 AM UTC to keep the test deterministic
			clock := quartz.NewMock(t)
			clock.Set(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))

			// Setup
			client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					IncludeProvisionerDaemon: true,
					Clock:                    clock,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureWorkspacePrebuilds: 1,
					},
				},
			})

			// Given: a template and a template version with preset and a prebuilt workspace
			presetID := uuid.New()
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
			dbgen.Preset(t, db, database.InsertPresetParams{
				ID:                presetID,
				TemplateVersionID: version.ID,
				DesiredInstances:  sql.NullInt32{Int32: 1, Valid: true},
			})
			workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:    database.PrebuildsSystemUserID,
				TemplateID: template.ID,
			}).Seed(database.WorkspaceBuild{
				TemplateVersionID: version.ID,
				TemplateVersionPresetID: uuid.NullUUID{
					UUID:  presetID,
					Valid: true,
				},
			}).WithAgent(func(agent []*proto.Agent) []*proto.Agent {
				return agent
			}).Do()

			// Mark the prebuilt workspace's agent as ready so the prebuild can be claimed
			ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong))
			agent, err := db.GetWorkspaceAgentAndLatestBuildByAuthToken(ctx, uuid.MustParse(workspaceBuild.AgentToken))
			require.NoError(t, err)
			err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
				ID:             agent.WorkspaceAgent.ID,
				LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			})
			require.NoError(t, err)

			// Given: a prebuilt workspace
			prebuild := coderdtest.MustWorkspace(t, client, workspaceBuild.Workspace.ID)

			// When: the lifecycle-update endpoint is called for the prebuilt workspace
			err = tc.endpoint(t, ctx, client, prebuild.ID)

			// Then: a 409 Conflict should be returned, with an error message specific to the lifecycle parameter
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusConflict, apiErr.StatusCode())
			require.Equal(t, tc.apiErrorMsg, apiErr.Response.Message)

			// Given: the prebuilt workspace is claimed by a user
			user, err := client.User(ctx, "testUser")
			require.NoError(t, err)
			claimedWorkspace, err := client.CreateUserWorkspace(ctx, user.ID.String(), codersdk.CreateWorkspaceRequest{
				TemplateVersionID:       version.ID,
				TemplateVersionPresetID: presetID,
				Name:                    coderdtest.RandomUsername(t),
				// The 'extend' endpoint requires the workspace to have an existing deadline.
				// To ensure this, we set the workspace's TTL to 1 hour.
				TTLMillis: ptr.Ref[int64](time.Hour.Milliseconds()),
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, claimedWorkspace.LatestBuild.ID)
			workspace := coderdtest.MustWorkspace(t, client, claimedWorkspace.ID)
			require.Equal(t, prebuild.ID, workspace.ID)

			// When: the same lifecycle-update endpoint is called for the claimed workspace
			err = tc.endpoint(t, ctx, client, workspace.ID)
			require.NoError(t, err)

			// Then: the workspace's lifecycle parameter should be updated accordingly
			tc.assertUpdate(t, clock, client, claimedWorkspace.ID)
		})
	}
}

func TestPrebuildActivityBump(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	clock.Set(dbtime.Now())

	// Setup
	log := testutil.Logger(t)
	client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			Clock:                    clock,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspacePrebuilds: 1,
			},
		},
	})

	// Given: a template and a template version with preset and a prebuilt workspace
	presetID := uuid.New()
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	// Configure activity bump on the template
	activityBump := time.Hour
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
		ctr.ActivityBumpMillis = ptr.Ref[int64](activityBump.Milliseconds())
	})
	dbgen.Preset(t, db, database.InsertPresetParams{
		ID:                presetID,
		TemplateVersionID: version.ID,
		DesiredInstances:  sql.NullInt32{Int32: 1, Valid: true},
	})
	// Given: a prebuild with an expired Deadline
	deadline := clock.Now().Add(-30 * time.Minute)
	wb := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:    database.PrebuildsSystemUserID,
		TemplateID: template.ID,
	}).Seed(database.WorkspaceBuild{
		TemplateVersionID: version.ID,
		TemplateVersionPresetID: uuid.NullUUID{
			UUID:  presetID,
			Valid: true,
		},
		Deadline: deadline,
	}).WithAgent(func(agent []*proto.Agent) []*proto.Agent {
		return agent
	}).Do()

	// Mark the prebuilt workspace's agent as ready so the prebuild can be claimed
	// nolint:gocritic
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong))
	agent, err := db.GetWorkspaceAgentAndLatestBuildByAuthToken(ctx, uuid.MustParse(wb.AgentToken))
	require.NoError(t, err)
	err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID:             agent.WorkspaceAgent.ID,
		LifecycleState: database.WorkspaceAgentLifecycleStateReady,
	})
	require.NoError(t, err)

	// Given: a prebuilt workspace with a Deadline and an empty MaxDeadline
	prebuild := coderdtest.MustWorkspace(t, client, wb.Workspace.ID)
	require.Equal(t, deadline.UTC(), prebuild.LatestBuild.Deadline.Time.UTC())
	require.Zero(t, prebuild.LatestBuild.MaxDeadline)

	// When: activity bump is applied to an unclaimed prebuild
	workspacestats.ActivityBumpWorkspace(ctx, log, db, prebuild.ID, clock.Now().Add(10*time.Hour))

	// Then: prebuild Deadline/MaxDeadline remain unchanged
	prebuild = coderdtest.MustWorkspace(t, client, wb.Workspace.ID)
	require.Equal(t, deadline.UTC(), prebuild.LatestBuild.Deadline.Time.UTC())
	require.Zero(t, prebuild.LatestBuild.MaxDeadline)

	// Given: the prebuilt workspace is claimed by a user
	user, err := client.User(ctx, "testUser")
	require.NoError(t, err)
	claimedWorkspace, err := client.CreateUserWorkspace(ctx, user.ID.String(), codersdk.CreateWorkspaceRequest{
		TemplateVersionID:       version.ID,
		TemplateVersionPresetID: presetID,
		Name:                    coderdtest.RandomUsername(t),
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, claimedWorkspace.LatestBuild.ID)
	workspace := coderdtest.MustWorkspace(t, client, claimedWorkspace.ID)
	require.Equal(t, prebuild.ID, workspace.ID)
	// Claimed workspaces have an empty Deadline and MaxDeadline
	require.Zero(t, workspace.LatestBuild.Deadline)
	require.Zero(t, workspace.LatestBuild.MaxDeadline)

	// Given: the claimed workspace has an expired Deadline
	err = db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
		ID:        workspace.LatestBuild.ID,
		Deadline:  deadline,
		UpdatedAt: clock.Now(),
	})
	require.NoError(t, err)
	workspace = coderdtest.MustWorkspace(t, client, claimedWorkspace.ID)

	// When: activity bump is applied to a claimed prebuild
	workspacestats.ActivityBumpWorkspace(ctx, log, db, workspace.ID, clock.Now().Add(10*time.Hour))

	// Then: Deadline is extended by the activity bump, MaxDeadline remains unset
	workspace = coderdtest.MustWorkspace(t, client, claimedWorkspace.ID)
	require.WithinDuration(t, clock.Now().Add(activityBump).UTC(), workspace.LatestBuild.Deadline.Time.UTC(), testutil.WaitMedium)
	require.Zero(t, workspace.LatestBuild.MaxDeadline)
}

func TestWorkspaceProvisionerdServerMetrics(t *testing.T) {
	t.Parallel()

	// Setup
	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	db, pb := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	logger := testutil.Logger(t)
	reg := prometheus.NewRegistry()
	provisionerdserverMetrics := provisionerdserver.NewMetrics(logger)
	err := provisionerdserverMetrics.Register(reg)
	require.NoError(t, err)
	client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			Database:                  db,
			Pubsub:                    pb,
			IncludeProvisionerDaemon:  true,
			Clock:                     clock,
			ProvisionerdServerMetrics: provisionerdserverMetrics,
		},
	})

	// Setup Prebuild reconciler
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(
		db, pb, cache,
		codersdk.PrebuildsConfig{},
		logger,
		clock,
		prometheus.NewRegistry(),
		notifications.NewNoopEnqueuer(),
		api.AGPL.BuildUsageChecker,
	)
	var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(db)
	api.AGPL.PrebuildsClaimer.Store(&claimer)

	organizationName, err := client.Organization(ctx, owner.OrganizationID)
	require.NoError(t, err)
	userClient, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

	// Setup template and template version with a preset with 1 prebuild instance
	versionPrebuild := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(1))
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, versionPrebuild.ID)
	templatePrebuild := coderdtest.CreateTemplate(t, client, owner.OrganizationID, versionPrebuild.ID)
	presetsPrebuild, err := client.TemplateVersionPresets(ctx, versionPrebuild.ID)
	require.NoError(t, err)
	require.Len(t, presetsPrebuild, 1)

	// Setup template and template version with a preset without prebuild instances
	versionNoPrebuild := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(0))
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, versionNoPrebuild.ID)
	templateNoPrebuild := coderdtest.CreateTemplate(t, client, owner.OrganizationID, versionNoPrebuild.ID)
	presetsNoPrebuild, err := client.TemplateVersionPresets(ctx, versionNoPrebuild.ID)
	require.NoError(t, err)
	require.Len(t, presetsNoPrebuild, 1)

	// Given: no histogram value for prebuilt workspaces creation
	prebuildCreationMetric := promhelp.MetricValue(t, reg, "coderd_workspace_creation_duration_seconds", prometheus.Labels{
		"organization_name": organizationName.Name,
		"template_name":     templatePrebuild.Name,
		"preset_name":       presetsPrebuild[0].Name,
		"type":              "prebuild",
	})
	require.Nil(t, prebuildCreationMetric)

	// Given: reconciliation loop runs and starts prebuilt workspace
	coderdenttest.MustRunReconciliationLoopForPreset(ctx, t, db, reconciler, presetsPrebuild[0])
	runningPrebuilds := coderdenttest.GetRunningPrebuilds(ctx, t, db, 1)
	require.Len(t, runningPrebuilds, 1)

	// Then: the histogram value for prebuilt workspace creation should be updated
	prebuildCreationHistogram := promhelp.HistogramValue(t, reg, "coderd_workspace_creation_duration_seconds", prometheus.Labels{
		"organization_name": organizationName.Name,
		"template_name":     templatePrebuild.Name,
		"preset_name":       presetsPrebuild[0].Name,
		"type":              "prebuild",
	})
	require.NotNil(t, prebuildCreationHistogram)
	require.Equal(t, uint64(1), prebuildCreationHistogram.GetSampleCount())

	// Given: a running prebuilt workspace, ready to be claimed
	prebuild := coderdtest.MustWorkspace(t, client, runningPrebuilds[0].ID)
	require.Equal(t, codersdk.WorkspaceTransitionStart, prebuild.LatestBuild.Transition)
	require.Nil(t, prebuild.DormantAt)
	require.Nil(t, prebuild.DeletingAt)

	// Given: no histogram value for prebuilt workspaces claim
	prebuildClaimMetric := promhelp.MetricValue(t, reg, "coderd_prebuilt_workspace_claim_duration_seconds", prometheus.Labels{
		"organization_name": organizationName.Name,
		"template_name":     templatePrebuild.Name,
		"preset_name":       presetsPrebuild[0].Name,
	})
	require.Nil(t, prebuildClaimMetric)

	// Given: the prebuilt workspace is claimed by a user
	workspace := coderdenttest.MustClaimPrebuild(ctx, t, client, userClient, user.Username, versionPrebuild, presetsPrebuild[0].ID)
	require.Equal(t, prebuild.ID, workspace.ID)

	// Then: the histogram value for prebuilt workspace claim should be updated
	prebuildClaimHistogram := promhelp.HistogramValue(t, reg, "coderd_prebuilt_workspace_claim_duration_seconds", prometheus.Labels{
		"organization_name": organizationName.Name,
		"template_name":     templatePrebuild.Name,
		"preset_name":       presetsPrebuild[0].Name,
	})
	require.NotNil(t, prebuildClaimHistogram)
	require.Equal(t, uint64(1), prebuildClaimHistogram.GetSampleCount())

	// Given: no histogram value for regular workspaces creation
	regularWorkspaceHistogramMetric := promhelp.MetricValue(t, reg, "coderd_workspace_creation_duration_seconds", prometheus.Labels{
		"organization_name": organizationName.Name,
		"template_name":     templateNoPrebuild.Name,
		"preset_name":       presetsNoPrebuild[0].Name,
		"type":              "regular",
	})
	require.Nil(t, regularWorkspaceHistogramMetric)

	// Given: a user creates a regular workspace (without prebuild pool)
	regularWorkspace, err := client.CreateUserWorkspace(ctx, user.ID.String(), codersdk.CreateWorkspaceRequest{
		TemplateVersionID:       versionNoPrebuild.ID,
		TemplateVersionPresetID: presetsNoPrebuild[0].ID,
		Name:                    coderdtest.RandomUsername(t),
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, regularWorkspace.LatestBuild.ID)

	// Then: the histogram value for regular workspace creation should be updated
	regularWorkspaceHistogram := promhelp.HistogramValue(t, reg, "coderd_workspace_creation_duration_seconds", prometheus.Labels{
		"organization_name": organizationName.Name,
		"template_name":     templateNoPrebuild.Name,
		"preset_name":       presetsNoPrebuild[0].Name,
		"type":              "regular",
	})
	require.NotNil(t, regularWorkspaceHistogram)
	require.Equal(t, uint64(1), regularWorkspaceHistogram.GetSampleCount())
}

// TestWorkspaceTemplateParamsChange tests a workspace with a parameter that
// validation changes on apply. The params used in create workspace are invalid
// according to the static params on import.
//
// This is testing that dynamic params defers input validation to terraform.
// It does not try to do this in coder/coder.
func TestWorkspaceTemplateParamsChange(t *testing.T) {
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

		data "coder_parameter" "param_min" {
			name = "param_min"
			type = "number"
			default = 10
		}

		data "coder_parameter" "param" {
			name    = "param"
			type    = "number"
			default = 12
			validation {
				min = data.coder_parameter.param_min.value
			}
		}
	`
	tfCliConfigPath := downloadProviders(t, mainTfTemplate)
	t.Setenv("TF_CLI_CONFIG_FILE", tfCliConfigPath)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false})
	dv := coderdtest.DeploymentValues(t)

	client, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			Logger: &logger,
			// We intentionally do not run a built-in provisioner daemon here.
			IncludeProvisionerDaemon: false,
			DeploymentValues:         dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		},
	})
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
	member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	_ = coderdenttest.NewExternalProvisionerDaemonTerraform(t, client, owner.OrganizationID, nil)

	// This can take a while, so set a relatively long timeout.
	ctx := testutil.Context(t, 2*testutil.WaitSuperLong)

	// Creating a template as a template admin must succeed
	templateFiles := map[string]string{"main.tf": mainTfTemplate}
	tarBytes := testutil.CreateTar(t, templateFiles)
	fi, err := templateAdmin.Upload(ctx, "application/x-tar", bytes.NewReader(tarBytes))
	require.NoError(t, err, "failed to upload file")

	tv, err := templateAdmin.CreateTemplateVersion(ctx, owner.OrganizationID, codersdk.CreateTemplateVersionRequest{
		Name:               testutil.GetRandomName(t),
		FileID:             fi.ID,
		StorageMethod:      codersdk.ProvisionerStorageMethodFile,
		Provisioner:        codersdk.ProvisionerTypeTerraform,
		UserVariableValues: []codersdk.VariableValue{},
	})
	require.NoError(t, err, "failed to create template version")
	coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, tv.ID)
	tpl := coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, tv.ID)

	// Set to dynamic params
	tpl, err = client.UpdateTemplateMeta(ctx, tpl.ID, codersdk.UpdateTemplateMeta{
		UseClassicParameterFlow: ptr.Ref(false),
	})
	require.NoError(t, err, "failed to update template meta")
	require.False(t, tpl.UseClassicParameterFlow, "template to use dynamic parameters")

	// When: we create a workspace build using the above template but with
	// parameter values that are different from those defined in the template.
	// The new values are not valid according to the original plan, but are valid.
	ws, err := member.CreateUserWorkspace(ctx, memberUser.Username, codersdk.CreateWorkspaceRequest{
		TemplateID: tpl.ID,
		Name:       coderdtest.RandomUsername(t),
		RichParameterValues: []codersdk.WorkspaceBuildParameter{
			{
				Name:  "param_min",
				Value: "5",
			},
			{
				Name:  "param",
				Value: "7",
			},
		},
	})

	// Then: the build should succeed. The updated value of param_min should be
	// used to validate param instead of the value defined in the temp
	require.NoError(t, err, "failed to create workspace")
	createBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, member, ws.LatestBuild.ID)
	require.Equal(t, createBuild.Status, codersdk.WorkspaceStatusRunning)

	// Now delete the workspace
	build, err := member.CreateWorkspaceBuild(ctx, ws.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionDelete,
	})
	require.NoError(t, err)
	build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, member, build.ID)
	require.Equal(t, codersdk.WorkspaceStatusDeleted, build.Status)
}

type testWorkspaceTagsTerraformCase struct {
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
	coderProviderTemplate := `
		terraform {
			required_providers {
				coder = {
					source = "coder/coder"
				}
			}
		}
	`
	tfCliConfigPath := downloadProviders(t, coderProviderTemplate)
	t.Setenv("TF_CLI_CONFIG_FILE", tfCliConfigPath)

	for _, tc := range []testWorkspaceTagsTerraformCase{
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
		t.Run(tc.name, func(t *testing.T) {
			t.Run("dynamic", func(t *testing.T) {
				workspaceTagsTerraform(t, tc, true)
			})

			// classic uses tfparse for tags. This sub test can be
			// removed when tf parse is removed.
			t.Run("classic", func(t *testing.T) {
				workspaceTagsTerraform(t, tc, false)
			})
		})
	}
}

func workspaceTagsTerraform(t *testing.T, tc testWorkspaceTagsTerraformCase, dynamic bool) {
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

	// This can take a while, so set a relatively long timeout.
	ctx := testutil.Context(t, 2*testutil.WaitSuperLong)

	emptyTar := testutil.CreateTar(t, map[string]string{"main.tf": ""})
	emptyFi, err := templateAdmin.Upload(ctx, "application/x-tar", bytes.NewReader(emptyTar))
	require.NoError(t, err)

	// This template version does not need to succeed in being created.
	// It will be in pending forever. We just need it to create a template.
	emptyTv, err := templateAdmin.CreateTemplateVersion(ctx, owner.OrganizationID, codersdk.CreateTemplateVersionRequest{
		Name:          testutil.GetRandomName(t),
		FileID:        emptyFi.ID,
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		Provisioner:   codersdk.ProvisionerTypeTerraform,
	})
	require.NoError(t, err)

	tpl := coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, emptyTv.ID, func(request *codersdk.CreateTemplateRequest) {
		request.UseClassicParameterFlow = ptr.Ref(!dynamic)
	})

	// The provisioner for the next template version
	_ = coderdenttest.NewExternalProvisionerDaemonTerraform(t, client, owner.OrganizationID, tc.provisionerTags)

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
		TemplateID:         tpl.ID,
	})
	require.NoError(t, err, "failed to create template version")
	coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, tv.ID)

	err = templateAdmin.UpdateActiveTemplateVersion(ctx, tpl.ID, codersdk.UpdateActiveTemplateVersion{
		ID: tv.ID,
	})
	require.NoError(t, err, "set to active template version")

	if !tc.skipCreateWorkspace {
		// Creating a workspace as a non-privileged user must succeed
		ws, err := member.CreateUserWorkspace(ctx, memberUser.Username, codersdk.CreateWorkspaceRequest{
			TemplateID:          tpl.ID,
			Name:                coderdtest.RandomUsername(t),
			RichParameterValues: tc.workspaceBuildParameters,
		})
		require.NoError(t, err, "failed to create workspace")
		tagJSON, _ := json.Marshal(ws.LatestBuild.Job.Tags)
		t.Logf("Created workspace build [%s] with tags: %s", ws.LatestBuild.Job.Type, tagJSON)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, member, ws.LatestBuild.ID)
	}
}

// downloadProviders is a test helper that caches Terraform providers and returns
// the path to a Terraform CLI config file that uses the cached providers.
// This uses the shared testutil caching infrastructure to avoid re-downloading
// providers on every test run. It is the responsibility of the caller to set
// TF_CLI_CONFIG_FILE.
// On Windows, provider caching is not supported and an empty string is returned.
func downloadProviders(t *testing.T, providersTf string) string {
	t.Helper()

	cacheRootDir := filepath.Join(testutil.PersistentCacheDir(t), "terraform_workspace_tags_test")
	templateFiles := map[string]string{"providers.tf": providersTf}
	testName := "TestWorkspaceTagsTerraform"

	cliConfigPath := testutil.CacheTFProviders(t, cacheRootDir, testName, templateFiles)
	if cliConfigPath != "" {
		t.Logf("Set TF_CLI_CONFIG_FILE=%s", cliConfigPath)
	}
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
	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

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

	t.Run("SharedWithGroup", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			_, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			sharedWorkspace   = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			ctx = testutil.Context(t, testutil.WaitMedium)
		)

		group, err := client.CreateGroup(ctx, orgOwner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "wibble",
		})
		require.NoError(t, err, "create group")

		client.UpdateWorkspaceACL(ctx, sharedWorkspace.ID, codersdk.UpdateWorkspaceACL{
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Shared: ptr.Ref(true),
		})
		require.NoError(t, err, "fetch workspaces")
		require.Equal(t, 1, workspaces.Count, "expected only one workspace")
		require.Equal(t, workspaces.Workspaces[0].ID, sharedWorkspace.ID)
	})

	t.Run("SharedWithUserAndGroup", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			_, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			sharedWorkspace   = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_, toShareWithUser = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
			ctx                = testutil.Context(t, testutil.WaitMedium)
		)

		group, err := client.CreateGroup(ctx, orgOwner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "wibble",
		})
		require.NoError(t, err, "create group")

		client.UpdateWorkspaceACL(ctx, sharedWorkspace.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				toShareWithUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Shared: ptr.Ref(true),
		})
		require.NoError(t, err, "fetch workspaces")
		require.Equal(t, 1, workspaces.Count, "expected only one workspace")
		require.Equal(t, workspaces.Workspaces[0].ID, sharedWorkspace.ID)
	})

	t.Run("NotSharedWithGroup", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			_, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			sharedWorkspace   = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			notSharedWorkspace = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			ctx = testutil.Context(t, testutil.WaitMedium)
		)

		group, err := client.CreateGroup(ctx, orgOwner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "wibble",
		})
		require.NoError(t, err, "create group")

		client.UpdateWorkspaceACL(ctx, sharedWorkspace.ID, codersdk.UpdateWorkspaceACL{
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Shared: ptr.Ref(false),
		})
		require.NoError(t, err, "fetch workspaces")
		require.Equal(t, 1, workspaces.Count, "expected only one workspace")
		require.Equal(t, workspaces.Workspaces[0].ID, notSharedWorkspace.ID)
	})

	t.Run("SharedWithGroupByID", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			_, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			sharedWorkspace   = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			ctx = testutil.Context(t, testutil.WaitMedium)
		)

		group, err := client.CreateGroup(ctx, orgOwner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "wibble",
		})
		require.NoError(t, err, "create group")
		err = client.UpdateWorkspaceACL(ctx, sharedWorkspace.ID, codersdk.UpdateWorkspaceACL{
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			SharedWithGroup: group.ID.String(),
		})
		require.NoError(t, err)
		require.Equal(t, 1, workspaces.Count)
		require.Equal(t, sharedWorkspace.ID, workspaces.Workspaces[0].ID)
	})

	t.Run("SharedWithGroupFilter", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			_, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			sharedWorkspace   = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			ctx = testutil.Context(t, testutil.WaitMedium)
		)

		group, err := client.CreateGroup(ctx, orgOwner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "wibble",
		})
		require.NoError(t, err, "create group")
		err = client.UpdateWorkspaceACL(ctx, sharedWorkspace.ID, codersdk.UpdateWorkspaceACL{
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)

		workspacesByID, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			SharedWithGroup: group.ID.String(),
		})
		require.NoError(t, err)
		require.Equal(t, 1, workspacesByID.Count)
		require.Equal(t, sharedWorkspace.ID, workspacesByID.Workspaces[0].ID)

		workspacesByName, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			SharedWithGroup: group.Name,
		})
		require.NoError(t, err)
		require.Equal(t, 1, workspacesByName.Count)
		require.Equal(t, sharedWorkspace.ID, workspacesByName.Workspaces[0].ID)

		workspacesByOrgAndName, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			SharedWithGroup: fmt.Sprintf("coder/%s", group.Name),
		})
		require.NoError(t, err)
		require.Equal(t, 1, workspacesByOrgAndName.Count)
		require.Equal(t, sharedWorkspace.ID, workspacesByOrgAndName.Workspaces[0].ID)
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
		require.WithinRange(t, *workspace.DormantAt, time.Now().Add(-time.Second), time.Now())
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

		daemons, err := db.GetProvisionerDaemons(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)
		require.Equal(t, len(daemons), 1)

		// Simulate a provisioner daemon failure:
		err = closer.Close()
		require.NoError(t, err)

		// Simulate it's subsequent deletion from the database:
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
		err = db.DeleteOldProvisionerDaemons(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)

		// Create a workspace that will not be able to provision due to a lack of provisioner daemons:
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)

		require.Equal(t, workspace.LatestBuild.Status, codersdk.WorkspaceStatusPending)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Count, 0)
		require.Equal(t, workspace.LatestBuild.MatchedProvisioners.Available, 0)

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

		daemons, err := db.GetProvisionerDaemons(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)
		require.Equal(t, len(daemons), 1)

		// Simulate a provisioner daemon failure:
		err = closer.Close()
		require.NoError(t, err)

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

func TestUpdateWorkspaceACL(t *testing.T) {
	t.Parallel()

	t.Run("OKWithGroup", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}
		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				DeploymentValues:         dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		orgID := adminUser.OrganizationID
		client, _ := coderdtest.CreateAnotherUser(t, adminClient, orgID)
		_, friend := coderdtest.CreateAnotherUser(t, adminClient, orgID)
		group := coderdtest.CreateGroup(t, adminClient, orgID, "bloob")

		tv := coderdtest.CreateTemplateVersion(t, adminClient, orgID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, tv.ID)
		template := coderdtest.CreateTemplate(t, adminClient, orgID, tv.ID)

		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := client.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				friend.ID.String(): codersdk.WorkspaceRoleUse,
			},
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleAdmin,
			},
		})
		require.NoError(t, err)

		workspaceACL, err := client.WorkspaceACL(ctx, ws.ID)
		require.NoError(t, err)
		require.Len(t, workspaceACL.Users, 1)
		require.Equal(t, workspaceACL.Users[0].ID, friend.ID)
		require.Equal(t, workspaceACL.Users[0].Role, codersdk.WorkspaceRoleUse)
		require.Len(t, workspaceACL.Groups, 1)
		require.Equal(t, workspaceACL.Groups[0].ID, group.ID)
		require.Equal(t, workspaceACL.Groups[0].Role, codersdk.WorkspaceRoleAdmin)
	})

	t.Run("UnknownIDs", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}
		adminClient := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			DeploymentValues:         dv,
		})
		adminUser := coderdtest.CreateFirstUser(t, adminClient)
		orgID := adminUser.OrganizationID
		client, _ := coderdtest.CreateAnotherUser(t, adminClient, orgID)

		tv := coderdtest.CreateTemplateVersion(t, adminClient, orgID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, tv.ID)
		template := coderdtest.CreateTemplate(t, adminClient, orgID, tv.ID)

		ws := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := client.UpdateWorkspaceACL(ctx, ws.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				uuid.NewString(): codersdk.WorkspaceRoleAdmin,
			},
			GroupRoles: map[string]codersdk.WorkspaceRole{
				uuid.NewString(): codersdk.WorkspaceRoleAdmin,
			},
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Len(t, cerr.Validations, 2)
		require.Equal(t, cerr.Validations[0].Field, "group_roles")
		require.Equal(t, cerr.Validations[1].Field, "user_roles")
	})
}

func TestDeleteWorkspaceACL(t *testing.T) {
	t.Parallel()

	t.Run("WorkspaceOwnerCanDelete_Groups", func(t *testing.T) {
		t.Parallel()

		var (
			client, db, admin = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
						dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}
					}),
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.ScopedRoleOrgAuditor(admin.OrganizationID))
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: admin.OrganizationID,
			}).Do().Workspace
		)

		ctx := testutil.Context(t, testutil.WaitMedium)

		group, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
			Name: "wibble",
		})
		require.NoError(t, err)
		err = workspaceOwnerClient.UpdateWorkspaceACL(ctx, workspace.ID, codersdk.UpdateWorkspaceACL{
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)

		err = workspaceOwnerClient.DeleteWorkspaceACL(ctx, workspace.ID)
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(ctx, workspace.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Groups)
	})

	t.Run("SharedGroupUsersCannotDelete", func(t *testing.T) {
		t.Parallel()

		var (
			client, db, admin = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
						dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}
					}),
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.ScopedRoleOrgAuditor(admin.OrganizationID))
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: admin.OrganizationID,
			}).Do().Workspace
			sharedClient, toShareWithUser = coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		)

		ctx := testutil.Context(t, testutil.WaitMedium)

		group, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
			Name: "wibble",
		})
		require.NoError(t, err)
		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{toShareWithUser.ID.String()},
		})
		require.NoError(t, err)
		err = workspaceOwnerClient.UpdateWorkspaceACL(ctx, workspace.ID, codersdk.UpdateWorkspaceACL{
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)

		err = sharedClient.DeleteWorkspaceACL(ctx, workspace.ID)
		require.Error(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(ctx, workspace.ID)
		require.NoError(t, err)
		require.Equal(t, acl.Groups[0].ID, group.ID)
	})
}

func TestWorkspacesSharedWith(t *testing.T) {
	t.Parallel()

	t.Run("ContainsActorsWithFullData", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})

		_, workspaceOwner := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        workspaceOwner.ID,
			OrganizationID: user.OrganizationID,
		}).Do().Workspace

		_, sharedWithUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Update a shared with user to have a name and avatar
		_, err := db.UpdateUserProfile(dbauthz.AsSystemRestricted(ctx), database.UpdateUserProfileParams{
			ID:        sharedWithUser.ID,
			Username:  sharedWithUser.Username,
			Name:      "Shared User Name",
			AvatarURL: "/emojis/1fae1.png",
		})
		require.NoError(t, err)

		// Create a shared with group with a name and avatar
		sharedWithGroup, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:      "shared-with-group",
			AvatarURL: "/emojis/1f60d.png",
		})
		require.NoError(t, err)

		// Share workspace with user and group
		err = client.UpdateWorkspaceACL(ctx, workspace.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				sharedWithUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
			GroupRoles: map[string]codersdk.WorkspaceRole{
				sharedWithGroup.ID.String(): codersdk.WorkspaceRoleAdmin,
			},
		})
		require.NoError(t, err)

		// Fetch workspace as client
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)
		require.NotNil(t, workspaces.Workspaces[0].SharedWith)
		require.Len(t, workspaces.Workspaces[0].SharedWith, 2)

		sharedWith := workspaces.Workspaces[0].SharedWith

		// Find actors in response
		var userActor, groupActor *codersdk.SharedWorkspaceActor
		for i := range sharedWith {
			if sharedWith[i].ActorType == codersdk.SharedWorkspaceActorTypeUser {
				userActor = &sharedWith[i]
			} else if sharedWith[i].ActorType == codersdk.SharedWorkspaceActorTypeGroup {
				groupActor = &sharedWith[i]
			}
		}

		require.NotNil(t, userActor, "expected to find user actor")
		assert.Equal(t, sharedWithUser.ID, userActor.ID)
		assert.Contains(t, userActor.Roles, codersdk.WorkspaceRoleUse)
		assert.Equal(t, "Shared User Name", userActor.Name)
		assert.Equal(t, "/emojis/1fae1.png", userActor.AvatarURL)

		require.NotNil(t, groupActor, "expected to find group actor")
		assert.Equal(t, sharedWithGroup.ID, groupActor.ID)
		assert.Equal(t, sharedWithGroup.Name, groupActor.Name)
		assert.Contains(t, groupActor.Roles, codersdk.WorkspaceRoleAdmin)
		assert.Equal(t, "/emojis/1f60d.png", groupActor.AvatarURL)
	})

	// /workspace endpoint should include the data too
	t.Run("WorkspaceResponseIncludesSharedWith", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})

		_, workspaceOwner := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        workspaceOwner.ID,
			OrganizationID: user.OrganizationID,
		}).Do().Workspace

		_, sharedWithUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Update a shared with user to have a name and avatar
		_, err := db.UpdateUserProfile(dbauthz.AsSystemRestricted(ctx), database.UpdateUserProfileParams{
			ID:        sharedWithUser.ID,
			Username:  sharedWithUser.Username,
			Name:      "Shared User Name",
			AvatarURL: "/emojis/1fae1.png",
		})
		require.NoError(t, err)

		// Create a shared with group with a name and avatar
		sharedWithGroup, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:      "shared-with-group",
			AvatarURL: "/emojis/1f60d.png",
		})
		require.NoError(t, err)

		// Share workspace with user and group
		err = client.UpdateWorkspaceACL(ctx, workspace.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				sharedWithUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
			GroupRoles: map[string]codersdk.WorkspaceRole{
				sharedWithGroup.ID.String(): codersdk.WorkspaceRoleAdmin,
			},
		})
		require.NoError(t, err)

		// Fetch from the /workspace endpoint as client
		ws, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.NotNil(t, ws.SharedWith)
		require.Len(t, ws.SharedWith, 2)

		sharedWith := ws.SharedWith

		// Find actors in response
		var userActor, groupActor *codersdk.SharedWorkspaceActor
		for i := range sharedWith {
			if sharedWith[i].ActorType == codersdk.SharedWorkspaceActorTypeUser {
				userActor = &sharedWith[i]
			} else if sharedWith[i].ActorType == codersdk.SharedWorkspaceActorTypeGroup {
				groupActor = &sharedWith[i]
			}
		}

		require.NotNil(t, userActor, "expected to find user actor")
		assert.Equal(t, sharedWithUser.ID, userActor.ID)
		assert.Contains(t, userActor.Roles, codersdk.WorkspaceRoleUse)
		assert.Equal(t, "Shared User Name", userActor.Name)
		assert.Equal(t, "/emojis/1fae1.png", userActor.AvatarURL)

		require.NotNil(t, groupActor, "expected to find group actor")
		assert.Equal(t, sharedWithGroup.ID, groupActor.ID)
		assert.Equal(t, sharedWithGroup.Name, groupActor.Name)
		assert.Contains(t, groupActor.Roles, codersdk.WorkspaceRoleAdmin)
		assert.Equal(t, "/emojis/1f60d.png", groupActor.AvatarURL)
	})

	// /workspace endpoint should include the data too
	t.Run("WorkspaceResponseIncludesSharedWith", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

		client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})

		_, workspaceOwner := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        workspaceOwner.ID,
			OrganizationID: user.OrganizationID,
		}).Do().Workspace

		_, sharedWithUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Update a shared with user to have a name and avatar
		_, err := db.UpdateUserProfile(dbauthz.AsSystemRestricted(ctx), database.UpdateUserProfileParams{
			ID:        sharedWithUser.ID,
			Username:  sharedWithUser.Username,
			Name:      "Shared User Name",
			AvatarURL: "/emojis/1fae1.png",
		})
		require.NoError(t, err)

		// Create a shared with group with a name and avatar
		sharedWithGroup, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:      "shared-with-group",
			AvatarURL: "/emojis/1f60d.png",
		})
		require.NoError(t, err)

		// Share workspace with user and group
		err = client.UpdateWorkspaceACL(ctx, workspace.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				sharedWithUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
			GroupRoles: map[string]codersdk.WorkspaceRole{
				sharedWithGroup.ID.String(): codersdk.WorkspaceRoleAdmin,
			},
		})
		require.NoError(t, err)

		// Fetch from the /workspace endpoint as client
		ws, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.NotNil(t, ws.SharedWith)
		require.Len(t, *ws.SharedWith, 2)

		sharedWith := *ws.SharedWith

		// Find user actor in response
		var userActor, groupActor *codersdk.SharedWorkspaceActor
		for i := range sharedWith {
			if sharedWith[i].ActorType == codersdk.SharedWorkspaceActorTypeUser {
				userActor = &sharedWith[i]
			} else if sharedWith[i].ActorType == codersdk.SharedWorkspaceActorTypeGroup {
				groupActor = &sharedWith[i]
			}
		}

		require.NotNil(t, userActor, "expected to find user actor")
		assert.Equal(t, sharedWithUser.ID, userActor.ID)
		assert.Contains(t, userActor.Roles, codersdk.WorkspaceRoleUse)
		assert.Equal(t, "Shared User Name", userActor.Name)
		assert.Equal(t, "/emojis/1fae1.png", userActor.AvatarURL)

		require.NotNil(t, groupActor, "expected to find group actor")
		assert.Equal(t, sharedWithGroup.ID, groupActor.ID)
		assert.Equal(t, sharedWithGroup.Name, groupActor.Name)
		assert.Contains(t, groupActor.Roles, codersdk.WorkspaceRoleAdmin)
		assert.Equal(t, "/emojis/1f60d.png", groupActor.AvatarURL)
	})
}
