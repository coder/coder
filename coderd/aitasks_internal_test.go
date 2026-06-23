package coderd

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestDeriveTaskCurrentState_Unit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name               string
		task               database.Task
		agentLifecycle     *codersdk.WorkspaceAgentLifecycle
		appHealth          *codersdk.WorkspaceAppHealth
		latestAppStatus    *codersdk.WorkspaceAppStatus
		latestBuild        codersdk.WorkspaceBuild
		expectCurrentState bool
		expectedTimestamp  time.Time
		expectedState      codersdk.TaskState
		expectedMessage    string
	}{
		{
			name: "NoAppStatus",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusActive,
			},
			agentLifecycle:  nil,
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Transition: codersdk.WorkspaceTransitionStart,
				CreatedAt:  now,
			},
			expectCurrentState: false,
		},
		{
			name: "BuildStartTransition_AppStatus_NewerThanBuild",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusActive,
			},
			agentLifecycle: nil,
			appHealth:      nil,
			latestAppStatus: &codersdk.WorkspaceAppStatus{
				State:     codersdk.WorkspaceAppStatusStateWorking,
				Message:   "Task is working",
				CreatedAt: now.Add(1 * time.Minute),
			},
			latestBuild: codersdk.WorkspaceBuild{
				Transition: codersdk.WorkspaceTransitionStart,
				CreatedAt:  now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now.Add(1 * time.Minute),
			expectedState:      codersdk.TaskState(codersdk.WorkspaceAppStatusStateWorking),
			expectedMessage:    "Task is working",
		},
		{
			name: "BuildStartTransition_StaleAppStatus_OlderThanBuild",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusActive,
			},
			agentLifecycle: nil,
			appHealth:      nil,
			latestAppStatus: &codersdk.WorkspaceAppStatus{
				State:     codersdk.WorkspaceAppStatusStateComplete,
				Message:   "Previous task completed",
				CreatedAt: now.Add(-1 * time.Minute),
			},
			latestBuild: codersdk.WorkspaceBuild{
				Transition: codersdk.WorkspaceTransitionStart,
				CreatedAt:  now,
			},
			expectCurrentState: false,
		},
		{
			name: "BuildStopTransition",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusActive,
			},
			agentLifecycle: nil,
			appHealth:      nil,
			latestAppStatus: &codersdk.WorkspaceAppStatus{
				State:     codersdk.WorkspaceAppStatusStateComplete,
				Message:   "Task completed before stop",
				CreatedAt: now.Add(-1 * time.Minute),
			},
			latestBuild: codersdk.WorkspaceBuild{
				Transition: codersdk.WorkspaceTransitionStop,
				CreatedAt:  now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now.Add(-1 * time.Minute),
			expectedState:      codersdk.TaskState(codersdk.WorkspaceAppStatusStateComplete),
			expectedMessage:    "Task completed before stop",
		},
		{
			name: "TaskInitializing_WorkspacePending",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  nil,
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusPending,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "Workspace is pending",
		},
		{
			name: "TaskInitializing_WorkspaceStarting",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  nil,
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusStarting,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "Workspace is starting",
		},
		{
			name: "TaskInitializing_AgentConnecting",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  ptr.Ref(codersdk.WorkspaceAgentLifecycleCreated),
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusRunning,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "Agent is connecting",
		},
		{
			name: "TaskInitializing_AgentStarting",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  ptr.Ref(codersdk.WorkspaceAgentLifecycleStarting),
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusRunning,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "Agent is starting",
		},
		{
			name: "TaskInitializing_AppInitializing",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
			appHealth:       ptr.Ref(codersdk.WorkspaceAppHealthInitializing),
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusRunning,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "App is initializing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ws := codersdk.Workspace{
				LatestBuild:     tt.latestBuild,
				LatestAppStatus: tt.latestAppStatus,
			}

			currentState := deriveTaskCurrentState(tt.task, ws, tt.agentLifecycle, tt.appHealth)

			if tt.expectCurrentState {
				require.NotNil(t, currentState)
				assert.Equal(t, tt.expectedTimestamp.UTC(), currentState.Timestamp.UTC())
				assert.Equal(t, tt.expectedState, currentState.State)
				assert.Equal(t, tt.expectedMessage, currentState.Message)
			} else {
				assert.Nil(t, currentState)
			}
		})
	}
}

// TestTaskAppHTTPClient_RejectsRedirect verifies the client built for dialing a
// workspace task app never follows redirects. A malicious app must not be able
// to bounce coderd's request to a different address via a 3xx Location header.
func TestTaskAppHTTPClient_RejectsRedirect(t *testing.T) {
	t.Parallel()

	// victim stands in for a different address (e.g. a different port) that a
	// followed redirect would reach. It must never be contacted.
	var victimHits atomic.Int64
	victim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		victimHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer victim.Close()

	// The task app redirects every request to the victim address.
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, victim.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer app.Close()

	// The dial honors addr like the real agent transport, so following the
	// redirect would actually reach the victim.
	client := taskAppHTTPClient(func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, app.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode, "redirect must be surfaced, not followed")
	require.Zero(t, victimHits.Load(), "redirect target must not be contacted")
}
