package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

func TestChatRoutesCompatibility(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	coderdtest.CreateFirstUser(t, client)

	for _, route := range []string{
		"/api/experimental/chats",
		"/api/experimental/chats/config/system-prompt",
		"/api/v2/chats",
		"/api/v2/chats/config/system-prompt",
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
		{http.MethodGet, "/api/v2/chats/model-configs"},
		{http.MethodPost, "/api/v2/chats/model-configs"},
		{http.MethodGet, "/api/v2/chats/providers"},
		{http.MethodGet, "/api/v2/chats/user-provider-configs"},
		{http.MethodGet, "/api/v2/chats/config/computer-use-provider"},
		{http.MethodGet, "/api/v2/chats/config/advisor"},
		{http.MethodGet, "/api/v2/chats/00000000-0000-0000-0000-000000000000/debug/runs"},
		{http.MethodGet, "/api/v2/chats/00000000-0000-0000-0000-000000000000/stream/desktop"},
		{http.MethodGet, "/api/v2/mcp/servers/not-a-uuid/oauth2/callback"},
		{http.MethodPost, "/api/v2/mcp/http/server"},
	} {
		res, err := client.Request(ctx, route.method, route.path, nil)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode, "%s %s", route.method, route.path)
	}
}
