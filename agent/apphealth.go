package agent

import (
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
	"github.com/coder/retry"
)

// WorkspaceAgentApps fetches the workspace apps.
type WorkspaceAgentApps func(context.Context) ([]codersdk.WorkspaceApp, error)

// PostWorkspaceAgentAppHealth updates the workspace app health.
type PostWorkspaceAgentAppHealth func(context.Context, codersdk.PostWorkspaceAppHealthsRequest) error

// WorkspaceAppHealthReporter is a function that checks and reports the health of the workspace apps until the passed context is canceled.
type WorkspaceAppHealthReporter func(ctx context.Context)

// NewWorkspaceAppHealthReporter creates a WorkspaceAppHealthReporter that reports app health to coderd.
func NewWorkspaceAppHealthReporter(logger slog.Logger, apps []codersdk.WorkspaceApp, postWorkspaceAgentAppHealth PostWorkspaceAgentAppHealth) WorkspaceAppHealthReporter {
	runHealthcheckLoop := func(ctx context.Context) error {
		// no need to run this loop if no apps for this workspace.
		if len(apps) == 0 {
			return nil
		}

		hasHealthchecksEnabled := false
		health := make(map[string]codersdk.WorkspaceAppHealth, 0)
		for _, app := range apps {
			health[app.DisplayName] = app.Health
			if !hasHealthchecksEnabled && app.Health != codersdk.WorkspaceAppHealthDisabled {
				hasHealthchecksEnabled = true
			}
		}

		// no need to run this loop if no health checks are configured.
		if !hasHealthchecksEnabled {
			return nil
		}

		// run a ticker for each app health check.
		var mu sync.RWMutex
		failures := make(map[string]int, 0)
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
						res.Body.Close()
						if res.StatusCode >= http.StatusInternalServerError {
							return xerrors.Errorf("error status code: %d", res.StatusCode)
						}

						return nil
					}()
					if err != nil {
						mu.Lock()
						if failures[app.DisplayName] < int(app.Healthcheck.Threshold) {
							// increment the failure count and keep status the same.
							// we will change it when we hit the threshold.
							failures[app.DisplayName]++
						} else {
							// set to unhealthy if we hit the failure threshold.
							// we stop incrementing at the threshold to prevent the failure value from increasing forever.
							health[app.DisplayName] = codersdk.WorkspaceAppHealthUnhealthy
						}
						mu.Unlock()
					} else {
						mu.Lock()
						// we only need one successful health check to be considered healthy.
						health[app.DisplayName] = codersdk.WorkspaceAppHealthHealthy
						failures[app.DisplayName] = 0
						mu.Unlock()
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
				err := postWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{
					Healths: lastHealth,
				})
				if err != nil {
					logger.Error(ctx, "failed to report workspace app stat", slog.Error(err))
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

func healthChanged(old map[string]codersdk.WorkspaceAppHealth, new map[string]codersdk.WorkspaceAppHealth) bool {
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

func copyHealth(h1 map[string]codersdk.WorkspaceAppHealth) map[string]codersdk.WorkspaceAppHealth {
	h2 := make(map[string]codersdk.WorkspaceAppHealth, 0)
	for k, v := range h1 {
		h2[k] = v
	}

	return h2
}
