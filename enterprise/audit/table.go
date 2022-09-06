package audit

import (
	"reflect"

	"github.com/coder/coder/coderd/database"
)

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
	&database.GitSSHKey{}: {
		"user_id":     ActionTrack,
		"created_at":  ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":  ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"private_key": ActionSecret, // We don't want to expose private keys in diffs.
		"public_key":  ActionTrack,  // Public keys are ok to expose in a diff.
	},
	&database.OrganizationMember{}: {
		"user_id":         ActionTrack,
		"organization_id": ActionTrack,
		"created_at":      ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":      ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"roles":           ActionTrack,
	},
	&database.Organization{}: {
		"id":          ActionTrack,
		"name":        ActionTrack,
		"description": ActionTrack,
		"created_at":  ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":  ActionIgnore, // Changes, but is implicit and not helpful in a diff.
	},
	&database.Template{}: {
		"id":                     ActionTrack,
		"created_at":             ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":             ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"organization_id":        ActionTrack,
		"deleted":                ActionIgnore, // Changes, but is implicit when a delete event is fired.
		"name":                   ActionTrack,
		"provisioner":            ActionTrack,
		"active_version_id":      ActionTrack,
		"description":            ActionTrack,
		"icon":                   ActionTrack,
		"max_ttl":                ActionTrack,
		"min_autostart_interval": ActionTrack,
		"created_by":             ActionTrack,
	},
	&database.TemplateVersion{}: {
		"id":              ActionTrack,
		"template_id":     ActionTrack,
		"organization_id": ActionTrack,
		"created_at":      ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":      ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"name":            ActionTrack,
		"readme":          ActionTrack,
		"job_id":          ActionIgnore, // Not helpful in a diff because jobs aren't tracked in audit logs.
		"created_by":      ActionTrack,
	},
	&database.User{}: {
		"id":              ActionTrack,
		"email":           ActionTrack,
		"username":        ActionTrack,
		"hashed_password": ActionSecret, // Do not expose a users hashed password.
		"created_at":      ActionIgnore, // Never changes.
		"updated_at":      ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"status":          ActionTrack,
		"rbac_roles":      ActionTrack,
		"login_type":      ActionIgnore,
		"avatar_url":      ActionIgnore,
	},
	&database.Workspace{}: {
		"id":                 ActionTrack,
		"created_at":         ActionIgnore, // Never changes.
		"updated_at":         ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"owner_id":           ActionTrack,
		"organization_id":    ActionTrack,
		"template_id":        ActionTrack,
		"deleted":            ActionIgnore, // Changes, but is implicit when a delete event is fired.
		"name":               ActionTrack,
		"autostart_schedule": ActionTrack,
		"ttl":                ActionTrack,
		"last_used_at":       ActionIgnore,
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
