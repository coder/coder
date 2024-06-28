package notifications_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	smtpmock "github.com/mocktools/go-smtp-mock/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// TestBasicNotificationRoundtrip enqueues a message to the store, waits for it to be acquired by a notifier,
// and passes it off to a fake handler.
// TODO: split this test up into table tests or separate tests.
func TestBasicNotificationRoundtrip(t *testing.T) {
	t.Parallel()

	// setup
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}
	ctx, logger, db, ps := setup(t)

	// given
	handler := &fakeHandler{}
	fakeHandlers, err := notifications.NewHandlerRegistry(handler)
	require.NoError(t, err)

	cfg := defaultNotificationsConfig()
	mgr, err := notifications.NewManager(cfg, db, logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(fakeHandlers)
	t.Cleanup(func() {
		require.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, db, defaultHelpers(), logger.Named("enqueuer"))
	require.NoError(t, err)

	client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	user := coderdtest.CreateFirstUser(t, client)

	// when
	sid, err := enq.Enqueue(ctx, user.UserID, notifications.TemplateWorkspaceDeleted, types.Labels{"type": "success"}, "test")
	require.NoError(t, err)
	fid, err := enq.Enqueue(ctx, user.UserID, notifications.TemplateWorkspaceDeleted, types.Labels{"type": "failure"}, "test")
	require.NoError(t, err)

	mgr.Run(ctx, 1)

	// then
	require.Eventually(t, func() bool { return handler.succeeded == sid.String() }, testutil.WaitLong, testutil.IntervalMedium)
	require.Eventually(t, func() bool { return handler.failed == fid.String() }, testutil.WaitLong, testutil.IntervalMedium)
}

func TestSMTPDispatch(t *testing.T) {
	t.Parallel()

	// setup
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}
	ctx, logger, db, ps := setup(t)

	// start mock SMTP server
	mockSMTPSrv := smtpmock.New(smtpmock.ConfigurationAttr{
		LogToStdout:       true,
		LogServerActivity: true,
	})
	require.NoError(t, mockSMTPSrv.Start())
	t.Cleanup(func() {
		require.NoError(t, mockSMTPSrv.Stop())
	})

	// given
	const from = "danny@coder.com"
	cfg := defaultNotificationsConfig()
	cfg.SMTP = codersdk.NotificationsEmailConfig{
		From:      from,
		Smarthost: serpent.HostPort{Host: "localhost", Port: fmt.Sprintf("%d", mockSMTPSrv.PortNumber())},
		Hello:     "localhost",
	}
	handler := newDispatchInterceptor(dispatch.NewSMTPHandler(cfg.SMTP, logger.Named("smtp")))
	fakeHandlers, err := notifications.NewHandlerRegistry(handler)
	require.NoError(t, err)

	mgr, err := notifications.NewManager(cfg, db, logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(fakeHandlers)
	t.Cleanup(func() {
		require.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, db, defaultHelpers(), logger.Named("enqueuer"))
	require.NoError(t, err)

	client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	first := coderdtest.CreateFirstUser(t, client)
	_, user := coderdtest.CreateAnotherUserMutators(t, client, first.OrganizationID, nil, func(r *codersdk.CreateUserRequest) {
		r.Email = "bob@coder.com"
		r.Username = "bob"
	})

	// when
	msgID, err := enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, types.Labels{}, "test")
	require.NoError(t, err)

	mgr.Run(ctx, 1)

	// then
	require.Eventually(t, func() bool {
		assert.Nil(t, handler.lastErr.Load())
		assert.True(t, handler.retryable.Load() == 0)
		return handler.sent.Load() == 1
	}, testutil.WaitLong, testutil.IntervalMedium)

	msgs := mockSMTPSrv.MessagesAndPurge()
	require.Len(t, msgs, 1)
	require.Contains(t, msgs[0].MsgRequest(), fmt.Sprintf("From: %s", from))
	require.Contains(t, msgs[0].MsgRequest(), fmt.Sprintf("To: %s", user.Email))
	require.Contains(t, msgs[0].MsgRequest(), fmt.Sprintf("Message-Id: %s", msgID))
}

func TestWebhookDispatch(t *testing.T) {
	t.Parallel()

	// setup
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}
	ctx, logger, db, ps := setup(t)

	var (
		msgID *uuid.UUID
		input types.Labels
	)

	sent := make(chan bool, 1)
	// Mock server to simulate webhook endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload dispatch.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)

		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.EqualValues(t, "1.0", payload.Version)
		require.Equal(t, *msgID, payload.MsgID)
		require.Equal(t, payload.Payload.Labels, input)
		require.Equal(t, payload.Payload.UserEmail, "bob@coder.com")
		// UserName is coalesced from `name` and `username`; in this case `name` wins.
		require.Equal(t, payload.Payload.UserName, "Robert McBobbington")
		require.Equal(t, payload.Payload.NotificationName, "Workspace Deleted")

		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("noted."))
		require.NoError(t, err)
		sent <- true
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)

	// given
	cfg := defaultNotificationsConfig()
	cfg.Method = serpent.String(database.NotificationMethodWebhook)
	cfg.Webhook = codersdk.NotificationsWebhookConfig{
		Endpoint: *serpent.URLOf(endpoint),
	}
	mgr, err := notifications.NewManager(cfg, db, logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mgr.Stop(ctx))
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, db, defaultHelpers(), logger.Named("enqueuer"))
	require.NoError(t, err)

	client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	first := coderdtest.CreateFirstUser(t, client)
	_, user := coderdtest.CreateAnotherUserMutators(t, client, first.OrganizationID, nil, func(r *codersdk.CreateUserRequest) {
		r.Email = "bob@coder.com"
		r.Username = "bob"
		r.Name = "Robert McBobbington"
	})

	// when
	input = types.Labels{
		"a": "b",
		"c": "d",
	}
	msgID, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, input, "test")
	require.NoError(t, err)

	mgr.Run(ctx, 1)

	// then
	require.Eventually(t, func() bool { return <-sent }, testutil.WaitShort, testutil.IntervalFast)
}

// TestBackpressure validates that delays in processing the buffered updates will result in slowed dequeue rates.
// As a side-effect, this also tests the graceful shutdown and flushing of the buffers.
func TestBackpressure(t *testing.T) {
	t.Parallel()

	// setup
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	ctx, logger, db, ps := setup(t)

	// Mock server to simulate webhook endpoint.
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload dispatch.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("noted."))
		require.NoError(t, err)

		received.Add(1)
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)

	cfg := defaultNotificationsConfig()
	cfg.Method = serpent.String(database.NotificationMethodWebhook)
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

	handler := newDispatchInterceptor(dispatch.NewWebhookHandler(cfg.Webhook, logger.Named("webhook")))
	fakeHandlers, err := notifications.NewHandlerRegistry(handler)
	require.NoError(t, err)

	// Intercept calls to submit the buffered updates to the store.
	storeInterceptor := &bulkUpdateInterceptor{Store: db}

	// given
	mgr, err := notifications.NewManager(cfg, storeInterceptor, logger.Named("manager"))
	require.NoError(t, err)
	mgr.WithHandlers(fakeHandlers)
	enq, err := notifications.NewStoreEnqueuer(cfg, db, defaultHelpers(), logger.Named("enqueuer"))
	require.NoError(t, err)

	client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	first := coderdtest.CreateFirstUser(t, client)
	_, user := coderdtest.CreateAnotherUserMutators(t, client, first.OrganizationID, nil, func(r *codersdk.CreateUserRequest) {
		r.Email = "bob@coder.com"
		r.Username = "bob"
	})

	// when
	const totalMessages = 30
	for i := 0; i < totalMessages; i++ {
		_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, types.Labels{"i": fmt.Sprintf("%d", i)}, "test")
		require.NoError(t, err)
	}

	// Start two notifiers.
	const notifiers = 2
	mgr.Run(ctx, notifiers)

	// then

	// Wait for 3 fetch intervals, then check progress.
	time.Sleep(fetchInterval * 3)

	// We expect the notifiers will have dispatched ONLY the initial batch of messages.
	// In other words, the notifiers should have dispatched 3 batches by now, but because the buffered updates have not
	// been processed there is backpressure.
	require.EqualValues(t, notifiers*batchSize, handler.sent.Load()+handler.err.Load())
	// We expect that the store will have received NO updates.
	require.EqualValues(t, 0, storeInterceptor.sent.Load()+storeInterceptor.failed.Load())

	// However, when we Stop() the manager the backpressure will be relieved and the buffered updates will ALL be flushed,
	// since all the goroutines blocked on writing updates to the buffer will be unblocked and will complete.
	require.NoError(t, mgr.Stop(ctx))
	require.EqualValues(t, notifiers*batchSize, storeInterceptor.sent.Load()+storeInterceptor.failed.Load())
}

func TestRetries(t *testing.T) {
	t.Parallel()

	// setup
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	ctx, logger, db, ps := setup(t)

	const maxAttempts = 3

	// Mock server to simulate webhook endpoint.
	receivedMap := make(map[uuid.UUID]*atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload dispatch.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)

		if _, ok := receivedMap[payload.MsgID]; !ok {
			receivedMap[payload.MsgID] = &atomic.Int32{}
		}

		counter := receivedMap[payload.MsgID]

		// Let the request succeed if this is its last attempt.
		if counter.Add(1) == maxAttempts {
			w.WriteHeader(http.StatusOK)
			_, err = w.Write([]byte("noted."))
			require.NoError(t, err)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		_, err = w.Write([]byte("retry again later..."))
		require.NoError(t, err)
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)

	cfg := defaultNotificationsConfig()
	cfg.Method = serpent.String(database.NotificationMethodWebhook)
	cfg.Webhook = codersdk.NotificationsWebhookConfig{
		Endpoint: *serpent.URLOf(endpoint),
	}

	cfg.MaxSendAttempts = maxAttempts

	// Tune intervals low to speed up test.
	cfg.StoreSyncInterval = serpent.Duration(time.Millisecond * 100)
	cfg.RetryInterval = serpent.Duration(time.Second) // query uses second-precision
	cfg.FetchInterval = serpent.Duration(time.Millisecond * 100)

	handler := newDispatchInterceptor(dispatch.NewWebhookHandler(cfg.Webhook, logger.Named("webhook")))
	fakeHandlers, err := notifications.NewHandlerRegistry(handler)
	require.NoError(t, err)

	// Intercept calls to submit the buffered updates to the store.
	storeInterceptor := &bulkUpdateInterceptor{Store: db}

	// given
	mgr, err := notifications.NewManager(cfg, storeInterceptor, logger.Named("manager"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mgr.Stop(ctx))
	})
	mgr.WithHandlers(fakeHandlers)
	enq, err := notifications.NewStoreEnqueuer(cfg, db, defaultHelpers(), logger.Named("enqueuer"))
	require.NoError(t, err)

	client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	first := coderdtest.CreateFirstUser(t, client)
	_, user := coderdtest.CreateAnotherUserMutators(t, client, first.OrganizationID, nil, func(r *codersdk.CreateUserRequest) {
		r.Email = "bob@coder.com"
		r.Username = "bob"
	})

	// when
	for i := 0; i < 1; i++ {
		_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, types.Labels{"i": fmt.Sprintf("%d", i)}, "test")
		require.NoError(t, err)
	}

	// Start two notifiers.
	const notifiers = 2
	mgr.Run(ctx, notifiers)

	// then
	require.Eventually(t, func() bool {
		return storeInterceptor.failed.Load() == maxAttempts-1 &&
			storeInterceptor.sent.Load() == 1
	}, testutil.WaitLong, testutil.IntervalFast)
}

type fakeHandler struct {
	succeeded string
	failed    string
}

func (*fakeHandler) NotificationMethod() database.NotificationMethod {
	return database.NotificationMethodSmtp
}

func (f *fakeHandler) Dispatcher(payload types.MessagePayload, _, _ string) (dispatch.DeliveryFunc, error) {
	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		if payload.Labels.Get("type") == "success" {
			f.succeeded = msgID.String()
		} else {
			f.failed = msgID.String()
		}
		return false, nil
	}, nil
}

type dispatchInterceptor struct {
	handler notifications.Handler

	sent        atomic.Int32
	retryable   atomic.Int32
	unretryable atomic.Int32
	err         atomic.Int32
	lastErr     atomic.Value
}

func newDispatchInterceptor(h notifications.Handler) *dispatchInterceptor {
	return &dispatchInterceptor{handler: h}
}

func (i *dispatchInterceptor) NotificationMethod() database.NotificationMethod {
	return i.handler.NotificationMethod()
}

func (i *dispatchInterceptor) Dispatcher(payload types.MessagePayload, title, body string) (dispatch.DeliveryFunc, error) {
	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		deliveryFn, err := i.handler.Dispatcher(payload, title, body)
		if err != nil {
			return false, err
		}

		retryable, err = deliveryFn(ctx, msgID)

		if err != nil {
			i.err.Add(1)
			i.lastErr.Store(err)
		}

		switch {
		case !retryable && err == nil:
			i.sent.Add(1)
		case retryable:
			i.retryable.Add(1)
		case !retryable && err != nil:
			i.unretryable.Add(1)
		}
		return retryable, err
	}, nil
}
