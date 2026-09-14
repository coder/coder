package cli

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
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
