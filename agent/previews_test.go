//go:build linux || darwin

package agent_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentOpenPreviewIntegration(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	selected := filepath.Join(root, "page.html")
	require.NoError(t, os.WriteFile(selected, []byte("<h1>preview</h1>"), 0o600))
	//nolint:dogsled // Only the SDK connection is used.
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
	ctx := testutil.Context(t, testutil.WaitLong)

	publication, err := conn.OpenPreview(ctx, workspacesdk.OpenPreviewRequest{Path: selected})
	require.NoError(t, err)
	require.Equal(t, selected, publication.Path)

	// AppHTTPClient dials the agent's tailnet address, like the workspace
	// app proxy does.
	url := fmt.Sprintf("http://127.0.0.1:%d/%s/", workspacesdk.AgentPreviewPort, publication.ID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	response, err := conn.AppHTTPClient().Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "<h1>preview</h1>", string(body))

	// Workspace processes cannot reach previews through the OS loopback.
	response, err = http.DefaultClient.Do(request)
	if err == nil {
		_ = response.Body.Close()
	}
	require.Error(t, err)
}
