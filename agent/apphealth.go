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

type WorkspaceAppHealthReporter func(ctx context.Context)

func NewWorkspaceAppHealthReporter(logger slog.Logger, client *codersdk.Client) WorkspaceAppHealthReporter {
	return func(ctx context.Context) {
		r := retry.New(time.Second, 30*time.Second)
		for {
			err := func() error {
				apps, err := client.WorkspaceAgentApps(ctx)
				if err != nil {
					if xerrors.Is(err, context.Canceled) {
						return nil
					}
					return xerrors.Errorf("getting workspace apps: %w", err)
				}

				if len(apps) == 0 {
					return nil
				}

				health := make(map[string]codersdk.WorkspaceAppHealth, 0)
				for _, app := range apps {
					health[app.Name] = app.Health
				}

				tickers := make(chan string)
				for _, app := range apps {
					if shouldStartTicker(app) {
						t := time.NewTicker(time.Duration(app.HealthcheckInterval) * time.Second)
						go func() {
							for {
								select {
								case <-ctx.Done():
									return
								case <-t.C:
									tickers <- app.Name
								}
							}
						}()
					}
				}
				var mu sync.RWMutex
				var failures map[string]int
				go func() {
					for {
						select {
						case <-ctx.Done():
							return
						case name := <-tickers:
							for _, app := range apps {
								if app.Name != name {
									continue
								}

								client := &http.Client{
									Timeout: time.Duration(app.HealthcheckInterval),
								}
								err := func() error {
									req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.HealthcheckURL, nil)
									if err != nil {
										return err
									}
									res, err := client.Do(req)
									if err != nil {
										return err
									}
									res.Body.Close()
									if res.StatusCode >= http.StatusInternalServerError {
										return xerrors.Errorf("error status code: %d", res.StatusCode)
									}

									return nil
								}()
								if err == nil {
									mu.Lock()
									failures[app.Name]++
									if failures[app.Name] > int(app.HealthcheckThreshold) {
										health[app.Name] = codersdk.WorkspaceAppHealthUnhealthy
									}
									mu.Unlock()
								} else {
									mu.Lock()
									failures[app.Name] = 0
									health[app.Name] = codersdk.WorkspaceAppHealthHealthy
									mu.Unlock()
								}
							}
						}
					}
				}()

				reportTicker := time.NewTicker(time.Second)
				lastHealth := make(map[string]codersdk.WorkspaceAppHealth, 0)
				for {
					select {
					case <-ctx.Done():
						return nil
					case <-reportTicker.C:
						mu.RLock()
						changed := healthChanged(lastHealth, health)
						mu.RUnlock()
						if changed {
							lastHealth = health
							err := client.PostWorkspaceAgentAppHealth(ctx, codersdk.PostWorkspaceAppHealthsRequest{
								Healths: health,
							})
							if err != nil {
								logger.Error(ctx, "failed to report workspace app stat", slog.Error(err))
							}
						}
					}
				}
			}()
			if err != nil {
				logger.Error(ctx, "failed running workspace app reporter", slog.Error(err))
				// continue loop with backoff on non-nil errors
				if r.Wait(ctx) {
					continue
				}
			}

			return
		}
	}
}

func shouldStartTicker(app codersdk.WorkspaceApp) bool {
	return app.HealthcheckEnabled && app.HealthcheckInterval > 0 && app.HealthcheckThreshold > 0 && app.HealthcheckURL != ""
}

func healthChanged(old map[string]codersdk.WorkspaceAppHealth, new map[string]codersdk.WorkspaceAppHealth) bool {
	for name, newValue := range new {
		oldValue, found := old[name]
		if !found {
			panic("workspace app lengths are not equal")
		}
		if newValue != oldValue {
			return true
		}
	}

	return false
}
