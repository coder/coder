package codersdk

import (
	"time"

	"github.com/google/uuid"
)

type GetInboxNotificationResponse struct {
	Notification InboxNotification `json:"notification"`
	UnreadCount  int               `json:"unread_count"`
}

type ListInboxNotificationsResponse struct {
	Notifications []InboxNotification `json:"notifications"`
	UnreadCount   int                 `json:"unread_count"`
}

type InboxNotification struct {
	ID         uuid.UUID                 `json:"id" format:"uuid"`
	UserID     uuid.UUID                 `json:"user_id" format:"uuid"`
	TemplateID uuid.UUID                 `json:"template_id" format:"uuid"`
	Targets    []uuid.UUID               `json:"targets" format:"uuid"`
	Title      string                    `json:"title"`
	Content    string                    `json:"content"`
	Icon       string                    `json:"icon"`
	Actions    []InboxNotificationAction `json:"actions"`
	ReadAt     *time.Time                `json:"read_at"`
	CreatedAt  time.Time                 `json:"created_at" format:"date-time"`
}

type InboxNotificationAction struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type UpdateInboxNotificationReadStatusRequest struct {
	IsRead bool `json:"is_read"`
}

type UpdateInboxNotificationReadStatusResponse struct {
	Notification InboxNotification `json:"notification"`
	UnreadCount  int               `json:"unread_count"`
}
