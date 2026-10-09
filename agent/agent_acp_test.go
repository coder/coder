package agent_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

type acpEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e acpEnvInfo) HomeDir() (string, error) { return e.home, nil }

func TestAgent_ACPInterface(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), t.TempDir()
	configDir := filepath.Join(home, ".coder", "acp")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "broken.json"), []byte(`{"command":"false"}`), 0o600))
	//nolint:dogsled // The integration fixture also returns unrelated transports.
	conn, client, _, _, _ := setupAgent(t, agentsdk.Manifest{Directory: dir}, 0, func(_ *agenttest.Client, opts *agent.Options) {
		opts.EnvInfo = acpEnvInfo{home: home}
		opts.Filesystem = afero.NewOsFs()
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := conn.ListACPHarnesses(ctx)
	require.NoError(t, err)
	tool := uuid.New()
	// Even a supplied tool-call header must bypass response recording for ACP.
	conn.SetExtraHeaders(http.Header{workspacesdk.CoderToolCallIDHeader: {tool.String()}})
	var catalog []workspacesdk.ACPHarness
	require.Eventually(t, func() bool { catalog, err = conn.ListACPHarnesses(ctx); return err == nil && len(catalog) == 1 }, testutil.WaitLong, testutil.IntervalFast)
	require.Equal(t, "broken", catalog[0].Slug)
	listed, err := conn.ListACPSessions(ctx)
	require.NoError(t, err)
	require.Empty(t, listed)
	require.Eventually(t, func() bool {
		for _, push := range client.ContextStatePushes() {
			for _, resource := range push.Resources {
				if harness := resource.GetAcpHarness(); harness != nil && harness.Slug == "broken" {
					return true
				}
			}
		}
		return false
	}, testutil.WaitLong, testutil.IntervalFast, "context pushes must include the harness catalog")
}
