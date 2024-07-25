package notifications

import "github.com/coder/coder/v2/coderd/database"

func ValidNotificationMethods() []string {
	return []string{
		string(database.NotificationMethodSmtp),
		string(database.NotificationMethodWebhook),
	}
}
