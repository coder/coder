package agent

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
	"github.com/coder/retry"
)

func reportAppHealth(ctx context.Context, logger slog.Logger, fetchApps FetchWorkspaceApps, reportHealth PostWorkspaceAppHealth) {
	r := retry.New(time.Second, 30*time.Second)
	for {
		err := func() error {
			apps, err := fetchApps(ctx)
			if err != nil {
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
								u, err := url.Parse(app.HealthcheckURL)
								if err != nil {
									return err
								}
								res, err := client.Do(&http.Request{
									Method: http.MethodGet,
									URL:    u,
								})
								if err != nil {
									return err
								}
								res.Body.Close()
								if res.StatusCode > 499 {
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
					mu.Unlock()
					if changed {
						lastHealth = health
						err := reportHealth(ctx, health)
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
			r.Wait(ctx)
			continue
		}

		return
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
