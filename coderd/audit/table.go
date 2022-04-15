package audit

import (
	"reflect"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

// Auditable is mostly a marker interface. It contains a definitive list of all
// auditable types. If you want to audit a new type, first define it in
// AuditableResources, then add it to this interface.
type Auditable interface {
	database.User |
		database.Workspace
}

type Action int

const (
	// ActionIgnore ignores diffing for the field.
	ActionIgnore = iota
	// ActionAuditable includes the value in the diff if the value changed.
	ActionAuditable
	// ActionSecret includes a zero value of the same type if the value changed.
	// It lets you indicate that a value changed, but without leaking its
	// contents.
	ActionSecret
)

// Map is a map of struct names to a map of field names that indicate that
// field's AuditType.
type Map map[string]map[string]Action

// AuditableResources contains a definitive list of all auditable resources and
// which fields are auditable.
var AuditableResources = auditMap(map[any]map[string]Action{
	&database.User{}: {
		"id":              ActionIgnore,    // Never changes.
		"email":           ActionAuditable, // A user can edit their email.
		"name":            ActionAuditable, // A user can edit their name.
		"revoked":         ActionAuditable, // An admin can revoke a user. This is different from deletion, which is implicit.
		"login_type":      ActionAuditable, // An admin can update the login type of a user.
		"hashed_password": ActionSecret,    // A user can change their own password.
		"created_at":      ActionIgnore,    // Never changes.
		"updated_at":      ActionIgnore,    // Changes, but is implicit and not helpful in a diff.
		"username":        ActionIgnore,    // A user cannot change their username.
	},
	&database.Workspace{}: {
		"id":                 ActionIgnore,    // Never changes.
		"created_at":         ActionIgnore,    // Never changes.
		"updated_at":         ActionIgnore,    // Changes, but is implicit and not helpful in a diff.
		"owner_id":           ActionIgnore,    // We don't allow workspaces to change ownership.
		"template_id":        ActionIgnore,    // We don't allow workspaces to change templates.
		"deleted":            ActionIgnore,    // Changes, but is implicit when a delete event is fired.
		"name":               ActionIgnore,    // We don't allow workspaces to change names.
		"autostart_schedule": ActionAuditable, // Autostart schedules are directly editable by users.
		"autostop_schedule":  ActionAuditable, // Autostart schedules are directly editable by users.
	},
})

// auditMap converts a map of pointers to a map of struct names as strings. It's
// a convenience wrapper so that structs can be passed in by value instead of
// manually typing struct names as strings.
func auditMap(m map[any]map[string]Action) Map {
	out := make(Map, len(m))

	for k, v := range m {
		out[reflect.TypeOf(k).Elem().Name()] = v
	}

	return out
}

func (t Action) String() string {
	switch t {
	case ActionIgnore:
		return "ignore"
	case ActionAuditable:
		return "auditable"
	case ActionSecret:
		return "secret"
	default:
		return "unknown"
	}
}

func (t Action) MarshalJSON() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *Action) UnmarshalJSON(b []byte) error {
	str := string(b)

	switch str {
	case "ignore":
		*t = ActionIgnore
	case "auditable":
		*t = ActionAuditable
	case "secret":
		*t = ActionSecret
	default:
		return xerrors.Errorf("unknown AuditType %q", str)
	}

	return nil
}
