package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

// TestWatchNotifications_CountsAndDedupes exercises the watcher's core behavior:
// it counts TemplateTemplateDeleted notifications up to the expected number,
// deduplicates repeated deliveries of the same instance, and ignores any other
// notification type.
func TestWatchNotifications_CountsAndDedupes(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	deletionType := notificationsLib.TemplateTemplateDeleted
	otherType := uuid.New()

	firstID := uuid.New()
	secondID := uuid.New()

	// Scripted deliveries. The watcher wants 2 deletion notifications.
	msgs := []codersdk.GetInboxNotificationResponse{
		{Notification: codersdk.InboxNotification{ID: firstID, TemplateID: deletionType}},
		// Duplicate instance ID: must be deduped by the seen set.
		{Notification: codersdk.InboxNotification{ID: firstID, TemplateID: deletionType}},
		// Non-deletion type: must be ignored.
		{Notification: codersdk.InboxNotification{ID: uuid.New(), TemplateID: otherType}},
		{Notification: codersdk.InboxNotification{ID: secondID, TemplateID: deletionType}},
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

	err = runner.watchNotifications(ctx, conn, codersdk.User{}, logger, 2)
	require.NoError(t, err)

	// Exactly two receipts: the duplicate and the non-deletion type are excluded.
	require.Len(t, runner.websocketDeletionReceiptTimes, 2)
}
