package notifications_test

import (
	"context"
	"sync/atomic"
	"testing"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
)

func defaultNotificationsConfig(method database.NotificationMethod) codersdk.NotificationsConfig {
	return codersdk.NotificationsConfig{
		Method:              serpent.String(method),
		MaxSendAttempts:     5,
		FetchInterval:       serpent.Duration(time.Millisecond * 100),
		StoreSyncInterval:   serpent.Duration(time.Millisecond * 200),
		LeasePeriod:         serpent.Duration(time.Second * 10),
		DispatchTimeout:     serpent.Duration(time.Second * 5),
		RetryInterval:       serpent.Duration(time.Millisecond * 50),
		LeaseCount:          10,
		StoreSyncBufferSize: 50,
		SMTP:                codersdk.NotificationsEmailConfig{},
		Webhook:             codersdk.NotificationsWebhookConfig{},
		Push:                codersdk.NotificationsPushConfig{},
	}
}

func defaultHelpers() map[string]any {
	return map[string]any{
		"base_url":     func() string { return "http://test.com" },
		"current_year": func() string { return "2024" },
		"logo_url":     func() string { return "https://coder.com/coder-logo-horizontal.png" },
		"app_name":     func() string { return "Coder" },
	}
}

func createSampleUser(t *testing.T, db database.Store) database.User {
	return dbgen.User(t, db, database.User{
		Email:    "bob@coder.com",
		Username: "bob",
	})
}

func createMetrics() *notifications.Metrics {
	return notifications.NewMetrics(prometheus.NewRegistry())
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

func (i *dispatchInterceptor) Dispatcher(payload types.MessagePayload, title, body string, _ template.FuncMap) (dispatch.DeliveryFunc, error) {
	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		deliveryFn, err := i.handler.Dispatcher(payload, title, body, defaultHelpers())
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

type dispatchCall struct {
	payload     types.MessagePayload
	title, body string
	result      chan<- dispatchResult
}

type dispatchResult struct {
	retryable bool
	err       error
}

type chanHandler struct {
	calls chan dispatchCall
}

func (c chanHandler) Dispatcher(payload types.MessagePayload, title, body string, _ template.FuncMap) (dispatch.DeliveryFunc, error) {
	result := make(chan dispatchResult)
	call := dispatchCall{
		payload: payload,
		title:   title,
		body:    body,
		result:  result,
	}
	return func(ctx context.Context, _ uuid.UUID) (bool, error) {
		select {
		case c.calls <- call:
			select {
			case r := <-result:
				return r.retryable, r.err
			case <-ctx.Done():
				return false, ctx.Err()
			}
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}, nil
}

var _ notifications.Handler = &chanHandler{}
