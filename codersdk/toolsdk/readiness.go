package toolsdk

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsrelated"
	"github.com/coder/quartz"
)

// WorkspaceReadinessArgs selects a workspace agent and bounds observation.
type WorkspaceReadinessArgs struct {
	Workspace string `json:"workspace"`
	WaitMs    int    `json:"wait_ms,omitempty"`
}

// WorkspaceReadinessResponse is a control-plane snapshot, not a connectivity
// probe or a guarantee that a subsequent operation will succeed.
type WorkspaceReadinessResponse struct {
	WorkspaceID      uuid.UUID                        `json:"workspace_id"`
	BuildID          uuid.UUID                        `json:"build_id"`
	BuildStatus      codersdk.WorkspaceStatus         `json:"build_status"`
	BuildTransition  codersdk.WorkspaceTransition     `json:"build_transition"`
	BuildError       string                           `json:"build_error,omitempty"`
	AgentID          *uuid.UUID                       `json:"agent_id,omitempty"`
	AgentName        string                           `json:"agent_name,omitempty"`
	ConnectionStatus codersdk.WorkspaceAgentStatus    `json:"connection_status,omitempty"`
	LifecycleState   codersdk.WorkspaceAgentLifecycle `json:"lifecycle_state,omitempty"`
	State            string                           `json:"state"`
	Reason           string                           `json:"reason"`
	Ready            bool                             `json:"ready"`
	WaitExpired      bool                             `json:"wait_expired"`
}

const maxReadinessWait = 30 * time.Second

// WorkspaceReadiness observes readiness without starting or connecting to a
// workspace. Startup failures are returned as state, not hidden as warnings.
var WorkspaceReadiness = Tool[WorkspaceReadinessArgs, WorkspaceReadinessResponse]{
	Tool: aisdk.Tool{
		Name: ToolNameWorkspaceReadiness,
		Description: "Observe workspace build and agent startup readiness without starting, connecting to, or extending workspace activity. " +
			"wait_ms defaults to 0 (one snapshot) and is bounded to 30000 milliseconds, including API requests. " +
			"Single-snapshot API requests are also bounded to 30 seconds. " +
			"Returns immediately on readiness or a state requiring action. On wait expiry, returns the last snapshot with wait_expired=true. " +
			"ready means the control plane reports a running build, a connected agent, and successful startup; it does not probe connectivity. " +
			"Use the existing workspace build and log tools to act on the reason, then observe again.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": workspaceAgentDescription,
					"minLength":   1,
				},
				"wait_ms": map[string]any{
					"type":        "integer",
					"description": "Maximum observation duration in milliseconds; 0 returns one snapshot.",
					"minimum":     0,
					"maximum":     maxReadinessWait.Milliseconds(),
					"default":     0,
				},
			},
			Required: []string{"workspace"},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceReadinessArgs) (WorkspaceReadinessResponse, error) {
		return observeWorkspaceReadiness(ctx, deps, args, quartz.NewReal())
	},
}

func observeWorkspaceReadiness(ctx context.Context, deps Deps, args WorkspaceReadinessArgs, clock quartz.Clock) (WorkspaceReadinessResponse, error) {
	if strings.TrimSpace(args.Workspace) == "" {
		return WorkspaceReadinessResponse{}, xerrors.New("workspace name cannot be empty")
	}
	if args.WaitMs < 0 || args.WaitMs > int(maxReadinessWait.Milliseconds()) {
		return WorkspaceReadinessResponse{}, xerrors.New("wait_ms must be between 0 and 30000")
	}
	if deps.coderClient == nil {
		return WorkspaceReadinessResponse{}, xerrors.New("workspace readiness requires an authenticated client")
	}

	duration := time.Duration(args.WaitMs) * time.Millisecond
	if duration == 0 {
		duration = maxReadinessWait
	}
	waitCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	timer := clock.AfterFunc(duration, func() { cancel(context.DeadlineExceeded) }, "readinessDeadline")
	defer timer.Stop()

	workspaceName, agentName, _ := strings.Cut(NormalizeWorkspaceInput(args.Workspace), ".")
	workspace, err := deps.coderClient.ResolveWorkspace(waitCtx, workspaceName)
	if err != nil {
		return WorkspaceReadinessResponse{}, xerrors.Errorf("resolve workspace: %w", err)
	}

	ticker := clock.NewTicker(time.Second, "readinessPoll")
	defer ticker.Stop()
	var snapshot WorkspaceReadinessResponse
	for {
		var pending bool
		snapshot, pending, err = workspaceReadinessSnapshot(workspace, agentName)
		if err != nil {
			return WorkspaceReadinessResponse{}, err
		}
		if !pending || args.WaitMs == 0 {
			return snapshot, nil
		}
		select {
		case <-waitCtx.Done():
			err = context.Cause(waitCtx)
		case <-ticker.C:
			// Pin the workspace identity after resolving its name. A deleted
			// workspace must never turn into a newly created namesake.
			workspace, err = deps.coderClient.Workspace(waitCtx, workspace.ID, codersdk.WorkspaceOptions{
				IncludeRelated: &wsrelated.Config{
					LatestBuild: &wsrelated.LatestBuild{
						Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{}},
					},
				},
			})
		}
		if err != nil {
			if ctx.Err() == nil && errors.Is(context.Cause(waitCtx), context.DeadlineExceeded) {
				snapshot.WaitExpired = true
				return snapshot, nil
			}
			return WorkspaceReadinessResponse{}, xerrors.Errorf("observe workspace readiness: %w", err)
		}
	}
}

func workspaceReadinessSnapshot(workspace codersdk.Workspace, agentName string) (WorkspaceReadinessResponse, bool, error) {
	build := workspace.LatestBuild
	res := WorkspaceReadinessResponse{
		WorkspaceID: workspace.ID, BuildID: build.ID,
		BuildStatus: build.Status, BuildTransition: build.Transition, BuildError: build.Job.Error,
	}
	state := func(name, reason string, pending bool) (WorkspaceReadinessResponse, bool, error) {
		res.State, res.Reason = name, reason
		return res, pending, nil
	}
	switch build.Job.Status {
	case codersdk.ProvisionerJobFailed:
		return state("build_failed", "Inspect the workspace build logs and resolve the build error before starting a new build.", false)
	case codersdk.ProvisionerJobCanceled, codersdk.ProvisionerJobCanceling:
		return state("build_canceled", "The build was canceled or is canceling. Inspect the build before starting another.", false)
	}
	switch build.Status {
	case codersdk.WorkspaceStatusPending, codersdk.WorkspaceStatusStarting:
		return state("pending", "The workspace build is still in progress. Observe again or inspect the build logs.", true)
	case codersdk.WorkspaceStatusStopped:
		return state("stopped", "Start the workspace with the workspace build tool before observing again.", false)
	case codersdk.WorkspaceStatusStopping, codersdk.WorkspaceStatusDeleting, codersdk.WorkspaceStatusDeleted:
		return state(string(build.Status), "The workspace is stopping or being deleted and is not ready for execution.", false)
	case codersdk.WorkspaceStatusFailed:
		return state("build_failed", "Inspect the workspace build logs and resolve the build error before starting a new build.", false)
	case codersdk.WorkspaceStatusCanceled, codersdk.WorkspaceStatusCanceling:
		return state("build_canceled", "The build was canceled or is canceling. Inspect the build before starting another.", false)
	case codersdk.WorkspaceStatusRunning:
		if build.Transition != codersdk.WorkspaceTransitionStart {
			return state("unavailable", "The latest build is not a start build. Inspect the workspace before executing commands.", false)
		}
	default:
		return state("unavailable", "The workspace build state is unknown. Inspect the workspace and build logs.", false)
	}

	agent, err := getWorkspaceAgent(workspace, agentName)
	if err != nil {
		return res, false, err
	}
	res.AgentID, res.AgentName = &agent.ID, agent.Name
	res.ConnectionStatus, res.LifecycleState = agent.Status, agent.LifecycleState
	// Report startup failure even when connection status also changed.
	switch agent.LifecycleState {
	case codersdk.WorkspaceAgentLifecycleStartError:
		return state("start_error", "Agent startup scripts failed. Inspect the workspace agent logs and resolve the startup error.", false)
	case codersdk.WorkspaceAgentLifecycleStartTimeout:
		return state("start_timeout", "Agent startup scripts exceeded their timeout. Inspect the workspace agent logs before proceeding.", false)
	}
	if agent.LifecycleState.ShuttingDown() {
		return state("unavailable", "The agent is shutting down or off. Inspect the workspace before executing commands.", false)
	}
	switch agent.Status {
	case codersdk.WorkspaceAgentDisconnected:
		return state("disconnected", "The agent is disconnected. Observe again or inspect the agent and build logs.", true)
	case codersdk.WorkspaceAgentTimeout:
		return state("connection_timeout", "The agent did not connect before its timeout. Inspect the agent and build logs.", false)
	case codersdk.WorkspaceAgentConnecting:
		return state("pending", "Waiting for the agent to connect. Observe again or inspect the agent logs.", true)
	case codersdk.WorkspaceAgentConnected:
		switch agent.LifecycleState {
		case codersdk.WorkspaceAgentLifecycleReady:
			res.Ready = true
			return state("ready", "The build is running and the connected agent completed startup.", false)
		case codersdk.WorkspaceAgentLifecycleCreated, codersdk.WorkspaceAgentLifecycleStarting:
			return state("pending", "Agent startup is still in progress. Observe again or inspect the agent logs.", true)
		}
	}
	return state("unavailable", "The agent state is unknown. Inspect the workspace agent logs before executing commands.", false)
}
