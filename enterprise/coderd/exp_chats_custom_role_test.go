package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// TestCustomRoleChatAccess validates that custom organization roles with
// chat permissions allow users to access chat resources across user
// boundaries within the same org.
//
// Key RBAC facts for context:
//   - Chat objects are org-scoped: ResourceChat.WithID(id).WithOwner(ownerID).InOrg(orgID)
//   - The postChats handler checks ActionCreate on ResourceChat.InOrg(orgID).WithOwner(userID),
//     so org-level custom role permissions can satisfy the create check.
//   - Read/update checks go through dbauthz on the actual chat object (which IS org-scoped),
//     so org-level custom role permissions apply for reads and updates on existing chats.
//   - The listChats handler hardcodes OwnerID to the calling user, so cross-user visibility
//     must be tested via GetChat, not ListChats.
func TestCustomRoleChatAccess(t *testing.T) {
	t.Parallel()

	// setupTest creates an enterprise server with custom roles + agents
	// experiment, a chat provider/model config, and an owner-created chat.
	type testSetup struct {
		ownerClient *codersdk.Client
		ownerExp    *codersdk.ExperimentalClient
		firstUser   codersdk.CreateFirstUserResponse
		ownerChat   codersdk.Chat
	}

	setupTest := func(t *testing.T) testSetup {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentAgents)}

		client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})
		expClient := codersdk.NewExperimentalClient(client)

		// Create a chat provider and model config so chats can be created.
		provider, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test-key",
			BaseURL:     "https://example.com",
		})
		require.NoError(t, err)
		_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-4o-mini",
			DisplayName:          "Test Model",
			IsDefault:            ptr.Ref(true),
			ContextLimit:         ptr.Ref(int64(1000)),
			CompressionThreshold: ptr.Ref(int32(70)),
		})
		require.NoError(t, err)

		// Owner creates a chat that subtests use to verify cross-user access.
		ownerChat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "hello from owner"},
			},
		})
		require.NoError(t, err)

		return testSetup{
			ownerClient: client,
			ownerExp:    expClient,
			firstUser:   firstUser,
			ownerChat:   ownerChat,
		}
	}

	// FullCRUD: A custom role granting read+update on chat combined with
	// agents-access should allow the member to create their own chats AND
	// read/update another user's chat via org-level perms.
	t.Run("FullCRUD", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		s := setupTest(t)

		// Create custom role with read+update chat permissions at org level.
		role := codersdk.Role{
			Name:           "chat-read-update",
			DisplayName:    "Chat Read Update",
			OrganizationID: s.firstUser.OrganizationID.String(),
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceChat: {codersdk.ActionRead, codersdk.ActionUpdate},
			}),
		}
		//nolint:gocritic // owner is required to create custom roles
		_, err := s.ownerClient.CreateOrganizationRole(ctx, role)
		require.NoError(t, err)

		// Create a member with the custom role AND agents-access.
		// agents-access is still needed for chat creation because
		// downstream dbauthz checks (e.g. GetDefaultChatModelConfig)
		// use un-org-scoped ResourceChat checks that only match
		// user-level permissions.
		memberRaw, _ := coderdtest.CreateAnotherUser(
			t, s.ownerClient, s.firstUser.OrganizationID,
			rbac.RoleIdentifier{Name: role.Name, OrganizationID: s.firstUser.OrganizationID},
			rbac.RoleAgentsAccess(),
		)
		memberExp := codersdk.NewExperimentalClient(memberRaw)

		// 1. Member can create their own chat.
		memberChat, err := memberExp.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: s.firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "hello from member"},
			},
		})
		require.NoError(t, err, "member with agents-access should create chats")

		// 2. Member can list their own chats.
		chats, err := memberExp.ListChats(ctx, nil)
		require.NoError(t, err)
		var foundOwn bool
		for _, c := range chats {
			if c.ID == memberChat.ID {
				foundOwn = true
			}
		}
		require.True(t, foundOwn, "member should see own chat in list")

		// 3. Member can read their own chat.
		gotChat, err := memberExp.GetChat(ctx, memberChat.ID)
		require.NoError(t, err)
		require.Equal(t, memberChat.ID, gotChat.ID)

		// 4. Member can read the OWNER's chat (cross-user via org-level read).
		gotOwnerChat, err := memberExp.GetChat(ctx, s.ownerChat.ID)
		require.NoError(t, err, "custom org-level read should grant access to owner's chat")
		require.Equal(t, s.ownerChat.ID, gotOwnerChat.ID)

		// 5. Member can read the owner's chat messages.
		messages, err := memberExp.GetChatMessages(ctx, s.ownerChat.ID, nil)
		require.NoError(t, err, "custom org-level read should grant access to owner's messages")
		require.NotEmpty(t, messages.Messages)

		// 6. Member can archive the owner's chat (cross-user via org-level update).
		err = memberExp.UpdateChat(ctx, s.ownerChat.ID, codersdk.UpdateChatRequest{
			Archived: ptr.Ref(true),
		})
		require.NoError(t, err, "custom org-level update should allow archiving owner's chat")
	})

	// ReadOtherUsersChat: A custom role granting ONLY read on chat
	// should allow the member to view another user's chat but not modify it.
	t.Run("ReadOtherUsersChat", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		s := setupTest(t)

		// Create custom role with read-only chat permission at org level.
		role := codersdk.Role{
			Name:           "chat-read-only",
			DisplayName:    "Chat Read Only",
			OrganizationID: s.firstUser.OrganizationID.String(),
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceChat: {codersdk.ActionRead},
			}),
		}
		//nolint:gocritic // owner is required to create custom roles
		_, err := s.ownerClient.CreateOrganizationRole(ctx, role)
		require.NoError(t, err)

		// Create a member with the custom role AND agents-access.
		// agents-access grants user-level chat create/read/update/delete
		// on own resources. The custom role adds org-level read on ALL
		// chats in the org.
		memberRaw, _ := coderdtest.CreateAnotherUser(
			t, s.ownerClient, s.firstUser.OrganizationID,
			rbac.RoleIdentifier{Name: role.Name, OrganizationID: s.firstUser.OrganizationID},
			rbac.RoleAgentsAccess(),
		)
		memberExp := codersdk.NewExperimentalClient(memberRaw)

		// 1. Member can read the owner's chat (via custom org-level read).
		gotChat, err := memberExp.GetChat(ctx, s.ownerChat.ID)
		require.NoError(t, err, "custom org-level read should grant access to owner's chat")
		require.Equal(t, s.ownerChat.ID, gotChat.ID)

		// 2. Member can read the owner's chat messages.
		messages, err := memberExp.GetChatMessages(ctx, s.ownerChat.ID, nil)
		require.NoError(t, err, "custom org-level read should grant access to owner's messages")
		require.NotEmpty(t, messages.Messages)

		// 3. Member CANNOT update (archive) the owner's chat — no org-level update.
		// The ChatParam middleware successfully loads the chat (member has
		// org-level read), but the subsequent ArchiveChatByID call fails
		// in dbauthz because the member lacks update permission. The
		// handler currently returns 500 for this case.
		err = memberExp.UpdateChat(ctx, s.ownerChat.ID, codersdk.UpdateChatRequest{
			Archived: ptr.Ref(true),
		})
		require.Error(t, err, "member without org-level update should not archive owner's chat")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.True(t, apiErr.StatusCode() == http.StatusInternalServerError || apiErr.StatusCode() == http.StatusNotFound,
			"expected 500 or 404, got %d", apiErr.StatusCode())
	})

	// NoOrgPermissions: A member with agents-access but NO custom org role
	// should not be able to access another user's chat. This is the baseline
	// control proving the custom role is what enables cross-user access.
	t.Run("NoOrgPermissions", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		s := setupTest(t)

		// Create a member with agents-access only (no custom org role).
		memberRaw, _ := coderdtest.CreateAnotherUser(
			t, s.ownerClient, s.firstUser.OrganizationID,
			rbac.RoleAgentsAccess(),
		)
		memberExp := codersdk.NewExperimentalClient(memberRaw)

		// 1. Member can create their own chat (via agents-access).
		_, err := memberExp.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: s.firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "hello from member"},
			},
		})
		require.NoError(t, err, "member with agents-access can create own chats")

		// 2. Member CANNOT read the owner's chat — no org-level read.
		_, err = memberExp.GetChat(ctx, s.ownerChat.ID)
		require.Error(t, err, "member without org-level read should not access owner's chat")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())

		// 3. Member CANNOT read the owner's chat messages.
		_, err = memberExp.GetChatMessages(ctx, s.ownerChat.ID, nil)
		require.Error(t, err, "member without org-level read should not read owner's messages")
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())

		// 4. Member CANNOT update the owner's chat.
		err = memberExp.UpdateChat(ctx, s.ownerChat.ID, codersdk.UpdateChatRequest{
			Archived: ptr.Ref(true),
		})
		require.Error(t, err, "member without org-level update should not archive owner's chat")
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})
}
