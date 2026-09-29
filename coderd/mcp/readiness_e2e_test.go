package mcp_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// Readiness is deliberately a control-plane observation. These fixtures set
// real persisted build/agent states; the ready case uses a live workspace agent.
//
//nolint:paralleltest,tparallel // Subtests advance the same mock clock sequentially.
func TestMCPHTTP_WorkspaceReadiness(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	owner, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: mcpDeploymentValues(t), Clock: clock,
	})
	defer closer.Close()
	first := coderdtest.CreateFirstUser(t, owner)
	client, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)
	session, err := newIsolatedMCPClient(ctx, api.AccessURL.String()+mcpserver.MCPEndpoint, "readiness", map[string]string{
		"Authorization": "Bearer " + client.SessionToken(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	connectAgent := func(id uuid.UUID, state database.WorkspaceAgentLifecycleState) {
		now := time.Now()
		require.NoError(t, api.Database.UpdateWorkspaceAgentConnectionByID(systemCtx, database.UpdateWorkspaceAgentConnectionByIDParams{
			ID: id, FirstConnectedAt: sql.NullTime{Time: now, Valid: true}, LastConnectedAt: sql.NullTime{Time: now, Valid: true},
			UpdatedAt: now,
		}))
		require.NoError(t, api.Database.UpdateWorkspaceAgentLifecycleStateByID(systemCtx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID: id, LifecycleState: state, StartedAt: sql.NullTime{Time: now, Valid: true}, ReadyAt: sql.NullTime{Time: now, Valid: true},
		}))
	}

	for _, tc := range []struct {
		name, state string
		waitMs      int
		lifecycle   database.WorkspaceAgentLifecycleState
	}{
		{name: "stopped", state: "stopped"},
		{name: "cold_build", state: "pending"},
		{name: "build_failure", state: "build_failed"},
		{name: "startup_failure", state: "start_error", lifecycle: database.WorkspaceAgentLifecycleStateStartError},
		{name: "ready", state: "ready", lifecycle: database.WorkspaceAgentLifecycleStateReady},
		{name: "wait_timeout", state: "pending", waitMs: 1500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			builder := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{OwnerID: user.ID, OrganizationID: first.OrganizationID})
			switch tc.name {
			case "stopped":
				builder = builder.Seed(database.WorkspaceBuild{Transition: database.WorkspaceTransitionStop})
			case "cold_build":
				builder = builder.Pending()
			case "build_failure":
				builder = builder.Failed(dbfake.WithJobError("fixture provisioning failure"))
			default:
				builder = builder.WithAgent()
			}
			fixture := builder.Do()
			agentID := uuid.Nil
			if len(fixture.Agents) > 0 {
				agentID = fixture.Agents[0].ID
			}
			if tc.name == "ready" {
				_ = agenttest.New(t, client.URL, fixture.AgentToken)
				coderdtest.NewWorkspaceAgentWaiter(t, client, fixture.Workspace.ID).WaitFor(coderdtest.AgentsReady)
			} else if tc.lifecycle != "" {
				connectAgent(agentID, tc.lifecycle)
			}
			before, err := api.Database.GetWorkspaceByID(systemCtx, fixture.Workspace.ID)
			require.NoError(t, err)
			buildsBefore, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{WorkspaceID: before.ID})
			require.NoError(t, err)
			type result struct {
				response *mcp.CallToolResult
				err      error
			}
			done := make(chan result, 1)
			var trap *quartz.Trap
			// Advance the production tool's injected clock only after its initial
			// real API read. No wall-clock sleep or alternate handler is involved.
			if tc.waitMs > 0 {
				trap = clock.Trap().NewTicker("readinessPoll")
			}
			go func() {
				response, err := session.CallTool(ctx, &mcp.CallToolParams{
					Name:      toolsdk.ToolNameWorkspaceReadiness,
					Arguments: toolsdk.WorkspaceReadinessArgs{Workspace: before.ID.String(), WaitMs: tc.waitMs},
				})
				done <- result{response, err}
			}()
			if trap != nil {
				tick := trap.MustWait(ctx)
				tick.MustRelease(ctx)
				clock.Advance(time.Second).MustWait(ctx)
				clock.Advance(500 * time.Millisecond).MustWait(ctx)
				trap.Close()
			}
			got := testutil.RequireReceive(ctx, t, done)
			require.NoError(t, got.err)
			require.False(t, got.response.IsError, "%+v", got.response.Content)
			require.Len(t, got.response.Content, 1)
			content, ok := got.response.Content[0].(*mcp.TextContent)
			require.True(t, ok)
			var observed toolsdk.WorkspaceReadinessResponse
			require.NoError(t, json.Unmarshal([]byte(content.Text), &observed))
			require.Equal(t, tc.state, observed.State)
			require.Equal(t, tc.state == "ready", observed.Ready)
			require.Equal(t, tc.name == "wait_timeout", observed.WaitExpired)
			require.Equal(t, before.ID, observed.WorkspaceID)
			require.Equal(t, buildsBefore[0].ID, observed.BuildID)
			if tc.name == "build_failure" {
				require.Equal(t, "fixture provisioning failure", observed.BuildError)
			}
			after, err := api.Database.GetWorkspaceByID(systemCtx, before.ID)
			require.NoError(t, err)
			buildsAfter, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{WorkspaceID: before.ID})
			require.NoError(t, err)
			require.Equal(t, before, after, "readiness must not renew activity or change the workspace")
			require.Len(t, buildsAfter, len(buildsBefore), "readiness must not create a build")
			for i, build := range buildsBefore {
				require.Equal(t, build.ID, buildsAfter[i].ID)
				require.Equal(t, build.Transition, buildsAfter[i].Transition)
				require.Equal(t, build.Deadline, buildsAfter[i].Deadline, "readiness must not renew a deadline")
				require.Equal(t, build.MaxDeadline, buildsAfter[i].MaxDeadline)
			}
		})
	}
}
