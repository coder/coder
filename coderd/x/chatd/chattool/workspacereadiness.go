package chattool

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

const (
	// BuildPollInterval is how often we check if the workspace
	// build has completed.
	BuildPollInterval = 2 * time.Second
	// BuildTimeout is the maximum time to wait for a workspace
	// build to complete before giving up.
	BuildTimeout = 10 * time.Minute
	// AgentConnectTimeout is the maximum time to wait for the
	// workspace agent to become reachable after a successful
	// build.
	AgentConnectTimeout = 2 * time.Minute
	// AgentRetryInterval is how often we retry connecting to
	// the workspace agent.
	AgentRetryInterval = 2 * time.Second
	// AgentAttemptTimeout is the timeout for a single connection
	// attempt to the workspace agent during the retry loop.
	AgentAttemptTimeout = 5 * time.Second
	// StartupScriptTimeout is the maximum time to wait for the
	// workspace agent's startup scripts to finish after the
	// agent is reachable.
	StartupScriptTimeout = 10 * time.Minute
	// StartupScriptPollInterval is how often we check the
	// agent's lifecycle state while waiting for startup scripts.
	StartupScriptPollInterval = 2 * time.Second
)

// WaitForBuild polls the workspace's latest build until it
// completes or the context expires.
func WaitForBuild(
	ctx context.Context,
	db database.Store,
	workspaceID uuid.UUID,
) error {
	buildCtx, cancel := context.WithTimeout(ctx, BuildTimeout)
	defer cancel()

	ticker := time.NewTicker(BuildPollInterval)
	defer ticker.Stop()

	for {
		build, err := db.GetLatestWorkspaceBuildByWorkspaceID(
			buildCtx, workspaceID,
		)
		if err != nil {
			return xerrors.Errorf("get latest build: %w", err)
		}

		job, err := db.GetProvisionerJobByID(buildCtx, build.JobID)
		if err != nil {
			return xerrors.Errorf("get provisioner job: %w", err)
		}

		switch job.JobStatus {
		case database.ProvisionerJobStatusSucceeded:
			return nil
		case database.ProvisionerJobStatusFailed:
			errMsg := "build failed"
			if job.Error.Valid {
				errMsg = job.Error.String
			}
			return xerrors.New(errMsg)
		case database.ProvisionerJobStatusCanceled:
			return xerrors.New("build was canceled")
		case database.ProvisionerJobStatusPending,
			database.ProvisionerJobStatusRunning,
			database.ProvisionerJobStatusCanceling:
			// Still in progress — keep waiting.
		default:
			return xerrors.Errorf("unexpected job status: %s", job.JobStatus)
		}

		select {
		case <-buildCtx.Done():
			return xerrors.Errorf(
				"timed out waiting for workspace build: %w",
				buildCtx.Err(),
			)
		case <-ticker.C:
		}
	}
}

// WaitForAgentReady waits for the workspace agent to become
// reachable and for its startup scripts to finish. It returns
// status fields suitable for merging into a tool response.
func WaitForAgentReady(
	ctx context.Context,
	db database.Store,
	agentID uuid.UUID,
	agentConnFn AgentConnFunc,
) map[string]any {
	result := map[string]any{}

	// Phase 1: retry connecting to the agent.
	if agentConnFn != nil {
		agentCtx, agentCancel := context.WithTimeout(ctx, AgentConnectTimeout)
		defer agentCancel()

		ticker := time.NewTicker(AgentRetryInterval)
		defer ticker.Stop()

		var lastErr error
		for {
			attemptCtx, attemptCancel := context.WithTimeout(agentCtx, AgentAttemptTimeout)
			conn, release, err := agentConnFn(attemptCtx, agentID)
			attemptCancel()
			if err == nil {
				release()
				_ = conn
				break
			}
			lastErr = err

			select {
			case <-agentCtx.Done():
				result["agent_status"] = "not_ready"
				result["agent_error"] = lastErr.Error()
				return result
			case <-ticker.C:
			}
		}
	}

	// Phase 2: poll lifecycle until startup scripts finish.
	if db != nil {
		scriptCtx, scriptCancel := context.WithTimeout(ctx, StartupScriptTimeout)
		defer scriptCancel()

		ticker := time.NewTicker(StartupScriptPollInterval)
		defer ticker.Stop()

		var lastState database.WorkspaceAgentLifecycleState
		for {
			row, err := db.GetWorkspaceAgentLifecycleStateByID(scriptCtx, agentID)
			if err == nil {
				lastState = row.LifecycleState
				switch lastState {
				case database.WorkspaceAgentLifecycleStateCreated,
					database.WorkspaceAgentLifecycleStateStarting:
					// Still in progress, keep polling.
				case database.WorkspaceAgentLifecycleStateReady:
					return result
				default:
					// Terminal non-ready state.
					result["startup_scripts"] = "startup_scripts_failed"
					result["lifecycle_state"] = string(lastState)
					return result
				}
			}

			select {
			case <-scriptCtx.Done():
				if errors.Is(scriptCtx.Err(), context.DeadlineExceeded) {
					result["startup_scripts"] = "startup_scripts_timeout"
				} else {
					result["startup_scripts"] = "startup_scripts_unknown"
				}
				return result
			case <-ticker.C:
			}
		}
	}

	return result
}
