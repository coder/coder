package chatd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	entchatd "github.com/coder/coder/v2/enterprise/coderd/x/chatd"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRelayMetricsInitialOpenSuccess(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	remoteWorkerID := uuid.New()
	dialer := func(context.Context, uuid.UUID, uuid.UUID, http.Header) ([]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error) {
		parts := make(chan codersdk.ChatStreamEvent, 1)
		parts <- codersdk.ChatStreamEvent{
			Type: codersdk.ChatStreamEventTypeMessagePart,
			MessagePart: &codersdk.ChatStreamMessagePart{
				Role: "assistant",
				Part: codersdk.ChatMessageText("hello"),
			},
		}
		return nil, parts, func() {}, nil
	}
	fn := entchatd.NewMultiReplicaSubscribeFn(entchatd.MultiReplicaSubscribeConfig{
		DialerFn:             dialer,
		PrometheusRegisterer: reg,
	})

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	statusNotifications := make(chan osschatd.StatusNotification)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	events := fn(ctx, osschatd.SubscribeFnParams{
		ChatID:              uuid.New(),
		Chat:                database.Chat{Status: database.ChatStatusRunning, WorkerID: uuid.NullUUID{UUID: remoteWorkerID, Valid: true}},
		WorkerID:            uuid.New(),
		StatusNotifications: statusNotifications,
		DB:                  db,
		Logger:              slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})
	_ = events

	require.Eventually(t, func() bool {
		return counterValue(t, reg, "coderd_chat_stream_relay_open_total", prometheus.Labels{"source": "initial", "result": "success"}) == 1 &&
			gaugeValue(t, reg, "coderd_chat_stream_relay_active", prometheus.Labels{}) == 1
	}, testutil.WaitMedium, testutil.IntervalFast)

	cancel()
	require.Eventually(t, func() bool {
		return counterValue(t, reg, "coderd_chat_stream_relay_close_total", prometheus.Labels{"reason": "context_done"}) == 1 &&
			gaugeValue(t, reg, "coderd_chat_stream_relay_active", prometheus.Labels{}) == 0
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestRelayMetricsDialFailureSchedulesReconnect(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()
	fn := entchatd.NewMultiReplicaSubscribeFn(entchatd.MultiReplicaSubscribeConfig{
		DialerFn: func(context.Context, uuid.UUID, uuid.UUID, http.Header) ([]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error) {
			return nil, nil, nil, xerrors.New("dial failed")
		},
		Clock:                mclk,
		PrometheusRegisterer: reg,
	})

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	statusNotifications := make(chan osschatd.StatusNotification, 1)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()

	_ = fn(ctx, osschatd.SubscribeFnParams{
		ChatID:              uuid.New(),
		Chat:                database.Chat{Status: database.ChatStatusWaiting},
		WorkerID:            uuid.New(),
		StatusNotifications: statusNotifications,
		DB:                  db,
		Logger:              slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})

	statusNotifications <- osschatd.StatusNotification{Status: database.ChatStatusRunning, WorkerID: uuid.New()}
	trapReconnect.MustWait(ctx).MustRelease(ctx)

	require.Eventually(t, func() bool {
		return counterValue(t, reg, "coderd_chat_stream_relay_open_total", prometheus.Labels{"source": "status_notification", "result": "dial_error"}) == 1 &&
			counterValue(t, reg, "coderd_chat_stream_relay_reconnect_scheduled_total", prometheus.Labels{"reason": "dial_failed"}) == 1
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestRelayMetricsRelayPartsClosedSchedulesReconnect(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()
	remoteWorkerID := uuid.New()
	fn := entchatd.NewMultiReplicaSubscribeFn(entchatd.MultiReplicaSubscribeConfig{
		DialerFn: func(context.Context, uuid.UUID, uuid.UUID, http.Header) ([]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error) {
			parts := make(chan codersdk.ChatStreamEvent, 1)
			parts <- codersdk.ChatStreamEvent{
				Type: codersdk.ChatStreamEventTypeMessagePart,
				MessagePart: &codersdk.ChatStreamMessagePart{
					Role: "assistant",
					Part: codersdk.ChatMessageText("hello"),
				},
			}
			close(parts)
			return nil, parts, func() {}, nil
		},
		Clock:                mclk,
		PrometheusRegisterer: reg,
	})

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	statusNotifications := make(chan osschatd.StatusNotification)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()

	_ = fn(ctx, osschatd.SubscribeFnParams{
		ChatID:              uuid.New(),
		Chat:                database.Chat{Status: database.ChatStatusRunning, WorkerID: uuid.NullUUID{UUID: remoteWorkerID, Valid: true}},
		WorkerID:            uuid.New(),
		StatusNotifications: statusNotifications,
		DB:                  db,
		Logger:              slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})

	trapReconnect.MustWait(ctx).MustRelease(ctx)
	require.Eventually(t, func() bool {
		return counterValue(t, reg, "coderd_chat_stream_relay_close_total", prometheus.Labels{"reason": "relay_parts_closed"}) == 1 &&
			counterValue(t, reg, "coderd_chat_stream_relay_reconnect_scheduled_total", prometheus.Labels{"reason": "relay_parts_closed"}) == 1 &&
			gaugeValue(t, reg, "coderd_chat_stream_relay_active", prometheus.Labels{}) == 0
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestRelayMetricsReconnectPollOutcomes(t *testing.T) {
	t.Parallel()

	t.Run("DBError", func(t *testing.T) {
		t.Parallel()
		relayReconnectPollTest(t, "db_error", func(db *dbmock.MockStore, chatID, remoteWorkerID uuid.UUID) {
			db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{}, xerrors.New("db boom"))
		}, func(reg *prometheus.Registry) bool {
			return counterValue(t, reg, "coderd_chat_stream_relay_reconnect_poll_total", prometheus.Labels{"result": "db_error"}) == 1 &&
				counterValue(t, reg, "coderd_chat_stream_relay_reconnect_scheduled_total", prometheus.Labels{"reason": "db_get_chat_failed"}) == 1
		})
	})

	t.Run("StillRemoteRunning", func(t *testing.T) {
		t.Parallel()
		relayReconnectPollTest(t, "still_remote_running", func(db *dbmock.MockStore, chatID, remoteWorkerID uuid.UUID) {
			db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
				ID:       chatID,
				Status:   database.ChatStatusRunning,
				WorkerID: uuid.NullUUID{UUID: remoteWorkerID, Valid: true},
			}, nil)
		}, func(reg *prometheus.Registry) bool {
			return counterValue(t, reg, "coderd_chat_stream_relay_reconnect_poll_total", prometheus.Labels{"result": "still_remote_running"}) == 1 &&
				counterValue(t, reg, "coderd_chat_stream_relay_open_total", prometheus.Labels{"source": "reconnect", "result": "dial_error"}) == 1
		})
	})

	t.Run("NotRemoteRunning", func(t *testing.T) {
		t.Parallel()
		relayReconnectPollTest(t, "not_remote_running", func(db *dbmock.MockStore, chatID, remoteWorkerID uuid.UUID) {
			db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
				ID:     chatID,
				Status: database.ChatStatusWaiting,
			}, nil)
		}, func(reg *prometheus.Registry) bool {
			return counterValue(t, reg, "coderd_chat_stream_relay_reconnect_poll_total", prometheus.Labels{"result": "not_remote_running"}) == 1
		})
	})
}

func relayReconnectPollTest(t *testing.T, name string, expectDB func(db *dbmock.MockStore, chatID, remoteWorkerID uuid.UUID), done func(reg *prometheus.Registry) bool) {
	t.Helper()

	reg := prometheus.NewRegistry()
	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()
	remoteWorkerID := uuid.New()
	chatID := uuid.New()
	fn := entchatd.NewMultiReplicaSubscribeFn(entchatd.MultiReplicaSubscribeConfig{
		DialerFn: func(context.Context, uuid.UUID, uuid.UUID, http.Header) ([]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error) {
			return nil, nil, nil, xerrors.New(name + " dial failed")
		},
		Clock:                mclk,
		PrometheusRegisterer: reg,
	})

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	expectDB(db, chatID, remoteWorkerID)
	statusNotifications := make(chan osschatd.StatusNotification, 1)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()

	_ = fn(ctx, osschatd.SubscribeFnParams{
		ChatID:              chatID,
		Chat:                database.Chat{Status: database.ChatStatusWaiting},
		WorkerID:            uuid.New(),
		StatusNotifications: statusNotifications,
		DB:                  db,
		Logger:              slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})

	statusNotifications <- osschatd.StatusNotification{Status: database.ChatStatusRunning, WorkerID: remoteWorkerID}
	trapReconnect.MustWait(ctx).MustRelease(ctx)
	mclk.Advance(500 * time.Millisecond).MustWait(ctx)

	require.Eventually(t, func() bool {
		return done(reg)
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func counterValue(t *testing.T, reg prometheus.Gatherer, metricName string, labels prometheus.Labels) float64 {
	t.Helper()
	metric := promhelp.MetricValue(t, reg, metricName, labels)
	if metric == nil {
		return 0
	}
	return metric.GetCounter().GetValue()
}

func gaugeValue(t *testing.T, reg prometheus.Gatherer, metricName string, labels prometheus.Labels) float64 {
	t.Helper()
	metric := promhelp.MetricValue(t, reg, metricName, labels)
	if metric == nil {
		return 0
	}
	return metric.GetGauge().GetValue()
}
