//go:build !slim

package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestExpChatContextRefreshAgent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	client, _, api := coderdtest.NewWithAPI(t, nil)
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	workDir := t.TempDir()
	extraDir := filepath.Join(workDir, "extra")
	require.NoError(t, os.Mkdir(extraDir, 0o700))
	extraAgents := filepath.Join(extraDir, "AGENTS.md")
	require.NoError(t, os.WriteFile(extraAgents, []byte("extra instructions"), 0o600))

	build := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        owner.UserID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Directory = workDir
		return agents
	}).Do()
	model := dbgen.ChatModelConfig(t, api.Database, database.ChatModelConfig{})
	chat := dbgen.Chat(t, api.Database, database.Chat{
		OrganizationID:    owner.OrganizationID,
		OwnerID:           owner.UserID,
		WorkspaceID:       uuid.NullUUID{UUID: build.Workspace.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: build.Agents[0].ID, Valid: true},
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
	})

	socketPath := testutil.AgentSocketPath(t)
	// An empty ACP configuration home prevents probing host-installed harnesses.
	home := t.TempDir()
	agenttest.New(t, client.URL, build.AgentToken, func(o *agent.Options) {
		o.EnvInfo = contextRefreshEnvInfo{home: home}
		o.SocketServerEnabled = true
		o.SocketPath = socketPath
	})
	coderdtest.NewWorkspaceAgentWaiter(t, client, build.Workspace.ID).Wait()
	require.Eventually(t, func() bool {
		got, err := expClient.GetChat(ctx, chat.ID)
		return err == nil && got.Context != nil
	}, testutil.WaitLong, testutil.IntervalFast)

	run := func(args ...string) string {
		t.Helper()
		args = append(args, "--socket-path", socketPath)
		inv, _ := clitest.New(t, append([]string{"exp", "chat", "context"}, args...)...)
		var out bytes.Buffer
		inv.Stdout = &out
		inv.Stderr = &out
		require.NoError(t, inv.WithContext(ctx).Run(), out.String())
		return out.String()
	}
	refresh := func() string {
		t.Helper()
		out := run("refresh", "--agent-url", client.URL.String(), "--agent-token", build.AgentToken)
		require.Regexp(t, `^Resolved agent context \([0-9]+ resources\); coderd stored it or a newer snapshot\.\n`, out)
		require.Regexp(t, `(?m)^Refreshed [0-9]+ drifted chat\(s\)\.\n$`, out)
		return out
	}
	assertPinned := func(wantExtra bool) {
		t.Helper()
		got, err := expClient.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, got.Context)
		var hasExtra bool
		for _, res := range got.Context.Resources {
			if res.Source == extraAgents {
				hasExtra = true
			}
		}
		require.Equal(t, wantExtra, hasExtra)
		require.False(t, got.Context.Dirty, "refresh must clear drift before returning")
	}

	// Repeat the workflow to exercise watcher and push ordering.
	const refreshCycles = 5
	for range refreshCycles {
		run("add", extraDir)
		t.Log(refresh())
		assertPinned(true)
		run("remove", extraDir)
		t.Log(refresh())
		assertPinned(false)
	}
	require.Contains(t, refresh(), "Refreshed 0 drifted chat(s).")
	assertPinned(false)
}

type contextRefreshEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e contextRefreshEnvInfo) HomeDir() (string, error) { return e.home, nil }
