package chatstate_test

import (
	"encoding/json"
	"testing"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateChatPersistsPrivateMCPServerConfigs(t *testing.T) {
	t.Parallel()

	fixture := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	configs := []codersdk.PrivateMCPServerConfig{{
		Name:    "private",
		URL:     "https://mcp.example.com",
		Headers: map[string]string{"Authorization": "Bearer private-canary"},
	}}
	raw, err := json.Marshal(configs)
	require.NoError(t, err)

	created, err := chatstate.CreateChat(ctx, fixture.DB, fixture.Pub, chatstate.CreateChatInput{
		OrganizationID:    fixture.Org.ID,
		OwnerID:           fixture.User.ID,
		LastModelConfigID: fixture.Model.ID,
		Title:             "private MCP",
		ClientType:        database.ChatClientTypeApi,
		PrivateMCPServerConfigs: pqtype.NullRawMessage{
			RawMessage: raw,
			Valid:      true,
		},
		InitialMessages: []chatstate.Message{
			userTextMessage("hello", fixture.User.ID, fixture.Model.ID),
		},
	})
	require.NoError(t, err)

	persisted, err := fixture.DB.GetChatPrivateMCPServerConfigsByChatID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(persisted))
}
