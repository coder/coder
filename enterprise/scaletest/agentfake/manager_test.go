package agentfake_test

import (
	"context"
	"database/sql"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/scaletest/agentfake"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// Asserts the TokenInfo shape (workspace IDs, agent names, tokens) returned by the enumeration loop.
func Test_Manager_EnumerateExternalAgents_returnsAllTokens(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceExternalAgent: 1,
			},
		},
	})

	const numWorkspaces = 3
	first := buildExternalAgentWorkspace(t, db, user, uuid.Nil)
	templateID := first.Workspace.TemplateID
	want := []agentfake.TokenInfo{{
		WorkspaceID:   first.Workspace.ID,
		WorkspaceName: first.Workspace.Name,
		AgentID:       first.Agents[0].ID,
		AgentName:     first.Agents[0].Name,
		Token:         first.AgentToken,
	}}
	for i := 1; i < numWorkspaces; i++ {
		r := buildExternalAgentWorkspace(t, db, user, templateID)
		want = append(want, agentfake.TokenInfo{
			WorkspaceID:   r.Workspace.ID,
			WorkspaceName: r.Workspace.Name,
			AgentID:       r.Agents[0].ID,
			AgentName:     r.Agents[0].Name,
			Token:         r.AgentToken,
		})
	}

	tmpl, err := client.Template(ctx, templateID)
	require.NoError(t, err)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := agentfake.NewManager(client, logger, agentfake.ManagerOptions{Template: tmpl.Name})

	got, err := m.EnumerateExternalAgents(ctx)
	require.NoError(t, err)

	// Order returned by coderd isn't guaranteed; sort both sides by WorkspaceID before comparing.
	sortTokenInfosByWorkspaceID(want)
	sortTokenInfosByWorkspaceID(got)

	require.Equal(t, len(want), len(got),
		"expected one TokenInfo per external-agent workspace under the template")
	for i := range want {
		assert.Equal(t, want[i].WorkspaceID, got[i].WorkspaceID, "WorkspaceID for entry %d", i)
		assert.Equal(t, want[i].AgentName, got[i].AgentName, "AgentName for entry %d", i)
		assert.Equal(t, want[i].Token, got[i].Token, "Token for entry %d", i)
		assert.NotEmpty(t, got[i].Token, "Token must be non-empty for entry %d", i)
	}
}

// Heavier-weight integration test for the agentfake harness: builds 5 external agents, sets up the client/Manager,
// and asserts that each of the agents the Manager sees via its enumeration function is properly connected and Ready.
func TestManager_FiveAgentsHeartbeat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceExternalAgent: 1,
			},
		},
	})

	const numAgents = 5
	first := buildExternalAgentWorkspace(t, db, user, uuid.Nil)
	templateID := first.Workspace.TemplateID
	workspaceIDs := []uuid.UUID{first.Workspace.ID}
	for i := 1; i < numAgents; i++ {
		r := buildExternalAgentWorkspace(t, db, user, templateID)
		workspaceIDs = append(workspaceIDs, r.Workspace.ID)
	}

	tmpl, err := client.Template(ctx, templateID)
	require.NoError(t, err)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	manager := agentfake.NewManager(client, logger, agentfake.ManagerOptions{
		Template: tmpl.Name,
	})
	t.Cleanup(func() { _ = manager.Close() })

	managerCtx, cancelManager := context.WithCancel(ctx)
	t.Cleanup(cancelManager)

	managerErr := make(chan error, 1)
	go func() {
		managerErr <- manager.Run(managerCtx)
	}()

	// Each workspace's agent must reach Connected. Share the outer test ctx (testutil.WaitLong) across all five waiters
	// so the total wait is bounded.
	for _, wsID := range workspaceIDs {
		coderdtest.NewWorkspaceAgentWaiter(t, client, wsID).WithContext(ctx).Wait()
	}

	// Each workspace's agent must also reach Lifecycle=ready. The fake sends UpdateLifecycle(READY) once per dRPC
	// connect; coderd persists that and exposes it on the agent.
	for _, wsID := range workspaceIDs {
		require.Eventually(t, func() bool {
			ws, err := client.Workspace(ctx, wsID)
			if err != nil {
				return false
			}
			for _, res := range ws.LatestBuild.Resources {
				for _, agent := range res.Agents {
					if agent.LifecycleState != codersdk.WorkspaceAgentLifecycleReady {
						return false
					}
				}
			}
			return true
		}, testutil.WaitLong, testutil.IntervalFast,
			"agent never reached Lifecycle=ready in workspace %s", wsID)
	}

	// Cleanly stop the Manager and confirm it exits without a non-context error.
	cancelManager()
	select {
	case err := <-managerErr:
		if err != nil {
			t.Fatalf("Manager.Run returned unexpected error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for Manager.Run to return: %v", ctx.Err())
	}
}

// Asserts that an authentication failure during enumeration produces a fatal error, so the retry loop in
// enumerateWithRetry surfaces it immediately rather than hammering endpoints with credentials that will never work.
func Test_Manager_EnumerateExternalAgents_invalidTokenIsFatal(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	r := buildExternalAgentWorkspace(t, db, user, uuid.Nil)
	tmpl, err := client.Template(ctx, r.Workspace.TemplateID)
	require.NoError(t, err)

	// Replace the client's session token with garbage to provoke a 401 from coderd's workspace-list endpoint.
	// The Manager should surface that as a fatal error.
	client.SetSessionToken("not-a-valid-session-token")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := agentfake.NewManager(client, logger, agentfake.ManagerOptions{Template: tmpl.Name})

	_, err = m.EnumerateExternalAgents(ctx)
	require.Error(t, err, "expected enumeration to fail with an invalid session token")
	require.True(t, agentfake.IsFatalEnumerationError(err),
		"expected error to be classified as fatal so the harness exits and Kubernetes can restart it; got: %v", err)
}

func sortTokenInfosByWorkspaceID(s []agentfake.TokenInfo) {
	sort.Slice(s, func(i, j int) bool {
		return s[i].WorkspaceID.String() < s[j].WorkspaceID.String()
	})
}

// buildExternalAgentWorkspace creates one workspace with a coder_external_agent resource, an agent, and
// HasExternalAgent=true on the latest build. If templateID is uuid.Nil, dbfake mints a fresh template (and the caller
// can pass the returned Workspace.TemplateID into subsequent calls to share the template).
func buildExternalAgentWorkspace(
	t *testing.T,
	db database.Store,
	user codersdk.CreateFirstUserResponse,
	templateID uuid.UUID,
) dbfake.WorkspaceResponse {
	t.Helper()

	ws := database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}
	if templateID != uuid.Nil {
		ws.TemplateID = templateID
	}
	return dbfake.WorkspaceBuild(t, db, ws).
		Seed(database.WorkspaceBuild{
			HasExternalAgent: sql.NullBool{Bool: true, Valid: true},
		}).
		Resource(&sdkproto.Resource{
			Name: "external",
			Type: "coder_external_agent",
		}).
		WithAgent().
		Do()
}
