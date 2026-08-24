package coderd_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

func TestChatRoutesCompatibility(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, nil)
	coderdtest.CreateFirstUser(t, client)

	promoted := []string{
		http.MethodGet + " /users/{user}/ai-provider-keys",
		http.MethodPut + " /users/{user}/ai-provider-keys/{aiProvider}",
		http.MethodDelete + " /users/{user}/ai-provider-keys/{aiProvider}",
		http.MethodGet + " /chats/files/{file}/download",
		http.MethodGet + " /organizations/{organization}/mcp-servers",
		http.MethodPost + " /organizations/{organization}/mcp-servers",
		http.MethodGet + " /organizations/{organization}/mcp-servers/{mcpserverconfig}",
		http.MethodPatch + " /organizations/{organization}/mcp-servers/{mcpserverconfig}",
		http.MethodDelete + " /organizations/{organization}/mcp-servers/{mcpserverconfig}",
		http.MethodGet + " /organizations/{organization}/mcp-servers/{mcpserverconfig}/acl",
		http.MethodPatch + " /organizations/{organization}/mcp-servers/{mcpserverconfig}/acl",
		http.MethodGet + " /organizations/{organization}/mcp-servers/{mcpserverconfig}/oauth2/connect",
		http.MethodGet + " /organizations/{organization}/chats/model-overrides",
		http.MethodPut + " /organizations/{organization}/chats/model-overrides/{context}",
		http.MethodGet + " /organizations/{organization}/members/{user}/chats/model-overrides",
		http.MethodPut + " /organizations/{organization}/members/{user}/chats/model-overrides/{context}",
		http.MethodGet + " /organizations/{organization}/chats/models",
		http.MethodPost + " /organizations/{organization}/chats/models",
		http.MethodGet + " /organizations/{organization}/chats/models/{model}",
		http.MethodPatch + " /organizations/{organization}/chats/models/{model}",
		http.MethodDelete + " /organizations/{organization}/chats/models/{model}",
		http.MethodGet + " /organizations/{organization}/chats/models/{model}/acl",
		http.MethodPatch + " /organizations/{organization}/chats/models/{model}/acl",
		http.MethodGet + " /chats/models",
		http.MethodGet + " /chats/by-workspace",
		http.MethodGet + " /chats",
		http.MethodPost + " /chats",
		http.MethodGet + " /chats/watch",
		http.MethodPost + " /chats/files",
		http.MethodPost + " /chats/files/{file}/download-url",
		http.MethodGet + " /chats/files/{file}",
		http.MethodGet + " /chats/config/system-prompt",
		http.MethodPut + " /chats/config/system-prompt",
		http.MethodGet + " /chats/config/plan-mode-instructions",
		http.MethodPut + " /chats/config/plan-mode-instructions",
		http.MethodGet + " /chats/config/personal-model-overrides",
		http.MethodPut + " /chats/config/personal-model-overrides",
		http.MethodGet + " /chats/config/debug-logging",
		http.MethodPut + " /chats/config/debug-logging",
		http.MethodGet + " /chats/config/user-debug-logging",
		http.MethodPut + " /chats/config/user-debug-logging",
		http.MethodGet + " /chats/config/user-prompt",
		http.MethodPut + " /chats/config/user-prompt",
		http.MethodGet + " /chats/config/user-compaction-thresholds",
		http.MethodPut + " /chats/config/user-compaction-thresholds/{modelConfig}",
		http.MethodDelete + " /chats/config/user-compaction-thresholds/{modelConfig}",
		http.MethodGet + " /chats/config/workspace-ttl",
		http.MethodPut + " /chats/config/workspace-ttl",
		http.MethodGet + " /chats/config/retention-days",
		http.MethodPut + " /chats/config/retention-days",
		http.MethodGet + " /chats/config/debug-retention-days",
		http.MethodPut + " /chats/config/debug-retention-days",
		http.MethodGet + " /chats/config/auto-archive-days",
		http.MethodPut + " /chats/config/auto-archive-days",
		http.MethodGet + " /chats/{chat}/acl",
		http.MethodPatch + " /chats/{chat}/acl",
		http.MethodGet + " /chats/{chat}",
		http.MethodPatch + " /chats/{chat}",
		http.MethodGet + " /chats/{chat}/cost",
		http.MethodGet + " /chats/{chat}/messages",
		http.MethodPost + " /chats/{chat}/messages",
		http.MethodPatch + " /chats/{chat}/messages/{message}",
		http.MethodGet + " /chats/{chat}/prompts",
		http.MethodGet + " /chats/{chat}/stream",
		http.MethodGet + " /chats/{chat}/stream/parts",
		http.MethodGet + " /chats/{chat}/stream/git",
		http.MethodPost + " /chats/{chat}/interrupt",
		http.MethodPost + " /chats/{chat}/compact",
		http.MethodPost + " /chats/{chat}/reconcile-invalid",
		http.MethodPost + " /chats/{chat}/tool-results",
		http.MethodPost + " /chats/{chat}/title/regenerate",
		http.MethodPost + " /chats/{chat}/title/propose",
		http.MethodGet + " /chats/{chat}/diff",
		http.MethodPut + " /chats/{chat}/context",
		http.MethodDelete + " /chats/{chat}/queue/{queuedMessage}",
		http.MethodPost + " /chats/{chat}/queue/{queuedMessage}/promote",
		http.MethodGet + " /mcp/servers/{mcpServer}/oauth2/callback",
		http.MethodDelete + " /mcp/servers/{mcpServer}/oauth2/disconnect",
	}

	for name, router := range map[string]chi.Router{
		"experimental": api.ExperimentalHandler,
		"v2":           api.APIHandler,
	} {
		routes := walkRoutes(t, router)
		for _, route := range promoted {
			require.Contains(t, routes, route, "%s route tree", name)
		}
	}

	v2Routes := walkRoutes(t, api.APIHandler)
	for _, excluded := range []string{
		http.MethodGet + " /chats/providers",
		http.MethodPost + " /chats/providers",
		http.MethodPatch + " /chats/providers/{providerConfig}",
		http.MethodDelete + " /chats/providers/{providerConfig}",
		http.MethodGet + " /chats/user-provider-configs",
		http.MethodPut + " /chats/user-provider-configs/{providerConfig}",
		http.MethodDelete + " /chats/user-provider-configs/{providerConfig}",
		http.MethodGet + " /chats/{chat}/debug/runs",
		http.MethodGet + " /chats/{chat}/debug/runs/{debugRun}",
		http.MethodGet + " /chats/config/computer-use-provider",
		http.MethodPut + " /chats/config/computer-use-provider",
		http.MethodGet + " /chats/config/advisor",
		http.MethodPut + " /chats/config/advisor",
		http.MethodGet + " /chats/{chat}/stream/desktop",
		http.MethodGet + " /chats/model-configs",
		http.MethodPost + " /chats/model-configs",
	} {
		require.NotContains(t, v2Routes, excluded)
	}
	for route := range v2Routes {
		require.NotContains(t, route, " /mcp/http")
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	for _, excluded := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/chats/config/computer-use-provider"},
		{http.MethodPut, "/api/v2/chats/config/computer-use-provider"},
		{http.MethodGet, "/api/v2/chats/config/advisor"},
		{http.MethodPut, "/api/v2/chats/config/advisor"},
		{http.MethodPost, "/api/v2/mcp/http/server"},
		{http.MethodGet, "/api/v2/chats/00000000-0000-0000-0000-000000000000/debug/runs"},
		{http.MethodGet, "/api/v2/chats/00000000-0000-0000-0000-000000000000/stream/desktop"},
		// Experimental-only segments that would otherwise fall into the
		// {chat} wildcard and 400 on UUID parsing.
		{http.MethodGet, "/api/v2/chats/model-configs"},
		{http.MethodPost, "/api/v2/chats/model-configs"},
		{http.MethodGet, "/api/v2/chats/providers"},
		{http.MethodGet, "/api/v2/chats/providers/00000000-0000-0000-0000-000000000000"},
		{http.MethodGet, "/api/v2/chats/user-provider-configs"},
	} {
		res, err := client.Request(ctx, excluded.method, excluded.path, nil)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode, "%s %s", excluded.method, excluded.path)
	}

	for _, path := range []string{
		"/api/v2/chats",
		"/api/v2/chats/config/system-prompt",
	} {
		res, err := client.Request(ctx, http.MethodGet, path, nil)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode, path)
	}
}

func walkRoutes(t *testing.T, router chi.Router) map[string]struct{} {
	t.Helper()

	routes := make(map[string]struct{})
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/")
		routes[method+" "+route] = struct{}{}
		return nil
	})
	require.NoError(t, err)
	return routes
}
