package toolsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func readinessWorkspace() codersdk.Workspace {
	return codersdk.Workspace{
		ID: uuid.New(), Name: "dev",
		LatestBuild: codersdk.WorkspaceBuild{
			ID: uuid.New(), Status: codersdk.WorkspaceStatusRunning,
			Transition: codersdk.WorkspaceTransitionStart,
			Job:        codersdk.ProvisionerJob{Status: codersdk.ProvisionerJobSucceeded},
			Resources: []codersdk.WorkspaceResource{{
				Agents: []codersdk.WorkspaceAgent{{
					ID: uuid.New(), Name: "main", Status: codersdk.WorkspaceAgentConnected,
					LifecycleState: codersdk.WorkspaceAgentLifecycleReady,
				}},
			}},
		},
	}
}

func readinessDeps(t *testing.T, serve func(http.ResponseWriter, *http.Request)) Deps {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.Equal(t, http.MethodGet, r.Method, "observation must not mutate the workspace") {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		serve(w, r)
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	deps, err := NewDeps(codersdk.New(u), WithAgentConnFunc(func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		t.Error("observation must not connect to the agent")
		return nil, nil, context.Canceled
	}))
	require.NoError(t, err)
	return deps
}

func TestWorkspaceReadinessStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, state string
		build       codersdk.WorkspaceStatus
		job         codersdk.ProvisionerJobStatus
		connection  codersdk.WorkspaceAgentStatus
		lifecycle   codersdk.WorkspaceAgentLifecycle
	}{
		{name: "Ready", state: "ready"},
		{name: "ColdBuildPending", build: codersdk.WorkspaceStatusPending, state: "pending"},
		{name: "ColdBuildStarting", build: codersdk.WorkspaceStatusStarting, state: "pending"},
		{name: "Stopped", build: codersdk.WorkspaceStatusStopped, state: "stopped"},
		{name: "Stopping", build: codersdk.WorkspaceStatusStopping, state: "stopping"},
		{name: "Deleting", build: codersdk.WorkspaceStatusDeleting, state: "deleting"},
		{name: "Deleted", build: codersdk.WorkspaceStatusDeleted, state: "deleted"},
		{name: "BuildFailed", job: codersdk.ProvisionerJobFailed, state: "build_failed"},
		{name: "BuildCanceled", job: codersdk.ProvisionerJobCanceled, state: "build_canceled"},
		{name: "Connecting", connection: codersdk.WorkspaceAgentConnecting, state: "pending"},
		{name: "Disconnected", connection: codersdk.WorkspaceAgentDisconnected, state: "disconnected"},
		{name: "ConnectionTimeout", connection: codersdk.WorkspaceAgentTimeout, state: "connection_timeout"},
		{name: "Starting", lifecycle: codersdk.WorkspaceAgentLifecycleStarting, state: "pending"},
		{name: "StartupError", lifecycle: codersdk.WorkspaceAgentLifecycleStartError, state: "start_error"},
		{name: "StartupTimeout", lifecycle: codersdk.WorkspaceAgentLifecycleStartTimeout, state: "start_timeout"},
		{name: "DisconnectedStartupError", connection: codersdk.WorkspaceAgentDisconnected, lifecycle: codersdk.WorkspaceAgentLifecycleStartError, state: "start_error"},
		{name: "ShuttingDown", lifecycle: codersdk.WorkspaceAgentLifecycleShuttingDown, state: "unavailable"},
		{name: "UnknownLifecycle", lifecycle: "future-state", state: "unavailable"},
		{name: "UnknownConnection", connection: "future-state", state: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ws := readinessWorkspace()
			if tt.build != "" {
				ws.LatestBuild.Status = tt.build
			}
			if tt.job != "" {
				ws.LatestBuild.Job.Status = tt.job
				ws.LatestBuild.Job.Error = "provisioning error"
			}
			agent := &ws.LatestBuild.Resources[0].Agents[0]
			if tt.connection != "" {
				agent.Status = tt.connection
			}
			if tt.lifecycle != "" {
				agent.LifecycleState = tt.lifecycle
			}
			if tt.build != "" {
				ws.LatestBuild.Resources = nil
			}
			var requests atomic.Int32
			deps := readinessDeps(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, "/api/v2/users/alice/workspace/dev", r.URL.Path)
				assert.NoError(t, json.NewEncoder(w).Encode(ws))
			})
			res, err := WorkspaceReadiness.Handler(testutil.Context(t, testutil.WaitLong), deps, WorkspaceReadinessArgs{Workspace: "alice--dev.main"})
			require.NoError(t, err)
			require.Equal(t, tt.state, res.State)
			require.Equal(t, tt.state == "ready", res.Ready)
			require.False(t, res.WaitExpired)
			require.Equal(t, ws.ID, res.WorkspaceID)
			require.Equal(t, ws.LatestBuild.ID, res.BuildID)
			require.Equal(t, ws.LatestBuild.Job.Error, res.BuildError)
			require.NotEmpty(t, res.Reason)
			require.EqualValues(t, 1, requests.Load())
		})
	}
}

func TestWorkspaceReadinessAgentSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, selector, wantError string }{
		{"Ambiguous", "dev", "multiple agents"},
		{"Missing", "dev.missing", "agent not found"},
		{"Explicit", "dev.main", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ws := readinessWorkspace()
			ws.LatestBuild.Resources[0].Agents = append(ws.LatestBuild.Resources[0].Agents, codersdk.WorkspaceAgent{
				ID: uuid.New(), Name: "other", Status: codersdk.WorkspaceAgentDisconnected,
			})
			deps := readinessDeps(t, func(w http.ResponseWriter, _ *http.Request) { assert.NoError(t, json.NewEncoder(w).Encode(ws)) })
			res, err := WorkspaceReadiness.Handler(t.Context(), deps, WorkspaceReadinessArgs{Workspace: tc.selector})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			require.True(t, res.Ready)
			require.Equal(t, ws.LatestBuild.Resources[0].Agents[0].ID, *res.AgentID)
		})
	}
}

func TestWorkspaceReadinessWait(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"ready", "build_failed", "start_error", "start_timeout", "expired", "canceled", "forbidden", "deleted", "blocked_request"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			clock := quartz.NewMock(t)
			tickTrap := clock.Trap().NewTicker("readinessPoll")
			defer tickTrap.Close()
			ws := readinessWorkspace()
			var requests atomic.Int32
			secondRequest := make(chan struct{}, 1)
			deps := readinessDeps(t, func(w http.ResponseWriter, r *http.Request) {
				switch requests.Add(1) {
				case 1:
					assert.Equal(t, "/api/v2/users/me/workspace/dev", r.URL.Path)
					cold := ws
					cold.LatestBuild.Status = codersdk.WorkspaceStatusPending
					cold.LatestBuild.Resources = nil
					assert.NoError(t, json.NewEncoder(w).Encode(cold))
				default:
					assert.Equal(t, "/api/v2/workspaces/"+ws.ID.String(), r.URL.Path)
					secondRequest <- struct{}{}
					if outcome == "blocked_request" {
						<-r.Context().Done()
						return
					}
					if outcome == "forbidden" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					if outcome == "deleted" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					next := ws
					switch outcome {
					case "expired":
						next.LatestBuild.Status = codersdk.WorkspaceStatusPending
					case "build_failed":
						next.LatestBuild.Job.Status = codersdk.ProvisionerJobFailed
					case "start_error":
						next.LatestBuild.Resources[0].Agents[0].LifecycleState = codersdk.WorkspaceAgentLifecycleStartError
					case "start_timeout":
						next.LatestBuild.Resources[0].Agents[0].LifecycleState = codersdk.WorkspaceAgentLifecycleStartTimeout
					}
					assert.NoError(t, json.NewEncoder(w).Encode(next))
				}
			})
			type result struct {
				response WorkspaceReadinessResponse
				err      error
			}
			done := make(chan result, 1)
			go func() {
				res, err := observeWorkspaceReadiness(ctx, deps, WorkspaceReadinessArgs{Workspace: "dev", WaitMs: 1500}, clock)
				done <- result{res, err}
			}()
			tickTrap.MustWait(ctx).MustRelease(ctx)
			switch outcome {
			case "expired":
				clock.Advance(time.Second).MustWait(ctx)
				testutil.RequireReceive(ctx, t, secondRequest)
				clock.Advance(500 * time.Millisecond).MustWait(ctx)
			case "canceled":
				cancel()
			default:
				clock.Advance(time.Second).MustWait(ctx)
				if outcome == "blocked_request" {
					testutil.RequireReceive(ctx, t, secondRequest)
					clock.Advance(500 * time.Millisecond).MustWait(ctx)
				}
			}
			got := testutil.RequireReceive(testutil.Context(t, testutil.WaitLong), t, done)
			switch outcome {
			case "canceled":
				require.ErrorIs(t, got.err, context.Canceled)
			case "forbidden", "deleted":
				var sdkErr *codersdk.Error
				require.ErrorAs(t, got.err, &sdkErr)
				if outcome == "forbidden" {
					require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
				} else {
					require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
				}
			case "expired", "blocked_request":
				require.NoError(t, got.err)
				require.Equal(t, "pending", got.response.State)
				require.False(t, got.response.Ready)
				require.True(t, got.response.WaitExpired)
			default:
				require.NoError(t, got.err)
				require.Equal(t, outcome, got.response.State)
				require.False(t, got.response.WaitExpired)
			}
		})
	}
}

func TestWorkspaceReadinessInitialRequestBound(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	requested := make(chan struct{}, 1)
	deps := readinessDeps(t, func(_ http.ResponseWriter, r *http.Request) {
		requested <- struct{}{}
		<-r.Context().Done()
	})
	done := make(chan error, 1)
	go func() {
		_, err := observeWorkspaceReadiness(ctx, deps, WorkspaceReadinessArgs{Workspace: "dev"}, clock)
		done <- err
	}()
	testutil.RequireReceive(ctx, t, requested)
	clock.Advance(maxReadinessWait).MustWait(ctx)
	require.Error(t, testutil.RequireReceive(ctx, t, done), "no successful snapshot exists to return")
}

func TestWorkspaceReadinessArgumentsAndRegistration(t *testing.T) {
	t.Parallel()
	for _, args := range []WorkspaceReadinessArgs{
		{}, {Workspace: " "}, {Workspace: "dev", WaitMs: -1}, {Workspace: "dev", WaitMs: 30001},
	} {
		_, err := WorkspaceReadiness.Handler(t.Context(), Deps{}, args)
		require.Error(t, err)
	}
	_, err := WorkspaceReadiness.Handler(t.Context(), Deps{}, WorkspaceReadinessArgs{Workspace: "dev"})
	require.ErrorContains(t, err, "authenticated client")
	tool := WorkspaceReadiness.Generic()
	for _, raw := range []string{
		`{"workspace":"dev","wait_ms":-1}`, `{"workspace":"dev","wait_ms":30001}`,
		`{"workspace":"dev","wait_ms":1.5}`, `{"workspace":""}`,
	} {
		_, err := tool.Handler(t.Context(), Deps{}, json.RawMessage(raw))
		var validationErr *ArgumentValidationError
		require.ErrorAs(t, err, &validationErr)
	}
	require.Equal(t, mcpReadOnlyAnnotations, tool.MCPAnnotations)
	require.False(t, tool.UserClientOptional)
	var matches int
	for _, registered := range All {
		if registered.Name == ToolNameWorkspaceReadiness {
			matches++
		}
	}
	require.Equal(t, 1, matches)
}
