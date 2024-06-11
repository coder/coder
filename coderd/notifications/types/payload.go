package types

type MessagePayload struct {
	Version string `json:"_version"`

	NotificationName string `json:"notification_name"`
	CreatedBy        string `json:"created_by"`

	UserID    string `json:"user_id"`
	UserEmail string `json:"user_email"`
	UserName  string `json:"user_name"`

	Actions []TemplateAction `json:"actions"`
	Labels  Labels           `json:"labels"`
}
