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
	modelConfig := insertAgentChatTestModelConfig(ctx, t, db, owner.ID)

	insertedChats := make([]database.Chat, 0, len(database.AllChatStatusValues())*2)
	for _, archived := range []bool{false, true} {
		for _, status := range database.AllChatStatusValues() {
			chat, err := db.InsertChat(ctx, database.InsertChatParams{
				OrganizationID:    org.ID,
				Status:            status,
				ClientType:        database.ChatClientTypeUi,
				OwnerID:           owner.ID,
				LastModelConfigID: modelConfig.ID,
				Title:             fmt.Sprintf("%s-archived-%t", status, archived),
				AgentID:           uuid.NullUUID{UUID: workspace.Agents[0].ID, Valid: true},
			})
			require.NoError(t, err)

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
