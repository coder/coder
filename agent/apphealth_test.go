package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/testutil"
)

func TestAppHealth_Healthy(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	apps := []codersdk.WorkspaceApp{
		{
			Slug:        "app1",
			Healthcheck: codersdk.Healthcheck{},
			Health:      codersdk.WorkspaceAppHealthDisabled,
		},
		{
			Slug: "app2",
			Healthcheck: codersdk.Healthcheck{
				// URL: We don't set the URL for this test because the setup will
				// create a httptest server for us and set it for us.
				Interval:  1,
				Threshold: 1,
			},
			Health: codersdk.WorkspaceAppHealthInitializing,
		},
	}
	handlers := []http.Handler{
		nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusOK, nil)
		}),
	}
	getApps, closeFn := setupAppReporter(ctx, t, apps, handlers)
	defer closeFn()
	apps, err := getApps(ctx)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAppHealthDisabled, apps[0].Health)
	require.Eventually(t, func() bool {
		apps, err := getApps(ctx)
		if err != nil {
			return false
		}

		return apps[1].Health == codersdk.WorkspaceAppHealthHealthy
	}, testutil.WaitLong, testutil.IntervalSlow)
}

func TestAppHealth_500(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	apps := []codersdk.WorkspaceApp{
		{
			Slug: "app2",
			Healthcheck: codersdk.Healthcheck{
				// URL: We don't set the URL for this test because the setup will
				// create a httptest server for us and set it for us.
				Interval:  1,
				Threshold: 1,
			},
			Health: codersdk.WorkspaceAppHealthInitializing,
		},
	}
	handlers := []http.Handler{
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusInternalServerError, nil)
		}),
	}
	getApps, closeFn := setupAppReporter(ctx, t, apps, handlers)
	defer closeFn()
	require.Eventually(t, func() bool {
		apps, err := getApps(ctx)
		if err != nil {
			return false
		}

		return apps[0].Health == codersdk.WorkspaceAppHealthUnhealthy
	}, testutil.WaitLong, testutil.IntervalSlow)
}

func TestAppHealth_Timeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	apps := []codersdk.WorkspaceApp{
		{
			Slug: "app2",
			Healthcheck: codersdk.Healthcheck{
				// URL: We don't set the URL for this test because the setup will
				// create a httptest server for us and set it for us.
				Interval:  1,
				Threshold: 1,
			},
			Health: codersdk.WorkspaceAppHealthInitializing,
		},
	}
	handlers := []http.Handler{
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// sleep longer than the interval to cause the health check to time out
			time.Sleep(2 * time.Second)
			httpapi.Write(r.Context(), w, http.StatusOK, nil)
		}),
	}
	getApps, closeFn := setupAppReporter(ctx, t, apps, handlers)
	defer closeFn()
	require.Eventually(t, func() bool {
		apps, err := getApps(ctx)
		if err != nil {
			return false
		}

		return apps[0].Health == codersdk.WorkspaceAppHealthUnhealthy
	}, testutil.WaitLong, testutil.IntervalSlow)
}

func TestAppHealth_NotSpamming(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	apps := []codersdk.WorkspaceApp{
		{
			Slug: "app2",
			Healthcheck: codersdk.Healthcheck{
				// URL: We don't set the URL for this test because the setup will
				// create a httptest server for us and set it for us.
				Interval:  1,
				Threshold: 1,
			},
			Health: codersdk.WorkspaceAppHealthInitializing,
		},
	}

	counter := new(int32)
	handlers := []http.Handler{
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(counter, 1)
		}),
	}
	_, closeFn := setupAppReporter(ctx, t, apps, handlers)
	defer closeFn()
	// Ensure we haven't made more than 2 (expected 1 + 1 for buffer) requests in the last second.
	// if there is a bug where we are spamming the healthcheck route this will catch it.
	time.Sleep(time.Second)
	require.LessOrEqual(t, atomic.LoadInt32(counter), int32(2))
}

func setupAppReporter(ctx context.Context, t *testing.T, apps []codersdk.WorkspaceApp, handlers []http.Handler) (agent.WorkspaceAgentApps, func()) {
	closers := []func(){}
	for i, handler := range handlers {
		if handler == nil {
			continue
		}
		ts := httptest.NewServer(handler)
		app := apps[i]
		app.Healthcheck.URL = ts.URL
		apps[i] = app
		closers = append(closers, ts.Close)
	}

	var mu sync.Mutex
	workspaceAgentApps := func(context.Context) ([]codersdk.WorkspaceApp, error) {
		mu.Lock()
		defer mu.Unlock()
		var newApps []codersdk.WorkspaceApp
		return append(newApps, apps...), nil
	}
	postWorkspaceAgentAppHealth := func(_ context.Context, req agentsdk.PostAppHealthsRequest) error {
		mu.Lock()
		for id, health := range req.Healths {
			for i, app := range apps {
				if app.ID != id {
					continue
				}
				app.Health = health
				apps[i] = app
			}
		}
		mu.Unlock()

		return nil
	}

	go agent.NewWorkspaceAppHealthReporter(slogtest.Make(t, nil).Leveled(slog.LevelDebug), apps, postWorkspaceAgentAppHealth)(ctx)

	return workspaceAgentApps, func() {
		for _, closeFn := range closers {
			closeFn()
		}
	}
}
