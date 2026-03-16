package taskstatus

import (
	"context"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/quartz"
)

const statusUpdatePrefix = "scaletest status update:"

// createExternalWorkspaceResult contains the results from creating an external workspace.
type createExternalWorkspaceResult struct {
	workspaceID uuid.UUID
	agentToken  string
}

type Runner struct {
	client  client
	updater appStatusUpdater
	cfg     Config

	logger slog.Logger

	// workspaceID is set after creating the external workspace
	workspaceID uuid.UUID

	mu            sync.Mutex
	reportTimes   map[int]time.Time
	doneReporting bool

	// testing only
	clock       quartz.Clock
	randFloat64 func() float64
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

// NewRunner creates a new Runner with the provided codersdk.Client and configuration.
func NewRunner(coderClient *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client:      newClient(coderClient),
		updater:     newAppStatusUpdater(coderClient),
		cfg:         cfg,
		clock:       quartz.NewReal(),
		randFloat64: rand.Float64,
		reportTimes: make(map[int]time.Time),
	}
}

func (r *Runner) Run(ctx context.Context, name string, logs io.Writer) error {
	shouldMarkConnectedDone := true
	defer func() {
		if shouldMarkConnectedDone {
			r.cfg.ConnectedWaitGroup.Done()
		}
	}()

	// ensure these labels are initialized, so we see the time series right away in prometheus.
	r.cfg.Metrics.MissingStatusUpdatesTotal.WithLabelValues(r.cfg.MetricLabelValues...).Add(0)
	r.cfg.Metrics.ReportTaskStatusErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Add(0)

	logs = loadtestutil.NewSyncWriter(logs)
	r.logger = slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug).Named(name)
	r.client.initialize(r.logger)

	// Create the external workspace
	r.logger.Info(ctx, "creating external workspace",
		slog.F("template_id", r.cfg.TemplateID),
		slog.F("workspace_name", r.cfg.WorkspaceName))

	result, err := r.createExternalWorkspace(ctx, codersdk.CreateWorkspaceRequest{
		TemplateID: r.cfg.TemplateID,
		Name:       r.cfg.WorkspaceName,
	})
	if err != nil {
		r.cfg.Metrics.ReportTaskStatusErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		return xerrors.Errorf("create external workspace: %w", err)
	}

	// Set the workspace ID
	r.workspaceID = result.workspaceID
	r.logger.Info(ctx, "created external workspace", slog.F("workspace_id", r.workspaceID))

	// Establish the dRPC connection using the agent token.
	if err := r.updater.initialize(ctx, r.logger, result.agentToken); err != nil {
		r.cfg.Metrics.ReportTaskStatusErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		return xerrors.Errorf("initialize app status updater: %w", err)
	}
	defer func() {
		if err := r.updater.close(); err != nil {
			r.logger.Error(ctx, "failed to close app status updater", slog.Error(err))
		}
	}()
	r.logger.Info(ctx, "initialized app status updater with agent token")

	workspaceUpdatesCtx, cancelWorkspaceUpdates := context.WithCancel(ctx)
	defer cancelWorkspaceUpdates()
	workspaceUpdatesResult := make(chan error, 1)
	shouldMarkConnectedDone = false // we are passing this responsibility to the watchWorkspaceUpdates goroutine
	go func() {
		workspaceUpdatesResult <- r.watchWorkspaceUpdates(workspaceUpdatesCtx)
	}()

	err = r.reportTaskStatus(ctx)
	if err != nil {
		return xerrors.Errorf("report task status: %w", err)
	}

	err = <-workspaceUpdatesResult
	if err != nil {
		return xerrors.Errorf("watch workspace: %w", err)
	}
	return nil
}

// Cleanup deletes the external workspace created by this runner.
func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if r.workspaceID == uuid.Nil {
		// No workspace was created, nothing to cleanup
		return nil
	}

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug).Named(id)

	logger.Info(ctx, "deleting external workspace", slog.F("workspace_id", r.workspaceID))

	err := r.client.deleteWorkspace(ctx, r.workspaceID)
	if err != nil {
		logger.Error(ctx, "failed to delete external workspace",
			slog.F("workspace_id", r.workspaceID),
			slog.Error(err))
		return xerrors.Errorf("delete external workspace: %w", err)
	}

	logger.Info(ctx, "successfully deleted external workspace", slog.F("workspace_id", r.workspaceID))
	return nil
}

func (r *Runner) watchWorkspaceUpdates(ctx context.Context) error {
	shouldMarkConnectedDone := true
	defer func() {
		if shouldMarkConnectedDone {
			r.cfg.ConnectedWaitGroup.Done()
		}
	}()
	updates, err := r.client.watchWorkspace(ctx, r.workspaceID)
	if err != nil {
		return xerrors.Errorf("watch workspace: %w", err)
	}
	shouldMarkConnectedDone = false
	r.cfg.ConnectedWaitGroup.Done()
	defer func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.cfg.Metrics.MissingStatusUpdatesTotal.
			WithLabelValues(r.cfg.MetricLabelValues...).
			Add(float64(len(r.reportTimes)))
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case workspace := <-updates:
			if workspace.LatestAppStatus == nil {
				continue
			}
			msgNo, ok := parseStatusMessage(workspace.LatestAppStatus.Message)
			if !ok {
				continue
			}

			r.mu.Lock()
			reportTime, ok := r.reportTimes[msgNo]
			delete(r.reportTimes, msgNo)
			allDone := r.doneReporting && len(r.reportTimes) == 0
			r.mu.Unlock()

			if !ok {
				return xerrors.Errorf("report time not found for message %d", msgNo)
			}
			latency := r.clock.Since(reportTime, "watchWorkspaceUpdates")
			r.cfg.Metrics.TaskStatusToWorkspaceUpdateLatencySeconds.
				WithLabelValues(r.cfg.MetricLabelValues...).
				Observe(latency.Seconds())
			if allDone {
				return nil
			}
		}
	}
}

func (r *Runner) reportTaskStatus(ctx context.Context) error {
	defer func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.doneReporting = true
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.cfg.StartReporting:
		r.logger.Info(ctx, "starting to report task status")
	}
	startedReporting := r.clock.Now("reportTaskStatus", "startedReporting")
	msgNo := 0

	getRandPeriod := func() time.Duration {
		// vary the period by +-50% so that updates are not synchronized across runners, which would create
		// artificially large instantaneous stress on Coder and the database.
		p := (r.randFloat64() + 0.5) * r.cfg.ReportStatusPeriod.Seconds()
		return time.Duration(p * float64(time.Second))
	}
	tmr := r.clock.NewTimer(getRandPeriod(), "reportTaskStatus")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tmr.C:
			tmr.Reset(getRandPeriod(), "reportTaskStatus", "tick")
		}
		r.mu.Lock()
		now := r.clock.Now("reportTaskStatus", "tick")
		r.reportTimes[msgNo] = now
		// It's important that we set doneReporting along with a final report, since the watchWorkspaceUpdates goroutine
		// needs an update to wake up and check if we're done. We could introduce a secondary signaling channel, but
		// it adds a lot of complexity and will be hard to test. We expect the tick period to be much smaller than the
		// report status duration, so one extra tick is not a big deal.
		if now.After(startedReporting.Add(r.cfg.ReportStatusDuration)) {
			r.doneReporting = true
		}
		r.mu.Unlock()

		err := r.updater.updateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
			Slug:    r.cfg.AppSlug,
			Message: statusUpdatePrefix + strconv.Itoa(msgNo),
			State:   agentproto.UpdateAppStatusRequest_WORKING,
			Uri:     "https://example.com/example-status/",
		})
		if err != nil {
			r.logger.Error(ctx, "failed to report task status", slog.Error(err))
			r.cfg.Metrics.ReportTaskStatusErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		}
		msgNo++
		// note that it's safe to read r.doneReporting here without a lock because we're the only goroutine that sets
		// it.
		if r.doneReporting {
			return nil
		}
	}
}

func parseStatusMessage(message string) (int, bool) {
	if !strings.HasPrefix(message, statusUpdatePrefix) {
		return 0, false
	}
	message = strings.TrimPrefix(message, statusUpdatePrefix)
	msgNo, err := strconv.Atoi(message)
	if err != nil {
		return 0, false
	}
	return msgNo, true
}

// createExternalWorkspace creates an external workspace and returns the workspace ID
// and agent token for the first external agent found in the workspace resources.
func (r *Runner) createExternalWorkspace(ctx context.Context, req codersdk.CreateWorkspaceRequest) (createExternalWorkspaceResult, error) {
	// Create the workspace
	workspace, err := r.client.CreateUserWorkspace(ctx, codersdk.Me, req)
	if err != nil {
		return createExternalWorkspaceResult{}, err
	}

	r.logger.Info(ctx, "waiting for workspace build to complete",
		slog.F("workspace_name", workspace.Name),
		slog.F("workspace_id", workspace.ID))

	// Poll the workspace until the build is complete
	var finalWorkspace codersdk.Workspace
	buildComplete := xerrors.New("build complete") // sentinel error
	waiter := r.clock.TickerFunc(ctx, 30*time.Second, func() error {
		// Get the workspace with latest build details
		workspace, err := r.client.WorkspaceByOwnerAndName(ctx, codersdk.Me, workspace.Name, codersdk.WorkspaceOptions{})
		if err != nil {
			r.logger.Error(ctx, "failed to poll workspace while waiting for build to complete", slog.Error(err))
			return nil
		}

		jobStatus := workspace.LatestBuild.Job.Status
		r.logger.Debug(ctx, "checking workspace build status",
			slog.F("status", jobStatus),
			slog.F("build_id", workspace.LatestBuild.ID))

		switch jobStatus {
		case codersdk.ProvisionerJobSucceeded:
			// Build succeeded
			r.logger.Info(ctx, "workspace build succeeded")
			finalWorkspace = workspace
			return buildComplete
		case codersdk.ProvisionerJobFailed:
			return xerrors.Errorf("workspace build failed: %s", workspace.LatestBuild.Job.Error)
		case codersdk.ProvisionerJobCanceled:
			return xerrors.Errorf("workspace build was canceled")
		case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning, codersdk.ProvisionerJobCanceling:
			// Still in progress, continue polling
			return nil
		default:
			return xerrors.Errorf("unexpected job status: %s", jobStatus)
		}
	}, "createExternalWorkspace")

	err = waiter.Wait()
	if err != nil && !xerrors.Is(err, buildComplete) {
		return createExternalWorkspaceResult{}, xerrors.Errorf("wait for build completion: %w", err)
	}

	// Find external agents in resources
	for _, resource := range finalWorkspace.LatestBuild.Resources {
		if resource.Type != "coder_external_agent" || len(resource.Agents) == 0 {
			continue
		}

		// Get credentials for the first agent
		agent := resource.Agents[0]
		credentials, err := r.client.WorkspaceExternalAgentCredentials(ctx, finalWorkspace.ID, agent.Name)
		if err != nil {
			return createExternalWorkspaceResult{}, err
		}

		return createExternalWorkspaceResult{
			workspaceID: finalWorkspace.ID,
			agentToken:  credentials.AgentToken,
		}, nil
	}

	return createExternalWorkspaceResult{}, xerrors.Errorf("no external agent found in workspace")
}
