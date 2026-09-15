package notifications_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/notifications"
)

// TestConfigValidate covers Config.Validate, focusing on the per-type
// ExpectedNotifications count guard added for --template-deletion-count: a count
// below 1 would let a watcher treat a type as already satisfied (receivedCounts
// starts at 0), so it must be rejected.
func TestConfigValidate(t *testing.T) {
	t.Parallel()

	validConfig := func() notifications.Config {
		return notifications.Config{
			SessionToken: "session-token",
			PreCreatedUser: codersdk.User{
				ReducedUser: codersdk.ReducedUser{
					MinimalUser: codersdk.MinimalUser{ID: uuid.New()},
				},
			},
			NotificationTimeout:   time.Minute,
			DialTimeout:           time.Minute,
			DialBarrier:           new(sync.WaitGroup),
			ReceivingWatchBarrier: new(sync.WaitGroup),
			Metrics:               notifications.NewMetrics(prometheus.NewRegistry()),
			ExpectedNotifications: map[uuid.UUID]int{
				notificationsLib.TemplateTemplateDeleted: 1,
			},
		}
	}

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, validConfig().Validate())
	})

	t.Run("ZeroExpectedCount", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		id := uuid.New()
		cfg.ExpectedNotifications = map[uuid.UUID]int{id: 0}

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "expected notification count")
		require.Contains(t, err.Error(), id.String())
	})
}
