package dispatch_test

import (
	"context"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/alerts"
	"github.com/coder/coder/v2/coderd/alerts/dispatch"
	"github.com/coder/coder/v2/coderd/alerts/types"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
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
				NotificationName: "test",
				AlertTemplateID:  alerts.TemplateWorkspaceDeleted.String(),
				UserID:           "valid",
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
				NotificationName: "test",
				AlertTemplateID:  alerts.TemplateWorkspaceDeleted.String(),
				UserID:           "invalid",
				Actions:          []types.TemplateAction{},
			},
			expectedErr:   "parse user ID",
			expectedRetry: false,
		},
		{
			name: "InvalidTemplateID",
			payload: types.MessagePayload{
				NotificationName: "test",
				AlertTemplateID:  "invalid",
				UserID:           "valid",
				Actions:          []types.TemplateAction{},
			},
			expectedErr:   "parse template ID",
			expectedRetry: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, pubsub := dbtestutil.NewDB(t)

			if tc.payload.UserID == "valid" {
				user := dbgen.User(t, db, database.User{})
				tc.payload.UserID = user.ID.String()
			}

			ctx := context.Background()

			handler := dispatch.NewInboxHandler(logger.Named("smtp"), db, pubsub)
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
				notifs, err := db.GetInboxAlertsByUserID(ctx, database.GetInboxAlertsByUserIDParams{
					UserID:     uid,
					ReadStatus: database.InboxAlertReadStatusAll,
				})

				require.NoError(t, err)
				require.Len(t, notifs, 1)
				require.Equal(t, tc.msgID, notifs[0].ID)
			}
		})
	}
}
