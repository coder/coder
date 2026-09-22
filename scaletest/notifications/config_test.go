package notifications_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/notifications"
)

// TestConfigValidate covers Config.Validate, focusing on the template-admin
// deletion-count guard: a template admin that expects fewer than one deletion
// would treat itself as already satisfied, so it must be rejected. A regular
// user's ExpectedDeletions is ignored and must never cause a validation error.
func TestConfigValidate(t *testing.T) {
	t.Parallel()

	baseConfig := func() notifications.Config {
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
		}
	}

	t.Run("ValidTemplateAdmin", func(t *testing.T) {
		t.Parallel()

		cfg := baseConfig()
		cfg.IsTemplateAdmin = true
		cfg.ExpectedDeletions = 1
		require.NoError(t, cfg.Validate())
	})

	t.Run("TemplateAdminZeroDeletions", func(t *testing.T) {
		t.Parallel()

		cfg := baseConfig()
		cfg.IsTemplateAdmin = true
		cfg.ExpectedDeletions = 0

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "expected_deletions must be at least 1")
	})

	t.Run("ValidRegularUser", func(t *testing.T) {
		t.Parallel()

		cfg := baseConfig()
		cfg.IsTemplateAdmin = false
		cfg.ExpectedDeletions = 0
		require.NoError(t, cfg.Validate())
	})

	t.Run("RegularUserDeletionsIgnored", func(t *testing.T) {
		t.Parallel()

		// A regular user's ExpectedDeletions is ignored, so a nonzero value must
		// not be rejected or otherwise mishandled.
		cfg := baseConfig()
		cfg.IsTemplateAdmin = false
		cfg.ExpectedDeletions = 5
		require.NoError(t, cfg.Validate())
	})
}
