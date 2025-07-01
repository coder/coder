package agent

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/quartz"
)

// PostWorkspaceAgentAppHealth updates the workspace app health.
type PostWorkspaceAgentAppHealth func(context.Context, agentsdk.PostAppHealthsRequest) error

// WorkspaceAppHealthReporter is a function that checks and reports the health of the workspace apps until the passed context is canceled.
type WorkspaceAppHealthReporter func(ctx context.Context)

// NewWorkspaceAppHealthReporter creates a WorkspaceAppHealthReporter that reports app health to coderd.
func NewWorkspaceAppHealthReporter(logger slog.Logger, apps []codersdk.WorkspaceApp, postWorkspaceAgentAppHealth PostWorkspaceAgentAppHealth) WorkspaceAppHealthReporter {
	return NewAppHealthReporterWithClock(logger, apps, postWorkspaceAgentAppHealth, quartz.NewReal())
}

// NewAppHealthReporterWithClock is only called directly by test code.  Product code should call
// NewAppHealthReporter.
func NewAppHealthReporterWithClock(
	logger slog.Logger,
	apps []codersdk.WorkspaceApp,
	postWorkspaceAgentAppHealth PostWorkspaceAgentAppHealth,
	clk quartz.Clock,
) WorkspaceAppHealthReporter {
	logger = logger.Named("apphealth")

	return func(ctx context.Context) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// no need to run this loop if no apps for this workspace.
		if len(apps) == 0 {
			return
		}

		hasHealthchecksEnabled := false
		health := make(map[uuid.UUID]codersdk.WorkspaceAppHealth, 0)
		for _, app := range apps {
			if app.Health == codersdk.WorkspaceAppHealthDisabled {
				continue
			}
			health[app.ID] = app.Health
			hasHealthchecksEnabled = true
		}

		// no need to run this loop if no health checks are configured.
		if !hasHealthchecksEnabled {
			return
		}

		// run a ticker for each app health check.
		var mu sync.RWMutex
		failures := make(map[uuid.UUID]int, 0)
		for _, nextApp := range apps {
			if !shouldStartTicker(nextApp) {
				continue
			}
			app := nextApp
			go func() {
				_ = clk.TickerFunc(ctx, time.Duration(app.Healthcheck.Interval)*time.Second, func() error {
					// We time out at the healthcheck interval to prevent getting too backed up, but
					// set it 1ms early so that it's not simultaneous with the next tick in testing,
					// which makes the test easier to understand.
					//
					// It would be idiomatic to use the http.Client.Timeout or a context.WithTimeout,
					// but we are passing this off to the native http library, which is not aware
					// of the clock library we are using. That means in testing, with a mock clock
					// it will compare mocked times with real times, and we will get strange results.
					// So, we just implement the timeout as a context we cancel with an AfterFunc
					reqCtx, reqCancel := context.WithCancel(ctx)
					timeout := clk.AfterFunc(
						time.Duration(app.Healthcheck.Interval)*time.Second-time.Millisecond,
						reqCancel,
						"timeout", app.Slug)
					defer timeout.Stop()

					err := func() error {
						req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, app.Healthcheck.URL, nil)
						if err != nil {
							return err
						}
						res, err := http.DefaultClient.Do(req)
						if err != nil {
							return err
						}
						// successful healthcheck is a non-5XX status code
						_ = res.Body.Close()
						if res.StatusCode >= http.StatusInternalServerError {
							return xerrors.Errorf("error status code: %d", res.StatusCode)
						}

						return nil
					}()
					if err != nil {
						nowUnhealthy := false
						mu.Lock()
						if failures[app.ID] < int(app.Healthcheck.Threshold) {
							// increment the failure count and keep status the same.
							// we will change it when we hit the threshold.
							failures[app.ID]++
						} else {
							// set to unhealthy if we hit the failure threshold.
							// we stop incrementing at the threshold to prevent the failure value from increasing forever.
							health[app.ID] = codersdk.WorkspaceAppHealthUnhealthy
							nowUnhealthy = true
						}
						mu.Unlock()
						logger.Debug(ctx, "error checking app health",
							slog.F("id", app.ID.String()),
							slog.F("slug", app.Slug),
							slog.F("now_unhealthy", nowUnhealthy), slog.Error(err),
						)
					} else {
						mu.Lock()
						// we only need one successful health check to be considered healthy.
						health[app.ID] = codersdk.WorkspaceAppHealthHealthy
						failures[app.ID] = 0
						mu.Unlock()
						logger.Debug(ctx, "workspace app healthy", slog.F("id", app.ID.String()), slog.F("slug", app.Slug))
					}
					return nil
				}, "healthcheck", app.Slug)
			}()
		}

		mu.Lock()
		lastHealth := copyHealth(health)
		mu.Unlock()
		reportTicker := clk.TickerFunc(ctx, time.Second, func() error {
			mu.RLock()
			changed := healthChanged(lastHealth, health)
			mu.RUnlock()
			if !changed {
				return nil
			}

			mu.Lock()
			lastHealth = copyHealth(health)
			mu.Unlock()
			err := postWorkspaceAgentAppHealth(ctx, agentsdk.PostAppHealthsRequest{
				Healths: lastHealth,
			})
			if err != nil {
				logger.Error(ctx, "failed to report workspace app health", slog.Error(err))
			} else {
				logger.Debug(ctx, "sent workspace app health", slog.F("health", lastHealth))
			}
			return nil
		}, "report")
		_ = reportTicker.Wait() // only possible error is context done
	}
}

func shouldStartTicker(app codersdk.WorkspaceApp) bool {
	return app.Healthcheck.URL != "" && app.Healthcheck.Interval > 0 && app.Healthcheck.Threshold > 0
}

func healthChanged(old map[uuid.UUID]codersdk.WorkspaceAppHealth, updated map[uuid.UUID]codersdk.WorkspaceAppHealth) bool {
	for name, newValue := range updated {
		oldValue, found := old[name]
		if !found {
			return true
		}
		if newValue != oldValue {
			return true
		}
	}

	return false
}

func copyHealth(h1 map[uuid.UUID]codersdk.WorkspaceAppHealth) map[uuid.UUID]codersdk.WorkspaceAppHealth {
	h2 := make(map[uuid.UUID]codersdk.WorkspaceAppHealth, 0)
	for k, v := range h1 {
		h2[k] = v
	}

	return h2
}
