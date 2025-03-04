package dispatch_test

import (
	"context"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
)

func TestInbox(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	tests := []struct {
		name          string
		msgID         uuid.UUID
		payload       types.MessagePayload
		expectedErr   string
		expectedRetry bool
	}{
		{
			name:  "OK",
			msgID: uuid.New(),
			payload: types.MessagePayload{
				NotificationName:       "test",
				NotificationTemplateID: notifications.TemplateWorkspaceDeleted.String(),
				UserID:                 "1e965b51-9465-43d8-ac20-c5f689f9c788",
				Actions: []types.TemplateAction{
					{
						Label: "View my workspace",
						URL:   "https://coder.com/workspaces/1",
					},
				},
			},
		},
		{
			name: "InvalidUserID",
			payload: types.MessagePayload{
				NotificationName:       "test",
				NotificationTemplateID: notifications.TemplateWorkspaceDeleted.String(),
				UserID:                 "invalid",
				Actions:                []types.TemplateAction{},
			},
			expectedErr:   "parse user ID",
			expectedRetry: false,
		},
		{
			name: "InvalidTemplateID",
			payload: types.MessagePayload{
				NotificationName:       "test",
				NotificationTemplateID: "invalid",
				UserID:                 "1e965b51-9465-43d8-ac20-c5f689f9c788",
				Actions:                []types.TemplateAction{},
			},
			expectedErr:   "parse template ID",
			expectedRetry: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			dbtestutil.DisableForeignKeysAndTriggers(t, db)

			ctx := context.Background()

			handler := dispatch.NewInboxHandler(logger.Named("smtp"), db)
			dispatcherFunc, err := handler.Dispatcher(tc.payload, "", "", nil)
			require.NoError(t, err)

			retryable, err := dispatcherFunc(ctx, tc.msgID)

			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
				require.Equal(t, tc.expectedRetry, retryable)
			} else {
				require.NoError(t, err)
				require.False(t, retryable)
				uid := uuid.MustParse(tc.payload.UserID)
				notifs, err := db.GetInboxNotificationsByUserID(ctx, database.GetInboxNotificationsByUserIDParams{
					UserID:     uid,
					ReadStatus: database.InboxNotificationReadStatusAll,
				})

				require.NoError(t, err)
				require.Len(t, notifs, 1)
				require.Equal(t, tc.msgID, notifs[0].ID)
			}
		})
	}
}
