package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

var failingPaginationUUID = uuid.MustParse("fba6966a-9061-4111-8e1a-f6a9fbea4b16")

func TestInboxNotification_Watch(t *testing.T) {
	t.Parallel()

	// I skip these tests specifically on windows as for now they are flaky - only on Windows.
	// For now the idea is that the runner takes too long to insert the entries, could be worth
	// investigating a manual Tx.
	// see: https://github.com/coder/internal/issues/503
	if runtime.GOOS == "windows" {
		t.Skip("our runners are randomly taking too long to insert entries")
	}

	t.Run("Failure Modes", func(t *testing.T) {
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
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{})
				firstUser := coderdtest.CreateFirstUser(t, client)
				client, _ = coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()

				resp, err := client.Request(ctx, http.MethodGet, "/api/v2/notifications/inbox/watch", nil,
					codersdk.ListInboxNotificationsRequestToQueryParams(codersdk.ListInboxNotificationsRequest{
						Targets:        tt.listTarget,
						Templates:      tt.listTemplate,
						ReadStatus:     tt.listReadStatus,
						StartingBefore: tt.listStartingBefore,
					})...)
				require.NoError(t, err)
				defer resp.Body.Close()

				err = codersdk.ReadBodyAsError(resp)
				require.ErrorContains(t, err, tt.expectedError)
			})
		}
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)

		db, ps := dbtestutil.NewDB(t)

		firstClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
			Pubsub:   ps,
			Database: db,
		})
		firstUser := coderdtest.CreateFirstUser(t, firstClient)
		member, memberClient := coderdtest.CreateAnotherUser(t, firstClient, firstUser.OrganizationID, rbac.RoleTemplateAdmin())

		u, err := member.URL.Parse("/api/v2/notifications/inbox/watch")
		require.NoError(t, err)

		// nolint:bodyclose
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

		inboxHandler := dispatch.NewInboxHandler(logger, db, ps)
		dispatchFunc, err := inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfMemory.String(),
		}, "notification title", "notification content", nil)
		require.NoError(t, err)

		_, err = dispatchFunc(ctx, uuid.New())
		require.NoError(t, err)

		_, message, err := wsConn.Read(ctx)
		require.NoError(t, err)

		var notif codersdk.GetInboxNotificationResponse
		err = json.Unmarshal(message, &notif)
		require.NoError(t, err)

		require.Equal(t, 1, notif.UnreadCount)
		require.Equal(t, memberClient.ID, notif.Notification.UserID)

		// check for the fallback icon logic
		require.Equal(t, codersdk.FallbackIconWorkspace, notif.Notification.Icon)
	})

	t.Run("OK - change format", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)

		db, ps := dbtestutil.NewDB(t)

		firstClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
			Pubsub:   ps,
			Database: db,
		})
		firstUser := coderdtest.CreateFirstUser(t, firstClient)
		member, memberClient := coderdtest.CreateAnotherUser(t, firstClient, firstUser.OrganizationID, rbac.RoleTemplateAdmin())

		u, err := member.URL.Parse("/api/v2/notifications/inbox/watch?format=plaintext")
		require.NoError(t, err)

		// nolint:bodyclose
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

		inboxHandler := dispatch.NewInboxHandler(logger, db, ps)
		dispatchFunc, err := inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfMemory.String(),
		}, "# Notification Title", "This is the __content__.", nil)
		require.NoError(t, err)

		_, err = dispatchFunc(ctx, uuid.New())
		require.NoError(t, err)

		_, message, err := wsConn.Read(ctx)
		require.NoError(t, err)

		var notif codersdk.GetInboxNotificationResponse
		err = json.Unmarshal(message, &notif)
		require.NoError(t, err)

		require.Equal(t, 1, notif.UnreadCount)
		require.Equal(t, memberClient.ID, notif.Notification.UserID)

		require.Equal(t, "Notification Title", notif.Notification.Title)
		require.Equal(t, "This is the content.", notif.Notification.Content)
	})

	t.Run("OK - filters on templates", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)

		db, ps := dbtestutil.NewDB(t)

		firstClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
			Pubsub:   ps,
			Database: db,
		})
		firstUser := coderdtest.CreateFirstUser(t, firstClient)
		member, memberClient := coderdtest.CreateAnotherUser(t, firstClient, firstUser.OrganizationID, rbac.RoleTemplateAdmin())

		u, err := member.URL.Parse(fmt.Sprintf("/api/v2/notifications/inbox/watch?templates=%v", notifications.TemplateWorkspaceOutOfMemory))
		require.NoError(t, err)

		// nolint:bodyclose
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

		inboxHandler := dispatch.NewInboxHandler(logger, db, ps)
		dispatchFunc, err := inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfMemory.String(),
		}, "memory related title", "memory related content", nil)
		require.NoError(t, err)

		_, err = dispatchFunc(ctx, uuid.New())
		require.NoError(t, err)

		_, message, err := wsConn.Read(ctx)
		require.NoError(t, err)

		var notif codersdk.GetInboxNotificationResponse
		err = json.Unmarshal(message, &notif)
		require.NoError(t, err)

		require.Equal(t, 1, notif.UnreadCount)
		require.Equal(t, memberClient.ID, notif.Notification.UserID)
		require.Equal(t, "memory related title", notif.Notification.Title)

		dispatchFunc, err = inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfDisk.String(),
		}, "disk related title", "disk related title", nil)
		require.NoError(t, err)

		_, err = dispatchFunc(ctx, uuid.New())
		require.NoError(t, err)

		dispatchFunc, err = inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfMemory.String(),
		}, "second memory related title", "second memory related title", nil)
		require.NoError(t, err)

		_, err = dispatchFunc(ctx, uuid.New())
		require.NoError(t, err)

		_, message, err = wsConn.Read(ctx)
		require.NoError(t, err)

		err = json.Unmarshal(message, &notif)
		require.NoError(t, err)

		require.Equal(t, 3, notif.UnreadCount)
		require.Equal(t, memberClient.ID, notif.Notification.UserID)
		require.Equal(t, "second memory related title", notif.Notification.Title)
	})

	t.Run("OK - filters on targets", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)

		db, ps := dbtestutil.NewDB(t)

		firstClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
			Pubsub:   ps,
			Database: db,
		})
		firstUser := coderdtest.CreateFirstUser(t, firstClient)
		member, memberClient := coderdtest.CreateAnotherUser(t, firstClient, firstUser.OrganizationID, rbac.RoleTemplateAdmin())

		correctTarget := uuid.New()

		u, err := member.URL.Parse(fmt.Sprintf("/api/v2/notifications/inbox/watch?targets=%v", correctTarget.String()))
		require.NoError(t, err)

		// nolint:bodyclose
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

		inboxHandler := dispatch.NewInboxHandler(logger, db, ps)
		dispatchFunc, err := inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfMemory.String(),
			Targets:                []uuid.UUID{correctTarget},
		}, "memory related title", "memory related content", nil)
		require.NoError(t, err)

		_, err = dispatchFunc(ctx, uuid.New())
		require.NoError(t, err)

		_, message, err := wsConn.Read(ctx)
		require.NoError(t, err)

		var notif codersdk.GetInboxNotificationResponse
		err = json.Unmarshal(message, &notif)
		require.NoError(t, err)

		require.Equal(t, 1, notif.UnreadCount)
		require.Equal(t, memberClient.ID, notif.Notification.UserID)
		require.Equal(t, "memory related title", notif.Notification.Title)

		dispatchFunc, err = inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfMemory.String(),
			Targets:                []uuid.UUID{uuid.New()},
		}, "second memory related title", "second memory related title", nil)
		require.NoError(t, err)

		_, err = dispatchFunc(ctx, uuid.New())
		require.NoError(t, err)

		dispatchFunc, err = inboxHandler.Dispatcher(types.MessagePayload{
			UserID:                 memberClient.ID.String(),
			NotificationTemplateID: notifications.TemplateWorkspaceOutOfMemory.String(),
			Targets:                []uuid.UUID{correctTarget},
		}, "another memory related title", "another memory related title", nil)
		require.NoError(t, err)

		_, err = dispatchFunc(ctx, uuid.New())
		require.NoError(t, err)

		_, message, err = wsConn.Read(ctx)
		require.NoError(t, err)

		err = json.Unmarshal(message, &notif)
		require.NoError(t, err)

		require.Equal(t, 3, notif.UnreadCount)
		require.Equal(t, memberClient.ID, notif.Notification.UserID)
		require.Equal(t, "another memory related title", notif.Notification.Title)
	})
}

func TestInboxNotifications_List(t *testing.T) {
	t.Parallel()

	// I skip these tests specifically on windows as for now they are flaky - only on Windows.
	// For now the idea is that the runner takes too long to insert the entries, could be worth
	// investigating a manual Tx.
	// see: https://github.com/coder/internal/issues/503
	if runtime.GOOS == "windows" {
		t.Skip("our runners are randomly taking too long to insert entries")
	}

	t.Run("Failure Modes", func(t *testing.T) {
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
	})

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

				Content:   fmt.Sprintf("Content of the notif %d", i),
				CreatedAt: dbtime.Now(),
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

	t.Run("OK check icons", func(t *testing.T) {
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
					switch i {
					case 0:
						return notifications.TemplateWorkspaceCreated
					case 1:
						return notifications.TemplateWorkspaceMarkedForDeletion
					case 2:
						return notifications.TemplateUserAccountActivated
					case 3:
						return notifications.TemplateTemplateDeprecated
					case 4:
						return notifications.TemplateTestNotification
					default:
						return uuid.New()
					}
				}(),
				Title:   fmt.Sprintf("Notification %d", i),
				Actions: json.RawMessage("[]"),
				Icon: func() string {
					if i == 9 {
						return "https://dev.coder.com/icon.png"
					}

					return ""
				}(),
				Content:   fmt.Sprintf("Content of the notif %d", i),
				CreatedAt: dbtime.Now(),
			})
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 10, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 10)

		require.Equal(t, "https://dev.coder.com/icon.png", notifs.Notifications[0].Icon)
		require.Equal(t, codersdk.FallbackIconWorkspace, notifs.Notifications[9].Icon)
		require.Equal(t, codersdk.FallbackIconWorkspace, notifs.Notifications[8].Icon)
		require.Equal(t, codersdk.FallbackIconAccount, notifs.Notifications[7].Icon)
		require.Equal(t, codersdk.FallbackIconTemplate, notifs.Notifications[6].Icon)
		require.Equal(t, codersdk.FallbackIconOther, notifs.Notifications[5].Icon)
		require.Equal(t, codersdk.FallbackIconOther, notifs.Notifications[4].Icon)
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
		require.Equal(t, codersdk.FallbackIconWorkspace, notifs.Notifications[0].Icon)
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
	// see: https://github.com/coder/internal/issues/503
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

func TestInboxNotifications_MarkAllAsRead(t *testing.T) {
	t.Parallel()

	// I skip these tests specifically on windows as for now they are flaky - only on Windows.
	// For now the idea is that the runner takes too long to insert the entries, could be worth
	// investigating a manual Tx.
	// see: https://github.com/coder/internal/issues/503
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

		err = client.MarkAllInboxNotificationsAsRead(ctx)
		require.NoError(t, err)

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 20)

		for i := range 10 {
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
		require.Equal(t, 10, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 25)
	})
}
