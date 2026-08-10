package coderd_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCheckPermissions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	adminClient := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	// Create adminClient, member, and org adminClient
	adminUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
	memberUser, err := memberClient.User(ctx, codersdk.Me)
	require.NoError(t, err)
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID, rbac.ScopedRoleOrgAdmin(adminUser.OrganizationID))
	orgAdminUser, err := orgAdminClient.User(ctx, codersdk.Me)
	require.NoError(t, err)

	version := coderdtest.CreateTemplateVersion(t, adminClient, adminUser.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)
	template := coderdtest.CreateTemplate(t, adminClient, adminUser.OrganizationID, version.ID)

	// With admin, member, and org admin
	const (
		readAllUsers           = "read-all-users"
		readOrgWorkspaces      = "read-org-workspaces"
		readMyself             = "read-myself"
		readOwnWorkspaces      = "read-own-workspaces"
		updateSpecificTemplate = "update-specific-template"
	)
	params := map[string]codersdk.AuthorizationCheck{
		readAllUsers: {
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceUser,
			},
			Action: "read",
		},
		readOrgWorkspaces: {
			Object: codersdk.AuthorizationObject{
				ResourceType:   codersdk.ResourceWorkspace,
				OrganizationID: adminUser.OrganizationID.String(),
			},
			Action: "read",
		},
		readMyself: {
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceUser,
				OwnerID:      "me",
			},
			Action: "read",
		},
		readOwnWorkspaces: {
			Object: codersdk.AuthorizationObject{
				ResourceType:   codersdk.ResourceWorkspace,
				OrganizationID: adminUser.OrganizationID.String(),
				OwnerID:        "me",
			},
			Action: "read",
		},
		updateSpecificTemplate: {
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceTemplate,
				ResourceID:   template.ID.String(),
			},
			Action: "update",
		},
	}

	testCases := []struct {
		Name   string
		Client *codersdk.Client
		UserID uuid.UUID
		Check  codersdk.AuthorizationResponse
	}{
		{
			Name:   "Admin",
			Client: adminClient,
			UserID: adminUser.UserID,
			Check: map[string]bool{
				readAllUsers:           true,
				readOrgWorkspaces:      true,
				readMyself:             true,
				readOwnWorkspaces:      true,
				updateSpecificTemplate: true,
			},
		},
		{
			Name:   "OrgAdmin",
			Client: orgAdminClient,
			UserID: orgAdminUser.ID,
			Check: map[string]bool{
				readAllUsers:           true,
				readOrgWorkspaces:      true,
				readMyself:             true,
				readOwnWorkspaces:      true,
				updateSpecificTemplate: true,
			},
		},
		{
			Name:   "Member",
			Client: memberClient,
			UserID: memberUser.ID,
			Check: map[string]bool{
				readAllUsers:           false,
				readOrgWorkspaces:      false,
				readMyself:             true,
				readOwnWorkspaces:      true,
				updateSpecificTemplate: false,
			},
		},
	}

	for _, c := range testCases {
		t.Run("CheckAuthorization/"+c.Name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			t.Cleanup(cancel)

			resp, err := c.Client.AuthCheck(ctx, codersdk.AuthorizationRequest{Checks: params})
			require.NoError(t, err, "check perms")
			require.Equal(t, c.Check, resp)
		})
	}

	// Enough same-typed checks in one request to push a group past the batching
	// threshold, exercising the grouped partial-evaluation path and the key
	// mapping. Reading org members is allowed for any member; updating them is
	// admin-only, so the two actions must map back to their keys distinctly.
	t.Run("CheckAuthorization/Grouped", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)

		grouped := make(map[string]codersdk.AuthorizationCheck)
		adminExpected := make(map[string]bool)
		memberExpected := make(map[string]bool)
		for i := 0; i < 55; i++ {
			readKey := fmt.Sprintf("read-members-%d", i)
			grouped[readKey] = codersdk.AuthorizationCheck{
				Object: codersdk.AuthorizationObject{
					ResourceType:   codersdk.ResourceOrganizationMember,
					OrganizationID: adminUser.OrganizationID.String(),
				},
				Action: "read",
			}
			adminExpected[readKey] = true
			memberExpected[readKey] = true

			updateKey := fmt.Sprintf("update-members-%d", i)
			grouped[updateKey] = codersdk.AuthorizationCheck{
				Object: codersdk.AuthorizationObject{
					ResourceType:   codersdk.ResourceOrganizationMember,
					OrganizationID: adminUser.OrganizationID.String(),
				},
				Action: "update",
			}
			adminExpected[updateKey] = true
			memberExpected[updateKey] = false
		}

		adminResp, err := adminClient.AuthCheck(ctx, codersdk.AuthorizationRequest{Checks: grouped})
		require.NoError(t, err)
		require.Equal(t, adminExpected, map[string]bool(adminResp))

		memberResp, err := memberClient.AuthCheck(ctx, codersdk.AuthorizationRequest{Checks: grouped})
		require.NoError(t, err)
		require.Equal(t, memberExpected, map[string]bool(memberResp))
	})

	// A member may create a workspace in an org it belongs to, so an
	// any_org=true check is allowed. Repeating it past the batching threshold
	// must not change the answer: AnyOrgOwner objects have no verified partial-
	// evaluation semantics, so they must stay on the per-object path rather than
	// being grouped into rbac.Filter's prepared query, which denies them.
	t.Run("CheckAuthorization/AnyOrg", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)

		check := codersdk.AuthorizationCheck{
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceWorkspace,
				OwnerID:      "me",
				AnyOrgOwner:  true,
			},
			Action: "create",
		}

		// Below the threshold: a single check is evaluated in full and allowed.
		single, err := memberClient.AuthCheck(ctx, codersdk.AuthorizationRequest{
			Checks: map[string]codersdk.AuthorizationCheck{"create-any-org": check},
		})
		require.NoError(t, err)
		require.True(t, single["create-any-org"], "single any_org check should be allowed")

		// The same check repeated past the threshold must stay allowed.
		grouped := make(map[string]codersdk.AuthorizationCheck)
		expected := make(map[string]bool)
		for i := 0; i < 55; i++ {
			key := fmt.Sprintf("create-any-org-%d", i)
			grouped[key] = check
			expected[key] = true
		}
		resp, err := memberClient.AuthCheck(ctx, codersdk.AuthorizationRequest{Checks: grouped})
		require.NoError(t, err)
		require.Equal(t, expected, map[string]bool(resp))
	})

	// A single (read, workspace) group past the batching threshold mixing
	// resource_id checks (concrete workspaces fetched via the maxFetch path)
	// with org-scoped checks (synthetic org-level objects). The member owns the
	// fetched workspace so those keys are allowed, but cannot read all org
	// workspaces so the org-scoped keys are denied. Both object shapes must map
	// back to the correct per-key verdict through the one prepared query.
	t.Run("CheckAuthorization/MixedGroup", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)

		workspace := coderdtest.CreateWorkspace(t, memberClient, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, memberClient, workspace.LatestBuild.ID)

		grouped := make(map[string]codersdk.AuthorizationCheck)
		expected := make(map[string]bool)
		// resource_id checks are capped at maxFetch (10); the member owns the
		// workspace, so reading it is allowed.
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("read-own-workspace-%d", i)
			grouped[key] = codersdk.AuthorizationCheck{
				Object: codersdk.AuthorizationObject{
					ResourceType: codersdk.ResourceWorkspace,
					ResourceID:   workspace.ID.String(),
				},
				Action: "read",
			}
			expected[key] = true
		}
		// Org-scoped checks push the (read, workspace) group past the threshold.
		// A member cannot read every workspace in the org, so these are denied.
		for i := 0; i < 45; i++ {
			key := fmt.Sprintf("read-org-workspace-%d", i)
			grouped[key] = codersdk.AuthorizationCheck{
				Object: codersdk.AuthorizationObject{
					ResourceType:   codersdk.ResourceWorkspace,
					OrganizationID: adminUser.OrganizationID.String(),
				},
				Action: "read",
			}
			expected[key] = false
		}

		resp, err := memberClient.AuthCheck(ctx, codersdk.AuthorizationRequest{Checks: grouped})
		require.NoError(t, err)
		require.Equal(t, expected, map[string]bool(resp))
	})
}
