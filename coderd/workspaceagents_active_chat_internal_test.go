package coderd

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

func TestActiveAgentChatDefinitionsAgree(t *testing.T) {
	t.Parallel()

	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitMedium))
	db, _ := dbtestutil.NewDB(t)

	org, err := db.GetDefaultOrganization(ctx)
	require.NoError(t, err)

	owner := dbgen.User(t, db, database.User{})
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
	}).WithAgent().Do()
	modelConfig := insertAgentChatTestModelConfig(t, db, owner.ID)

	insertedChats := make([]database.Chat, 0, len(database.AllChatStatusValues())*2)
	for _, archived := range []bool{false, true} {
		for _, status := range database.AllChatStatusValues() {
			chat := dbgen.Chat(t, db, database.Chat{
				OrganizationID:    org.ID,
				Status:            status,
				OwnerID:           owner.ID,
				LastModelConfigID: modelConfig.ID,
				Title:             fmt.Sprintf("%s-archived-%t", status, archived),
				AgentID:           uuid.NullUUID{UUID: workspace.Agents[0].ID, Valid: true},
			})

			if archived {
				_, err = db.ArchiveChatByID(ctx, chat.ID)
				require.NoError(t, err)

				chat, err = db.GetChatByID(ctx, chat.ID)
				require.NoError(t, err)
			}

			insertedChats = append(insertedChats, chat)
		}
	}

	activeChats, err := db.GetActiveChatsByAgentID(ctx, workspace.Agents[0].ID)
	require.NoError(t, err)

	activeByID := make(map[uuid.UUID]bool, len(activeChats))
	for _, chat := range activeChats {
		activeByID[chat.ID] = true
	}

	for _, chat := range insertedChats {
		require.Equalf(
			t,
			isActiveAgentChat(chat),
			activeByID[chat.ID],
			"status=%s archived=%t",
			chat.Status,
			chat.Archived,
		)
	}
}

func TestActiveAgentChatsIncludeInheritedACLs(t *testing.T) {
	t.Parallel()

	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitMedium))
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	org, err := db.GetDefaultOrganization(ctx)
	require.NoError(t, err)

	owner := dbgen.User(t, db, database.User{})
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
	}).WithAgent().Do()
	modelConfig := insertAgentChatTestModelConfig(t, db, owner.ID)

	root, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           owner.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "root-active-chat",
		AgentID:           uuid.NullUUID{UUID: workspace.Agents[0].ID, Valid: true},
	})
	require.NoError(t, err)

	child, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusRunning,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           owner.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "child-active-chat",
		AgentID:           uuid.NullUUID{UUID: workspace.Agents[0].ID, Valid: true},
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	require.NoError(t, err)

	rootUserACL := database.ChatACL{
		owner.ID.String(): {Permissions: []policy.Action{policy.ActionRead, policy.ActionSSH}},
	}
	rootGroupACL := database.ChatACL{
		org.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
	}

	userACLValue, err := rootUserACL.Value()
	require.NoError(t, err)
	groupACLValue, err := rootGroupACL.Value()
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(
		ctx,
		`UPDATE chats SET user_acl = $1::jsonb, group_acl = $2::jsonb WHERE id = $3`,
		userACLValue,
		groupACLValue,
		root.ID,
	)
	require.NoError(t, err)

	activeChats, err := db.GetActiveChatsByAgentID(ctx, workspace.Agents[0].ID)
	require.NoError(t, err)
	require.Len(t, activeChats, 2)

	activeByID := make(map[uuid.UUID]database.Chat, len(activeChats))
	for _, chat := range activeChats {
		activeByID[chat.ID] = chat
	}

	fetchedRoot, ok := activeByID[root.ID]
	require.True(t, ok)
	require.Equal(t, rootUserACL, fetchedRoot.UserACL)
	require.Equal(t, rootGroupACL, fetchedRoot.GroupACL)

	fetchedChild, ok := activeByID[child.ID]
	require.True(t, ok)
	require.True(t, fetchedChild.ParentChatID.Valid)
	require.Equal(t, rootUserACL, fetchedChild.UserACL)
	require.Equal(t, rootGroupACL, fetchedChild.GroupACL)
}
