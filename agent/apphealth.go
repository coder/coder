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
	"github.com/coder/retry"
)

// WorkspaceAgentApps fetches the workspace apps.
type WorkspaceAgentApps func(context.Context) ([]codersdk.WorkspaceApp, error)

// PostWorkspaceAgentAppHealth updates the workspace app health.
type PostWorkspaceAgentAppHealth func(context.Context, agentsdk.PostAppHealthsRequest) error

// WorkspaceAppHealthReporter is a function that checks and reports the health of the workspace apps until the passed context is canceled.
type WorkspaceAppHealthReporter func(ctx context.Context)

// NewWorkspaceAppHealthReporter creates a WorkspaceAppHealthReporter that reports app health to coderd.
func NewWorkspaceAppHealthReporter(logger slog.Logger, apps []codersdk.WorkspaceApp, postWorkspaceAgentAppHealth PostWorkspaceAgentAppHealth) WorkspaceAppHealthReporter {
	logger = logger.Named("apphealth")

	runHealthcheckLoop := func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// no need to run this loop if no apps for this workspace.
		if len(apps) == 0 {
			return nil
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
			return nil
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
				t := time.NewTicker(time.Duration(app.Healthcheck.Interval) * time.Second)
				defer t.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
					}
					// we set the http timeout to the healthcheck interval to prevent getting too backed up.
					client := &http.Client{
						Timeout: time.Duration(app.Healthcheck.Interval) * time.Second,
					}
					err := func() error {
						req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.Healthcheck.URL, nil)
						if err != nil {
							return err
						}
						res, err := client.Do(req)
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

					t.Reset(time.Duration(app.Healthcheck.Interval) * time.Second)
				}
			}()
		}

		mu.Lock()
		lastHealth := copyHealth(health)
		mu.Unlock()
		reportTicker := time.NewTicker(time.Second)
		defer reportTicker.Stop()
		// every second we check if the health values of the apps have changed
		// and if there is a change we will report the new values.
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-reportTicker.C:
				mu.RLock()
				changed := healthChanged(lastHealth, health)
				mu.RUnlock()
				if !changed {
					continue
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
			}
		}
	}

	return func(ctx context.Context) {
		for r := retry.New(time.Second, 30*time.Second); r.Wait(ctx); {
			err := runHealthcheckLoop(ctx)
			if err == nil || xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
				return
			}
			logger.Error(ctx, "failed running workspace app reporter", slog.Error(err))
		}
	}
}

func shouldStartTicker(app codersdk.WorkspaceApp) bool {
	return app.Healthcheck.URL != "" && app.Healthcheck.Interval > 0 && app.Healthcheck.Threshold > 0
}

func healthChanged(old map[uuid.UUID]codersdk.WorkspaceAppHealth, new map[uuid.UUID]codersdk.WorkspaceAppHealth) bool {
	for name, newValue := range new {
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
