package notifications_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/codersdk"
)

func defaultNotificationsMutator(method database.NotificationMethod) func(vals *codersdk.DeploymentValues) {
	return func(vals *codersdk.DeploymentValues) {
		vals.Notifications.Method = serpent.String(method)
		vals.Notifications.MaxSendAttempts = 5
		vals.Notifications.FetchInterval = serpent.Duration(time.Millisecond * 100)
		vals.Notifications.StoreSyncInterval = serpent.Duration(time.Millisecond * 200)
		vals.Notifications.LeasePeriod = serpent.Duration(time.Second * 10)
		vals.Notifications.DispatchTimeout = serpent.Duration(time.Second * 5)
		vals.Notifications.RetryInterval = serpent.Duration(time.Millisecond * 50)
		vals.Notifications.LeaseCount = 10
		vals.Notifications.StoreSyncBufferSize = 50
	}
}

func defaultHelpers() map[string]any {
	return map[string]any{
		"base_url":     func() string { return "http://test.com" },
		"current_year": func() string { return "2024" },
	}
}

func createSampleOrgMember(t *testing.T, db database.Store, mut ...func(u *database.User, o *database.Organization)) database.OrganizationMember {
	org := database.Organization{}
	user := database.User{
		Email: "user@coder.com",
	}

	for _, fn := range mut {
		fn(&user, &org)
	}

	o := dbgen.Organization(t, db, org)
	u := dbgen.User(t, db, user)

	return dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID, OrganizationID: o.ID})
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

func (i *dispatchInterceptor) Dispatcher(payload types.MessagePayload, title, body string) (dispatch.DeliveryFunc, error) {
	return func(ctx context.Context, cfgResolver runtimeconfig.Resolver, msgID uuid.UUID) (retryable bool, err error) {
		deliveryFn, err := i.handler.Dispatcher(payload, title, body)
		if err != nil {
			return false, err
		}

		retryable, err = deliveryFn(ctx, cfgResolver, msgID)

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
