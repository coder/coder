package notifications_test

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	smtpmock "github.com/mocktools/go-smtp-mock/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/dispatch/smtptest"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/syncmap"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

// updateGoldenFiles is a flag that can be set to update golden files.
var updateGoldenFiles = flag.Bool("update", false, "Update golden files")

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

// TestBasicNotificationRoundtrip enqueues a message to the store, waits for it to be acquired by a notifier,
// passes it off to a fake handler, and ensures the results are synchronized to the store.
func TestBasicNotificationRoundtrip(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it relies on business-logic only implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)
	method := database.NotificationMethodSmtp

	// GIVEN: a manager with standard config but a faked dispatch handler
	handler := &fakeHandler{}
	interceptor := &syncInterceptor{Store: store}
	cfg := defaultNotificationsConfig(method)
	cfg.RetryInterval = serpent.Duration(time.Hour) // Ensure retries don't interfere with the test
	mgr, err := notifications.NewManager(cfg, interceptor, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           handler,
		database.NotificationMethodInbox: &fakeHandler{},
	})
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, store)

	// WHEN: 2 messages are enqueued
	sid, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"type": "success"}, "test")
	require.NoError(t, err)
	fid, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"type": "failure"}, "test")
	require.NoError(t, err)

	mgr.Run(ctx)

	// THEN: we expect that the handler will have received the notifications for dispatch
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()
		return slices.Contains(handler.succeeded, sid[0].String()) &&
			slices.Contains(handler.failed, fid[0].String())
	}, testutil.WaitLong, testutil.IntervalFast)

	// THEN: we expect the store to be called with the updates of the earlier dispatches
	require.Eventually(t, func() bool {
		return interceptor.sent.Load() == 2 &&
			interceptor.failed.Load() == 2
	}, testutil.WaitLong, testutil.IntervalFast)

	// THEN: we verify that the store contains notifications in their expected state
	success, err := store.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusSent,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, success, 2)
	failed, err := store.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusTemporaryFailure,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, failed, 2)
}

func TestSMTPDispatch(t *testing.T) {
	t.Parallel()

	// SETUP

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	// start mock SMTP server
	mockSMTPSrv := smtpmock.New(smtpmock.ConfigurationAttr{
		LogToStdout:       false,
		LogServerActivity: true,
	})
	require.NoError(t, mockSMTPSrv.Start())
	t.Cleanup(func() {
		assert.NoError(t, mockSMTPSrv.Stop())
	})

	// GIVEN: an SMTP setup referencing a mock SMTP server
	const from = "danny@coder.com"
	method := database.NotificationMethodSmtp
	cfg := defaultNotificationsConfig(method)
	cfg.SMTP = codersdk.NotificationsEmailConfig{
		From:      from,
		Smarthost: serpent.String(fmt.Sprintf("localhost:%d", mockSMTPSrv.PortNumber())),
		Hello:     "localhost",
	}
	handler := newDispatchInterceptor(dispatch.NewSMTPHandler(cfg.SMTP, logger.Named("smtp")))
	mgr, err := notifications.NewManager(cfg, store, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           handler,
		database.NotificationMethodInbox: &fakeHandler{},
	})
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, store)

	// WHEN: a message is enqueued
	msgID, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{}, "test")
	require.NoError(t, err)
	require.Len(t, msgID, 2)

	mgr.Run(ctx)

	// THEN: wait until the dispatch interceptor validates that the messages were dispatched
	require.Eventually(t, func() bool {
		assert.Nil(t, handler.lastErr.Load())
		assert.True(t, handler.retryable.Load() == 0)
		return handler.sent.Load() == 1
	}, testutil.WaitLong, testutil.IntervalMedium)

	// THEN: we verify that the expected message was received by the mock SMTP server
	msgs := mockSMTPSrv.MessagesAndPurge()
	require.Len(t, msgs, 1)
	require.Contains(t, msgs[0].MsgRequest(), fmt.Sprintf("From: %s", from))
	require.Contains(t, msgs[0].MsgRequest(), fmt.Sprintf("To: %s", user.Email))
	require.Contains(t, msgs[0].MsgRequest(), fmt.Sprintf("Message-Id: %s", msgID[0]))
}

func TestWebhookDispatch(t *testing.T) {
	t.Parallel()

	// SETUP

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	sent := make(chan dispatch.WebhookPayload, 1)
	// Mock server to simulate webhook endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload dispatch.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("noted."))
		assert.NoError(t, err)
		sent <- payload
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)

	// GIVEN: a webhook setup referencing a mock HTTP server to receive the webhook
	cfg := defaultNotificationsConfig(database.NotificationMethodWebhook)
	cfg.Webhook = codersdk.NotificationsWebhookConfig{
		Endpoint: *serpent.URLOf(endpoint),
	}
	mgr, err := notifications.NewManager(cfg, store, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	const (
		email    = "bob@coder.com"
		name     = "Robert McBobbington"
		username = "bob"
	)
	user := dbgen.User(t, store, database.User{
		Email:    email,
		Username: username,
		Name:     name,
	})

	// WHEN: a notification is enqueued (including arbitrary labels)
	input := map[string]string{
		"a": "b",
		"c": "d",
	}
	msgID, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, input, "test")
	require.NoError(t, err)

	mgr.Run(ctx)

	// THEN: the webhook is received by the mock server and has the expected contents
	payload := testutil.RequireRecvCtx(testutil.Context(t, testutil.WaitShort), t, sent)
	require.EqualValues(t, "1.1", payload.Version)
	require.Equal(t, msgID[0], payload.MsgID)
	require.Equal(t, payload.Payload.Labels, input)
	require.Equal(t, payload.Payload.UserEmail, email)
	// UserName is coalesced from `name` and `username`; in this case `name` wins.
	// This is not strictly necessary for this test, but it's testing some side logic which is too small for its own test.
	require.Equal(t, payload.Payload.UserName, name)
	require.Equal(t, payload.Payload.UserUsername, username)
	// Right now we don't have a way to query notification templates by ID in dbmem, and it's not necessary to add this
	// just to satisfy this test. We can safely assume that as long as this value is not empty that the given value was delivered.
	require.NotEmpty(t, payload.Payload.NotificationName)
}

// TestBackpressure validates that delays in processing the buffered updates will result in slowed dequeue rates.
// As a side-effect, this also tests the graceful shutdown and flushing of the buffers.
func TestBackpressure(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it relies on business-logic only implemented in the database")
	}

	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)
	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitShort))

	const method = database.NotificationMethodWebhook
	cfg := defaultNotificationsConfig(method)

	// Tune the queue to fetch often.
	const fetchInterval = time.Millisecond * 200
	const batchSize = 10
	cfg.FetchInterval = serpent.Duration(fetchInterval)
	cfg.LeaseCount = serpent.Int64(batchSize)
	// never time out for this test
	cfg.LeasePeriod = serpent.Duration(time.Hour)
	cfg.DispatchTimeout = serpent.Duration(time.Hour - time.Millisecond)

	// Shrink buffers down and increase flush interval to provoke backpressure.
	// Flush buffers every 5 fetch intervals.
	const syncInterval = time.Second
	cfg.StoreSyncInterval = serpent.Duration(syncInterval)
	cfg.StoreSyncBufferSize = serpent.Int64(2)

	handler := &chanHandler{calls: make(chan dispatchCall)}

	// Intercept calls to submit the buffered updates to the store.
	storeInterceptor := &syncInterceptor{Store: store}

	mClock := quartz.NewMock(t)
	syncTrap := mClock.Trap().NewTicker("Manager", "storeSync")
	defer syncTrap.Close()
	fetchTrap := mClock.Trap().TickerFunc("notifier", "fetchInterval")
	defer fetchTrap.Close()

	// GIVEN: a notification manager whose updates will be intercepted
	mgr, err := notifications.NewManager(cfg, storeInterceptor, pubsub, defaultHelpers(), createMetrics(),
		logger.Named("manager"), notifications.WithTestClock(mClock))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           handler,
		database.NotificationMethodInbox: handler,
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), mClock)
	require.NoError(t, err)

	user := createSampleUser(t, store)

	// WHEN: a set of notifications are enqueued, which causes backpressure due to the batchSize which can be processed per fetch
	const totalMessages = 30
	for i := range totalMessages {
		_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"i": fmt.Sprintf("%d", i)}, "test")
		require.NoError(t, err)
	}

	// Start the notifier.
	mgr.Run(ctx)
	syncTrap.MustWait(ctx).Release()
	fetchTrap.MustWait(ctx).Release()

	// THEN:

	// Trigger a fetch
	w := mClock.Advance(fetchInterval)

	// one batch of dispatches is sent
	for range batchSize {
		call := testutil.RequireRecvCtx(ctx, t, handler.calls)
		testutil.RequireSendCtx(ctx, t, call.result, dispatchResult{
			retryable: false,
			err:       nil,
		})
	}

	// The first fetch will not complete, because of the short sync buffer of 2. This is the
	// backpressure.
	select {
	case <-time.After(testutil.IntervalMedium):
		// success
	case <-w.Done():
		t.Fatal("fetch completed despite backpressure")
	}

	// We expect that the store will have received NO updates.
	require.EqualValues(t, 0, storeInterceptor.sent.Load()+storeInterceptor.failed.Load())

	// However, when we Stop() the manager the backpressure will be relieved and the buffered updates will ALL be flushed,
	// since all the goroutines that were blocked (on writing updates to the buffer) will be unblocked and will complete.
	// Stop() waits for the in-progress flush to complete, meaning we have to advance the time such that sync triggers
	// a total of (batchSize/StoreSyncBufferSize)-1 times. The -1 is because once we run the penultimate sync, it
	// clears space in the buffer for the last dispatches of the batch, which allows graceful shutdown to continue
	// immediately, without waiting for the last trigger.
	stopErr := make(chan error, 1)
	go func() {
		stopErr <- mgr.Stop(ctx)
	}()
	elapsed := fetchInterval
	syncEnd := time.Duration(batchSize/cfg.StoreSyncBufferSize.Value()-1) * cfg.StoreSyncInterval.Value()
	t.Logf("will advance until %dms have elapsed", syncEnd.Milliseconds())
	for elapsed < syncEnd {
		d, wt := mClock.AdvanceNext()
		elapsed += d
		t.Logf("elapsed: %dms", elapsed.Milliseconds())
		// fetches complete immediately, since TickerFunc only allows one call to the callback in flight at at time.
		wt.MustWait(ctx)
		if elapsed%cfg.StoreSyncInterval.Value() == 0 {
			numSent := cfg.StoreSyncBufferSize.Value() * int64(elapsed/cfg.StoreSyncInterval.Value())
			t.Logf("waiting for %d messages", numSent)
			require.Eventually(t, func() bool {
				// need greater or equal because the last set of messages can come immediately due
				// to graceful shut down
				return int64(storeInterceptor.sent.Load()) >= numSent
			}, testutil.WaitShort, testutil.IntervalFast)
		}
	}
	t.Log("done advancing")
	// The batch completes
	w.MustWait(ctx)

	require.NoError(t, testutil.RequireRecvCtx(ctx, t, stopErr))
	require.EqualValues(t, batchSize, storeInterceptor.sent.Load()+storeInterceptor.failed.Load())
}

func TestRetries(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it relies on business-logic only implemented in the database")
	}

	const maxAttempts = 3
	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	// GIVEN: a mock HTTP server which will receive webhooksand a map to track the dispatch attempts

	receivedMap := syncmap.New[uuid.UUID, int]()
	// Mock server to simulate webhook endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload dispatch.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)

		count, _ := receivedMap.LoadOrStore(payload.MsgID, 0)
		count++
		receivedMap.Store(payload.MsgID, count)

		// Let the request succeed if this is its last attempt.
		if count == maxAttempts {
			w.WriteHeader(http.StatusOK)
			_, err = w.Write([]byte("noted."))
			assert.NoError(t, err)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		_, err = w.Write([]byte("retry again later..."))
		assert.NoError(t, err)
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)

	method := database.NotificationMethodWebhook
	cfg := defaultNotificationsConfig(method)
	cfg.Webhook = codersdk.NotificationsWebhookConfig{
		Endpoint: *serpent.URLOf(endpoint),
	}

	cfg.MaxSendAttempts = maxAttempts

	// Tune intervals low to speed up test.
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100)
	cfg.RetryInterval = serpent.Duration(time.Second) // query uses second-precision
	cfg.FetchInterval = serpent.Duration(time.Millisecond * 100)

	handler := newDispatchInterceptor(dispatch.NewWebhookHandler(cfg.Webhook, logger.Named("webhook")))

	// Intercept calls to submit the buffered updates to the store.
	storeInterceptor := &syncInterceptor{Store: store}

	mgr, err := notifications.NewManager(cfg, storeInterceptor, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           handler,
		database.NotificationMethodInbox: &fakeHandler{},
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, store)

	// WHEN: a few notifications are enqueued, which will all fail until their final retry (determined by the mock server)
	const msgCount = 5
	for i := 0; i < msgCount; i++ {
		_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"i": fmt.Sprintf("%d", i)}, "test")
		require.NoError(t, err)
	}

	mgr.Run(ctx)

	// the number of tries is equal to the number of messages times the number of attempts
	// times 2 as the Enqueue method pushes into both the defined dispatch method and inbox
	nbTries := msgCount * maxAttempts * 2

	// THEN: we expect to see all but the final attempts failing on webhook, and all messages to fail on inbox
	require.Eventually(t, func() bool {
		// nolint:gosec
		return storeInterceptor.failed.Load() == int32(nbTries-msgCount) &&
			storeInterceptor.sent.Load() == msgCount
	}, testutil.WaitLong, testutil.IntervalFast)
}

// TestExpiredLeaseIsRequeued validates that notification messages which are left in "leased" status will be requeued once their lease expires.
// "leased" is the status which messages are set to when they are acquired for processing, and this should not be a terminal
// state unless the Manager shuts down ungracefully; the Manager is responsible for updating these messages' statuses once
// they have been processed.
func TestExpiredLeaseIsRequeued(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it relies on business-logic only implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	// GIVEN: a manager which has its updates intercepted and paused until measurements can be taken

	const (
		leasePeriod = time.Second
		msgCount    = 5
		method      = database.NotificationMethodSmtp
	)

	cfg := defaultNotificationsConfig(method)
	// Set low lease period to speed up tests.
	cfg.LeasePeriod = serpent.Duration(leasePeriod)
	cfg.DispatchTimeout = serpent.Duration(leasePeriod - time.Millisecond)

	noopInterceptor := newNoopStoreSyncer(store)

	// nolint:gocritic // Unit test.
	mgrCtx, cancelManagerCtx := context.WithCancel(dbauthz.AsNotifier(context.Background()))
	t.Cleanup(cancelManagerCtx)

	mgr, err := notifications.NewManager(cfg, noopInterceptor, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, store)

	// WHEN: a few notifications are enqueued which will all succeed
	var msgs []string
	for i := 0; i < msgCount; i++ {
		ids, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted,
			map[string]string{"type": "success", "index": fmt.Sprintf("%d", i)}, "test")
		require.NoError(t, err)
		require.Len(t, ids, 2)
		msgs = append(msgs, ids[0].String(), ids[1].String())
	}

	mgr.Run(mgrCtx)

	// THEN:

	// Wait for the messages to be acquired
	<-noopInterceptor.acquiredChan
	// Then cancel the context, forcing the notification manager to shutdown ungracefully (simulating a crash); leaving messages in "leased" status.
	cancelManagerCtx()

	// Fetch any messages currently in "leased" status, and verify that they're exactly the ones we enqueued.
	leased, err := store.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusLeased,
		Limit:  msgCount * 2,
	})
	require.NoError(t, err)

	var leasedIDs []string
	for _, msg := range leased {
		leasedIDs = append(leasedIDs, msg.ID.String())
	}

	sort.Strings(msgs)
	sort.Strings(leasedIDs)
	require.EqualValues(t, msgs, leasedIDs)

	// Wait out the lease period; all messages should be eligible to be re-acquired.
	time.Sleep(leasePeriod + time.Millisecond)

	// Start a new notification manager.
	// Intercept calls to submit the buffered updates to the store.
	storeInterceptor := &syncInterceptor{Store: store}
	handler := newDispatchInterceptor(&fakeHandler{})
	mgr, err = notifications.NewManager(cfg, storeInterceptor, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           handler,
		database.NotificationMethodInbox: &fakeHandler{},
	})

	// Use regular context now.
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	mgr.Run(ctx)

	// Wait until all messages are sent & updates flushed to the database.
	require.Eventually(t, func() bool {
		return handler.sent.Load() == msgCount &&
			storeInterceptor.sent.Load() == msgCount*2
	}, testutil.WaitLong, testutil.IntervalFast)

	// Validate that no more messages are in "leased" status.
	leased, err = store.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusLeased,
		Limit:  msgCount,
	})
	require.NoError(t, err)
	require.Len(t, leased, 0)
}

// TestInvalidConfig validates that misconfigurations lead to errors.
func TestInvalidConfig(t *testing.T) {
	t.Parallel()

	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	// GIVEN: invalid config with dispatch period <= lease period
	const (
		leasePeriod = time.Second
		method      = database.NotificationMethodSmtp
	)
	cfg := defaultNotificationsConfig(method)
	cfg.LeasePeriod = serpent.Duration(leasePeriod)
	cfg.DispatchTimeout = serpent.Duration(leasePeriod)

	// WHEN: the manager is created with invalid config
	_, err := notifications.NewManager(cfg, store, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))

	// THEN: the manager will fail to be created, citing invalid config as error
	require.ErrorIs(t, err, notifications.ErrInvalidDispatchTimeout)
}

func TestNotifierPaused(t *testing.T) {
	t.Parallel()

	// Setup.

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	// Prepare the test.
	handler := &fakeHandler{}
	method := database.NotificationMethodSmtp
	user := createSampleUser(t, store)

	const fetchInterval = time.Millisecond * 100
	cfg := defaultNotificationsConfig(method)
	cfg.FetchInterval = serpent.Duration(fetchInterval)
	mgr, err := notifications.NewManager(cfg, store, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		method:                           handler,
		database.NotificationMethodInbox: &fakeHandler{},
	})
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	// Pause the notifier.
	settingsJSON, err := json.Marshal(&codersdk.NotificationsSettings{NotifierPaused: true})
	require.NoError(t, err)
	err = store.UpsertNotificationsSettings(ctx, string(settingsJSON))
	require.NoError(t, err)

	// Start the manager so that notifications are processed, except it will be paused at this point.
	// If it is started before pausing, there's a TOCTOU possibility between checking whether the notifier is paused or
	// not, and processing the messages (see notifier.run).
	mgr.Run(ctx)

	// Notifier is paused, enqueue the next message.
	sid, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"type": "success", "i": "1"}, "test")
	require.NoError(t, err)

	// Ensure we have a pending message and it's the expected one.
	pendingMessages, err := store.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusPending,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, pendingMessages, 2)
	require.Equal(t, pendingMessages[0].ID.String(), sid[0].String())
	require.Equal(t, pendingMessages[1].ID.String(), sid[1].String())

	// Wait a few fetch intervals to be sure that no new notifications are being sent.
	// TODO: use quartz instead.
	// nolint:gocritic // These magic numbers are fine.
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()

		return len(handler.succeeded)+len(handler.failed) == 0
	}, fetchInterval*5, testutil.IntervalFast)

	// Unpause the notifier.
	settingsJSON, err = json.Marshal(&codersdk.NotificationsSettings{NotifierPaused: false})
	require.NoError(t, err)
	err = store.UpsertNotificationsSettings(ctx, string(settingsJSON))
	require.NoError(t, err)

	// Notifier is running again, message should be dequeued.
	// nolint:gocritic // These magic numbers are fine.
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()
		return slices.Contains(handler.succeeded, sid[0].String())
	}, fetchInterval*5, testutil.IntervalFast)
}

//go:embed events.go
var events []byte

// enumerateAllTemplates gets all the template names from the coderd/notifications/events.go file.
// TODO(dannyk): use code-generation to create a list of all templates: https://github.com/coder/team-coconut/issues/36
func enumerateAllTemplates(t *testing.T) ([]string, error) {
	t.Helper()

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, "", bytes.NewBuffer(events), parser.AllErrors)
	if err != nil {
		return nil, err
	}

	var out []string
	// Traverse the AST and extract variable names.
	ast.Inspect(node, func(n ast.Node) bool {
		// Check if the node is a declaration statement.
		if decl, ok := n.(*ast.GenDecl); ok && decl.Tok == token.VAR {
			for _, spec := range decl.Specs {
				// Type assert the spec to a ValueSpec.
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range valueSpec.Names {
						out = append(out, name.String())
					}
				}
			}
		}
		return true
	})

	return out, nil
}

func TestNotificationTemplates_Golden(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it relies on the notification templates added by migrations in the database")
	}

	const (
		username = "bob"
		password = "🤫"

		hello = "localhost"

		from = "system@coder.com"
		hint = "run \"DB=ci make gen/golden-files\" and commit the changes"
	)

	tests := []struct {
		name    string
		id      uuid.UUID
		payload types.MessagePayload

		appName string
		logoURL string
	}{
		{
			name: "TemplateWorkspaceDeleted",
			id:   notifications.TemplateWorkspaceDeleted,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"name":      "bobby-workspace",
					"reason":    "autodeleted due to dormancy",
					"initiator": "autobuild",
				},
				Targets: []uuid.UUID{
					uuid.MustParse("5c6ea841-ca63-46cc-9c37-78734c7a788b"),
					uuid.MustParse("b8355e3a-f3c5-4dd1-b382-7eb1fae7db52"),
				},
			},
		},
		{
			name: "TemplateWorkspaceAutobuildFailed",
			id:   notifications.TemplateWorkspaceAutobuildFailed,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"name":   "bobby-workspace",
					"reason": "autostart",
				},
				Targets: []uuid.UUID{
					uuid.MustParse("5c6ea841-ca63-46cc-9c37-78734c7a788b"),
					uuid.MustParse("b8355e3a-f3c5-4dd1-b382-7eb1fae7db52"),
				},
			},
		},
		{
			name: "TemplateWorkspaceDormant",
			id:   notifications.TemplateWorkspaceDormant,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"name":           "bobby-workspace",
					"reason":         "breached the template's threshold for inactivity",
					"initiator":      "autobuild",
					"dormancyHours":  "24",
					"timeTilDormant": "24 hours",
				},
			},
		},
		{
			name: "TemplateWorkspaceAutoUpdated",
			id:   notifications.TemplateWorkspaceAutoUpdated,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"name":                     "bobby-workspace",
					"template_version_name":    "1.0",
					"template_version_message": "template now includes catnip",
				},
			},
		},
		{
			name: "TemplateWorkspaceMarkedForDeletion",
			id:   notifications.TemplateWorkspaceMarkedForDeletion,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"name":           "bobby-workspace",
					"reason":         "template updated to new dormancy policy",
					"dormancyHours":  "24",
					"timeTilDormant": "24 hours",
				},
			},
		},
		{
			name: "TemplateUserAccountCreated",
			id:   notifications.TemplateUserAccountCreated,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"created_account_name":      "bobby",
					"created_account_user_name": "William Tables",
					"initiator":                 "rob",
				},
			},
		},
		{
			name: "TemplateUserAccountDeleted",
			id:   notifications.TemplateUserAccountDeleted,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"deleted_account_name":      "bobby",
					"deleted_account_user_name": "William Tables",
					"initiator":                 "rob",
				},
			},
		},
		{
			name: "TemplateUserAccountSuspended",
			id:   notifications.TemplateUserAccountSuspended,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"suspended_account_name":      "bobby",
					"suspended_account_user_name": "William Tables",
					"initiator":                   "rob",
				},
			},
		},
		{
			name: "TemplateUserAccountActivated",
			id:   notifications.TemplateUserAccountActivated,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"activated_account_name":      "bobby",
					"activated_account_user_name": "William Tables",
					"initiator":                   "rob",
				},
			},
		},
		{
			name: "TemplateYourAccountSuspended",
			id:   notifications.TemplateYourAccountSuspended,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"suspended_account_name": "bobby",
					"initiator":              "rob",
				},
			},
		},
		{
			name: "TemplateYourAccountActivated",
			id:   notifications.TemplateYourAccountActivated,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"activated_account_name": "bobby",
					"initiator":              "rob",
				},
			},
		},
		{
			name: "TemplateTemplateDeleted",
			id:   notifications.TemplateTemplateDeleted,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"name":      "Bobby's Template",
					"initiator": "rob",
				},
			},
		},
		{
			name: "TemplateWorkspaceManualBuildFailed",
			id:   notifications.TemplateWorkspaceManualBuildFailed,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"name":                     "bobby-workspace",
					"template_name":            "bobby-template",
					"template_version_name":    "bobby-template-version",
					"initiator":                "joe",
					"workspace_owner_username": "mrbobby",
					"workspace_build_number":   "3",
				},
			},
		},
		{
			name: "TemplateWorkspaceBuildsFailedReport",
			id:   notifications.TemplateWorkspaceBuildsFailedReport,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				// We need to use floats as `json.Unmarshal` unmarshal numbers in `map[string]any` to floats.
				Data: map[string]any{
					"report_frequency": "week",
					"templates": []map[string]any{
						{
							"name":          "bobby-first-template",
							"display_name":  "Bobby First Template",
							"failed_builds": 4.0,
							"total_builds":  55.0,
							"versions": []map[string]any{
								{
									"template_version_name": "bobby-template-version-1",
									"failed_count":          3.0,
									"failed_builds": []map[string]any{
										{
											"workspace_owner_username": "mtojek",
											"workspace_name":           "workspace-1",
											"workspace_id":             "24f5bd8f-1566-4374-9734-c3efa0454dc7",
											"build_number":             1234.0,
										},
										{
											"workspace_owner_username": "johndoe",
											"workspace_name":           "my-workspace-3",
											"workspace_id":             "372a194b-dcde-43f1-b7cf-8a2f3d3114a0",
											"build_number":             5678.0,
										},
										{
											"workspace_owner_username": "jack",
											"workspace_name":           "workwork",
											"workspace_id":             "1386d294-19c1-4351-89e2-6cae1afb9bfe",
											"build_number":             774.0,
										},
									},
								},
								{
									"template_version_name": "bobby-template-version-2",
									"failed_count":          1.0,
									"failed_builds": []map[string]any{
										{
											"workspace_owner_username": "ben",
											"workspace_name":           "cool-workspace",
											"workspace_id":             "86fd99b1-1b6e-4b7e-b58e-0aee6e35c159",
											"build_number":             8888.0,
										},
									},
								},
							},
						},
						{
							"name":          "bobby-second-template",
							"display_name":  "Bobby Second Template",
							"failed_builds": 5.0,
							"total_builds":  50.0,
							"versions": []map[string]any{
								{
									"template_version_name": "bobby-template-version-1",
									"failed_count":          3.0,
									"failed_builds": []map[string]any{
										{
											"workspace_owner_username": "daniellemaywood",
											"workspace_name":           "workspace-9",
											"workspace_id":             "cd469690-b6eb-4123-b759-980be7a7b278",
											"build_number":             9234.0,
										},
										{
											"workspace_owner_username": "johndoe",
											"workspace_name":           "my-workspace-7",
											"workspace_id":             "c447d472-0800-4529-a836-788754d5e27d",
											"build_number":             8678.0,
										},
										{
											"workspace_owner_username": "jack",
											"workspace_name":           "workworkwork",
											"workspace_id":             "919db6df-48f0-4dc1-b357-9036a2c40f86",
											"build_number":             374.0,
										},
									},
								},
								{
									"template_version_name": "bobby-template-version-2",
									"failed_count":          2.0,
									"failed_builds": []map[string]any{
										{
											"workspace_owner_username": "ben",
											"workspace_name":           "more-cool-workspace",
											"workspace_id":             "c8fb0652-9290-4bf2-a711-71b910243ac2",
											"build_number":             8878.0,
										},
										{
											"workspace_owner_username": "ben",
											"workspace_name":           "less-cool-workspace",
											"workspace_id":             "703d718d-2234-4990-9a02-5b1df6cf462a",
											"build_number":             8848.0,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "TemplateUserRequestedOneTimePasscode",
			id:   notifications.TemplateUserRequestedOneTimePasscode,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby/drop-table+user@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"one_time_passcode": "fad9020b-6562-4cdb-87f1-0486f1bea415",
				},
			},
		},
		{
			name: "TemplateWorkspaceDeleted_CustomAppearance",
			id:   notifications.TemplateWorkspaceDeleted,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"name":      "bobby-workspace",
					"reason":    "autodeleted due to dormancy",
					"initiator": "autobuild",
				},
			},
			appName: "Custom Application Name",
			logoURL: "https://custom.application/logo.png",
		},
		{
			name: "TemplateTemplateDeprecated",
			id:   notifications.TemplateTemplateDeprecated,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"template":     "alpha",
					"message":      "This template has been replaced by beta",
					"organization": "coder",
				},
			},
		},
		{
			name: "TemplateWorkspaceCreated",
			id:   notifications.TemplateWorkspaceCreated,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"workspace":                "bobby-workspace",
					"template":                 "bobby-template",
					"version":                  "alpha",
					"workspace_owner_username": "mrbobby",
				},
			},
		},
		{
			name: "TemplateWorkspaceManuallyUpdated",
			id:   notifications.TemplateWorkspaceManuallyUpdated,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"organization":             "bobby-organization",
					"initiator":                "bobby",
					"workspace":                "bobby-workspace",
					"template":                 "bobby-template",
					"version":                  "alpha",
					"workspace_owner_username": "mrbobby",
				},
			},
		},
		{
			name: "TemplateWorkspaceOutOfMemory",
			id:   notifications.TemplateWorkspaceOutOfMemory,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"workspace": "bobby-workspace",
					"threshold": "90%",
				},
			},
		},
		{
			name: "TemplateWorkspaceOutOfDisk",
			id:   notifications.TemplateWorkspaceOutOfDisk,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"workspace": "bobby-workspace",
				},
				Data: map[string]any{
					"volumes": []map[string]any{
						{
							"path":      "/home/coder",
							"threshold": "90%",
						},
					},
				},
			},
		},
		{
			name: "TemplateWorkspaceOutOfDisk_MultipleVolumes",
			id:   notifications.TemplateWorkspaceOutOfDisk,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels: map[string]string{
					"workspace": "bobby-workspace",
				},
				Data: map[string]any{
					"volumes": []map[string]any{
						{
							"path":      "/home/coder",
							"threshold": "90%",
						},
						{
							"path":      "/dev/coder",
							"threshold": "80%",
						},
						{
							"path":      "/etc/coder",
							"threshold": "95%",
						},
					},
				},
			},
		},
		{
			name: "TemplateTestNotification",
			id:   notifications.TemplateTestNotification,
			payload: types.MessagePayload{
				UserName:     "Bobby",
				UserEmail:    "bobby@coder.com",
				UserUsername: "bobby",
				Labels:       map[string]string{},
			},
		},
	}

	// We must have a test case for every notification_template. This is enforced below:
	allTemplates, err := enumerateAllTemplates(t)
	require.NoError(t, err)
	for _, name := range allTemplates {
		var found bool
		for _, tc := range tests {
			if tc.name == name {
				found = true
			}
		}

		require.Truef(t, found, "could not find test case for %q", name)
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			t.Run("smtp", func(t *testing.T) {
				t.Parallel()

				// Spin up the DB
				db, logger, user := func() (*database.Store, *slog.Logger, *codersdk.User) {
					adminClient, _, api := coderdtest.NewWithAPI(t, nil)
					db := api.Database
					firstUser := coderdtest.CreateFirstUser(t, adminClient)

					_, user := coderdtest.CreateAnotherUserMutators(
						t,
						adminClient,
						firstUser.OrganizationID,
						[]rbac.RoleIdentifier{rbac.RoleUserAdmin()},
						func(r *codersdk.CreateUserRequestWithOrgs) {
							r.Username = tc.payload.UserUsername
							r.Email = tc.payload.UserEmail
							r.Name = tc.payload.UserName
						},
					)

					// With the introduction of notifications that can be disabled
					// by default, we want to make sure the user preferences have
					// the notification enabled.
					_, err := adminClient.UpdateUserNotificationPreferences(
						context.Background(),
						user.ID,
						codersdk.UpdateUserNotificationPreferences{
							TemplateDisabledMap: map[string]bool{
								tc.id.String(): false,
							},
						})
					require.NoError(t, err)

					return &db, &api.Logger, &user
				}()

				// nolint:gocritic // Unit test.
				ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))

				_, pubsub := dbtestutil.NewDB(t)

				// smtp config shared between client and server
				smtpConfig := codersdk.NotificationsEmailConfig{
					Hello: hello,
					From:  from,

					Auth: codersdk.NotificationsEmailAuthConfig{
						Username: username,
						Password: password,
					},
				}

				// Spin up the mock SMTP server
				backend := smtptest.NewBackend(smtptest.Config{
					AuthMechanisms: []string{sasl.Login},

					AcceptedIdentity: smtpConfig.Auth.Identity.String(),
					AcceptedUsername: username,
					AcceptedPassword: password,
				})

				// Create a mock SMTP server which conditionally listens for plain or TLS connections.
				srv, listen, err := smtptest.CreateMockSMTPServer(backend, false)
				require.NoError(t, err)
				t.Cleanup(func() {
					err := srv.Shutdown(ctx)
					require.NoError(t, err)
				})

				var hp serpent.HostPort
				require.NoError(t, hp.Set(listen.Addr().String()))
				smtpConfig.Smarthost = serpent.String(hp.String())

				// Start mock SMTP server in the background.
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					assert.NoError(t, srv.Serve(listen))
				}()

				// Wait for the server to become pingable.
				require.Eventually(t, func() bool {
					cl, err := smtptest.PingClient(listen, false, smtpConfig.TLS.StartTLS.Value())
					if err != nil {
						t.Logf("smtp not yet dialable: %s", err)
						return false
					}

					if err = cl.Noop(); err != nil {
						t.Logf("smtp not yet noopable: %s", err)
						return false
					}

					if err = cl.Close(); err != nil {
						t.Logf("smtp didn't close properly: %s", err)
						return false
					}

					return true
				}, testutil.WaitShort, testutil.IntervalFast)

				smtpCfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
				smtpCfg.SMTP = smtpConfig

				smtpManager, err := notifications.NewManager(
					smtpCfg,
					*db,
					pubsub,
					defaultHelpers(),
					createMetrics(),
					logger.Named("manager"),
				)
				require.NoError(t, err)

				// we apply ApplicationName and LogoURL changes directly in the db
				// as appearance changes are enterprise features and we do not want to mix those
				// can't use the api
				if tc.appName != "" {
					// nolint:gocritic // Unit test.
					err = (*db).UpsertApplicationName(dbauthz.AsSystemRestricted(ctx), "Custom Application")
					require.NoError(t, err)
				}

				if tc.logoURL != "" {
					// nolint:gocritic // Unit test.
					err = (*db).UpsertLogoURL(dbauthz.AsSystemRestricted(ctx), "https://custom.application/logo.png")
					require.NoError(t, err)
				}

				smtpManager.Run(ctx)

				notificationCfg := defaultNotificationsConfig(database.NotificationMethodSmtp)

				smtpEnqueuer, err := notifications.NewStoreEnqueuer(
					notificationCfg,
					*db,
					defaultHelpers(),
					logger.Named("enqueuer"),
					quartz.NewReal(),
				)
				require.NoError(t, err)

				_, err = smtpEnqueuer.EnqueueWithData(
					ctx,
					user.ID,
					tc.id,
					tc.payload.Labels,
					tc.payload.Data,
					user.Username,
					tc.payload.Targets...,
				)
				require.NoError(t, err)

				// Wait for the message to be fetched
				var msg *smtptest.Message
				require.Eventually(t, func() bool {
					msg = backend.LastMessage()
					return msg != nil && len(msg.Contents) > 0
				}, testutil.WaitShort, testutil.IntervalFast)

				body := normalizeGoldenEmail([]byte(msg.Contents))

				err = smtpManager.Stop(ctx)
				require.NoError(t, err)

				partialName := strings.Split(t.Name(), "/")[1]
				goldenFile := filepath.Join("testdata", "rendered-templates", "smtp", partialName+".html.golden")
				if *updateGoldenFiles {
					err = os.MkdirAll(filepath.Dir(goldenFile), 0o755)
					require.NoError(t, err, "want no error creating golden file directory")
					err = os.WriteFile(goldenFile, body, 0o600)
					require.NoError(t, err, "want no error writing body golden file")
					return
				}

				wantBody, err := os.ReadFile(goldenFile)
				require.NoError(t, err, fmt.Sprintf("missing golden notification body file. %s", hint))
				require.Empty(
					t,
					cmp.Diff(wantBody, body),
					fmt.Sprintf("golden file mismatch: %s. If this is expected, %s. (-want +got). ", goldenFile, hint),
				)
			})

			t.Run("webhook", func(t *testing.T) {
				t.Parallel()

				// Spin up the DB
				db, logger, user := func() (*database.Store, *slog.Logger, *codersdk.User) {
					adminClient, _, api := coderdtest.NewWithAPI(t, nil)
					db := api.Database
					firstUser := coderdtest.CreateFirstUser(t, adminClient)

					_, user := coderdtest.CreateAnotherUserMutators(
						t,
						adminClient,
						firstUser.OrganizationID,
						[]rbac.RoleIdentifier{rbac.RoleUserAdmin()},
						func(r *codersdk.CreateUserRequestWithOrgs) {
							r.Username = tc.payload.UserUsername
							r.Email = tc.payload.UserEmail
							r.Name = tc.payload.UserName
						},
					)

					// With the introduction of notifications that can be disabled
					// by default, we want to make sure the user preferences have
					// the notification enabled.
					_, err := adminClient.UpdateUserNotificationPreferences(
						context.Background(),
						user.ID,
						codersdk.UpdateUserNotificationPreferences{
							TemplateDisabledMap: map[string]bool{
								tc.id.String(): false,
							},
						})
					require.NoError(t, err)

					return &db, &api.Logger, &user
				}()

				_, pubsub := dbtestutil.NewDB(t)
				// nolint:gocritic // Unit test.
				ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))

				// Spin up the mock webhook server
				var body []byte
				var readErr error
				webhookReceived := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)

					body, readErr = io.ReadAll(r.Body)
					close(webhookReceived)
				}))
				t.Cleanup(server.Close)

				endpoint, err := url.Parse(server.URL)
				require.NoError(t, err)

				webhookCfg := defaultNotificationsConfig(database.NotificationMethodWebhook)

				webhookCfg.Webhook = codersdk.NotificationsWebhookConfig{
					Endpoint: *serpent.URLOf(endpoint),
				}

				webhookManager, err := notifications.NewManager(
					webhookCfg,
					*db,
					pubsub,
					defaultHelpers(),
					createMetrics(),
					logger.Named("manager"),
				)
				require.NoError(t, err)

				webhookManager.Run(ctx)

				httpEnqueuer, err := notifications.NewStoreEnqueuer(
					defaultNotificationsConfig(database.NotificationMethodWebhook),
					*db,
					defaultHelpers(),
					logger.Named("enqueuer"),
					quartz.NewReal(),
				)
				require.NoError(t, err)

				_, err = httpEnqueuer.EnqueueWithData(
					ctx,
					user.ID,
					tc.id,
					tc.payload.Labels,
					tc.payload.Data,
					user.Username,
					tc.payload.Targets...,
				)
				require.NoError(t, err)

				select {
				case <-time.After(testutil.WaitShort):
					require.Fail(t, "timed out waiting for webhook to be received")
				case <-webhookReceived:
				}
				// Handle the body that was read in the http server here.
				// We need to do it here because we can't call require.* in a separate goroutine, such as the http server handler
				require.NoError(t, readErr)
				var prettyJSON bytes.Buffer
				err = json.Indent(&prettyJSON, body, "", "  ")
				require.NoError(t, err)

				content := normalizeGoldenWebhook(prettyJSON.Bytes())

				partialName := strings.Split(t.Name(), "/")[1]
				goldenFile := filepath.Join("testdata", "rendered-templates", "webhook", partialName+".json.golden")
				if *updateGoldenFiles {
					err = os.MkdirAll(filepath.Dir(goldenFile), 0o755)
					require.NoError(t, err, "want no error creating golden file directory")
					err = os.WriteFile(goldenFile, content, 0o600)
					require.NoError(t, err, "want no error writing body golden file")
					return
				}

				wantBody, err := os.ReadFile(goldenFile)
				require.NoError(t, err, fmt.Sprintf("missing golden notification body file. %s", hint))
				wantBody = normalizeLineEndings(wantBody)
				require.Equal(t, wantBody, content, fmt.Sprintf("smtp notification does not match golden file. If this is expected, %s", hint))
			})
		})
	}
}

// normalizeLineEndings ensures that all line endings are normalized to \n.
// Required for Windows compatibility.
func normalizeLineEndings(content []byte) []byte {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	content = bytes.ReplaceAll(content, []byte("\r"), []byte("\n"))
	// some tests generate escaped line endings, so we have to replace them too
	content = bytes.ReplaceAll(content, []byte("\\r\\n"), []byte("\\n"))
	content = bytes.ReplaceAll(content, []byte("\\r"), []byte("\\n"))
	return content
}

func normalizeGoldenEmail(content []byte) []byte {
	const (
		constantDate      = "Fri, 11 Oct 2024 09:03:06 +0000"
		constantMessageID = "02ee4935-73be-4fa1-a290-ff9999026b13@blush-whale-48"
		constantBoundary  = "bbe61b741255b6098bb6b3c1f41b885773df633cb18d2a3002b68e4bc9c4"
	)

	dateRegex := regexp.MustCompile(`Date: .+`)
	messageIDRegex := regexp.MustCompile(`Message-Id: .+`)
	boundaryRegex := regexp.MustCompile(`boundary=([0-9a-zA-Z]+)`)
	submatches := boundaryRegex.FindSubmatch(content)
	if len(submatches) == 0 {
		return content
	}

	boundary := submatches[1]

	content = dateRegex.ReplaceAll(content, []byte("Date: "+constantDate))
	content = messageIDRegex.ReplaceAll(content, []byte("Message-Id: "+constantMessageID))
	content = bytes.ReplaceAll(content, boundary, []byte(constantBoundary))

	return content
}

func normalizeGoldenWebhook(content []byte) []byte {
	const constantUUID = "00000000-0000-0000-0000-000000000000"
	uuidRegex := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	content = uuidRegex.ReplaceAll(content, []byte(constantUUID))
	content = normalizeLineEndings(content)

	return content
}

func TestDisabledByDefaultBeforeEnqueue(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it is testing business-logic implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	cfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)
	user := createSampleUser(t, store)

	// We want to try enqueuing a notification on a template that is disabled
	// by default. We expect this to fail.
	templateID := notifications.TemplateWorkspaceManuallyUpdated
	_, err = enq.Enqueue(ctx, user.ID, templateID, map[string]string{}, "test")
	require.ErrorIs(t, err, notifications.ErrCannotEnqueueDisabledNotification, "enqueuing did not fail with expected error")
}

// TestDisabledBeforeEnqueue ensures that notifications cannot be enqueued once a user has disabled that notification template
func TestDisabledBeforeEnqueue(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it is testing business-logic implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	// GIVEN: an enqueuer & a sample user
	cfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)
	user := createSampleUser(t, store)

	// WHEN: the user has a preference set to not receive the "workspace deleted" notification
	templateID := notifications.TemplateWorkspaceDeleted
	n, err := store.UpdateUserNotificationPreferences(ctx, database.UpdateUserNotificationPreferencesParams{
		UserID:                  user.ID,
		NotificationTemplateIds: []uuid.UUID{templateID},
		Disableds:               []bool{true},
	})
	require.NoError(t, err, "failed to set preferences")
	require.EqualValues(t, 1, n, "unexpected number of affected rows")

	// THEN: enqueuing the "workspace deleted" notification should fail with an error
	_, err = enq.Enqueue(ctx, user.ID, templateID, map[string]string{}, "test")
	require.ErrorIs(t, err, notifications.ErrCannotEnqueueDisabledNotification, "enqueueing did not fail with expected error")
}

// TestDisabledAfterEnqueue ensures that notifications enqueued before a notification template was disabled will not be
// sent, and will instead be marked as "inhibited".
func TestDisabledAfterEnqueue(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it is testing business-logic implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	method := database.NotificationMethodSmtp
	cfg := defaultNotificationsConfig(method)

	mgr, err := notifications.NewManager(cfg, store, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)
	user := createSampleUser(t, store)

	// GIVEN: a notification is enqueued which has not (yet) been disabled
	templateID := notifications.TemplateWorkspaceDeleted
	msgID, err := enq.Enqueue(ctx, user.ID, templateID, map[string]string{}, "test")
	require.NoError(t, err)

	// Disable the notification template.
	n, err := store.UpdateUserNotificationPreferences(ctx, database.UpdateUserNotificationPreferencesParams{
		UserID:                  user.ID,
		NotificationTemplateIds: []uuid.UUID{templateID},
		Disableds:               []bool{true},
	})
	require.NoError(t, err, "failed to set preferences")
	require.EqualValues(t, 1, n, "unexpected number of affected rows")

	// WHEN: running the manager to trigger dequeueing of (now-disabled) messages
	mgr.Run(ctx)

	// THEN: the message should not be sent, and must be set to "inhibited"
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		m, err := store.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
			Status: database.NotificationMessageStatusInhibited,
			Limit:  10,
		})
		assert.NoError(ct, err)
		if assert.Equal(ct, len(m), 2) {
			assert.Contains(ct, []string{m[0].ID.String(), m[1].ID.String()}, msgID[0].String())
			assert.Contains(ct, m[0].StatusReason.String, "disabled by user")
		}
	}, testutil.WaitLong, testutil.IntervalFast, "did not find the expected inhibited message")
}

func TestCustomNotificationMethod(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it relies on business-logic only implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	received := make(chan uuid.UUID, 1)

	// SETUP:
	// Start mock server to simulate webhook endpoint.
	mockWebhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload dispatch.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)

		received <- payload.MsgID
		close(received)

		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("noted."))
		require.NoError(t, err)
	}))
	defer mockWebhookSrv.Close()

	// Start mock SMTP server.
	mockSMTPSrv := smtpmock.New(smtpmock.ConfigurationAttr{
		LogToStdout:       false,
		LogServerActivity: true,
	})
	require.NoError(t, mockSMTPSrv.Start())
	t.Cleanup(func() {
		assert.NoError(t, mockSMTPSrv.Stop())
	})

	endpoint, err := url.Parse(mockWebhookSrv.URL)
	require.NoError(t, err)

	// GIVEN: a notification template which has a method explicitly set
	var (
		tmpl          = notifications.TemplateWorkspaceDormant
		defaultMethod = database.NotificationMethodSmtp
		customMethod  = database.NotificationMethodWebhook
	)
	out, err := store.UpdateNotificationTemplateMethodByID(ctx, database.UpdateNotificationTemplateMethodByIDParams{
		ID:     tmpl,
		Method: database.NullNotificationMethod{NotificationMethod: customMethod, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, customMethod, out.Method.NotificationMethod)

	// GIVEN: a manager configured with multiple dispatch methods
	cfg := defaultNotificationsConfig(defaultMethod)
	cfg.SMTP = codersdk.NotificationsEmailConfig{
		From:      "danny@coder.com",
		Hello:     "localhost",
		Smarthost: serpent.String(fmt.Sprintf("localhost:%d", mockSMTPSrv.PortNumber())),
	}
	cfg.Webhook = codersdk.NotificationsWebhookConfig{
		Endpoint: *serpent.URLOf(endpoint),
	}

	mgr, err := notifications.NewManager(cfg, store, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = mgr.Stop(ctx)
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	// WHEN: a notification of that template is enqueued, it should be delivered with the configured method - not the default.
	user := createSampleUser(t, store)
	msgID, err := enq.Enqueue(ctx, user.ID, tmpl, map[string]string{}, "test")
	require.NoError(t, err)

	// THEN: the notification should be received by the custom dispatch method
	mgr.Run(ctx)

	receivedMsgID := testutil.RequireRecvCtx(ctx, t, received)
	require.Equal(t, msgID[0].String(), receivedMsgID.String())

	// Ensure no messages received by default method (SMTP):
	msgs := mockSMTPSrv.MessagesAndPurge()
	require.Len(t, msgs, 0)

	// Enqueue a notification which does not have a custom method set to ensure default works correctly.
	msgID, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{}, "test")
	require.NoError(t, err)
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		msgs := mockSMTPSrv.MessagesAndPurge()
		if assert.Len(ct, msgs, 1) {
			assert.Contains(ct, msgs[0].MsgRequest(), fmt.Sprintf("Message-Id: %s", msgID[0]))
		}
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestNotificationsTemplates(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		// Notification system templates are only served from the database and not dbmem at this time.
		t.Skip("This test requires postgres; it relies on business-logic only implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	api := coderdtest.New(t, createOpts(t))

	// GIVEN: the first user (owner) and a regular member
	firstUser := coderdtest.CreateFirstUser(t, api)
	memberClient, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID, rbac.RoleMember())

	// WHEN: requesting system notification templates as owner should work
	templates, err := api.GetSystemNotificationTemplates(ctx)
	require.NoError(t, err)
	require.True(t, len(templates) > 1)

	// WHEN: requesting system notification templates as member should work
	templates, err = memberClient.GetSystemNotificationTemplates(ctx)
	require.NoError(t, err)
	require.True(t, len(templates) > 1)
}

func createOpts(t *testing.T) *coderdtest.Options {
	t.Helper()

	dt := coderdtest.DeploymentValues(t)
	return &coderdtest.Options{
		DeploymentValues: dt,
	}
}

// TestNotificationDuplicates validates that identical notifications cannot be sent on the same day.
func TestNotificationDuplicates(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it is testing the dedupe hash trigger in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
	store, pubsub := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	method := database.NotificationMethodSmtp
	cfg := defaultNotificationsConfig(method)

	mgr, err := notifications.NewManager(cfg, store, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})

	// Set the time to a known value.
	mClock := quartz.NewMock(t)
	mClock.Set(time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC))

	enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), mClock)
	require.NoError(t, err)
	user := createSampleUser(t, store)

	// GIVEN: two notifications are enqueued with identical properties.
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted,
		map[string]string{"initiator": "danny"}, "test", user.ID)
	require.NoError(t, err)

	// WHEN: the second is enqueued, the enqueuer will reject the request.
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted,
		map[string]string{"initiator": "danny"}, "test", user.ID)
	require.ErrorIs(t, err, notifications.ErrDuplicate)

	// THEN: when the clock is advanced 24h, the notification will be accepted.
	// NOTE: the time is used in the dedupe hash, so by advancing 24h we're creating a distinct notification from the one
	// which was enqueued "yesterday".
	mClock.Advance(time.Hour * 24)
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted,
		map[string]string{"initiator": "danny"}, "test", user.ID)
	require.NoError(t, err)
}

func TestNotificationMethodCannotDefaultToInbox(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)

	cfg := defaultNotificationsConfig(database.NotificationMethodInbox)

	_, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewMock(t))
	require.ErrorIs(t, err, notifications.InvalidDefaultNotificationMethodError{Method: string(database.NotificationMethodInbox)})
}

func TestNotificationTargetMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		defaultMethod    database.NotificationMethod
		defaultEnabled   bool
		inboxEnabled     bool
		expectedEnqueued int
	}{
		{
			name:             "NoDefaultAndNoInbox",
			defaultMethod:    database.NotificationMethodSmtp,
			defaultEnabled:   false,
			inboxEnabled:     false,
			expectedEnqueued: 0,
		},
		{
			name:             "DefaultAndNoInbox",
			defaultMethod:    database.NotificationMethodSmtp,
			defaultEnabled:   true,
			inboxEnabled:     false,
			expectedEnqueued: 1,
		},
		{
			name:             "NoDefaultAndInbox",
			defaultMethod:    database.NotificationMethodSmtp,
			defaultEnabled:   false,
			inboxEnabled:     true,
			expectedEnqueued: 1,
		},
		{
			name:             "DefaultAndInbox",
			defaultMethod:    database.NotificationMethodSmtp,
			defaultEnabled:   true,
			inboxEnabled:     true,
			expectedEnqueued: 2,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// nolint:gocritic // Unit test.
			ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
			store, pubsub := dbtestutil.NewDB(t)
			logger := testutil.Logger(t)

			cfg := defaultNotificationsConfig(tt.defaultMethod)
			cfg.Inbox.Enabled = serpent.Bool(tt.inboxEnabled)

			// If the default method is not enabled, we want to ensure the config
			// is wiped out.
			if !tt.defaultEnabled {
				cfg.SMTP = codersdk.NotificationsEmailConfig{}
				cfg.Webhook = codersdk.NotificationsWebhookConfig{}
			}

			mgr, err := notifications.NewManager(cfg, store, pubsub, defaultHelpers(), createMetrics(), logger.Named("manager"))
			require.NoError(t, err)
			t.Cleanup(func() {
				assert.NoError(t, mgr.Stop(ctx))
			})

			// Set the time to a known value.
			mClock := quartz.NewMock(t)
			mClock.Set(time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC))

			enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), mClock)
			require.NoError(t, err)
			user := createSampleUser(t, store)

			// When: A notification is enqueued, it enqueues the correct amount of notifications.
			enqueued, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted,
				map[string]string{"initiator": "danny"}, "test", user.ID)
			require.NoError(t, err)
			require.Len(t, enqueued, tt.expectedEnqueued)
		})
	}
}

func TestNotificationOneTimePasswordDeliveryTargets(t *testing.T) {
	t.Parallel()

	t.Run("Inbox", func(t *testing.T) {
		t.Parallel()

		// nolint:gocritic // Unit test.
		ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
		store, _ := dbtestutil.NewDB(t)
		logger := testutil.Logger(t)

		// Given: Coder Inbox is enabled and SMTP/Webhook are disabled.
		cfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
		cfg.Inbox.Enabled = true
		cfg.SMTP = codersdk.NotificationsEmailConfig{}
		cfg.Webhook = codersdk.NotificationsWebhookConfig{}

		enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewMock(t))
		require.NoError(t, err)
		user := createSampleUser(t, store)

		// When: A one-time-passcode notification is sent, it does not enqueue a notification.
		enqueued, err := enq.Enqueue(ctx, user.ID, notifications.TemplateUserRequestedOneTimePasscode,
			map[string]string{"one_time_passcode": "1234"}, "test", user.ID)
		require.NoError(t, err)
		require.Len(t, enqueued, 0)
	})

	t.Run("SMTP", func(t *testing.T) {
		t.Parallel()

		// nolint:gocritic // Unit test.
		ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
		store, _ := dbtestutil.NewDB(t)
		logger := testutil.Logger(t)

		// Given: Coder Inbox/Webhook are disabled and SMTP is enabled.
		cfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
		cfg.Inbox.Enabled = false
		cfg.Webhook = codersdk.NotificationsWebhookConfig{}

		enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewMock(t))
		require.NoError(t, err)
		user := createSampleUser(t, store)

		// When: A one-time-passcode notification is sent, it does enqueue a notification.
		enqueued, err := enq.Enqueue(ctx, user.ID, notifications.TemplateUserRequestedOneTimePasscode,
			map[string]string{"one_time_passcode": "1234"}, "test", user.ID)
		require.NoError(t, err)
		require.Len(t, enqueued, 1)
	})

	t.Run("Webhook", func(t *testing.T) {
		t.Parallel()

		// nolint:gocritic // Unit test.
		ctx := dbauthz.AsNotifier(testutil.Context(t, testutil.WaitSuperLong))
		store, _ := dbtestutil.NewDB(t)
		logger := testutil.Logger(t)

		// Given: Coder Inbox/SMTP are disabled and Webhook is enabled.
		cfg := defaultNotificationsConfig(database.NotificationMethodWebhook)
		cfg.Inbox.Enabled = false
		cfg.SMTP = codersdk.NotificationsEmailConfig{}

		enq, err := notifications.NewStoreEnqueuer(cfg, store, defaultHelpers(), logger.Named("enqueuer"), quartz.NewMock(t))
		require.NoError(t, err)
		user := createSampleUser(t, store)

		// When: A one-time-passcode notification is sent, it does enqueue a notification.
		enqueued, err := enq.Enqueue(ctx, user.ID, notifications.TemplateUserRequestedOneTimePasscode,
			map[string]string{"one_time_passcode": "1234"}, "test", user.ID)
		require.NoError(t, err)
		require.Len(t, enqueued, 1)
	})
}

type fakeHandler struct {
	mu                sync.RWMutex
	succeeded, failed []string
}

func (f *fakeHandler) Dispatcher(payload types.MessagePayload, _, _ string, _ template.FuncMap) (dispatch.DeliveryFunc, error) {
	return func(_ context.Context, msgID uuid.UUID) (retryable bool, err error) {
		f.mu.Lock()
		defer f.mu.Unlock()

		if payload.Labels["type"] == "success" {
			f.succeeded = append(f.succeeded, msgID.String())
			return false, nil
		}

		f.failed = append(f.failed, msgID.String())
		return true, xerrors.New("oops")
	}, nil
}

// noopStoreSyncer pretends to perform store syncs, but does not; leading to messages being stuck in "leased" state.
type noopStoreSyncer struct {
	*acquireSignalingInterceptor
}

func newNoopStoreSyncer(db notifications.Store) *noopStoreSyncer {
	return &noopStoreSyncer{newAcquireSignalingInterceptor(db)}
}

func (*noopStoreSyncer) BulkMarkNotificationMessagesSent(_ context.Context, arg database.BulkMarkNotificationMessagesSentParams) (int64, error) {
	return int64(len(arg.IDs)), nil
}

func (*noopStoreSyncer) BulkMarkNotificationMessagesFailed(_ context.Context, arg database.BulkMarkNotificationMessagesFailedParams) (int64, error) {
	return int64(len(arg.IDs)), nil
}

type acquireSignalingInterceptor struct {
	notifications.Store
	acquiredChan chan struct{}
}

func newAcquireSignalingInterceptor(db notifications.Store) *acquireSignalingInterceptor {
	return &acquireSignalingInterceptor{
		Store:        db,
		acquiredChan: make(chan struct{}, 1),
	}
}

func (n *acquireSignalingInterceptor) AcquireNotificationMessages(ctx context.Context, params database.AcquireNotificationMessagesParams) ([]database.AcquireNotificationMessagesRow, error) {
	messages, err := n.Store.AcquireNotificationMessages(ctx, params)
	n.acquiredChan <- struct{}{}
	return messages, err
}
