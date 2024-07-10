package types

// MessagePayload describes the JSON payload to be stored alongside the notification message, which specifies all of its
// metadata, labels, and routing information.
//
// Any BC-incompatible changes must bump the version, and special handling must be put in place to unmarshal multiple versions.
type MessagePayload struct {
	Version string `json:"_version"`

	NotificationName string `json:"notification_name"`
	CreatedBy        string `json:"created_by"`

	UserID    string `json:"user_id"`
	UserEmail string `json:"user_email"`
	UserName  string `json:"user_name"`

	Actions []TemplateAction  `json:"actions"`
	Labels  map[string]string `json:"labels"`
}
