package coderd_test

import (
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

	for _, route := range []string{
		"/api/v2/tasks",
		"/api/v2/tasks/me",
		"/api/experimental/tasks",
		"/api/experimental/tasks/me",
	} {
		res, err := client.Request(ctx, http.MethodGet, route, nil)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode, "route %s should not be registered", route)
	}

	// Sanity check that unrelated routes still work, so the assertions above
	// are not passing because the whole API is broken.
	res, err := client.Request(ctx, http.MethodGet, "/api/v2/workspaces", nil)
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}
