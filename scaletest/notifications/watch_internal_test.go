package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

// TestWatchNotifications_CountsPerTypeAndDedupes exercises the multi-notification
// behavior added for --template-deletion-count: watchNotifications must wait for
// the requested count of each notification type, deduplicate repeated deliveries
// of the same notification instance, ignore extra deliveries beyond the requested
// count, and record one receipt time per counted notification.
func TestWatchNotifications_CountsPerTypeAndDedupes(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	typeOnce := uuid.New()    // expected once
	typeTwice := uuid.New()   // expected twice (e.g. two template deletions)
	typeIgnored := uuid.New() // not under test

	onceID := uuid.New()
	twiceFirstID := uuid.New()
	twiceSecondID := uuid.New()

	// Scripted deliveries. The watcher wants 1 of typeOnce and 2 of typeTwice.
	msgs := []codersdk.GetInboxNotificationResponse{
		{Notification: codersdk.InboxNotification{ID: onceID, TemplateID: typeOnce}},
		// Extra delivery of an already-satisfied type: must be skipped by the
		// received-count >= want gate and not recorded.
		{Notification: codersdk.InboxNotification{ID: uuid.New(), TemplateID: typeOnce}},
		// Unexpected type: ignored entirely.
		{Notification: codersdk.InboxNotification{ID: uuid.New(), TemplateID: typeIgnored}},
		{Notification: codersdk.InboxNotification{ID: twiceFirstID, TemplateID: typeTwice}},
		// Duplicate instance ID: must be deduped by the seen set.
		{Notification: codersdk.InboxNotification{ID: twiceFirstID, TemplateID: typeTwice}},
		{Notification: codersdk.InboxNotification{ID: twiceSecondID, TemplateID: typeTwice}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		for _, m := range msgs {
			b, err := json.Marshal(m)
			if err != nil {
				return
			}
			if err := conn.Write(r.Context(), websocket.MessageText, b); err != nil {
				return
			}
		}
		// Drain reads so the client's close handshake completes promptly instead
		// of waiting out the close grace period.
		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	// nolint:bodyclose // The websocket upgrade response body is closed via conn.
	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "done")

	runner := NewRunner(nil, Config{Metrics: NewMetrics(prometheus.NewRegistry())})
	expected := map[uuid.UUID]int{typeOnce: 1, typeTwice: 2}

	err = runner.watchNotifications(ctx, conn, codersdk.User{}, logger, expected)
	require.NoError(t, err)

	// typeOnce: exactly one receipt despite the extra delivery.
	require.Len(t, runner.websocketReceiptTimes[typeOnce], 1)
	// typeTwice: exactly two receipts despite the duplicate instance ID.
	require.Len(t, runner.websocketReceiptTimes[typeTwice], 2)
	// Unexpected type never recorded.
	require.NotContains(t, runner.websocketReceiptTimes, typeIgnored)
}
