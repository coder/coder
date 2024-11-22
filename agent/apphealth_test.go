package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestAppHealth_Healthy(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	apps := []codersdk.WorkspaceApp{
		{
			ID:          uuid.UUID{1},
			Slug:        "app1",
			Healthcheck: codersdk.Healthcheck{},
			Health:      codersdk.WorkspaceAppHealthDisabled,
		},
		{
			ID:   uuid.UUID{2},
			Slug: "app2",
			Healthcheck: codersdk.Healthcheck{
				// URL: We don't set the URL for this test because the setup will
				// create a httptest server for us and set it for us.
				Interval:  1,
				Threshold: 1,
			},
			Health: codersdk.WorkspaceAppHealthInitializing,
		},
		{
			ID:   uuid.UUID{3},
			Slug: "app3",
			Healthcheck: codersdk.Healthcheck{
				Interval:  2,
				Threshold: 1,
			},
			Health: codersdk.WorkspaceAppHealthInitializing,
		},
	}
	checks2 := 0
	checks3 := 0
	handlers := []http.Handler{
		nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			checks2++
			httpapi.Write(r.Context(), w, http.StatusOK, nil)
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			checks3++
			httpapi.Write(r.Context(), w, http.StatusOK, nil)
		}),
	}
	mClock := quartz.NewMock(t)
	healthcheckTrap := mClock.Trap().TickerFunc("healthcheck")
	defer healthcheckTrap.Close()
	reportTrap := mClock.Trap().TickerFunc("report")
	defer reportTrap.Close()

	fakeAPI, closeFn := setupAppReporter(ctx, t, slices.Clone(apps), handlers, mClock)
	defer closeFn()
	healthchecksStarted := make([]string, 2)
	for i := 0; i < 2; i++ {
		c := healthcheckTrap.MustWait(ctx)
		c.Release()
		healthchecksStarted[i] = c.Tags[1]
	}
	slices.Sort(healthchecksStarted)
	require.Equal(t, []string{"app2", "app3"}, healthchecksStarted)

	// advance the clock 1ms before the report ticker starts, so that it's not
	// simultaneous with the checks.
	mClock.Advance(time.Millisecond).MustWait(ctx)
	reportTrap.MustWait(ctx).Release()

	mClock.Advance(999 * time.Millisecond).MustWait(ctx) // app2 is now healthy

	mClock.Advance(time.Millisecond).MustWait(ctx) // report gets triggered
	update := testutil.RequireRecvCtx(ctx, t, fakeAPI.AppHealthCh())
	require.Len(t, update.GetUpdates(), 2)
	applyUpdate(t, apps, update)
	require.Equal(t, codersdk.WorkspaceAppHealthHealthy, apps[1].Health)
	require.Equal(t, codersdk.WorkspaceAppHealthInitializing, apps[2].Health)

	mClock.Advance(999 * time.Millisecond).MustWait(ctx) // app3 is now healthy

	mClock.Advance(time.Millisecond).MustWait(ctx) // report gets triggered
	update = testutil.RequireRecvCtx(ctx, t, fakeAPI.AppHealthCh())
	require.Len(t, update.GetUpdates(), 2)
	applyUpdate(t, apps, update)
	require.Equal(t, codersdk.WorkspaceAppHealthHealthy, apps[1].Health)
	require.Equal(t, codersdk.WorkspaceAppHealthHealthy, apps[2].Health)

	// ensure we aren't spamming
	require.Equal(t, 2, checks2)
	require.Equal(t, 1, checks3)
}

func TestAppHealth_500(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	apps := []codersdk.WorkspaceApp{
		{
			ID:   uuid.UUID{2},
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

	mClock := quartz.NewMock(t)
	healthcheckTrap := mClock.Trap().TickerFunc("healthcheck")
	defer healthcheckTrap.Close()
	reportTrap := mClock.Trap().TickerFunc("report")
	defer reportTrap.Close()

	fakeAPI, closeFn := setupAppReporter(ctx, t, slices.Clone(apps), handlers, mClock)
	defer closeFn()
	healthcheckTrap.MustWait(ctx).Release()
	// advance the clock 1ms before the report ticker starts, so that it's not
	// simultaneous with the checks.
	mClock.Advance(time.Millisecond).MustWait(ctx)
	reportTrap.MustWait(ctx).Release()

	mClock.Advance(999 * time.Millisecond).MustWait(ctx) // check gets triggered
	mClock.Advance(time.Millisecond).MustWait(ctx)       // report gets triggered, but unsent since we are at the threshold

	mClock.Advance(999 * time.Millisecond).MustWait(ctx) // 2nd check, crosses threshold
	mClock.Advance(time.Millisecond).MustWait(ctx)       // 2nd report, sends update

	update := testutil.RequireRecvCtx(ctx, t, fakeAPI.AppHealthCh())
	require.Len(t, update.GetUpdates(), 1)
	applyUpdate(t, apps, update)
	require.Equal(t, codersdk.WorkspaceAppHealthUnhealthy, apps[0].Health)
}

func TestAppHealth_Timeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	apps := []codersdk.WorkspaceApp{
		{
			ID:   uuid.UUID{2},
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
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			// allow the request to time out
			<-r.Context().Done()
		}),
	}
	mClock := quartz.NewMock(t)
	start := mClock.Now()

	// for this test, it's easier to think in the number of milliseconds elapsed
	// since start.
	ms := func(n int) time.Time {
		return start.Add(time.Duration(n) * time.Millisecond)
	}
	healthcheckTrap := mClock.Trap().TickerFunc("healthcheck")
	defer healthcheckTrap.Close()
	reportTrap := mClock.Trap().TickerFunc("report")
	defer reportTrap.Close()
	timeoutTrap := mClock.Trap().AfterFunc("timeout")
	defer timeoutTrap.Close()

	fakeAPI, closeFn := setupAppReporter(ctx, t, apps, handlers, mClock)
	defer closeFn()
	healthcheckTrap.MustWait(ctx).Release()
	// advance the clock 1ms before the report ticker starts, so that it's not
	// simultaneous with the checks.
	mClock.Set(ms(1)).MustWait(ctx)
	reportTrap.MustWait(ctx).Release()

	w := mClock.Set(ms(1000)) // 1st check starts
	timeoutTrap.MustWait(ctx).Release()
	mClock.Set(ms(1001)).MustWait(ctx) // report tick, no change
	mClock.Set(ms(1999))               // timeout pops
	w.MustWait(ctx)                    // 1st check finished
	w = mClock.Set(ms(2000))           // 2nd check starts
	timeoutTrap.MustWait(ctx).Release()
	mClock.Set(ms(2001)).MustWait(ctx) // report tick, no change
	mClock.Set(ms(2999))               // timeout pops
	w.MustWait(ctx)                    // 2nd check finished
	// app is now unhealthy after 2 timeouts
	mClock.Set(ms(3000)) // 3rd check starts
	timeoutTrap.MustWait(ctx).Release()
	mClock.Set(ms(3001)).MustWait(ctx) // report tick, sends changes

	update := testutil.RequireRecvCtx(ctx, t, fakeAPI.AppHealthCh())
	require.Len(t, update.GetUpdates(), 1)
	applyUpdate(t, apps, update)
	require.Equal(t, codersdk.WorkspaceAppHealthUnhealthy, apps[0].Health)
}

func setupAppReporter(
	ctx context.Context, t *testing.T,
	apps []codersdk.WorkspaceApp,
	handlers []http.Handler,
	clk quartz.Clock,
) (*agenttest.FakeAgentAPI, func()) {
	closers := []func(){}
	for _, app := range apps {
		require.NotEqual(t, uuid.Nil, app.ID, "all apps must have ID set")
	}
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

	// We don't care about manifest or stats in this test since it's not using
	// a full agent and these RPCs won't get called.
	//
	// We use a proper fake agent API so we can test the conversion code and the
	// request code as well. Before we were bypassing these by using a custom
	// post function.
	fakeAAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)

	go agent.NewAppHealthReporterWithClock(
		testutil.Logger(t),
		apps, agentsdk.AppHealthPoster(fakeAAPI), clk,
	)(ctx)

	return fakeAAPI, func() {
		for _, closeFn := range closers {
			closeFn()
		}
	}
}

func applyUpdate(t *testing.T, apps []codersdk.WorkspaceApp, req *proto.BatchUpdateAppHealthRequest) {
	t.Helper()
	for _, update := range req.Updates {
		updateID, err := uuid.FromBytes(update.Id)
		require.NoError(t, err)
		updateHealth := codersdk.WorkspaceAppHealth(strings.ToLower(proto.AppHealth_name[int32(update.Health)]))

		for i, app := range apps {
			if app.ID != updateID {
				continue
			}
			app.Health = updateHealth
			apps[i] = app
		}
	}
}
