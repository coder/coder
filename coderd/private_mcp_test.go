package coderd_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPostChatsPrivateMCPServerConfigsPersistedAndOmitted(t *testing.T) {
	t.Parallel()

	const (
		serverURL   = "https://mcp.example.com/v1"
		headerName  = "X-Private-Canary"
		headerValue = "private-canary-value-12345"
	)
	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t, func(options *coderdtest.Options) {
		options.ChatWorkerDisabled = true
	})
	user := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "private MCP API test",
		}},
		PrivateMCPServerConfigs: []codersdk.PrivateMCPServerConfig{{
			Name:    "private",
			URL:     serverURL,
			Headers: map[string]string{headerName: headerValue},
		}},
	})
	require.NoError(t, err)

	persisted, err := db.GetChatPrivateMCPServerConfigsByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
	require.NoError(t, err)
	require.Contains(t, string(persisted), serverURL)
	require.Contains(t, string(persisted), headerName)
	require.Contains(t, string(persisted), headerValue)
	shared, err := db.GetMCPServerConfigsByOrganization(dbauthz.AsSystemRestricted(ctx), user.OrganizationID)
	require.NoError(t, err)
	require.Empty(t, shared, "private configs must not create shared MCP registrations")

	getChat, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	messages, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	for _, value := range []any{chat, getChat, messages} {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "private_mcp_server_configs")
		require.NotContains(t, string(encoded), serverURL)
		require.NotContains(t, string(encoded), headerName)
		require.NotContains(t, string(encoded), headerValue)
	}
}
