package taskstatus

import (
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/quartz"
)

const statusUpdatePrefix = "scaletest status update:"

type Runner struct {
	client  client
	patcher appStatusPatcher
	cfg     Config

	logger slog.Logger

	// workspaceID is set after creating the external workspace
	workspaceID uuid.UUID

	mu            sync.Mutex
	reportTimes   map[int]time.Time
	doneReporting bool

	// testing only
	clock quartz.Clock
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

// NewRunner creates a new Runner with the provided codersdk.Client and configuration.
func NewRunner(coderClient *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client:      newClient(coderClient),
		patcher:     newAppStatusPatcher(coderClient),
		cfg:         cfg,
		clock:       quartz.NewReal(),
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

	logs = loadtestutil.NewSyncWriter(logs)
	r.logger = slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug).Named(name)
	r.client.initialize(r.logger)

	// Create the external workspace
	r.logger.Info(ctx, "creating external workspace",
		slog.F("template_id", r.cfg.TemplateID),
		slog.F("workspace_name", r.cfg.WorkspaceName))

	result, err := r.client.createExternalWorkspace(ctx, codersdk.CreateWorkspaceRequest{
		TemplateID: r.cfg.TemplateID,
		Name:       r.cfg.WorkspaceName,
	})
	if err != nil {
		return xerrors.Errorf("create external workspace: %w", err)
	}

	// Set the workspace ID
	r.workspaceID = result.WorkspaceID
	r.logger.Info(ctx, "created external workspace", slog.F("workspace_id", r.workspaceID))

	// Initialize the patcher with the agent token
	r.patcher.initialize(r.logger, result.AgentToken)
	r.logger.Info(ctx, "initialized app status patcher with agent token")

	// ensure these labels are initialized, so we see the time series right away in prometheus.
	r.cfg.Metrics.MissingStatusUpdatesTotal.WithLabelValues(r.cfg.MetricLabelValues...).Add(0)
	r.cfg.Metrics.ReportTaskStatusErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Add(0)

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

	done := xerrors.New("done reporting task status") // sentinel error
	waiter := r.clock.TickerFunc(ctx, r.cfg.ReportStatusPeriod, func() error {
		r.mu.Lock()
		now := r.clock.Now("reportTaskStatus", "tick")
		r.reportTimes[msgNo] = now
		// It's important that we set doneReporting along with a final report, since the watchWorkspaceUpdates goroutine
		// needs a update to wake up and check if we're done. We could introduce a secondary signaling channel, but
		// it adds a lot of complexity and will be hard to test. We expect the tick period to be much smaller than the
		// report status duration, so one extra tick is not a big deal.
		if now.After(startedReporting.Add(r.cfg.ReportStatusDuration)) {
			r.doneReporting = true
		}
		r.mu.Unlock()

		err := r.patcher.patchAppStatus(ctx, agentsdk.PatchAppStatus{
			AppSlug: r.cfg.AppSlug,
			Message: statusUpdatePrefix + strconv.Itoa(msgNo),
			State:   codersdk.WorkspaceAppStatusStateWorking,
			URI:     "https://example.com/example-status/",
		})
		if err != nil {
			r.logger.Error(ctx, "failed to report task status", slog.Error(err))
			r.cfg.Metrics.ReportTaskStatusErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		}
		msgNo++
		// note that it's safe to read r.doneReporting here without a lock because we're the only goroutine that sets
		// it.
		if r.doneReporting {
			return done // causes the ticker to exit due to the sentinel error
		}
		return nil
	}, "reportTaskStatus")
	err := waiter.Wait()
	if xerrors.Is(err, done) {
		return nil
	}
	return err
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
