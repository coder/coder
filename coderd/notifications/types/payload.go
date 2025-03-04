package types

import "github.com/google/uuid"

// MessagePayload describes the JSON payload to be stored alongside the notification message, which specifies all of its
// metadata, labels, and routing information.
//
// Any BC-incompatible changes must bump the version, and special handling must be put in place to unmarshal multiple versions.
type MessagePayload struct {
	Version string `json:"_version"`

	NotificationName       string `json:"notification_name"`
	NotificationTemplateID string `json:"notification_template_id"`

	UserID       string `json:"user_id"`
	UserEmail    string `json:"user_email"`
	UserName     string `json:"user_name"`
	UserUsername string `json:"user_username"`

	Actions []TemplateAction  `json:"actions"`
	Labels  map[string]string `json:"labels"`
	Data    map[string]any    `json:"data"`
	Targets []uuid.UUID       `json:"targets"`
}
