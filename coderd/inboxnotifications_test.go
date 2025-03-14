package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

const (
	inboxNotificationsPageSize = 25
)

var (
	failingPaginationUUID = uuid.MustParse("fba6966a-9061-4111-8e1a-f6a9fbea4b16")
)

func TestInboxNotification_Watch(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		logger := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		db, ps := dbtestutil.NewDB(t)
		db.DisableForeignKeysAndTriggers(ctx)

		firstClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, firstClient)
		member, memberClient := coderdtest.CreateAnotherUser(t, firstClient, firstUser.OrganizationID, rbac.RoleTemplateAdmin())

		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			member.WatchInboxNotificationx(ctx)
		}))
		defer srv.Close()

		u, err := member.URL.Parse("/api/v2/notifications/inbox/watch")
		require.NoError(t, err)

		// nolint: bodyclose
		wsConn, resp, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
			HTTPHeader: http.Header{
				"Coder-Session-Token": []string{member.SessionToken()},
			},
		})
		if err != nil {
			if resp.StatusCode != http.StatusSwitchingProtocols {
				err = codersdk.ReadBodyAsError(resp)
			}
			require.NoError(t, err)
		}
		defer wsConn.Close(websocket.StatusNormalClosure, "done")
		_, cnc := codersdk.WebsocketNetConn(ctx, wsConn, websocket.MessageBinary)

		inboxHandler := dispatch.NewInboxHandler(logger, db, ps)
		dispatchFunc, err := inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfMemory.String(),
		}, "notification title", "notification content", nil)
		require.NoError(t, err)

		msgID := uuid.New()
		_, err = dispatchFunc(ctx, msgID)
		require.NoError(t, err)

		op := make([]byte, 1024)
		mt, err := cnc.Read(op)
		require.NoError(t, err)
		require.Equal(t, websocket.MessageText, mt)
	})
}

func TestInboxNotifications_List(t *testing.T) {
	t.Parallel()

	// I skip these tests specifically on windows as for now they are flaky - only on Windows.
	// For now the idea is that the runner takes too long to insert the entries, could be worth
	// investigating a manual Tx.
	if runtime.GOOS == "windows" {
		t.Skip("our runners are randomly taking too long to insert entries")
	}

	// create table-based tests for errors and repeting use cases
	tests := []struct {
		name               string
		expectedError      string
		listTemplate       string
		listTarget         string
		listReadStatus     string
		listStartingBefore string
	}{
		{"nok - wrong targets", `Query param "targets" has invalid values`, "", "wrong_target", "", ""},
		{"nok - wrong templates", `Query param "templates" has invalid values`, "wrong_template", "", "", ""},
		{"nok - wrong read status", "starting_before query parameter should be any of 'all', 'read', 'unread'", "", "", "erroneous", ""},
		{"nok - wrong starting before", `Query param "starting_before" must be a valid uuid`, "", "", "", "xxx-xxx-xxx"},
		{"nok - not found starting before", `Failed to get notification by id`, "", "", "", failingPaginationUUID.String()},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
			firstUser := coderdtest.CreateFirstUser(t, client)
			client, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
			require.NoError(t, err)
			require.NotNil(t, notifs)
			require.Equal(t, 0, notifs.UnreadCount)
			require.Empty(t, notifs.Notifications)

			// create a new notifications to fill the database with data
			for i := range 20 {
				dbgen.NotificationInbox(t, api.Database, database.InsertInboxNotificationParams{
					ID:         uuid.New(),
					UserID:     member.ID,
					TemplateID: notifications.TemplateWorkspaceOutOfMemory,
					Title:      fmt.Sprintf("Notification %d", i),
					Actions:    json.RawMessage("[]"),
					Content:    fmt.Sprintf("Content of the notif %d", i),
					CreatedAt:  dbtime.Now(),
				})
			}

			notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
				Templates:      tt.listTemplate,
				Targets:        tt.listTarget,
				ReadStatus:     tt.listReadStatus,
				StartingBefore: tt.listStartingBefore,
			})
			require.ErrorContains(t, err, tt.expectedError)
			require.Empty(t, notifs.Notifications)
			require.Zero(t, notifs.UnreadCount)
		})
	}

	t.Run("OK empty", func(t *testing.T) {
		t.Parallel()

		client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		client, _ = coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)

		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)
	})

	t.Run("OK with pagination", func(t *testing.T) {
		t.Parallel()

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		client, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		for i := range 40 {
			dbgen.NotificationInbox(t, api.Database, database.InsertInboxNotificationParams{
				ID:         uuid.New(),
				UserID:     member.ID,
				TemplateID: notifications.TemplateWorkspaceOutOfMemory,
				Title:      fmt.Sprintf("Notification %d", i),
				Actions:    json.RawMessage("[]"),
				Content:    fmt.Sprintf("Content of the notif %d", i),
				CreatedAt:  dbtime.Now(),
			})
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 40, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, inboxNotificationsPageSize)

		require.Equal(t, "Notification 39", notifs.Notifications[0].Title)

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			StartingBefore: notifs.Notifications[inboxNotificationsPageSize-1].ID.String(),
		})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 40, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 15)

		require.Equal(t, "Notification 14", notifs.Notifications[0].Title)
	})

	t.Run("OK with template filter", func(t *testing.T) {
		t.Parallel()

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		client, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		for i := range 10 {
			dbgen.NotificationInbox(t, api.Database, database.InsertInboxNotificationParams{
				ID:     uuid.New(),
				UserID: member.ID,
				TemplateID: func() uuid.UUID {
					if i%2 == 0 {
						return notifications.TemplateWorkspaceOutOfMemory
					}

					return notifications.TemplateWorkspaceOutOfDisk
				}(),
				Title:     fmt.Sprintf("Notification %d", i),
				Actions:   json.RawMessage("[]"),
				Content:   fmt.Sprintf("Content of the notif %d", i),
				CreatedAt: dbtime.Now(),
			})
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			Templates: notifications.TemplateWorkspaceOutOfMemory.String(),
		})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 10, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 5)

		require.Equal(t, "Notification 8", notifs.Notifications[0].Title)
	})

	t.Run("OK with target filter", func(t *testing.T) {
		t.Parallel()

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		client, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		filteredTarget := uuid.New()

		for i := range 10 {
			dbgen.NotificationInbox(t, api.Database, database.InsertInboxNotificationParams{
				ID:         uuid.New(),
				UserID:     member.ID,
				TemplateID: notifications.TemplateWorkspaceOutOfMemory,
				Targets: func() []uuid.UUID {
					if i%2 == 0 {
						return []uuid.UUID{filteredTarget}
					}

					return []uuid.UUID{}
				}(),
				Title:     fmt.Sprintf("Notification %d", i),
				Actions:   json.RawMessage("[]"),
				Content:   fmt.Sprintf("Content of the notif %d", i),
				CreatedAt: dbtime.Now(),
			})
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			Targets: filteredTarget.String(),
		})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 10, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 5)

		require.Equal(t, "Notification 8", notifs.Notifications[0].Title)
	})

	t.Run("OK with multiple filters", func(t *testing.T) {
		t.Parallel()

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		client, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		filteredTarget := uuid.New()

		for i := range 10 {
			dbgen.NotificationInbox(t, api.Database, database.InsertInboxNotificationParams{
				ID:     uuid.New(),
				UserID: member.ID,
				TemplateID: func() uuid.UUID {
					if i < 5 {
						return notifications.TemplateWorkspaceOutOfMemory
					}

					return notifications.TemplateWorkspaceOutOfDisk
				}(),
				Targets: func() []uuid.UUID {
					if i%2 == 0 {
						return []uuid.UUID{filteredTarget}
					}

					return []uuid.UUID{}
				}(),
				Title:     fmt.Sprintf("Notification %d", i),
				Actions:   json.RawMessage("[]"),
				Content:   fmt.Sprintf("Content of the notif %d", i),
				CreatedAt: dbtime.Now(),
			})
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			Targets:   filteredTarget.String(),
			Templates: notifications.TemplateWorkspaceOutOfDisk.String(),
		})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 10, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 2)

		require.Equal(t, "Notification 8", notifs.Notifications[0].Title)
	})
}

func TestInboxNotifications_ReadStatus(t *testing.T) {
	t.Parallel()

	// I skip these tests specifically on windows as for now they are flaky - only on Windows.
	// For now the idea is that the runner takes too long to insert the entries, could be worth
	// investigating a manual Tx.
	if runtime.GOOS == "windows" {
		t.Skip("our runners are randomly taking too long to insert entries")
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		client, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		for i := range 20 {
			dbgen.NotificationInbox(t, api.Database, database.InsertInboxNotificationParams{
				ID:         uuid.New(),
				UserID:     member.ID,
				TemplateID: notifications.TemplateWorkspaceOutOfMemory,
				Title:      fmt.Sprintf("Notification %d", i),
				Actions:    json.RawMessage("[]"),
				Content:    fmt.Sprintf("Content of the notif %d", i),
				CreatedAt:  dbtime.Now(),
			})
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 20, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 20)

		updatedNotif, err := client.UpdateInboxNotificationReadStatus(ctx, notifs.Notifications[19].ID.String(), codersdk.UpdateInboxNotificationReadStatusRequest{
			IsRead: true,
		})
		require.NoError(t, err)
		require.NotNil(t, updatedNotif)
		require.NotZero(t, updatedNotif.Notification.ReadAt)
		require.Equal(t, 19, updatedNotif.UnreadCount)

		updatedNotif, err = client.UpdateInboxNotificationReadStatus(ctx, notifs.Notifications[19].ID.String(), codersdk.UpdateInboxNotificationReadStatusRequest{
			IsRead: false,
		})
		require.NoError(t, err)
		require.NotNil(t, updatedNotif)
		require.Nil(t, updatedNotif.Notification.ReadAt)
		require.Equal(t, 20, updatedNotif.UnreadCount)

	})

	t.Run("NOK - wrong id", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		client, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		for i := range 20 {
			dbgen.NotificationInbox(t, api.Database, database.InsertInboxNotificationParams{
				ID:         uuid.New(),
				UserID:     member.ID,
				TemplateID: notifications.TemplateWorkspaceOutOfMemory,
				Title:      fmt.Sprintf("Notification %d", i),
				Actions:    json.RawMessage("[]"),
				Content:    fmt.Sprintf("Content of the notif %d", i),
				CreatedAt:  dbtime.Now(),
			})
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 20, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 20)

		updatedNotif, err := client.UpdateInboxNotificationReadStatus(ctx, "xxx-xxx-xxx", codersdk.UpdateInboxNotificationReadStatusRequest{
			IsRead: true,
		})
		require.ErrorContains(t, err, `Invalid UUID "xxx-xxx-xxx"`)
		require.Equal(t, 0, updatedNotif.UnreadCount)
		require.Empty(t, updatedNotif.Notification)
	})
	t.Run("NOK - unknown id", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		client, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		for i := range 20 {
			dbgen.NotificationInbox(t, api.Database, database.InsertInboxNotificationParams{
				ID:         uuid.New(),
				UserID:     member.ID,
				TemplateID: notifications.TemplateWorkspaceOutOfMemory,
				Title:      fmt.Sprintf("Notification %d", i),
				Actions:    json.RawMessage("[]"),
				Content:    fmt.Sprintf("Content of the notif %d", i),
				CreatedAt:  dbtime.Now(),
			})
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 20, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 20)

		updatedNotif, err := client.UpdateInboxNotificationReadStatus(ctx, failingPaginationUUID.String(), codersdk.UpdateInboxNotificationReadStatusRequest{
			IsRead: true,
		})
		require.ErrorContains(t, err, `Failed to update inbox notification read status`)
		require.Equal(t, 0, updatedNotif.UnreadCount)
		require.Empty(t, updatedNotif.Notification)
	})
}
