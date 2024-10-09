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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/google/uuid"
	smtpmock "github.com/mocktools/go-smtp-mock/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/syncmap"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// updateGoldenFiles is a flag that can be set to update golden files.
var updateGoldenFiles = flag.Bool("update", false, "Update golden files")

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
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
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)
	method := database.NotificationMethodSmtp

	// GIVEN: a manager with standard config but a faked dispatch handler
	handler := &fakeHandler{}
	interceptor := &syncInterceptor{Store: api.Database}
	cfg := defaultNotificationsConfig(method)
	cfg.RetryInterval = serpent.Duration(time.Hour) // Ensure retries don't interfere with the test
	mgr, err := notifications.NewManager(cfg, interceptor, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{method: handler})
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

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
		return slices.Contains(handler.succeeded, sid.String()) &&
			slices.Contains(handler.failed, fid.String())
	}, testutil.WaitLong, testutil.IntervalFast)

	// THEN: we expect the store to be called with the updates of the earlier dispatches
	require.Eventually(t, func() bool {
		return interceptor.sent.Load() == 1 &&
			interceptor.failed.Load() == 1
	}, testutil.WaitLong, testutil.IntervalFast)

	// THEN: we verify that the store contains notifications in their expected state
	success, err := api.Database.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusSent,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, success, 1)
	failed, err := api.Database.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusTemporaryFailure,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, failed, 1)
}

func TestSMTPDispatch(t *testing.T) {
	t.Parallel()

	// SETUP

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

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
		Smarthost: serpent.HostPort{Host: "localhost", Port: fmt.Sprintf("%d", mockSMTPSrv.PortNumber())},
		Hello:     "localhost",
	}
	handler := newDispatchInterceptor(dispatch.NewSMTPHandler(cfg.SMTP, defaultHelpers(), api.Logger.Named("smtp")))
	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{method: handler})
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

	// WHEN: a message is enqueued
	msgID, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{}, "test")
	require.NoError(t, err)

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
	require.Contains(t, msgs[0].MsgRequest(), fmt.Sprintf("Message-Id: %s", msgID))
}

func TestWebhookDispatch(t *testing.T) {
	t.Parallel()

	// SETUP

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

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
	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	const (
		email    = "bob@coder.com"
		name     = "Robert McBobbington"
		username = "bob"
	)
	user := dbgen.User(t, api.Database, database.User{
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
	require.Equal(t, *msgID, payload.MsgID)
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

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	// Mock server to simulate webhook endpoint.
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload dispatch.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("noted."))
		assert.NoError(t, err)

		received.Add(1)
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)

	method := database.NotificationMethodWebhook
	cfg := defaultNotificationsConfig(method)
	cfg.Webhook = codersdk.NotificationsWebhookConfig{
		Endpoint: *serpent.URLOf(endpoint),
	}

	// Tune the queue to fetch often.
	const fetchInterval = time.Millisecond * 200
	const batchSize = 10
	cfg.FetchInterval = serpent.Duration(fetchInterval)
	cfg.LeaseCount = serpent.Int64(batchSize)

	// Shrink buffers down and increase flush interval to provoke backpressure.
	// Flush buffers every 5 fetch intervals.
	const syncInterval = time.Second
	cfg.StoreSyncInterval = serpent.Duration(syncInterval)
	cfg.StoreSyncBufferSize = serpent.Int64(2)

	handler := newDispatchInterceptor(dispatch.NewWebhookHandler(cfg.Webhook, api.Logger.Named("webhook")))

	// Intercept calls to submit the buffered updates to the store.
	storeInterceptor := &syncInterceptor{Store: api.Database}

	// GIVEN: a notification manager whose updates will be intercepted
	mgr, err := notifications.NewManager(cfg, storeInterceptor, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{method: handler})
	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

	// WHEN: a set of notifications are enqueued, which causes backpressure due to the batchSize which can be processed per fetch
	const totalMessages = 30
	for i := 0; i < totalMessages; i++ {
		_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"i": fmt.Sprintf("%d", i)}, "test")
		require.NoError(t, err)
	}

	// Start the notifier.
	mgr.Run(ctx)

	// THEN:

	// Wait for 3 fetch intervals, then check progress.
	time.Sleep(fetchInterval * 3)

	// We expect the notifier will have dispatched ONLY the initial batch of messages.
	// In other words, the notifier should have dispatched 3 batches by now, but because the buffered updates have not
	// been processed: there is backpressure.
	require.EqualValues(t, batchSize, handler.sent.Load()+handler.err.Load())
	// We expect that the store will have received NO updates.
	require.EqualValues(t, 0, storeInterceptor.sent.Load()+storeInterceptor.failed.Load())

	// However, when we Stop() the manager the backpressure will be relieved and the buffered updates will ALL be flushed,
	// since all the goroutines that were blocked (on writing updates to the buffer) will be unblocked and will complete.
	require.NoError(t, mgr.Stop(ctx))
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
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

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

	handler := newDispatchInterceptor(dispatch.NewWebhookHandler(cfg.Webhook, api.Logger.Named("webhook")))

	// Intercept calls to submit the buffered updates to the store.
	storeInterceptor := &syncInterceptor{Store: api.Database}

	mgr, err := notifications.NewManager(cfg, storeInterceptor, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{method: handler})
	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

	// WHEN: a few notifications are enqueued, which will all fail until their final retry (determined by the mock server)
	const msgCount = 5
	for i := 0; i < msgCount; i++ {
		_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"i": fmt.Sprintf("%d", i)}, "test")
		require.NoError(t, err)
	}

	mgr.Run(ctx)

	// THEN: we expect to see all but the final attempts failing
	require.Eventually(t, func() bool {
		// We expect all messages to fail all attempts but the final;
		return storeInterceptor.failed.Load() == msgCount*(maxAttempts-1) &&
			// ...and succeed on the final attempt.
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
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

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

	noopInterceptor := newNoopStoreSyncer(api.Database)

	// nolint:gocritic // Unit test.
	mgrCtx, cancelManagerCtx := context.WithCancel(dbauthz.AsSystemRestricted(context.Background()))
	t.Cleanup(cancelManagerCtx)

	mgr, err := notifications.NewManager(cfg, noopInterceptor, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	user := createSampleUser(t, api.Database)

	// WHEN: a few notifications are enqueued which will all succeed
	var msgs []string
	for i := 0; i < msgCount; i++ {
		id, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted,
			map[string]string{"type": "success", "index": fmt.Sprintf("%d", i)}, "test")
		require.NoError(t, err)
		msgs = append(msgs, id.String())
	}

	mgr.Run(mgrCtx)

	// THEN:

	// Wait for the messages to be acquired
	<-noopInterceptor.acquiredChan
	// Then cancel the context, forcing the notification manager to shutdown ungracefully (simulating a crash); leaving messages in "leased" status.
	cancelManagerCtx()

	// Fetch any messages currently in "leased" status, and verify that they're exactly the ones we enqueued.
	leased, err := api.Database.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusLeased,
		Limit:  msgCount,
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
	storeInterceptor := &syncInterceptor{Store: api.Database}
	handler := newDispatchInterceptor(&fakeHandler{})
	mgr, err = notifications.NewManager(cfg, storeInterceptor, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{method: handler})

	// Use regular context now.
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	mgr.Run(ctx)

	// Wait until all messages are sent & updates flushed to the database.
	require.Eventually(t, func() bool {
		return handler.sent.Load() == msgCount &&
			storeInterceptor.sent.Load() == msgCount
	}, testutil.WaitLong, testutil.IntervalFast)

	// Validate that no more messages are in "leased" status.
	leased, err = api.Database.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusLeased,
		Limit:  msgCount,
	})
	require.NoError(t, err)
	require.Len(t, leased, 0)
}

// TestInvalidConfig validates that misconfigurations lead to errors.
func TestInvalidConfig(t *testing.T) {
	t.Parallel()

	_, _, api := coderdtest.NewWithAPI(t, nil)

	// GIVEN: invalid config with dispatch period <= lease period
	const (
		leasePeriod = time.Second
		method      = database.NotificationMethodSmtp
	)
	cfg := defaultNotificationsConfig(method)
	cfg.LeasePeriod = serpent.Duration(leasePeriod)
	cfg.DispatchTimeout = serpent.Duration(leasePeriod)

	// WHEN: the manager is created with invalid config
	_, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))

	// THEN: the manager will fail to be created, citing invalid config as error
	require.ErrorIs(t, err, notifications.ErrInvalidDispatchTimeout)
}

func TestNotifierPaused(t *testing.T) {
	t.Parallel()

	// Setup.

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	// Prepare the test.
	handler := &fakeHandler{}
	method := database.NotificationMethodSmtp
	user := createSampleUser(t, api.Database)

	const fetchInterval = time.Millisecond * 100
	cfg := defaultNotificationsConfig(method)
	cfg.FetchInterval = serpent.Duration(fetchInterval)
	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{method: handler})
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)

	// Pause the notifier.
	settingsJSON, err := json.Marshal(&codersdk.NotificationsSettings{NotifierPaused: true})
	require.NoError(t, err)
	err = api.Database.UpsertNotificationsSettings(ctx, string(settingsJSON))
	require.NoError(t, err)

	// Start the manager so that notifications are processed, except it will be paused at this point.
	// If it is started before pausing, there's a TOCTOU possibility between checking whether the notifier is paused or
	// not, and processing the messages (see notifier.run).
	mgr.Run(ctx)

	// Notifier is paused, enqueue the next message.
	sid, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"type": "success", "i": "1"}, "test")
	require.NoError(t, err)

	// Ensure we have a pending message and it's the expected one.
	pendingMessages, err := api.Database.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
		Status: database.NotificationMessageStatusPending,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, pendingMessages, 1)
	require.Equal(t, pendingMessages[0].ID.String(), sid.String())

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
	err = api.Database.UpsertNotificationsSettings(ctx, string(settingsJSON))
	require.NoError(t, err)

	// Notifier is running again, message should be dequeued.
	// nolint:gocritic // These magic numbers are fine.
	require.Eventually(t, func() bool {
		handler.mu.RLock()
		defer handler.mu.RUnlock()
		return slices.Contains(handler.succeeded, sid.String())
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

	tests := []struct {
		name    string
		id      uuid.UUID
		payload types.MessagePayload
	}{
		{
			name: "TemplateWorkspaceDeleted",
			id:   notifications.TemplateWorkspaceDeleted,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"name":      "bobby-workspace",
					"reason":    "autodeleted due to dormancy",
					"initiator": "autobuild",
				},
			},
		},
		{
			name: "TemplateWorkspaceAutobuildFailed",
			id:   notifications.TemplateWorkspaceAutobuildFailed,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"name":   "bobby-workspace",
					"reason": "autostart",
				},
			},
		},
		{
			name: "TemplateWorkspaceDormant",
			id:   notifications.TemplateWorkspaceDormant,
			payload: types.MessagePayload{
				UserName: "Bobby",
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
				UserName: "Bobby",
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
				UserName: "Bobby",
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
				UserName: "Bobby",
				Labels: map[string]string{
					"created_account_name":      "bobby",
					"created_account_user_name": "William Tables",
					"account_creator":           "rob",
				},
			},
		},
		{
			name: "TemplateUserAccountDeleted",
			id:   notifications.TemplateUserAccountDeleted,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"deleted_account_name":      "bobby",
					"deleted_account_user_name": "william tables",
					"account_deleter_user_name": "rob",
				},
			},
		},
		{
			name: "TemplateUserAccountSuspended",
			id:   notifications.TemplateUserAccountSuspended,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"suspended_account_name":      "bobby",
					"suspended_account_user_name": "william tables",
					"account_suspender_user_name": "rob",
				},
			},
		},
		{
			name: "TemplateUserAccountActivated",
			id:   notifications.TemplateUserAccountActivated,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"activated_account_name":      "bobby",
					"activated_account_user_name": "william tables",
					"account_activator_user_name": "rob",
				},
			},
		},
		{
			name: "TemplateYourAccountSuspended",
			id:   notifications.TemplateYourAccountSuspended,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"suspended_account_name":      "bobby",
					"account_suspender_user_name": "rob",
				},
			},
		},
		{
			name: "TemplateYourAccountActivated",
			id:   notifications.TemplateYourAccountActivated,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"activated_account_name":      "bobby",
					"account_activator_user_name": "rob",
				},
			},
		},
		{
			name: "TemplateTemplateDeleted",
			id:   notifications.TemplateTemplateDeleted,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"name":         "bobby-template",
					"display_name": "Bobby's Template",
					"initiator":    "rob",
				},
			},
		},
		{
			name: "TemplateWorkspaceManualBuildFailed",
			id:   notifications.TemplateWorkspaceManualBuildFailed,
			payload: types.MessagePayload{
				UserName: "Bobby",
				Labels: map[string]string{
					"name":                     "bobby-workspace",
					"template_name":            "bobby-template",
					"template_display_name":    "William's Template",
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
				UserName: "Bobby",
				Labels: map[string]string{
					"template_name":         "bobby-first-template",
					"template_display_name": "Bobby First Template",
				},
				// We need to use floats as `json.Unmarshal` unmarshal numbers in `map[string]any` to floats.
				Data: map[string]any{
					"failed_builds":    4.0,
					"total_builds":     55.0,
					"report_frequency": "week",
					"template_versions": []map[string]any{
						{
							"template_version_name": "bobby-template-version-1",
							"failed_count":          3.0,
							"failed_builds": []map[string]any{
								{
									"workspace_owner_username": "mtojek",
									"workspace_name":           "workspace-1",
									"build_number":             1234.0,
								},
								{
									"workspace_owner_username": "johndoe",
									"workspace_name":           "my-workspace-3",
									"build_number":             5678.0,
								},
								{
									"workspace_owner_username": "jack",
									"workspace_name":           "workwork",
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
									"build_number":             8888.0,
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
				UserName: "Bobby",
				Labels: map[string]string{
					"one_time_passcode": "fad9020b-6562-4cdb-87f1-0486f1bea415",
				},
			},
		},
	}

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

			_, _, sql := dbtestutil.NewDBWithSQLDB(t)

			var (
				titleTmpl string
				bodyTmpl  string
			)
			err := sql.
				QueryRow("SELECT title_template, body_template FROM notification_templates WHERE id = $1 LIMIT 1", tc.id).
				Scan(&titleTmpl, &bodyTmpl)
			require.NoError(t, err, "failed to query body template for template:", tc.id)

			title, err := render.GoTemplate(titleTmpl, tc.payload, defaultHelpers())
			require.NotContainsf(t, title, render.NoValue, "template %q is missing a label value", tc.name)
			require.NoError(t, err, "failed to render notification title template")
			require.NotEmpty(t, title, "title should not be empty")

			body, err := render.GoTemplate(bodyTmpl, tc.payload, defaultHelpers())
			require.NoError(t, err, "failed to render notification body template")
			require.NotEmpty(t, body, "body should not be empty")

			partialName := strings.Split(t.Name(), "/")[1]
			bodyGoldenFile := filepath.Join("testdata", "rendered-templates", partialName+"-body.md.golden")
			titleGoldenFile := filepath.Join("testdata", "rendered-templates", partialName+"-title.md.golden")

			if *updateGoldenFiles {
				err = os.MkdirAll(filepath.Dir(bodyGoldenFile), 0o755)
				require.NoError(t, err, "want no error creating golden file directory")
				err = os.WriteFile(bodyGoldenFile, []byte(body), 0o600)
				require.NoError(t, err, "want no error writing body golden file")
				err = os.WriteFile(titleGoldenFile, []byte(title), 0o600)
				require.NoError(t, err, "want no error writing title golden file")
				return
			}

			const hint = "run \"DB=ci make update-golden-files\" and commit the changes"

			wantBody, err := os.ReadFile(bodyGoldenFile)
			require.NoError(t, err, fmt.Sprintf("missing golden notification body file. %s", hint))
			wantTitle, err := os.ReadFile(titleGoldenFile)
			require.NoError(t, err, fmt.Sprintf("missing golden notification title file. %s", hint))

			require.Equal(t, string(wantBody), body, fmt.Sprintf("rendered template body does not match golden file. If this is expected, %s", hint))
			require.Equal(t, string(wantTitle), title, fmt.Sprintf("rendered template title does not match golden file. If this is expected, %s", hint))
		})
	}
}

// TestDisabledBeforeEnqueue ensures that notifications cannot be enqueued once a user has disabled that notification template
func TestDisabledBeforeEnqueue(t *testing.T) {
	t.Parallel()

	// SETUP
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres; it is testing business-logic implemented in the database")
	}

	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	// GIVEN: an enqueuer & a sample user
	cfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)
	user := createSampleUser(t, api.Database)

	// WHEN: the user has a preference set to not receive the "workspace deleted" notification
	templateID := notifications.TemplateWorkspaceDeleted
	n, err := api.Database.UpdateUserNotificationPreferences(ctx, database.UpdateUserNotificationPreferencesParams{
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
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	method := database.NotificationMethodSmtp
	cfg := defaultNotificationsConfig(method)

	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), quartz.NewReal())
	require.NoError(t, err)
	user := createSampleUser(t, api.Database)

	// GIVEN: a notification is enqueued which has not (yet) been disabled
	templateID := notifications.TemplateWorkspaceDeleted
	msgID, err := enq.Enqueue(ctx, user.ID, templateID, map[string]string{}, "test")
	require.NoError(t, err)

	// Disable the notification template.
	n, err := api.Database.UpdateUserNotificationPreferences(ctx, database.UpdateUserNotificationPreferencesParams{
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
		m, err := api.Database.GetNotificationMessagesByStatus(ctx, database.GetNotificationMessagesByStatusParams{
			Status: database.NotificationMessageStatusInhibited,
			Limit:  10,
		})
		assert.NoError(ct, err)
		if assert.Equal(ct, len(m), 1) {
			assert.Equal(ct, m[0].ID.String(), msgID.String())
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
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

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
		template      = notifications.TemplateWorkspaceDormant
		defaultMethod = database.NotificationMethodSmtp
		customMethod  = database.NotificationMethodWebhook
	)
	out, err := api.Database.UpdateNotificationTemplateMethodByID(ctx, database.UpdateNotificationTemplateMethodByIDParams{
		ID:     template,
		Method: database.NullNotificationMethod{NotificationMethod: customMethod, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, customMethod, out.Method.NotificationMethod)

	// GIVEN: a manager configured with multiple dispatch methods
	cfg := defaultNotificationsConfig(defaultMethod)
	cfg.SMTP = codersdk.NotificationsEmailConfig{
		From:      "danny@coder.com",
		Hello:     "localhost",
		Smarthost: serpent.HostPort{Host: "localhost", Port: fmt.Sprintf("%d", mockSMTPSrv.PortNumber())},
	}
	cfg.Webhook = codersdk.NotificationsWebhookConfig{
		Endpoint: *serpent.URLOf(endpoint),
	}

	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = mgr.Stop(ctx)
	})

	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger, quartz.NewReal())
	require.NoError(t, err)

	// WHEN: a notification of that template is enqueued, it should be delivered with the configured method - not the default.
	user := createSampleUser(t, api.Database)
	msgID, err := enq.Enqueue(ctx, user.ID, template, map[string]string{}, "test")
	require.NoError(t, err)

	// THEN: the notification should be received by the custom dispatch method
	mgr.Run(ctx)

	receivedMsgID := testutil.RequireRecvCtx(ctx, t, received)
	require.Equal(t, msgID.String(), receivedMsgID.String())

	// Ensure no messages received by default method (SMTP):
	msgs := mockSMTPSrv.MessagesAndPurge()
	require.Len(t, msgs, 0)

	// Enqueue a notification which does not have a custom method set to ensure default works correctly.
	msgID, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{}, "test")
	require.NoError(t, err)
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		msgs := mockSMTPSrv.MessagesAndPurge()
		if assert.Len(ct, msgs, 1) {
			assert.Contains(ct, msgs[0].MsgRequest(), fmt.Sprintf("Message-Id: %s", msgID))
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
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
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
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	_, _, api := coderdtest.NewWithAPI(t, nil)

	method := database.NotificationMethodSmtp
	cfg := defaultNotificationsConfig(method)

	mgr, err := notifications.NewManager(cfg, api.Database, defaultHelpers(), createMetrics(), api.Logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(ctx))
	})

	// Set the time to a known value.
	mClock := quartz.NewMock(t)
	mClock.Set(time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC))

	enq, err := notifications.NewStoreEnqueuer(cfg, api.Database, defaultHelpers(), api.Logger.Named("enqueuer"), mClock)
	require.NoError(t, err)
	user := createSampleUser(t, api.Database)

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

type fakeHandler struct {
	mu                sync.RWMutex
	succeeded, failed []string
}

func (f *fakeHandler) Dispatcher(payload types.MessagePayload, _, _ string) (dispatch.DeliveryFunc, error) {
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
