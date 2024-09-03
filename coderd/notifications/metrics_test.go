package notifications_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/testutil"
)

func TestMetrics(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it relies on business-logic only implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)
	template := notifications.TemplateWorkspaceDeleted

	const (
		method      = database.NotificationMethodSmtp
		maxAttempts = 3
		debug       = false
	)

	// GIVEN: a notification manager whose intervals are tuned low (for test speed) and whose dispatches are intercepted
	cfg := defaultNotificationsConfig(method)
	cfg.MaxSendAttempts = maxAttempts
	// Tune the intervals low to increase test speed.
	cfg.FetchInterval = serpent.Duration(time.Millisecond * 50)
	cfg.RetryInterval = serpent.Duration(time.Millisecond * 50)
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100) // Twice as long as fetch interval to ensure we catch pending updates.

	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), metrics, api.Logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	handler := &fakeHandler{}
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method: handler,
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

	// Build fingerprints for the two different series we expect.
	methodTemplateFP := fingerprintLabels(notifications.LabelMethod, string(method), notifications.LabelTemplateID, template.String())
	methodFP := fingerprintLabels(notifications.LabelMethod, string(method))

	expected := map[string]func(metric *dto.Metric, series string) bool{
		"coderd_notifications_dispatch_attempts_total": func(metric *dto.Metric, series string) bool {
			// This metric has 3 possible dispositions; find if any of them match first before we check the metric's value.
			results := map[string]float64{
				notifications.ResultSuccess:  1,               // Only 1 successful delivery.
				notifications.ResultTempFail: maxAttempts - 1, // 2 temp failures, on the 3rd it'll be marked permanent failure.
				notifications.ResultPermFail: 1,               // 1 permanent failure after retries exhausted.
			}

			var match string
			for result, val := range results {
				seriesFP := fingerprintLabels(notifications.LabelMethod, string(method), notifications.LabelTemplateID, template.String(), notifications.LabelResult, result)
				if !hasMatchingFingerprint(metric, seriesFP) {
					continue
				}

				match = result

				if debug {
					t.Logf("coderd_notifications_dispatch_attempts_total{result=%q} == %v: %v", result, val, metric.Counter.GetValue())
				}

				break
			}

			// Could not find a matching series.
			if match == "" {
				assert.Failf(t, "found unexpected series %q", series)
				return false
			}

			// nolint:forcetypeassert // Already checked above.
			target := results[match]
			return metric.Counter.GetValue() == target
		},
		"coderd_notifications_retry_count": func(metric *dto.Metric, series string) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodTemplateFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_retry_count == %v: %v", maxAttempts-1, metric.Counter.GetValue())
			}

			// 1 original attempts + 2 retries = maxAttempts
			return metric.Counter.GetValue() == maxAttempts-1
		},
		"coderd_notifications_queued_seconds": func(metric *dto.Metric, series string) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_queued_seconds > 0: %v", metric.Histogram.GetSampleSum())
			}

			// Notifications will queue for a non-zero amount of time.
			return metric.Histogram.GetSampleSum() > 0
		},
		"coderd_notifications_dispatcher_send_seconds": func(metric *dto.Metric, series string) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodFP), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_dispatcher_send_seconds > 0: %v", metric.Histogram.GetSampleSum())
			}

			// Dispatches should take a non-zero amount of time.
			return metric.Histogram.GetSampleSum() > 0
		},
		"coderd_notifications_inflight_dispatches": func(metric *dto.Metric, series string) bool {
			// This is a gauge, so it can be difficult to get the timing right to catch it.
			// See TestInflightDispatchesMetric for a more precise test.
			return true
		},
		"coderd_notifications_pending_updates": func(metric *dto.Metric, series string) bool {
			// This is a gauge, so it can be difficult to get the timing right to catch it.
			// See TestPendingUpdatesMetric for a more precise test.
			return true
		},
		"coderd_notifications_synced_updates_total": func(metric *dto.Metric, series string) bool {
			if debug {
				t.Logf("coderd_notifications_synced_updates_total = %v: %v", maxAttempts+1, metric.Counter.GetValue())
			}

			// 1 message will exceed its maxAttempts, 1 will succeed on the first try.
			return metric.Counter.GetValue() == maxAttempts+1
		},
	}

	// WHEN: 2 notifications are enqueued, 1 of which will fail until its retries are exhausted, and another which will succeed
	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "success"}, "test") // this will succeed
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "failure"}, "test2") // this will fail and retry (maxAttempts - 1) times
	require.NoError(t, err)

	mgr.Run(ctx)

	// THEN: expect all the defined metrics to be present and have their expected values
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
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
			if !assert.Truef(ct, ok, "found unexpected metric family %q", family.GetName()) {
				t.Logf("found unexpected metric family %q", family.GetName())
				// Bail out fast if precondition is not met.
				ct.FailNow()
			}

			for _, metric := range family.Metric {
				assert.True(ct, hasExpectedValue(metric, metric.String()))
			}
		}

		// One message will succeed.
		assert.Equal(ct, succeeded, 1)
		// One message will fail, and exhaust its maxAttempts.
		assert.Equal(ct, failed, maxAttempts)
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestPendingUpdatesMetric(t *testing.T) {
	t.Parallel()

	// SETUP
	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)
	template := notifications.TemplateWorkspaceDeleted

	const method = database.NotificationMethodSmtp

	// GIVEN: a notification manager whose store updates are intercepted so we can read the number of pending updates set in the metric
	cfg := defaultNotificationsConfig(method)
	cfg.RetryInterval = serpent.Duration(time.Hour) // Delay retries so they don't interfere.
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100)

	syncer := &syncInterceptor{Store: api.Database}
	interceptor := newUpdateSignallingInterceptor(syncer)
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().NewTicker("Manager", "storeSync")
	defer trap.Close()
	mgr, err := notifications.NewManager(cfg, interceptor, defaultHelpers(), metrics, api.Logger.Named("manager"),
		notifications.WithTestClock(mClock))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	handler := &fakeHandler{}
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method: handler,
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

	// WHEN: 2 notifications are enqueued, one of which will fail and one which will succeed
	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "success"}, "test") // this will succeed
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "failure"}, "test2") // this will fail and retry (maxAttempts - 1) times
	require.NoError(t, err)

	mgr.Run(ctx)
	trap.MustWait(ctx).Release() // ensures ticker has been set

	// THEN:
	// Wait until the handler has dispatched the given notifications.
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()

		return len(handler.succeeded) == 1 && len(handler.failed) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// Both handler calls should be pending in the metrics.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(metrics.PendingUpdates) == float64(2)
	}, testutil.WaitShort, testutil.IntervalFast)

	// THEN:
	// Trigger syncing updates
	mClock.Advance(cfg.StoreSyncInterval.Value()).MustWait(ctx)

	// Wait until we intercept the calls to sync the pending updates to the store.
	success := testutil.RequireRecvCtx(testutil.Context(t, testutil.WaitShort), t, interceptor.updateSuccess)
	require.EqualValues(t, 1, success)
	failure := testutil.RequireRecvCtx(testutil.Context(t, testutil.WaitShort), t, interceptor.updateFailure)
	require.EqualValues(t, 1, failure)

	// Validate that the store synced the expected number of updates.
	require.Eventually(t, func() bool {
		return syncer.sent.Load() == 1 && syncer.failed.Load() == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// Wait for the updates to be synced and the metric to reflect that.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(metrics.PendingUpdates) == 0
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestInflightDispatchesMetric(t *testing.T) {
	t.Parallel()

	// SETUP
	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)
	template := notifications.TemplateWorkspaceDeleted

	const method = database.NotificationMethodSmtp

	// GIVEN: a notification manager whose dispatches are intercepted and delayed to measure the number of inflight requests
	cfg := defaultNotificationsConfig(method)
	cfg.LeaseCount = 10
	cfg.FetchInterval = serpent.Duration(time.Millisecond * 50)
	cfg.RetryInterval = serpent.Duration(time.Hour) // Delay retries so they don't interfere.
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100)

	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), metrics, api.Logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})

	handler := &fakeHandler{}
	// Delayer will delay all dispatches by 2x fetch intervals to ensure we catch the requests inflight.
	delayer := newDelayingHandler(cfg.FetchInterval.Value()*2, handler)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method: delayer,
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

	// WHEN: notifications are enqueued which will succeed (and be delayed during dispatch)
	const msgCount = 2
	for i := 0; i < msgCount; i++ {
		_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "success", "i": strconv.Itoa(i)}, "test")
		require.NoError(t, err)
	}

	mgr.Run(ctx)

	// THEN:
	// Ensure we see the dispatches of the messages inflight.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(metrics.InflightDispatches.WithLabelValues(string(method), template.String())) == msgCount
	}, testutil.WaitShort, testutil.IntervalFast)

	// Wait until the handler has dispatched the given notifications.
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()

		return len(handler.succeeded) == msgCount
	}, testutil.WaitShort, testutil.IntervalFast)

	// Wait for the updates to be synced and the metric to reflect that.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(metrics.InflightDispatches) == 0
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestCustomMethodMetricCollection(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		// UpdateNotificationTemplateMethodByID only makes sense with a real database.
		t.Skip("This test requires postgres; it relies on business-logic only implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	var (
		reg             = prometheus.NewRegistry()
		metrics         = notifications.NewMetrics(reg)
		template        = notifications.TemplateWorkspaceDeleted
		anotherTemplate = notifications.TemplateWorkspaceDormant
	)

	const (
		customMethod  = database.NotificationMethodWebhook
		defaultMethod = database.NotificationMethodSmtp
	)

	// GIVEN: a template whose notification method differs from the default.
	out, err := api.Database.UpdateNotificationTemplateMethodByID(ctx, database.UpdateNotificationTemplateMethodByIDParams{
		ID:     template,
		Method: database.NullNotificationMethod{NotificationMethod: customMethod, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, customMethod, out.Method.NotificationMethod)

	// WHEN: two notifications (each with different templates) are enqueued.
	cfg := defaultNotificationsConfig(defaultMethod)
	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), metrics, api.Logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})

	smtpHandler := &fakeHandler{}
	webhookHandler := &fakeHandler{}
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		defaultMethod: smtpHandler,
		customMethod:  webhookHandler,
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

	_, err = enq.Enqueue(ctx, user.ID, template, map[string]string{"type": "success"}, "test")
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, anotherTemplate, map[string]string{"type": "success"}, "test")
	require.NoError(t, err)

	mgr.Run(ctx)

	// THEN: the fake handlers to "dispatch" the notifications.
	require.Eventually(t, func() bool {
		smtpHandler.mu.RLock()
		webhookHandler.mu.RLock()
		defer smtpHandler.mu.RUnlock()
		defer webhookHandler.mu.RUnlock()

		return len(smtpHandler.succeeded) == 1 && len(smtpHandler.failed) == 0 &&
			len(webhookHandler.succeeded) == 1 && len(webhookHandler.failed) == 0
	}, testutil.WaitShort, testutil.IntervalFast)

	// THEN: we should have metric series for both the default and custom notification methods.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(metrics.DispatchAttempts.WithLabelValues(string(defaultMethod), anotherTemplate.String(), notifications.ResultSuccess)) > 0 &&
			promtest.ToFloat64(metrics.DispatchAttempts.WithLabelValues(string(customMethod), template.String(), notifications.ResultSuccess)) > 0
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

	return lbsSet.Fingerprint() // FastFingerprint does not sort the labels.
}

// updateSignallingInterceptor intercepts bulk update calls to the store, and waits on the "proceed" condition to be
// signaled by the caller so it can continue.
type updateSignallingInterceptor struct {
	notifications.Store
	updateSuccess chan int
	updateFailure chan int
}

func newUpdateSignallingInterceptor(interceptor notifications.Store) *updateSignallingInterceptor {
	return &updateSignallingInterceptor{
		Store:         interceptor,
		updateSuccess: make(chan int, 1),
		updateFailure: make(chan int, 1),
	}
}

func (u *updateSignallingInterceptor) BulkMarkNotificationMessagesSent(ctx context.Context, arg database.BulkMarkNotificationMessagesSentParams) (int64, error) {
	u.updateSuccess <- len(arg.IDs)
	return u.Store.BulkMarkNotificationMessagesSent(ctx, arg)
}

func (u *updateSignallingInterceptor) BulkMarkNotificationMessagesFailed(ctx context.Context, arg database.BulkMarkNotificationMessagesFailedParams) (int64, error) {
	u.updateFailure <- len(arg.IDs)
	return u.Store.BulkMarkNotificationMessagesFailed(ctx, arg)
}

type delayingHandler struct {
	h notifications.Handler

	delay time.Duration
}

func newDelayingHandler(delay time.Duration, handler notifications.Handler) *delayingHandler {
	return &delayingHandler{
		delay: delay,
		h:     handler,
	}
}

func (d *delayingHandler) Dispatcher(payload types.MessagePayload, title, body string) (dispatch.DeliveryFunc, error) {
	deliverFn, err := d.h.Dispatcher(payload, title, body)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		time.Sleep(d.delay)

		return deliverFn(ctx, msgID)
	}, nil
}
