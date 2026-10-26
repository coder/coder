package coderd_test

import (
	"encoding/json"
	"maps"
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

	insert := func(seed database.ChatMessage, parts ...codersdk.ChatMessagePart) database.ChatMessage {
		content, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)

		seed.ChatID = chat.ID
		if seed.Role == "" {
			seed.Role = database.ChatMessageRoleTool
		}
		seed.Content = content

		return dbgen.ChatMessage(t, db, seed)
	}

	resultPart := func(overrides map[string]any) codersdk.ChatMessagePart {
		result := map[string]any{
			"ok":           true,
			"workspace_id": workspaceID.String(),
			"agent_name":   "main",
		}
		maps.Copy(result, overrides)

		raw, err := json.Marshal(result)
		require.NoError(t, err)

		return codersdk.ChatMessageToolResult(uuid.NewString(), "open_preview", raw, false, false)
	}

	insertResult := func(overrides map[string]any) database.ChatMessage {
		return insert(database.ChatMessage{}, resultPart(overrides))
	}

	insertResult(map[string]any{"title": "Web app", "port": 3000})
	insertResult(map[string]any{"title": "Docs", "preview_id": "fedcba98", "path": "/home/coder/site"})
	insertResult(map[string]any{"title": "Updated web app", "port": 3000})
	insertResult(map[string]any{"title": "Docs", "preview_id": "0badc0de", "path": "/home/coder/site"})
	// Another tool's result in the same message must not rename a preview.
	otherTool := resultPart(map[string]any{"title": "Other tool", "port": 3000})
	otherTool.ToolName = "other_tool"
	// Stored text encodes NUL characters, so the endpoint must decode it.
	insert(database.ChatMessage{}, otherTool, resultPart(map[string]any{
		"title":    "Editor\x00\uE000",
		"port":     3000,
		"url_path": "/?folder=/home/coder/\uE000",
	}))

	// None of these may appear or rename a listed preview.
	insertResult(map[string]any{"title": "Old workspace", "port": 3000, "workspace_id": otherWorkspaceID.String()})
	insertResult(map[string]any{"title": "Short port", "port": 999})
	insertResult(map[string]any{"title": "Big port", "port": 65536})
	insertResult(map[string]any{"title": "Bad ID", "preview_id": "evil.com/#"})
	insertResult(map[string]any{"title": "Neither"})
	insertResult(map[string]any{"title": "Both", "port": 3000, "preview_id": "fedcba98"})
	insertResult(map[string]any{"title": "Bad ID with port", "port": 3000, "preview_id": "evil.com/#"})
	insertResult(map[string]any{"title": "Short port with ID", "port": 999, "preview_id": "fedcba98"})
	insertResult(map[string]any{"title": "Bad agent", "port": 3000, "agent_name": "x@evil.com"})
	insertResult(map[string]any{"title": "Other host", "port": 3000, "url_path": "//evil.example"})

	insert(database.ChatMessage{}, codersdk.ChatMessageToolResult(uuid.NewString(), "open_preview", nil, false, false))
	insert(database.ChatMessage{Role: database.ChatMessageRoleAssistant},
		resultPart(map[string]any{"title": "Assistant role", "port": 3000}))

	insert(database.ChatMessage{Visibility: database.ChatMessageVisibilityModel},
		resultPart(map[string]any{"title": "Model only", "port": 3000}))

	deleted := insertResult(map[string]any{"title": "Deleted", "port": 3000})
	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, db.SoftDeleteChatMessageByID(dbauthz.AsSystemRestricted(ctx), deleted.ID))

	previews, err := memberClient.GetChatPreviews(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatPreview{
		{Title: "Updated web app", AgentName: "main", Port: 3000},
		{Title: "Docs", AgentName: "main", PreviewID: "0badc0de", Path: "/home/coder/site"},
		{Title: "Editor\x00\uE000", AgentName: "main", Port: 3000, URLPath: "/?folder=/home/coder/\uE000"},
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
