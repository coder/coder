package coderd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestAIAgentIdentity is the acceptance test for work package WP1 in
// poc_audit/work_breakdown.md. It is being built incrementally.
//
// Revision 1 establishes only that the harness works: that a script attached
// to a workspace agent in a fake build reaches the agent's manifest and is
// executed. It asserts nothing about AI agent identity yet, because none of
// that code exists.
//
// The script is run from the manifest rather than driven directly by the
// test, so that later revisions exercise the whole call path instead of
// stubbing its first hop.
func TestAIAgentIdentity(t *testing.T) {
	t.Parallel()

	t.Run("HarnessRunsStartupScript", func(t *testing.T) {
		t.Parallel()

		// The marker is how we observe that the script ran. The agent runs in
		// this process, so a file it touches is visible to the test.
		markerDir := t.TempDir()
		marker := filepath.Join(markerDir, "ai-agent-probe-ran")

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			for _, agent := range agents {
				agent.Scripts = append(agent.Scripts, &proto.Script{
					DisplayName: "ai-agent-probe",
					Script:      fmt.Sprintf("touch %q", marker),
					RunOnStart:  true,
					LogPath:     "ai-agent-probe.log",
				})
			}
			return agents
		}).Do()

		_ = agenttest.New(t, client.URL, r.AgentToken)
		coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).AgentNames([]string{}).Wait()

		require.Eventually(t, func() bool {
			_, err := os.Stat(marker)
			return err == nil
		}, testutil.WaitLong, testutil.IntervalMedium,
			"startup script from the manifest did not run; marker %q was never created", marker)

		// Guard against the marker existing for some reason other than the
		// script having run.
		info, err := os.Stat(marker)
		require.NoError(t, err)
		assert.False(t, info.IsDir(), "marker should be a file created by the script")
	})
}
