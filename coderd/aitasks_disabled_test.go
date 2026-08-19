package coderd_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

// TestTasksDisabled asserts that a deployment without CODER_ENABLE_AI_TASKS
// serves no Tasks routes, on either the stable or the experimental prefix.
func TestTasksDisabled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	values := coderdtest.DeploymentValues(t)
	values.EnableAITasks = false

	client := coderdtest.New(t, &coderdtest.Options{DeploymentValues: values})
	coderdtest.CreateFirstUser(t, client)

	// Only the user-facing routes are asserted here. The agent-side
	// /workspaceagents/me/tasks route is also gated, but it sits behind agent
	// authentication, so a user token cannot tell a missing route from a
	// rejected one. TestEndpointsDocumented covers its absence, since an
	// undocumented registered route fails that test.
	for _, route := range []string{
		"/api/v2/tasks",
		"/api/v2/tasks/me",
		"/api/experimental/tasks",
		"/api/experimental/tasks/me",
	} {
		res, err := client.Request(ctx, http.MethodGet, route, nil)
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, wantStatus, res.StatusCode, "route %s should not be registered", route)
		require.Equal(t, string(wantBody), string(body), "route %s should be indistinguishable from a route that never existed", route)
	}

	// Sanity check that unrelated routes still work, so the assertions above
	// are not passing because the whole API is broken.
	res, err = client.Request(ctx, http.MethodGet, "/api/v2/workspaces", nil)
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}
