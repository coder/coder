package audit

import (
	"reflect"

	"github.com/coder/coder/coderd/database"
)

// Auditable is mostly a marker interface. It contains a definitive list of all
// auditable types. If you want to audit a new type, first define it in
// AuditableResources, then add it to this interface.
type Auditable interface {
	database.User |
		database.Workspace
}

type Action string

const (
	// ActionIgnore ignores diffing for the field.
	ActionIgnore = "ignore"
	// ActionTrack includes the value in the diff if the value changed.
	ActionTrack = "track"
	// ActionSecret includes a zero value of the same type if the value changed.
	// It lets you indicate that a value changed, but without leaking its
	// contents.
	ActionSecret = "secret"
)

// Table is a map of struct names to a map of field names that indicate that
// field's AuditType.
type Table map[string]map[string]Action

// AuditableResources contains a definitive list of all auditable resources and
// which fields are auditable.
var AuditableResources = auditMap(map[any]map[string]Action{
	&database.User{}: {
		"id":              ActionIgnore, // Never changes.
		"email":           ActionTrack,  // A user can edit their email.
		"username":        ActionIgnore, // A user cannot change their username.
		"hashed_password": ActionSecret, // A user can change their own password.
		"created_at":      ActionIgnore, // Never changes.
		"updated_at":      ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"status":          ActionTrack,  // A user can update another user status
		"rbac_roles":      ActionTrack,  // A user's roles are mutable
	},
	&database.Workspace{}: {
		"id":                 ActionIgnore, // Never changes.
		"created_at":         ActionIgnore, // Never changes.
		"updated_at":         ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"owner_id":           ActionIgnore, // We don't allow workspaces to change ownership.
		"template_id":        ActionIgnore, // We don't allow workspaces to change templates.
		"deleted":            ActionIgnore, // Changes, but is implicit when a delete event is fired.
		"name":               ActionIgnore, // We don't allow workspaces to change names.
		"autostart_schedule": ActionTrack,  // Autostart schedules are directly editable by users.
		"autostop_schedule":  ActionTrack,  // Autostart schedules are directly editable by users.
	},
})

// auditMap converts a map of struct pointers to a map of struct names as
// strings. It's a convenience wrapper so that structs can be passed in by value
// instead of manually typing struct names as strings.
func auditMap(m map[any]map[string]Action) Table {
	out := make(Table, len(m))

	for k, v := range m {
		out[structName(reflect.TypeOf(k).Elem())] = v
	}

	return out
}

func (t Action) String() string {
	return string(t)
}
