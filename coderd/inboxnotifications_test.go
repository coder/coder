package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	inboxNotificationsPageSize = 25
)

func TestInboxNotifications_List(t *testing.T) {
	t.Parallel()

	// I skip these tests specifically on windows as for now they are flaky - only on Windows.
	// For now the idea is that the runner takes too long to insert the entries, could be worth
	// investigating a manual Tx.
	if runtime.GOOS == "windows" {
		t.Skip("our runners are randomly taking too long to insert entries")
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
			StartingBefore: notifs.Notifications[inboxNotificationsPageSize-1].ID,
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

	updatedNotif, err := client.UpdateInboxNotificationReadStatus(ctx, notifs.Notifications[19].ID, codersdk.UpdateInboxNotificationReadStatusRequest{
		IsRead: true,
	})
	require.NoError(t, err)
	require.NotNil(t, updatedNotif)
	require.NotZero(t, updatedNotif.Notification.ReadAt)
	require.Equal(t, 19, updatedNotif.UnreadCount)
}
