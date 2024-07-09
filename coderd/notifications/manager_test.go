package notifications_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/testutil"
)

func TestBufferedUpdates(t *testing.T) {
	t.Parallel()

	// setup
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	ctx, logger, db := setup(t)
	interceptor := &syncInterceptor{Store: db}
	santa := &santaHandler{}

	cfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
	cfg.StoreSyncInterval = serpent.Duration(time.Hour) // Ensure we don't sync the store automatically.

	mgr, err := notifications.NewManager(cfg, interceptor, createMetrics(), logger.Named("notifications-manager"))
	require.NoError(t, err)
	mgr.WithHandlers(map[database.NotificationMethod]notifications.Handler{
		database.NotificationMethodSmtp: santa,
	})
	enq, err := notifications.NewStoreEnqueuer(cfg, interceptor, defaultHelpers(), logger.Named("notifications-enqueuer"))
	require.NoError(t, err)

	user := dbgen.User(t, db, database.User{})

	// given
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"nice": "true"}, "") // Will succeed.
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"nice": "true"}, "") // Will succeed.
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"nice": "false"}, "") // Will fail.
	require.NoError(t, err)

	// when
	mgr.Run(ctx)

	// then

	const (
		expectedSuccess = 2
		expectedFailure = 1
	)

	// Wait for messages to be dispatched.
	require.Eventually(t, func() bool {
		return santa.naughty.Load() == expectedFailure &&
			santa.nice.Load() == expectedSuccess
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Wait for the expected number of buffered updates to be accumulated.
	require.Eventually(t, func() bool {
		success, failure := mgr.BufferedUpdatesCount()
		return success == expectedSuccess && failure == expectedFailure
	}, testutil.WaitShort, testutil.IntervalFast)

	// Stop the manager which forces an update of buffered updates.
	require.NoError(t, mgr.Stop(ctx))

	// Wait until both success & failure updates have been sent to the store.
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		if err := interceptor.err.Load(); err != nil {
			ct.Errorf("bulk update encountered error: %s", err)
			// Panic when an unexpected error occurs.
			ct.FailNow()
		}

		assert.EqualValues(ct, expectedFailure, interceptor.failed.Load())
		assert.EqualValues(ct, expectedSuccess, interceptor.sent.Load())
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestBuildPayload(t *testing.T) {
	t.Parallel()

	// given
	const label = "Click here!"
	const url = "http://xyz.com/"
	helpers := map[string]any{
		"my_label": func() string { return label },
		"my_url":   func() string { return url },
	}

	db := dbmem.New()
	interceptor := newEnqueueInterceptor(db,
		// Inject custom message metadata to influence the payload construction.
		func() database.FetchNewMessageMetadataRow {
			// Inject template actions which use injected help functions.
			actions := []types.TemplateAction{
				{
					Label: "{{ my_label }}",
					URL:   "{{ my_url }}",
				},
			}
			out, err := json.Marshal(actions)
			assert.NoError(t, err)

			return database.FetchNewMessageMetadataRow{
				NotificationName: "My Notification",
				Actions:          out,
				UserID:           uuid.New(),
				UserEmail:        "bob@bob.com",
				UserName:         "bobby",
			}
		})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true, IgnoredErrorIs: []error{}}).Leveled(slog.LevelDebug)
	enq, err := notifications.NewStoreEnqueuer(defaultNotificationsConfig(database.NotificationMethodSmtp), interceptor, helpers, logger.Named("notifications-enqueuer"))
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)

	// when
	_, err = enq.Enqueue(ctx, uuid.New(), notifications.TemplateWorkspaceDeleted, nil, "test")
	require.NoError(t, err)

	// then
	payload := testutil.RequireRecvCtx(ctx, t, interceptor.payload)
	require.Len(t, payload.Actions, 1)
	require.Equal(t, label, payload.Actions[0].Label)
	require.Equal(t, url, payload.Actions[0].URL)
}

func TestStopBeforeRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true, IgnoredErrorIs: []error{}}).Leveled(slog.LevelDebug)
	mgr, err := notifications.NewManager(defaultNotificationsConfig(database.NotificationMethodSmtp), dbmem.New(), createMetrics(), logger.Named("notifications-manager"))
	require.NoError(t, err)

	// Call stop before notifier is started with Run().
	require.Eventually(t, func() bool {
		assert.NoError(t, mgr.Stop(ctx))
		return true
	}, testutil.WaitShort, testutil.IntervalFast)
}

type syncInterceptor struct {
	notifications.Store

	sent   atomic.Int32
	failed atomic.Int32
	err    atomic.Value
}

func (b *syncInterceptor) BulkMarkNotificationMessagesSent(ctx context.Context, arg database.BulkMarkNotificationMessagesSentParams) (int64, error) {
	updated, err := b.Store.BulkMarkNotificationMessagesSent(ctx, arg)
	b.sent.Add(int32(updated))
	if err != nil {
		b.err.Store(err)
	}
	return updated, err
}

func (b *syncInterceptor) BulkMarkNotificationMessagesFailed(ctx context.Context, arg database.BulkMarkNotificationMessagesFailedParams) (int64, error) {
	updated, err := b.Store.BulkMarkNotificationMessagesFailed(ctx, arg)
	b.failed.Add(int32(updated))
	if err != nil {
		b.err.Store(err)
	}
	return updated, err
}

// santaHandler only dispatches nice messages.
type santaHandler struct {
	naughty atomic.Int32
	nice    atomic.Int32
}

func (s *santaHandler) Dispatcher(payload types.MessagePayload, _, _ string) (dispatch.DeliveryFunc, error) {
	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		if payload.Labels["nice"] != "true" {
			s.naughty.Add(1)
			return false, xerrors.New("be nice")
		}

		s.nice.Add(1)
		return false, nil
	}, nil
}

type enqueueInterceptor struct {
	notifications.Store

	payload    chan types.MessagePayload
	metadataFn func() database.FetchNewMessageMetadataRow
}

func newEnqueueInterceptor(db notifications.Store, metadataFn func() database.FetchNewMessageMetadataRow) *enqueueInterceptor {
	return &enqueueInterceptor{Store: db, payload: make(chan types.MessagePayload, 1), metadataFn: metadataFn}
}

func (e *enqueueInterceptor) EnqueueNotificationMessage(_ context.Context, arg database.EnqueueNotificationMessageParams) (uuid.UUID, error) {
	var payload types.MessagePayload
	err := json.Unmarshal(arg.Payload, &payload)
	if err != nil {
		return uuid.UUID{}, err
	}

	e.payload <- payload
	return uuid.UUID{}, err
}

func (e *enqueueInterceptor) FetchNewMessageMetadata(_ context.Context, _ database.FetchNewMessageMetadataParams) (database.FetchNewMessageMetadataRow, error) {
	return e.metadataFn(), nil
}
