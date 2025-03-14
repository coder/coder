package notifications_test
import (
	"errors"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"text/template"
	"time"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/testutil"
)
func TestBufferedUpdates(t *testing.T) {
	t.Parallel()
	// setup
	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)
	interceptor := &syncInterceptor{Store: store}
	santa := &santaHandler{}
	santaInbox := &santaHandler{}
	cfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
	cfg.StoreSyncInterval = serpent.Duration(time.Hour) // Ensure we don't sync the store automatically.
	// GIVEN: a manager which will pass or fail notifications based on their "nice" labels
	mgr, err := notifications.NewManager(cfg, interceptor, defaultHelpers(), createMetrics(), logger.Named("notifications-manager"))
	require.NoError(t, err)
	handlers := map[database.NotificationMethod]notifications.Handler{
		database.NotificationMethodSmtp:  santa,
		database.NotificationMethodInbox: santaInbox,
	}
	mgr.WithHandlers(handlers)
	enq, err := notifications.NewStoreEnqueuer(cfg, interceptor, defaultHelpers(), logger.Named("notifications-enqueuer"), quartz.NewReal())
	require.NoError(t, err)
	user := dbgen.User(t, store, database.User{})
	// WHEN: notifications are enqueued which should succeed and fail
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"nice": "true", "i": "0"}, "") // Will succeed.
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"nice": "true", "i": "1"}, "") // Will succeed.
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, user.ID, notifications.TemplateWorkspaceDeleted, map[string]string{"nice": "false", "i": "2"}, "") // Will fail.
	require.NoError(t, err)
	mgr.Run(ctx)
	// THEN:
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
		return success == expectedSuccess*len(handlers) && failure == expectedFailure*len(handlers)
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
		assert.EqualValues(ct, expectedFailure*len(handlers), interceptor.failed.Load())
		assert.EqualValues(ct, expectedSuccess*len(handlers), interceptor.sent.Load())
	}, testutil.WaitMedium, testutil.IntervalFast)
}
func TestBuildPayload(t *testing.T) {
	t.Parallel()
	// SETUP
	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)
	// GIVEN: a set of helpers to be injected into the templates
	const label = "Click here!"
	const baseURL = "http://xyz.com"
	const url = baseURL + "/@bobby/my-workspace"
	helpers := map[string]any{
		"my_label": func() string { return label },
		"my_url":   func() string { return baseURL },
	}
	// GIVEN: an enqueue interceptor which returns mock metadata
	interceptor := newEnqueueInterceptor(store,
		// Inject custom message metadata to influence the payload construction.
		func() database.FetchNewMessageMetadataRow {
			// Inject template actions which use injected help functions.
			actions := []types.TemplateAction{
				{
					Label: "{{ my_label }}",
					URL:   "{{ my_url }}/@{{.UserName}}/{{.Labels.name}}",
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
	enq, err := notifications.NewStoreEnqueuer(defaultNotificationsConfig(database.NotificationMethodSmtp), interceptor, helpers, logger.Named("notifications-enqueuer"), quartz.NewReal())
	require.NoError(t, err)
	// WHEN: a notification is enqueued
	_, err = enq.Enqueue(ctx, uuid.New(), notifications.TemplateWorkspaceDeleted, map[string]string{
		"name": "my-workspace",
	}, "test")
	require.NoError(t, err)
	// THEN: expect that a payload will be constructed and have the expected values
	payload := testutil.RequireRecvCtx(ctx, t, interceptor.payload)
	require.Len(t, payload.Actions, 1)
	require.Equal(t, label, payload.Actions[0].Label)
	require.Equal(t, url, payload.Actions[0].URL)
}
func TestStopBeforeRun(t *testing.T) {
	t.Parallel()
	// SETUP
	// nolint:gocritic // Unit test.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitSuperLong))
	store, _ := dbtestutil.NewDB(t)
	logger := testutil.Logger(t)
	// GIVEN: a standard manager
	mgr, err := notifications.NewManager(defaultNotificationsConfig(database.NotificationMethodSmtp), store, defaultHelpers(), createMetrics(), logger.Named("notifications-manager"))
	require.NoError(t, err)
	// THEN: validate that the manager can be stopped safely without Run() having been called yet
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
func (s *santaHandler) Dispatcher(payload types.MessagePayload, _, _ string, _ template.FuncMap) (dispatch.DeliveryFunc, error) {
	return func(_ context.Context, _ uuid.UUID) (retryable bool, err error) {
		if payload.Labels["nice"] != "true" {
			s.naughty.Add(1)
			return false, errors.New("be nice")
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
	return &enqueueInterceptor{Store: db, payload: make(chan types.MessagePayload, 2), metadataFn: metadataFn}
}
func (e *enqueueInterceptor) EnqueueNotificationMessage(_ context.Context, arg database.EnqueueNotificationMessageParams) error {
	var payload types.MessagePayload
	err := json.Unmarshal(arg.Payload, &payload)
	if err != nil {
		return err
	}
	e.payload <- payload
	return err
}
func (e *enqueueInterceptor) FetchNewMessageMetadata(_ context.Context, _ database.FetchNewMessageMetadataParams) (database.FetchNewMessageMetadataRow, error) {
	return e.metadataFn(), nil
}
