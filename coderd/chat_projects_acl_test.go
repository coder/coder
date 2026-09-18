package coderd_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestChatProjectACLSharingLifecycle(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	mAudit := audit.NewMock()
	notifyEnq := &notificationstest.FakeEnqueuer{}
	client, db := newChatClientWithDatabase(t,
		func(opts *coderdtest.Options) {
			opts.Auditor = mAudit
			opts.NotificationsEnqueuer = notifyEnq
			opts.DeploymentValues.Experiments = serpent.StringArray{
				string(codersdk.ExperimentChatProjects),
			}
		},
		withChatWorkerDisabled,
	)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)

	sharedClient, sharedUser := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	sharedExp := codersdk.NewExperimentalClient(sharedClient)
	nonSharedClient, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	nonSharedExp := codersdk.NewExperimentalClient(nonSharedClient)
	groupMemberClient, groupMember := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	groupMemberExp := codersdk.NewExperimentalClient(groupMemberClient)
	sharedGroup := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: sharedGroup.ID, UserID: groupMember.ID})

	project := createChatProject(t, client, firstUser.OrganizationID, "Shared Project")
	ownerChat := createChatInProject(t, client, firstUser.OrganizationID, &project.ID)

	_, err := sharedExp.GetChatProject(ctx, project.ID)
	requireSDKError(t, err, http.StatusNotFound)
	_, err = sharedExp.GetChatProjectACL(ctx, project.ID)
	requireSDKError(t, err, http.StatusNotFound)

	err = client.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			sharedUser.ID.String(): codersdk.ChatProjectRoleRead,
		},
		GroupRoles: map[string]codersdk.ChatProjectRole{
			sharedGroup.ID.String(): codersdk.ChatProjectRoleRead,
		},
	})
	require.NoError(t, err)
	require.True(t, mAudit.Contains(t, database.AuditLog{
		Action:       database.AuditActionWrite,
		ResourceType: database.ResourceTypeChatProject,
		ResourceID:   project.ID,
		UserID:       firstUser.UserID,
	}))

	// Only the direct user grant is notified; group grants are not expanded.
	var sent []*notificationstest.FakeNotification
	testutil.Eventually(ctx, t, func(context.Context) bool {
		sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateChatProjectShared))
		return len(sent) == 1
	}, testutil.IntervalFast)
	require.Equal(t, sharedUser.ID, sent[0].UserID)
	require.Equal(t, firstUser.UserID.String(), sent[0].CreatedBy)
	require.Equal(t, map[string]string{
		"project_id":   project.ID.String(),
		"project_name": project.Name,
		"initiator":    coderdtest.FirstUserParams.Username,
	}, sent[0].Labels)
	require.Equal(t, []uuid.UUID{project.ID}, sent[0].Targets)

	// Re-applying the same grants does not notify again.
	notifyEnq.Clear()
	err = client.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			sharedUser.ID.String(): codersdk.ChatProjectRoleRead,
		},
	})
	require.NoError(t, err)
	require.Empty(t, notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateChatProjectShared)))

	acl, err := client.GetChatProjectACL(ctx, project.ID)
	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]codersdk.ChatProjectRole{
		sharedUser.ID: codersdk.ChatProjectRoleRead,
	}, chatProjectUserRoles(acl.Users))
	require.Equal(t, map[uuid.UUID]codersdk.ChatProjectRole{
		sharedGroup.ID: codersdk.ChatProjectRoleRead,
	}, chatProjectGroupRoles(acl.Groups))
	require.Len(t, acl.Groups, 1)
	require.Empty(t, acl.Groups[0].Members)
	require.Equal(t, 1, acl.Groups[0].TotalMemberCount)

	// Readers see the project, its ACL, and the list entry, and can start
	// their own chats in it.
	sharedACL, err := sharedExp.GetChatProjectACL(ctx, project.ID)
	require.NoError(t, err)
	require.Equal(t, chatProjectUserRoles(acl.Users), chatProjectUserRoles(sharedACL.Users))
	sharedProject, err := sharedExp.GetChatProject(ctx, project.ID)
	require.NoError(t, err)
	require.Equal(t, project.ID, sharedProject.ID)
	projects, err := sharedExp.ListChatProjects(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{project.ID}, chatProjectIDs(projects))
	sharedChat := createChatInProject(t, sharedExp, firstUser.OrganizationID, &project.ID)
	require.NotNil(t, sharedChat.ProjectID)
	require.Equal(t, project.ID, *sharedChat.ProjectID)

	groupProject, err := groupMemberExp.GetChatProject(ctx, project.ID)
	require.NoError(t, err)
	require.Equal(t, project.ID, groupProject.ID)

	// Sharing the project does not share the chats inside it.
	_, err = sharedExp.GetChat(ctx, ownerChat.ID)
	requireSDKError(t, err, http.StatusNotFound)
	_, err = nonSharedExp.GetChatProject(ctx, project.ID)
	requireSDKError(t, err, http.StatusNotFound)

	// Readers cannot manage or re-share the project.
	_, err = sharedExp.UpdateChatProject(ctx, project.ID, codersdk.UpdateChatProjectRequest{Name: new("renamed")})
	requireSDKError(t, err, http.StatusNotFound)
	err = sharedExp.DeleteChatProject(ctx, project.ID)
	requireSDKError(t, err, http.StatusNotFound)
	err = sharedExp.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			uuid.NewString(): codersdk.ChatProjectRoleRead,
		},
	})
	requireSDKError(t, err, http.StatusForbidden)

	err = client.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			strings.ToUpper(firstUser.UserID.String()): codersdk.ChatProjectRoleRead,
		},
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Equal(t, "Cannot change your own project sharing role.", sdkErr.Message)

	err = client.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			uuid.NewString(): codersdk.ChatProjectRoleRead,
		},
	})
	sdkErr = requireSDKError(t, err, http.StatusBadRequest)
	require.Equal(t, "Invalid request to update chat project ACL.", sdkErr.Message)

	// Revoking the user grant hides the project again while the group grant
	// keeps working, and the reader's own chat in it stays theirs.
	err = client.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			sharedUser.ID.String(): codersdk.ChatProjectRoleDeleted,
		},
	})
	require.NoError(t, err)
	_, err = sharedExp.GetChatProject(ctx, project.ID)
	requireSDKError(t, err, http.StatusNotFound)
	projects, err = sharedExp.ListChatProjects(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Empty(t, projects)
	_, err = sharedExp.GetChat(ctx, sharedChat.ID)
	require.NoError(t, err)
	_, err = groupMemberExp.GetChatProject(ctx, project.ID)
	require.NoError(t, err)

	err = client.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		GroupRoles: map[string]codersdk.ChatProjectRole{
			sharedGroup.ID.String(): codersdk.ChatProjectRoleDeleted,
		},
	})
	require.NoError(t, err)
	_, err = groupMemberExp.GetChatProject(ctx, project.ID)
	requireSDKError(t, err, http.StatusNotFound)
	acl, err = client.GetChatProjectACL(ctx, project.ID)
	require.NoError(t, err)
	require.Empty(t, acl.Users)
	require.Empty(t, acl.Groups)
}

func TestChatProjectACLOrgAdminCanShare(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	notifyEnq := &notificationstest.FakeEnqueuer{}
	client, _ := newChatClientWithDatabase(t,
		func(opts *coderdtest.Options) {
			opts.NotificationsEnqueuer = notifyEnq
			opts.DeploymentValues.Experiments = serpent.StringArray{
				string(codersdk.ExperimentChatProjects),
			}
		},
		withChatWorkerDisabled,
	)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	adminClient, admin := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID))
	adminExp := codersdk.NewExperimentalClient(adminClient)
	_, directUser := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)

	project := createChatProject(t, client, firstUser.OrganizationID, "Admin Shared Project")

	err := adminExp.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			directUser.ID.String(): codersdk.ChatProjectRoleRead,
		},
	})
	require.NoError(t, err)

	var sent []*notificationstest.FakeNotification
	testutil.Eventually(ctx, t, func(context.Context) bool {
		sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateChatProjectShared))
		return len(sent) == 1
	}, testutil.IntervalFast)
	require.Equal(t, directUser.ID, sent[0].UserID)
	require.Equal(t, admin.ID.String(), sent[0].CreatedBy)
	require.Equal(t, admin.Username, sent[0].Labels["initiator"])
}

//nolint:paralleltest // This test verifies a process-wide RBAC kill switch.
func TestChatProjectSharingDisabled(t *testing.T) {
	previous := rbac.ChatACLDisabled()
	rbac.SetChatACLDisabled(false)
	rbac.ReloadBuiltinRoles(nil)
	t.Cleanup(func() {
		rbac.ReloadBuiltinRoles(nil)
		rbac.SetChatACLDisabled(previous)
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	values := coderdtest.DeploymentValues(t)
	values.DisableChatSharing = true
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentChatProjects)}
	store, pubsub := dbtestutil.NewDB(t)
	client := newChatClient(t,
		func(opts *coderdtest.Options) {
			opts.DeploymentValues = values
			opts.Database = store
			opts.Pubsub = pubsub
		},
		withChatWorkerDisabled,
	)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	viewerClient, viewer := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	viewerExp := codersdk.NewExperimentalClient(viewerClient)

	project := createChatProject(t, client, firstUser.OrganizationID, "Disabled Sharing")
	err := store.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID: project.ID,
		UserACL: database.ChatACL{
			viewer.ID.String(): database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}},
		},
		GroupACL: database.ChatACL{},
	})
	require.NoError(t, err)

	// Existing grants are ignored while sharing is disabled.
	_, err = viewerExp.GetChatProject(ctx, project.ID)
	requireSDKError(t, err, http.StatusNotFound)
	projects, err := viewerExp.ListChatProjects(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Empty(t, projects)

	_, err = client.GetChatProjectACL(ctx, project.ID)
	sdkErr := requireSDKError(t, err, http.StatusForbidden)
	require.Equal(t, "Chat sharing is disabled for this deployment.", sdkErr.Message)
	err = client.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			viewer.ID.String(): codersdk.ChatProjectRoleRead,
		},
	})
	requireSDKError(t, err, http.StatusForbidden)
}

func chatProjectUserRoles(users []codersdk.ChatProjectUser) map[uuid.UUID]codersdk.ChatProjectRole {
	roles := make(map[uuid.UUID]codersdk.ChatProjectRole, len(users))
	for _, user := range users {
		roles[user.ID] = user.Role
	}
	return roles
}

func chatProjectGroupRoles(groups []codersdk.ChatProjectGroup) map[uuid.UUID]codersdk.ChatProjectRole {
	roles := make(map[uuid.UUID]codersdk.ChatProjectRole, len(groups))
	for _, group := range groups {
		roles[group.ID] = group.Role
	}
	return roles
}

func chatProjectIDs(projects []codersdk.ChatProject) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}
	return ids
}
