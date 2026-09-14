package cli

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/coder/v2/testutil"
)

// TestNotificationTriggersTriggerTimeFor covers the per-notification latency
// correlation logic. TemplateTemplateDeleted notifications carry the deleted
// template's ID in their targets, so a receipt must be measured against that
// template's own deletion time regardless of the order notifications are
// delivered in. The batch start is only a fallback for receipts that carry no
// usable target (notably SMTP summaries).
func TestNotificationTriggersTriggerTimeFor(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	template1 := uuid.New()
	template2 := uuid.New()
	template3 := uuid.New()

	batchStart := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	delete1 := batchStart.Add(1 * time.Second)
	delete2 := batchStart.Add(2 * time.Second)
	delete3 := batchStart.Add(3 * time.Second)

	deleteTimes := map[uuid.UUID]time.Time{
		template1: delete1,
		template2: delete2,
		template3: delete3,
	}

	cases := []struct {
		name     string
		triggers notificationTriggers
		targets  []uuid.UUID
		wantTime time.Time
		wantOK   bool
	}{
		{
			// The org ID is an unrelated target that must be skipped in favor of
			// the matching template ID.
			name:     "matches template target ignoring unrelated targets",
			triggers: notificationTriggers{deleteTimes: deleteTimes, batchStart: batchStart},
			targets:  []uuid.UUID{orgID, template2},
			wantTime: delete2,
			wantOK:   true,
		},
		{
			// template3 was deleted last but is referenced here; identity lookup
			// must return delete3, whereas positional/index pairing would not.
			name:     "correlates by target identity not delivery order",
			triggers: notificationTriggers{deleteTimes: deleteTimes, batchStart: batchStart},
			targets:  []uuid.UUID{template3},
			wantTime: delete3,
			wantOK:   true,
		},
		{
			// Iteration returns the first matching target, which is deterministic.
			name:     "first matching target wins",
			triggers: notificationTriggers{deleteTimes: deleteTimes, batchStart: batchStart},
			targets:  []uuid.UUID{template1, template2},
			wantTime: delete1,
			wantOK:   true,
		},
		{
			name:     "falls back to batch start when no target matches",
			triggers: notificationTriggers{deleteTimes: deleteTimes, batchStart: batchStart},
			targets:  []uuid.UUID{orgID},
			wantTime: batchStart,
			wantOK:   true,
		},
		{
			name:     "falls back to batch start when targets empty",
			triggers: notificationTriggers{deleteTimes: deleteTimes, batchStart: batchStart},
			targets:  nil,
			wantTime: batchStart,
			wantOK:   true,
		},
		{
			// No matching target and no batch start means there is nothing to
			// measure against, so the receipt is skipped by the caller.
			name:     "no match and zero batch start returns not ok",
			triggers: notificationTriggers{deleteTimes: deleteTimes},
			targets:  []uuid.UUID{orgID},
			wantTime: time.Time{},
			wantOK:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotTime, gotOK := tc.triggers.triggerTimeFor(tc.targets)
			require.Equal(t, tc.wantOK, gotOK)
			require.True(t, tc.wantTime.Equal(gotTime), "want %s, got %s", tc.wantTime, gotTime)
		})
	}
}

// TestComputeNotificationLatencies verifies that every recorded receipt produces
// its own latency sample: websocket receipts are correlated to the deleted
// template named in their targets, SMTP receipts are measured against the batch
// start, and failed runs are skipped entirely.
func TestComputeNotificationLatencies(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	notificationType := notificationsLib.TemplateTemplateDeleted
	deletedTemplate1 := uuid.New()
	deletedTemplate2 := uuid.New()

	batchStart := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	delete1 := batchStart.Add(1 * time.Second)
	delete2 := batchStart.Add(2 * time.Second)

	triggerCh := make(chan notificationTriggers, 1)
	triggerCh <- notificationTriggers{
		deleteTimes: map[uuid.UUID]time.Time{
			deletedTemplate1: delete1,
			deletedTemplate2: delete2,
		},
		batchStart: batchStart,
	}

	results := harness.Results{
		Runs: map[string]harness.RunResult{
			// Failed runs are skipped, so their receipts must not be measured.
			"failed": {
				Error: xerrors.New("run failed"),
				Metrics: map[string]any{
					notifications.WebsocketNotificationReceiptTimeMetric: map[uuid.UUID][]notifications.ReceivedNotification{
						notificationType: {
							{ReceiptTime: delete1.Add(time.Second), Targets: []uuid.UUID{deletedTemplate1}},
						},
					},
				},
			},
			// Two websocket receipts of the same type, each correlated to its own
			// deleted template, must yield two samples.
			"websocket": {
				Metrics: map[string]any{
					notifications.WebsocketNotificationReceiptTimeMetric: map[uuid.UUID][]notifications.ReceivedNotification{
						notificationType: {
							{ReceiptTime: delete1.Add(3 * time.Second), Targets: []uuid.UUID{deletedTemplate1}},
							{ReceiptTime: delete2.Add(5 * time.Second), Targets: []uuid.UUID{deletedTemplate2}},
						},
					},
				},
			},
			// SMTP summaries carry no targets and are measured against batchStart.
			"smtp": {
				Metrics: map[string]any{
					notifications.SMTPNotificationReceiptTimeMetric: map[uuid.UUID][]time.Time{
						notificationType: {batchStart.Add(4 * time.Second)},
					},
				},
			},
		},
	}

	reg := prometheus.NewRegistry()
	metrics := notifications.NewMetrics(reg)

	require.NoError(t, computeNotificationLatencies(ctx, logger, triggerCh, results, metrics))

	require.Equal(t, uint64(2), latencySampleCount(t, reg, notifications.NotificationTypeWebsocket),
		"each websocket receipt should produce its own latency sample")
	require.Equal(t, uint64(1), latencySampleCount(t, reg, notifications.NotificationTypeSMTP),
		"each SMTP receipt should produce a batch-relative latency sample")
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
