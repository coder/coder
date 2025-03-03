package notifications_test

import (
	"context"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"text/template"
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
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)
	tmpl := notifications.TemplateWorkspaceDeleted

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

	mgr, err := notifications.NewManager(cfg, store, defaultHelpers(), metrics, logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	handler := &fakeHandler{}
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           handler,
		database.NotificationMethodInbox: &fakeHandler{},
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, store)

	// Build fingerprints for the two different series we expect.
	methodTemplateFP := fingerprintLabels(notifications.LabelMethod, string(method), notifications.LabelTemplateID, tmpl.String())
	methodTemplateFPWithInbox := fingerprintLabels(notifications.LabelMethod, string(database.NotificationMethodInbox), notifications.LabelTemplateID, tmpl.String())

	methodFP := fingerprintLabels(notifications.LabelMethod, string(method))
	methodFPWithInbox := fingerprintLabels(notifications.LabelMethod, string(database.NotificationMethodInbox))

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
				seriesFP := fingerprintLabels(notifications.LabelMethod, string(method), notifications.LabelTemplateID, tmpl.String(), notifications.LabelResult, result)
				seriesFPWithInbox := fingerprintLabels(notifications.LabelMethod, string(database.NotificationMethodInbox), notifications.LabelTemplateID, tmpl.String(), notifications.LabelResult, result)
				if !hasMatchingFingerprint(metric, seriesFP) && !hasMatchingFingerprint(metric, seriesFPWithInbox) {
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
			assert.Truef(t, hasMatchingFingerprint(metric, methodTemplateFP) || hasMatchingFingerprint(metric, methodTemplateFPWithInbox), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_retry_count == %v: %v", maxAttempts-1, metric.Counter.GetValue())
			}

			// 1 original attempts + 2 retries = maxAttempts
			return metric.Counter.GetValue() == maxAttempts-1
		},
		"coderd_notifications_queued_seconds": func(metric *dto.Metric, series string) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodFP) || hasMatchingFingerprint(metric, methodFPWithInbox), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_queued_seconds > 0: %v", metric.Histogram.GetSampleSum())
			}

			// This check is extremely flaky on windows. It fails more often than not, but not always.
			if runtime.GOOS == "windows" {
				return true
			}

			// Notifications will queue for a non-zero amount of time.
			return metric.Histogram.GetSampleSum() > 0
		},
		"coderd_notifications_dispatcher_send_seconds": func(metric *dto.Metric, series string) bool {
			assert.Truef(t, hasMatchingFingerprint(metric, methodFP) || hasMatchingFingerprint(metric, methodFPWithInbox), "found unexpected series %q", series)

			if debug {
				t.Logf("coderd_notifications_dispatcher_send_seconds > 0: %v", metric.Histogram.GetSampleSum())
			}

			// This check is extremely flaky on windows. It fails more often than not, but not always.
			if runtime.GOOS == "windows" {
				return true
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
			t.Logf("values : %v", metric.Counter.GetValue())
			t.Logf("max Attempt : %v", maxAttempts+1)
			return metric.Counter.GetValue() == (maxAttempts+1)*2 // *2 because we have 2 enqueuers.
		},
	}

	// WHEN: 2 notifications are enqueued, 1 of which will fail until its retries are exhausted, and another which will succeed
	_, err = enq.Enqueue(ctx, user.ID, tmpl, map[string]string{"type": "success"}, "test") // this will succeed
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, tmpl, map[string]string{"type": "failure"}, "test2") // this will fail and retry (maxAttempts - 1) times
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
				t.Logf("metric ----> %q", metric)
				t.Logf("metric(string) ----> %q", metric.String())
				t.Logf("family(GetName) ----> %q", family.GetName())
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
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)
	tmpl := notifications.TemplateWorkspaceDeleted

	const method = database.NotificationMethodSmtp

	// GIVEN: a notification manager whose store updates are intercepted so we can read the number of pending updates set in the metric
	cfg := defaultNotificationsConfig(method)
	cfg.RetryInterval = serpent.Duration(time.Hour) // Delay retries so they don't interfere.
	cfg.FetchInterval = serpent.Duration(time.Millisecond * 50)
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100)

	syncer := &syncInterceptor{Store: store}
	interceptor := newUpdateSignallingInterceptor(syncer)
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().NewTicker("Manager", "storeSync")
	defer trap.Close()
	fetchTrap := mClock.Trap().TickerFunc("notifier", "fetchInterval")
	defer fetchTrap.Close()
	mgr, err := notifications.NewManager(cfg, interceptor, defaultHelpers(), metrics, logger.Named("manager"),
		notifications.WithTestClock(mClock))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	handler := &fakeHandler{}
	inboxHandler := &fakeHandler{}

	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           handler,
		database.NotificationMethodInbox: inboxHandler,
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, store)

	// WHEN: 2 notifications are enqueued, one of which will fail and one which will succeed
	_, err = enq.Enqueue(ctx, user.ID, tmpl, map[string]string{"type": "success"}, "test") // this will succeed
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, tmpl, map[string]string{"type": "failure"}, "test2") // this will fail and retry (maxAttempts - 1) times
	require.NoError(t, err)

	mgr.Run(ctx)
	trap.MustWait(ctx).Release() // ensures ticker has been set
	fetchTrap.MustWait(ctx).Release()

	// Advance to the first fetch
	mClock.Advance(cfg.FetchInterval.Value()).MustWait(ctx)

	// THEN:
	// handler has dispatched the given notifications.
	func() {
		handler.mu.RLock()
		defer handler.mu.RUnlock()

		require.Len(t, handler.succeeded, 1)
		require.Len(t, handler.failed, 1)
	}()

	// Both handler calls should be pending in the metrics.
	require.EqualValues(t, 4, promtest.ToFloat64(metrics.PendingUpdates))

	// THEN:
	// Trigger syncing updates
	mClock.Advance(cfg.StoreSyncInterval.Value() - cfg.FetchInterval.Value()).MustWait(ctx)

	// Wait until we intercept the calls to sync the pending updates to the store.
	success := testutil.RequireRecvCtx(testutil.Context(t, testutil.WaitShort), t, interceptor.updateSuccess)
	require.EqualValues(t, 2, success)
	failure := testutil.RequireRecvCtx(testutil.Context(t, testutil.WaitShort), t, interceptor.updateFailure)
	require.EqualValues(t, 2, failure)

	// Validate that the store synced the expected number of updates.
	require.Eventually(t, func() bool {
		return syncer.sent.Load() == 2 && syncer.failed.Load() == 2
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
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)
	tmpl := notifications.TemplateWorkspaceDeleted

	const method = database.NotificationMethodSmtp

	// GIVEN: a notification manager whose dispatches are intercepted and delayed to measure the number of inflight requests
	cfg := defaultNotificationsConfig(method)
	cfg.LeaseCount = 10
	cfg.FetchInterval = serpent.Duration(time.Millisecond * 50)
	cfg.RetryInterval = serpent.Duration(time.Hour) // Delay retries so they don't interfere.
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100)

	mgr, err := notifications.NewManager(cfg, store, defaultHelpers(), metrics, logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})

	handler := &fakeHandler{}
	const msgCount = 2

	// Barrier handler will wait until all notification messages are in-flight.
	barrier := newBarrierHandler(msgCount, handler)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           barrier,
		database.NotificationMethodInbox: &fakeHandler{},
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, store)

	// WHEN: notifications are enqueued which will succeed (and be delayed during dispatch)
	for i := 0; i < msgCount; i++ {
		_, err = enq.Enqueue(ctx, user.ID, tmpl, map[string]string{"type": "success", "i": strconv.Itoa(i)}, "test")
		require.NoError(t, err)
	}

	mgr.Run(ctx)

	// THEN:
	// Ensure we see the dispatches of the messages inflight.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(metrics.InflightDispatches.WithLabelValues(string(method), tmpl.String())) == msgCount
	}, testutil.WaitShort, testutil.IntervalFast)

	for i := 0; i < msgCount; i++ {
		barrier.wg.Done()
	}

	// Wait until the handler has dispatched the given notifications.
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()

		return len(handler.succeeded) == msgCount
	}, testutil.WaitShort, testutil.IntervalFast)

	// Wait for the updates to be synced and the metric to reflect that.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(metrics.InflightDispatches.WithLabelValues(string(method), tmpl.String())) == 0
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
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	var (
		reg             = prometheus.NewRegistry()
		metrics         = notifications.NewMetrics(reg)
		tmpl            = notifications.TemplateWorkspaceDeleted
		anotherTemplate = notifications.TemplateWorkspaceDormant
	)

	const (
		customMethod  = database.NotificationMethodWebhook
		defaultMethod = database.NotificationMethodSmtp
	)

	// GIVEN: a template whose notification method differs from the default.
	out, err := store.UpdateNotificationTemplateMethodByID(ctx, database.UpdateNotificationTemplateMethodByIDParams{
		ID:     tmpl,
		Method: database.NullNotificationMethod{NotificationMethod: customMethod, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, customMethod, out.Method.NotificationMethod)

	// WHEN: two notifications (each with different templates) are enqueued.
	cfg := defaultNotificationsConfig(defaultMethod)
	mgr, err := notifications.NewManager(cfg, store, defaultHelpers(), metrics, logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})

	smtpHandler := &fakeHandler{}
	webhookHandler := &fakeHandler{}
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		defaultMethod:                    smtpHandler,
		customMethod:                     webhookHandler,
		database.NotificationMethodInbox: &fakeHandler{},
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, store)

	_, err = enq.Enqueue(ctx, user.ID, tmpl, map[string]string{"type": "success"}, "test")
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
			promtest.ToFloat64(metrics.DispatchAttempts.WithLabelValues(string(customMethod), tmpl.String(), notifications.ResultSuccess)) > 0
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

type barrierHandler struct {
	h notifications.Handler

	wg *sync.WaitGroup
}

func newBarrierHandler(total int, handler notifications.Handler) *barrierHandler {
	var wg sync.WaitGroup
	wg.Add(total)

	return &barrierHandler{
		h:  handler,
		wg: &wg,
	}
}

func (bh *barrierHandler) Dispatcher(payload types.MessagePayload, title, body string, helpers template.FuncMap) (dispatch.DeliveryFunc, error) {
	deliverFn, err := bh.h.Dispatcher(payload, title, body, helpers)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		bh.wg.Wait()

		return deliverFn(ctx, msgID)
	}, nil
}
