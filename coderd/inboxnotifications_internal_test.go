package coderd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
)

func TestInboxNotifications_SetInboxNotificationIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		icon         string
		templateID   uuid.UUID
		expectedIcon string
	}{
		{"WorkspaceCreated", "", notifications.TemplateWorkspaceCreated, codersdk.FallbackIconWorkspace},
		{"UserAccountCreated", "", notifications.TemplateUserAccountCreated, codersdk.FallbackIconAccount},
		{"TemplateDeleted", "", notifications.TemplateTemplateDeleted, codersdk.FallbackIconTemplate},
		{"TestNotification", "", notifications.TemplateTestNotification, codersdk.FallbackIconOther},
		{"TestExistingIcon", "https://cdn.coder.com/icon_notif.png", notifications.TemplateTemplateDeleted, "https://cdn.coder.com/icon_notif.png"},
		{"UnknownTemplate", "", uuid.New(), codersdk.FallbackIconOther},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			notif := codersdk.InboxNotification{
				ID:         uuid.New(),
				UserID:     uuid.New(),
				TemplateID: tt.templateID,
				Title:      "notification title",
				Content:    "notification content",
				Icon:       tt.icon,
				CreatedAt:  time.Now(),
			}

			notif = ensureNotificationIcon(notif)
			require.Equal(t, tt.expectedIcon, notif.Icon)
		})
	}
}
