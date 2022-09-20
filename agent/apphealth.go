package agent

import (
	"context"
	"sync"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
)

func reportAppHealth(ctx context.Context, logger slog.Logger, fetcher FetchWorkspaceApps, reporter PostWorkspaceAppHealth) {
	apps, err := fetcher(ctx)
	if err != nil {
		logger.Error(ctx, "failed to fetch workspace apps", slog.Error(err))
		return
	}

	if len(apps) == 0 {
		return
	}

	health := make(map[string]codersdk.WorkspaceAppHealth, 0)
	for _, app := range apps {
		health[app.Name] = app.Health
	}

	tickers := make(chan string, 0)
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

					func() {
						// do curl
						var err error
						if err != nil {
							mu.Lock()
							failures[app.Name]++
							mu.Unlock()
							return
						}
						mu.Lock()
						failures[app.Name] = 0
						mu.Unlock()
					}()
				}
			}
		}
	}()

	reportTicker := time.NewTicker(time.Second)
	lastHealth := make(map[string]codersdk.WorkspaceAppHealth, 0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-reportTicker.C:
			mu.RLock()
			changed := healthChanged(lastHealth, health)
			mu.Unlock()
			if changed {
				lastHealth = health
				err := reporter(ctx, health)
				if err != nil {
					logger.Error(ctx, "failed to report workspace app stat", slog.Error(err))
				}
			}
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
