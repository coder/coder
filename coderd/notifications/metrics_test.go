package notifications_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/testutil"
)

func TestMetrics(t *testing.T) {
	t.Parallel()

	// setup
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	ctx, logger, store := setup(t)

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)
	template := notifications.TemplateWorkspaceDeleted

	const (
		method      = database.NotificationMethodSmtp
		maxAttempts = 3
		debug       = false
	)

	// given
	cfg := defaultNotificationsConfig(method)
	cfg.MaxSendAttempts = maxAttempts
	// Tune the intervals low to increase test speed.
	cfg.FetchInterval = serpent.Duration(time.Millisecond * 50)
	cfg.RetryInterval = serpent.Duration(time.Millisecond * 50)
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100) // Twice as long as fetch interval to ensure we catch pending updates.

	mgr, err := notifications.NewManager(cfg, store, metrics, logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	handler := &fakeHandler{}
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method: handler,
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"))
	require.NoError(t, err)

	// when
	user := createSampleUser(t, store)
	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "success"}, "test") // this will succeed
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "failure"}, "test2") // this will fail and retry (maxAttempts - 1) times
	require.NoError(t, err)

	mgr.Run(ctx)

	// Build fingerprints for the two different series we expect.
	methodTemplateFP := fingerprintLabels(notifications.LabelMethod, string(method), notifications.LabelTemplateID, template.String())
	methodFP := fingerprintLabels(notifications.LabelMethod, string(method))

	var seenPendingUpdates bool

	expected := map[string]func(series string, metric *dto.Metric) bool{
		"coderd_notifications_dispatched_count": func(series string, metric *dto.Metric) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodTemplateFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_dispatched_count == 1: %v", metric.Counter.GetValue())
			}

			// Only 1 message will be dispatched successfully
			return metric.Counter.GetValue() == 1
		},
		"coderd_notifications_temporary_failures_count": func(series string, metric *dto.Metric) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodTemplateFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_temporary_failures_count == %v: %v", maxAttempts-1, metric.Counter.GetValue())
			}

			// 2 temp failures, on the 3rd it'll be marked permanent failure
			return metric.Counter.GetValue() == maxAttempts-1
		},
		"coderd_notifications_permanent_failures_count": func(series string, metric *dto.Metric) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodTemplateFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_permanent_failures_count == 1: %v", metric.Counter.GetValue())
			}

			// 1 permanent failure after retries exhausted
			return metric.Counter.GetValue() == 1
		},
		"coderd_notifications_retry_count": func(series string, metric *dto.Metric) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodTemplateFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_retry_count == %v: %v", maxAttempts-1, metric.Counter.GetValue())
			}

			// 1 original attempts + 2 retries = maxAttempts
			return metric.Counter.GetValue() == maxAttempts-1
		},
		"coderd_notifications_queued_seconds": func(series string, metric *dto.Metric) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_queued_seconds > 0: %v", metric.Histogram.GetSampleSum())
			}

			// Notifications will queue for a non-zero amount of time.
			return metric.Histogram.GetSampleSum() > 0
		},
		"coderd_notifications_dispatcher_send_seconds": func(series string, metric *dto.Metric) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_dispatcher_send_seconds > 0: %v", metric.Histogram.GetSampleSum())
			}

			// Dispatches should take a non-zero amount of time.
			return metric.Histogram.GetSampleSum() > 0
		},
		"coderd_notifications_pending_updates": func(series string, metric *dto.Metric) bool {
			// This is a gauge - so we just have to prove it was _once_ set.
			// See TestPendingUpdatesMetrics for a more precise test.
			if !seenPendingUpdates {
				seenPendingUpdates = metric.Gauge.GetValue() > 0
			}

			if debug {
				t.Logf("coderd_notifications_pending_updates: %v", seenPendingUpdates)
			}

			return seenPendingUpdates
		},
	}

	// then
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()

		gathered, err := reg.Gather()
		assert.NoError(t, err)

		succeeded := len(handler.succeeded)
		failed := len(handler.failed)
		if debug {
			t.Logf("SUCCEEDED == 1: %v, FAILED == %v: %v\n", succeeded, maxAttempts, failed)
		}

		// Ensure that all metrics have a) the expected label combinations (series) and b) the expected values.
		for _, family := range gathered {
			hasExpectedValue, ok := expected[family.GetName()]
			assert.Truef(t, ok, "found unexpected metric family %q", family.GetName())

			for _, metric := range family.Metric {
				if !hasExpectedValue(family.String(), metric) {
					return false
				}
			}
		}

		// One message will succeed.
		return succeeded == 1 &&
			// One message will fail, and exhaust its maxAttempts.
			failed == maxAttempts
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestPendingUpdatesMetrics(t *testing.T) {
	t.Parallel()

	// setup
	ctx := context.Background()
	store := dbmem.New()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)
	template := notifications.TemplateWorkspaceDeleted

	const method = database.NotificationMethodSmtp

	// given
	cfg := defaultNotificationsConfig(method)
	cfg.FetchInterval = serpent.Duration(time.Millisecond * 50)
	cfg.RetryInterval = serpent.Duration(time.Hour) // Delay retries so they don't interfere.
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100)

	syncer := &syncInterceptor{Store: store}
	interceptor := newUpdateSignallingInterceptor(syncer)
	mgr, err := notifications.NewManager(cfg, interceptor, metrics, logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	handler := &fakeHandler{}
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method: handler,
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"))
	require.NoError(t, err)

	// when
	user := createSampleUser(t, store)
	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "success"}, "test") // this will succeed
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "failure"}, "test2") // this will fail and retry (maxAttempts - 1) times
	require.NoError(t, err)

	mgr.Run(ctx)

	// Wait until the handler has dispatched the given notifications.
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()

		return len(handler.succeeded) == 1 && len(handler.failed) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// Wait until we intercept the calls to sync the pending updates to the store.
	success := testutil.RequireRecvCtx(testutil.Context(t, testutil.WaitShort), t, interceptor.updateSuccess)
	failure := testutil.RequireRecvCtx(testutil.Context(t, testutil.WaitShort), t, interceptor.updateFailure)

	// Ensure that the value set in the metric is equivalent to the number of actual pending updates.
	pending := promtest.ToFloat64(metrics.PendingUpdates)
	require.EqualValues(t, pending, success+failure)

	// Unpause the interceptor so the updates can proceed.
	interceptor.proceed.Broadcast()

	// Validate that the store synced the expected number of updates.
	require.Eventually(t, func() bool {
		return syncer.sent.Load() == 1 && syncer.failed.Load() == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// Wait for the updates to be synced and the metric to reflect that.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(metrics.PendingUpdates) == 0
	}, testutil.WaitShort, testutil.IntervalFast)
}

// hasMatchingFingerprint checks if the given metric's series fingerprint matches the reference fingerprint.
func hasMatchingFingerprint(metric *dto.Metric, fp model.Fingerprint) bool {
	return fingerprintLabelPairs(metric.Label) == fp
}

// fingerprintLabelPairs produces a fingerprint unique to the given combination of label pairs.
func fingerprintLabelPairs(lbs []*dto.LabelPair) model.Fingerprint {
	pairs := make([]string, 0, len(lbs)*2)
	for _, lp := range lbs {
		pairs = append(pairs, lp.GetName(), lp.GetValue())
	}

	return fingerprintLabels(pairs...)
}

// fingerprintLabels produces a fingerprint unique to the given pairs of label values.
// MUST contain an even number of arguments (key:value), otherwise it will panic.
func fingerprintLabels(lbs ...string) model.Fingerprint {
	if len(lbs)%2 != 0 {
		panic("imbalanced set of label pairs given")
	}

	lbsSet := make(model.LabelSet, len(lbs)/2)
	for i := 0; i < len(lbs); i += 2 {
		k := lbs[i]
		v := lbs[i+1]
		lbsSet[model.LabelName(k)] = model.LabelValue(v)
	}

	return lbsSet.FastFingerprint()
}

// updateSignallingInterceptor intercepts bulk update calls to the store, and waits on the "proceed" condition to be
// signaled by the caller so it can continue.
type updateSignallingInterceptor struct {
	notifications.Store

	proceed *sync.Cond

	updateSuccess chan int
	updateFailure chan int
}

func newUpdateSignallingInterceptor(interceptor notifications.Store) *updateSignallingInterceptor {
	return &updateSignallingInterceptor{
		Store: interceptor,

		proceed: sync.NewCond(&sync.Mutex{}),

		updateSuccess: make(chan int, 1),
		updateFailure: make(chan int, 1),
	}
}

func (u *updateSignallingInterceptor) BulkMarkNotificationMessagesSent(ctx context.Context, arg database.BulkMarkNotificationMessagesSentParams) (int64, error) {
	u.updateSuccess <- len(arg.IDs)

	u.proceed.L.Lock()
	defer u.proceed.L.Unlock()

	// Wait until signaled so we have a chance to read the number of pending updates.
	u.proceed.Wait()

	return u.Store.BulkMarkNotificationMessagesSent(ctx, arg)
}

func (u *updateSignallingInterceptor) BulkMarkNotificationMessagesFailed(ctx context.Context, arg database.BulkMarkNotificationMessagesFailedParams) (int64, error) {
	u.updateFailure <- len(arg.IDs)

	u.proceed.L.Lock()
	defer u.proceed.L.Unlock()

	// Wait until signaled so we have a chance to read the number of pending updates.
	u.proceed.Wait()

	return u.Store.BulkMarkNotificationMessagesFailed(ctx, arg)
}
