package coderd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/alerts"
	"github.com/coder/coder/v2/codersdk"
)

func TestInboxAlerts_ensureNotificationIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		icon         string
		templateID   uuid.UUID
		expectedIcon string
	}{
		{"WorkspaceCreated", "", alerts.TemplateWorkspaceCreated, codersdk.InboxAlertFallbackIconWorkspace},
		{"UserAccountCreated", "", alerts.TemplateUserAccountCreated, codersdk.InboxAlertFallbackIconAccount},
		{"TemplateDeleted", "", alerts.TemplateTemplateDeleted, codersdk.InboxAlertFallbackIconTemplate},
		{"TestNotification", "", alerts.TemplateTestNotification, codersdk.InboxAlertFallbackIconOther},
		{"TestExistingIcon", "https://cdn.coder.com/icon_notif.png", alerts.TemplateTemplateDeleted, "https://cdn.coder.com/icon_notif.png"},
		{"UnknownTemplate", "", uuid.New(), codersdk.InboxAlertFallbackIconOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			notif := codersdk.InboxAlert{
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
