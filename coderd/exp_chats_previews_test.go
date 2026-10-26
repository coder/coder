package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatPreviews(t *testing.T) {
	t.Parallel()

	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	ownerClient, owner := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	otherClient, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(ownerClient)

	newWorkspaceID := func() uuid.UUID {
		return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        owner.ID,
		}).Do().Workspace.ID
	}
	workspaceID, otherWorkspaceID := newWorkspaceID(), newWorkspaceID()
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: firstUser.OrganizationID})

	newChat := func(ownerID uuid.UUID, workspace uuid.NullUUID) database.Chat {
		return dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           ownerID,
			WorkspaceID:       workspace,
			LastModelConfigID: model.ID,
		})
	}

	chat := newChat(owner.ID, uuid.NullUUID{UUID: workspaceID, Valid: true})

	insert := func(part codersdk.ChatMessagePart, seed database.ChatMessage) database.ChatMessage {
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{part})
		require.NoError(t, err)

		seed.ChatID = chat.ID
		seed.Role = database.ChatMessageRoleTool
		seed.Content = content

		return dbgen.ChatMessage(t, db, seed)
	}

	resultPart := func(isError bool, overrides map[string]any) codersdk.ChatMessagePart {
		result := map[string]any{
			"ok":           true,
			"workspace_id": workspaceID.String(),
			"agent_name":   "main",
		}
		for key, override := range overrides {
			result[key] = override
		}

		raw, err := json.Marshal(result)
		require.NoError(t, err)

		return codersdk.ChatMessageToolResult(uuid.NewString(), "open_preview", raw, isError, false)
	}

	insertResult := func(overrides map[string]any) database.ChatMessage {
		return insert(resultPart(false, overrides), database.ChatMessage{})
	}

	insertResult(map[string]any{"title": "Web app", "port": 3000})
	insertResult(map[string]any{"title": "Docs", "preview_id": "fedcba9876543210", "path": "/home/coder/site"})
	insertResult(map[string]any{"title": "Updated web app", "port": 3000})
	insertResult(map[string]any{"title": "Editor", "port": 3000, "url_path": "/?folder=/home/coder"})

	// None of these may appear or rename a listed preview.
	insertResult(map[string]any{"title": "Old workspace", "port": 3000, "workspace_id": otherWorkspaceID.String()})
	insert(resultPart(true, map[string]any{"title": "Failed", "port": 3000}), database.ChatMessage{})
	insert(codersdk.ChatMessageToolResult(uuid.NewString(), "open_preview", nil, false, false), database.ChatMessage{})
	insertResult(map[string]any{"title": "Short port", "port": 999})
	insertResult(map[string]any{"title": "Bad ID", "preview_id": "evil.com/#"})
	insertResult(map[string]any{"title": "Both", "port": 3000, "preview_id": "fedcba9876543210"})
	insertResult(map[string]any{"title": "Bad agent", "port": 3000, "agent_name": "x@evil.com"})
	insertResult(map[string]any{"title": "Other host", "port": 3000, "url_path": "//evil.example"})

	mcpPart := resultPart(false, map[string]any{"title": "MCP", "port": 3000})
	mcpPart.MCPServerConfigID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	insert(mcpPart, database.ChatMessage{})

	insert(resultPart(false, map[string]any{"title": "Model only", "port": 3000}), database.ChatMessage{
		Visibility: database.ChatMessageVisibilityModel,
	})

	deleted := insertResult(map[string]any{"title": "Deleted", "port": 3000})
	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, db.SoftDeleteChatMessageByID(dbauthz.AsSystemRestricted(ctx), deleted.ID))

	previews, err := memberClient.GetChatPreviews(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatPreview{
		{Title: "Updated web app", AgentName: "main", Port: 3000},
		{Title: "Docs", AgentName: "main", PreviewID: "fedcba9876543210", Path: "/home/coder/site"},
		{Title: "Editor", AgentName: "main", Port: 3000, URLPath: "/?folder=/home/coder"},
	}, previews)

	unbound := newChat(owner.ID, uuid.NullUUID{})
	previews, err = memberClient.GetChatPreviews(ctx, unbound.ID)
	require.NoError(t, err)
	require.Empty(t, previews)

	_, err = codersdk.NewExperimentalClient(otherClient).GetChatPreviews(ctx, chat.ID)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
}
