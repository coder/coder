package autostart

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner     *createusers.Runner
	workspacebuildRunner *workspacebuild.Runner

	autostartTotalLatency       time.Duration
	autostartJobCreationLatency time.Duration
	autostartJobAcquiredLatency time.Duration
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	reachedBarrier := false
	defer func() {
		if !reachedBarrier {
			r.cfg.SetupBarrier.Done()
		}
	}()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	r.createUserRunner = createusers.NewRunner(r.client, r.cfg.User)
	newUserAndToken, err := r.createUserRunner.RunReturningUser(ctx, id, logs)
	if err != nil {
		r.cfg.Metrics.AddError("", "create_user")
		return xerrors.Errorf("create user: %w", err)
	}
	newUser := newUserAndToken.User

	newUserClient := codersdk.New(r.client.URL,
		codersdk.WithSessionToken(newUserAndToken.SessionToken),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies())

	//nolint:gocritic // short log is fine
	logger.Info(ctx, "user created", slog.F("username", newUser.Username), slog.F("user_id", newUser.ID.String()))

	workspaceBuildConfig := r.cfg.Workspace
	workspaceBuildConfig.OrganizationID = r.cfg.User.OrganizationID
	workspaceBuildConfig.UserID = newUser.ID.String()
	// We'll wait for the build ourselves to avoid multiple API requests
	workspaceBuildConfig.NoWaitForBuild = true
	workspaceBuildConfig.NoWaitForAgents = true

	r.workspacebuildRunner = workspacebuild.NewRunner(newUserClient, workspaceBuildConfig)
	workspace, err := r.workspacebuildRunner.RunReturningWorkspace(ctx, id, logs)
	if err != nil {
		r.cfg.Metrics.AddError(newUser.Username, "create_workspace")
		return xerrors.Errorf("create workspace: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workspaceUpdates, err := newUserClient.WatchWorkspace(watchCtx, workspace.ID)
	if err != nil {
		r.cfg.Metrics.AddError(newUser.Username, "watch_workspace")
		return xerrors.Errorf("watch workspace: %w", err)
	}

	createWorkspaceCtx, cancel2 := context.WithTimeout(ctx, r.cfg.WorkspaceJobTimeout)
	defer cancel2()

	err = waitForWorkspaceUpdate(createWorkspaceCtx, logger, workspaceUpdates, func(ws codersdk.Workspace) bool {
		return ws.LatestBuild.Transition == codersdk.WorkspaceTransitionStart &&
			ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded
	})
	if err != nil {
		r.cfg.Metrics.AddError(newUser.Username, "wait_for_initial_build")
		return xerrors.Errorf("timeout waiting for initial workspace build to complete: %w", err)
	}

	logger.Info(ctx, "stopping workspace", slog.F("workspace_name", workspace.Name))

	_, err = newUserClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
	})
	if err != nil {
		r.cfg.Metrics.AddError(newUser.Username, "create_stop_build")
		return xerrors.Errorf("create stop build: %w", err)
	}

	stopBuildCtx, cancel3 := context.WithTimeout(ctx, r.cfg.WorkspaceJobTimeout)
	defer cancel3()

	err = waitForWorkspaceUpdate(stopBuildCtx, logger, workspaceUpdates, func(ws codersdk.Workspace) bool {
		return ws.LatestBuild.Transition == codersdk.WorkspaceTransitionStop &&
			ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded
	})
	if err != nil {
		r.cfg.Metrics.AddError(newUser.Username, "wait_for_stop_build")
		return xerrors.Errorf("timeout waiting for stop build to complete: %w", err)
	}

	logger.Info(ctx, "workspace stopped successfully", slog.F("workspace_name", workspace.Name))

	logger.Info(ctx, "waiting for all runners to reach barrier")
	reachedBarrier = true
	r.cfg.SetupBarrier.Done()
	r.cfg.SetupBarrier.Wait()
	logger.Info(ctx, "all runners reached barrier, proceeding with autostart schedule")

	testStartTime := time.Now().UTC()
	autostartTime := testStartTime.Add(r.cfg.AutostartDelay).Round(time.Minute)
	schedule := fmt.Sprintf("CRON_TZ=UTC %d %d * * *", autostartTime.Minute(), autostartTime.Hour())

	logger.Info(ctx, "setting autostart schedule for workspace", slog.F("workspace_name", workspace.Name), slog.F("schedule", schedule))

	err = newUserClient.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
		Schedule: &schedule,
	})
	if err != nil {
		r.cfg.Metrics.AddError(newUser.Username, "update_workspace_autostart")
		return xerrors.Errorf("update workspace autostart: %w", err)
	}

	logger.Info(ctx, "waiting for workspace to autostart", slog.F("workspace_name", workspace.Name))

	autostartInitiateCtx, cancel4 := context.WithDeadline(ctx, autostartTime.Add(r.cfg.AutostartDelay))
	defer cancel4()

	logger.Info(ctx, "listening for workspace updates to detect autostart build")

	err = waitForWorkspaceUpdate(autostartInitiateCtx, logger, workspaceUpdates, func(ws codersdk.Workspace) bool {
		if ws.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
			return false
		}

		// The job has been created, but it might be pending
		if r.autostartJobCreationLatency == 0 {
			r.autostartJobCreationLatency = time.Since(autostartTime)
			r.cfg.Metrics.RecordJobCreation(r.autostartJobCreationLatency, newUser.Username, workspace.Name)
		}

		if ws.LatestBuild.Job.Status == codersdk.ProvisionerJobRunning ||
			ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded {
			// Job is no longer pending, but it might not have finished
			if r.autostartJobAcquiredLatency == 0 {
				r.autostartJobAcquiredLatency = time.Since(autostartTime)
				r.cfg.Metrics.RecordJobAcquired(r.autostartJobAcquiredLatency, newUser.Username, workspace.Name)
			}
			return ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded
		}

		return false
	})
	if err != nil {
		r.cfg.Metrics.AddError(newUser.Username, "wait_for_autostart_build")
		return xerrors.Errorf("timeout waiting for autostart build to be created: %w", err)
	}

	r.autostartTotalLatency = time.Since(autostartTime)

	logger.Info(ctx, "autostart workspace build complete", slog.F("duration", r.autostartTotalLatency))
	r.cfg.Metrics.RecordCompletion(r.autostartTotalLatency, newUser.Username, workspace.Name)

	return nil
}

func waitForWorkspaceUpdate(ctx context.Context, logger slog.Logger, updates <-chan codersdk.Workspace, shouldBreak func(codersdk.Workspace) bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case updatedWorkspace, ok := <-updates:
			if !ok {
				return xerrors.New("workspace updates channel closed")
			}
			logger.Debug(ctx, "received workspace update", slog.F("update", updatedWorkspace))
			if shouldBreak(updatedWorkspace) {
				return nil
			}
		}
	}
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if r.workspacebuildRunner != nil {
		_, _ = fmt.Fprintln(logs, "Cleaning up workspace...")
		if err := r.workspacebuildRunner.Cleanup(ctx, id, logs); err != nil {
			return xerrors.Errorf("cleanup workspace: %w", err)
		}
	}

	if r.createUserRunner != nil {
		_, _ = fmt.Fprintln(logs, "Cleaning up user...")
		if err := r.createUserRunner.Cleanup(ctx, id, logs); err != nil {
			return xerrors.Errorf("cleanup user: %w", err)
		}
	}

	return nil
}

const (
	AutostartTotalLatencyMetric       = "autostart_total_latency_seconds"
	AutostartJobCreationLatencyMetric = "autostart_job_creation_latency_seconds"
	AutostartJobAcquiredLatencyMetric = "autostart_job_acquired_latency_seconds"
)

func (r *Runner) GetMetrics() map[string]any {
	return map[string]any{
		AutostartTotalLatencyMetric:       r.autostartTotalLatency.Seconds(),
		AutostartJobCreationLatencyMetric: r.autostartJobCreationLatency.Seconds(),
		AutostartJobAcquiredLatencyMetric: r.autostartJobAcquiredLatency.Seconds(),
	}
}
