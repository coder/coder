package coderd

import (
	"database/sql"
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

func insertAgentChatTestModelConfig(
	t testing.TB,
	db database.Store,
	userID uuid.UUID,
) database.ChatModelConfig {
	t.Helper()

	createdBy := uuid.NullUUID{UUID: userID, Valid: true}

	provider := dbgen.AIProvider(t, db, database.AIProvider{
		Type:        database.AIProviderTypeOpenai,
		Name:        "test-openai",
		DisplayName: sql.NullString{String: "OpenAI", Valid: true},
	})
	dbgen.AIProviderKey(t, db, database.AIProviderKey{
		ProviderID: provider.ID,
		APIKey:     "test-api-key",
	})

	return dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
		CreatedBy:    createdBy,
		UpdatedBy:    createdBy,
		IsDefault:    true,
	})
}
