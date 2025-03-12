package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestInboxNotifications_List(t *testing.T) {
	t.Parallel()

	t.Run("OK empty", func(t *testing.T) {
		t.Parallel()

		client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)

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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		// nolint:gocritic // used only to seed database
		notifierCtx := dbauthz.AsNotifier(ctx)

		for i := range 60 {
			_, err = api.Database.InsertInboxNotification(notifierCtx, database.InsertInboxNotificationParams{
				ID:         uuid.New(),
				UserID:     firstUser.UserID,
				TemplateID: notifications.TemplateWorkspaceOutOfMemory,
				Title:      fmt.Sprintf("Notification %d", i),
				Actions:    json.RawMessage("[]"),
				Content:    fmt.Sprintf("Content of the notif %d", i),
				CreatedAt:  dbtime.Now(),
			})
			require.NoError(t, err)
		}

		time.Sleep(1 * time.Second)

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 60, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 25)

		require.Equal(t, "Notification 59", notifs.Notifications[0].Title)

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			StartingBefore: notifs.Notifications[24].ID,
		})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 60, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 25)

		require.Equal(t, "Notification 34", notifs.Notifications[0].Title)
		require.Equal(t, "Notification 10", notifs.Notifications[24].Title)

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			StartingBefore: notifs.Notifications[24].ID,
		})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 60, notifs.UnreadCount)
		require.Len(t, notifs.Notifications, 10)

		require.Equal(t, "Notification 9", notifs.Notifications[0].Title)
	})

	t.Run("OK with template filter", func(t *testing.T) {
		t.Parallel()

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		// nolint:gocritic // used only to seed database
		notifierCtx := dbauthz.AsNotifier(ctx)

		for i := range 10 {
			_, err = api.Database.InsertInboxNotification(notifierCtx, database.InsertInboxNotificationParams{
				ID:     uuid.New(),
				UserID: firstUser.UserID,
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
			require.NoError(t, err)
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			Templates: []uuid.UUID{notifications.TemplateWorkspaceOutOfMemory},
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		// nolint:gocritic // used only to seed database
		notifierCtx := dbauthz.AsNotifier(ctx)

		filteredTarget := uuid.New()

		for i := range 10 {
			_, err = api.Database.InsertInboxNotification(notifierCtx, database.InsertInboxNotificationParams{
				ID:         uuid.New(),
				UserID:     firstUser.UserID,
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
			require.NoError(t, err)
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			Targets: []uuid.UUID{filteredTarget},
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
		require.NoError(t, err)
		require.NotNil(t, notifs)
		require.Equal(t, 0, notifs.UnreadCount)
		require.Empty(t, notifs.Notifications)

		// nolint:gocritic // used only to seed database
		notifierCtx := dbauthz.AsNotifier(ctx)

		filteredTarget := uuid.New()

		for i := range 10 {
			_, err = api.Database.InsertInboxNotification(notifierCtx, database.InsertInboxNotificationParams{
				ID:     uuid.New(),
				UserID: firstUser.UserID,
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
			require.NoError(t, err)
		}

		notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{
			Targets:   []uuid.UUID{filteredTarget},
			Templates: []uuid.UUID{notifications.TemplateWorkspaceOutOfDisk},
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

	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
	firstUser := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	notifs, err := client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
	require.NoError(t, err)
	require.NotNil(t, notifs)
	require.Equal(t, 0, notifs.UnreadCount)
	require.Empty(t, notifs.Notifications)

	// nolint:gocritic // used only to seed database
	notifierCtx := dbauthz.AsNotifier(ctx)

	for i := range 20 {
		_, err = api.Database.InsertInboxNotification(notifierCtx, database.InsertInboxNotificationParams{
			ID:         uuid.New(),
			UserID:     firstUser.UserID,
			TemplateID: notifications.TemplateWorkspaceOutOfMemory,
			Title:      fmt.Sprintf("Notification %d", i),
			Actions:    json.RawMessage("[]"),
			Content:    fmt.Sprintf("Content of the notif %d", i),
			CreatedAt:  dbtime.Now(),
		})
		require.NoError(t, err)
	}

	notifs, err = client.ListInboxNotifications(ctx, codersdk.ListInboxNotificationsRequest{})
	require.NoError(t, err)
	require.NotNil(t, notifs)
	require.Equal(t, 20, notifs.UnreadCount)
	require.Len(t, notifs.Notifications, 20)

	updatedNotif, err := client.UpdateInboxNotificationReadStatus(ctx, notifs.Notifications[19].ID, codersdk.UpdateInboxNotificationReadStatusRequest{
		IsRead: true,
	})
	require.NoError(t, err)
	require.NotNil(t, updatedNotif)
	require.NotZero(t, updatedNotif.Notification.ReadAt)
	require.Equal(t, 19, updatedNotif.UnreadCount)
}
