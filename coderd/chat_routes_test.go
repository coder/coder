package coderd_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/testutil"
)

func TestChatRouteMounts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		OrganizationID: firstUser.OrganizationID,
	})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: model.ID,
	})

	for _, route := range []string{
		"/api/v2/chats",
		"/api/v2/chats/config/system-prompt",
		fmt.Sprintf("/api/experimental/chats/%s/debug/runs", chat.ID),
	} {
		res, err := client.Request(ctx, http.MethodGet, route, nil)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode, route)
	}

	for _, route := range []struct {
		method string
		path   string
	}{
		// Promoted routes no longer answer on /api/experimental.
		{http.MethodGet, "/api/experimental/chats"},
		{http.MethodGet, "/api/experimental/chats/config/system-prompt"},
		{http.MethodGet, fmt.Sprintf("/api/experimental/organizations/%s/chats/models", firstUser.OrganizationID)},
		{http.MethodGet, fmt.Sprintf("/api/experimental/organizations/%s/mcp-servers", firstUser.OrganizationID)},
		{http.MethodDelete, fmt.Sprintf("/api/experimental/mcp/servers/%s/oauth2/disconnect", uuid.New())},
		{http.MethodGet, "/api/experimental/users/me/ai-provider-keys"},
		// Never-promoted routes are absent from /api/v2.
		{http.MethodGet, "/api/v2/chats/model-configs"},
		{http.MethodPost, "/api/v2/chats/model-configs"},
		{http.MethodGet, "/api/v2/chats/config/computer-use-provider"},
		{http.MethodGet, "/api/v2/chats/config/advisor"},
		{http.MethodGet, fmt.Sprintf("/api/v2/chats/%s/debug/runs", chat.ID)},
		{http.MethodGet, fmt.Sprintf("/api/v2/chats/%s/stream/desktop", chat.ID)},
		{http.MethodGet, "/api/v2/mcp/servers/not-a-uuid/oauth2/callback"},
		{http.MethodPost, "/api/v2/mcp/http/server"},
	} {
		res, err := client.Request(ctx, route.method, route.path, nil)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode, "%s %s", route.method, route.path)
	}
}
