package cli

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/coder/v2/testutil"
)

// TestComputeNotificationLatencies verifies that every recorded receipt produces
// its own latency sample measured against the batch trigger time, and that
// failed runs are skipped entirely.
func TestComputeNotificationLatencies(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	triggerTime := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)

	results := harness.Results{
		Runs: map[string]harness.RunResult{
			// Failed runs are skipped, so their receipts must not be measured.
			"failed": {
				Error: xerrors.New("run failed"),
				Metrics: map[string]any{
					notifications.WebsocketNotificationReceiptTimeMetric: []time.Time{triggerTime.Add(time.Second)},
				},
			},
			// Two websocket receipts must yield two samples.
			"websocket": {
				Metrics: map[string]any{
					notifications.WebsocketNotificationReceiptTimeMetric: []time.Time{triggerTime.Add(3 * time.Second), triggerTime.Add(5 * time.Second)},
				},
			},
			// SMTP receipts are also measured against the batch trigger time.
			"smtp": {
				Metrics: map[string]any{
					notifications.SMTPNotificationReceiptTimeMetric: []time.Time{triggerTime.Add(4 * time.Second)},
				},
			},
		},
	}

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)

	require.NoError(t, computeNotificationLatencies(ctx, logger, triggerTime, results, metrics))

	require.Equal(t, uint64(2), latencySampleCount(t, reg, notifications.NotificationTypeWebsocket),
		"each websocket receipt should produce its own latency sample")
	require.Equal(t, uint64(1), latencySampleCount(t, reg, notifications.NotificationTypeSMTP),
		"each SMTP receipt should produce a latency sample")
}

// latencySampleCount returns how many latency observations were recorded for the
// given notification type across all label sets.
func latencySampleCount(t *testing.T, reg *prometheus.Registry, notificationType notifications.NotificationType) uint64 {
	t.Helper()

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var total uint64
	for _, mf := range mfs {
		if mf.GetName() != "coderd_scaletest_notification_delivery_latency_seconds" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetName() == "notification_type" && label.GetValue() == string(notificationType) {
					total += m.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return total
}

// TestFilterScaletestUsersByPrefix covers the pure user-selection logic behind
// the notifications scaletest user reuse: an infix pool must not pick up users
// from the default pool (isolation), non-scaletest users are ignored, and the
// username guard rejects users that only match the prefix in another field.
func TestFilterScaletestUsersByPrefix(t *testing.T) {
	t.Parallel()

	users := []codersdk.User{
		scaletestUser("scaletest-notif-aaaaaaaa-0", "aaaaaaaa-0@scaletest.local"),
		scaletestUser("scaletest-notif-bbbbbbbb-1", "bbbbbbbb-1@scaletest.local"),
		// Default pool: a scaletest user that is NOT in the notif pool.
		scaletestUser("scaletest-cccccccc-0", "cccccccc-0@scaletest.local"),
		// Not a scaletest user at all.
		scaletestUser("regular-user", "regular@example.com"),
		// Scaletest email but a username that does not start with the prefix; the
		// guard must reject it even though a search could surface it.
		scaletestUser("admin", "scaletest-notif-dddddddd-9@scaletest.local"),
	}

	cases := []struct {
		name   string
		prefix string
		want   []string
	}{
		{
			name:   "infix isolates its own pool",
			prefix: "scaletest-notif-",
			want: []string{
				"scaletest-notif-aaaaaaaa-0",
				"scaletest-notif-bbbbbbbb-1",
			},
		},
		{
			name:   "default prefix selects all scaletest users",
			prefix: loadtestutil.ScaleTestPrefix + "-",
			want: []string{
				"scaletest-notif-aaaaaaaa-0",
				"scaletest-notif-bbbbbbbb-1",
				"scaletest-cccccccc-0",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := filterScaletestUsersByPrefix(users, tc.prefix)

			gotNames := make([]string, 0, len(got))
			for _, u := range got {
				gotNames = append(gotNames, u.Username)
			}
			require.ElementsMatch(t, tc.want, gotNames)
		})
	}
}

func scaletestUser(username, email string) codersdk.User {
	return codersdk.User{
		ReducedUser: codersdk.ReducedUser{
			MinimalUser: codersdk.MinimalUser{Username: username},
			Email:       email,
		},
	}
}
