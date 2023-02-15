package audit

import (
	"reflect"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

// This mapping creates a relationship between an Auditable Resource
// and the Audit Actions we track for that resource.
// It is important to maintain this mapping when adding a new Auditable Resource to the
// AuditableResources map (below) as our documentation - generated in scripts/auditdocgen/main.go -
// depends upon it.
var AuditActionMap = map[string][]codersdk.AuditAction{
	"GitSSHKey":       {codersdk.AuditActionCreate},
	"Template":        {codersdk.AuditActionWrite, codersdk.AuditActionDelete},
	"TemplateVersion": {codersdk.AuditActionCreate, codersdk.AuditActionWrite},
	"User":            {codersdk.AuditActionCreate, codersdk.AuditActionWrite, codersdk.AuditActionDelete},
	"Workspace":       {codersdk.AuditActionCreate, codersdk.AuditActionWrite, codersdk.AuditActionDelete},
	"WorkspaceBuild":  {codersdk.AuditActionStart, codersdk.AuditActionStop},
	"Group":           {codersdk.AuditActionCreate, codersdk.AuditActionWrite, codersdk.AuditActionDelete},
	"APIKey":          {codersdk.AuditActionWrite},
	"License":         {codersdk.AuditActionCreate, codersdk.AuditActionDelete},
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
	&database.GitSSHKey{}: {
		"user_id":     ActionTrack,
		"created_at":  ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":  ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"private_key": ActionSecret, // We don't want to expose private keys in diffs.
		"public_key":  ActionTrack,  // Public keys are ok to expose in a diff.
	},
	&database.Template{}: {
		"id":                               ActionTrack,
		"created_at":                       ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":                       ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"organization_id":                  ActionIgnore, /// Never changes.
		"deleted":                          ActionIgnore, // Changes, but is implicit when a delete event is fired.
		"name":                             ActionTrack,
		"display_name":                     ActionTrack,
		"provisioner":                      ActionTrack,
		"active_version_id":                ActionTrack,
		"description":                      ActionTrack,
		"icon":                             ActionTrack,
		"default_ttl":                      ActionTrack,
		"min_autostart_interval":           ActionTrack,
		"created_by":                       ActionTrack,
		"is_private":                       ActionTrack,
		"group_acl":                        ActionTrack,
		"user_acl":                         ActionTrack,
		"allow_user_cancel_workspace_jobs": ActionTrack,
	},
	&database.TemplateVersion{}: {
		"id":              ActionTrack,
		"template_id":     ActionTrack,
		"organization_id": ActionIgnore, // Never changes.
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
		"last_seen_at":    ActionIgnore,
		"deleted":         ActionTrack,
	},
	&database.Workspace{}: {
		"id":                 ActionTrack,
		"created_at":         ActionIgnore, // Never changes.
		"updated_at":         ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"owner_id":           ActionTrack,
		"organization_id":    ActionIgnore, // Never changes.
		"template_id":        ActionTrack,
		"deleted":            ActionIgnore, // Changes, but is implicit when a delete event is fired.
		"name":               ActionTrack,
		"autostart_schedule": ActionTrack,
		"ttl":                ActionTrack,
		"last_used_at":       ActionIgnore,
	},
	&database.WorkspaceBuild{}: {
		"id":                  ActionIgnore,
		"created_at":          ActionIgnore,
		"updated_at":          ActionIgnore,
		"workspace_id":        ActionIgnore,
		"template_version_id": ActionTrack,
		"build_number":        ActionIgnore,
		"transition":          ActionIgnore,
		"initiator_id":        ActionIgnore,
		"provisioner_state":   ActionIgnore,
		"job_id":              ActionIgnore,
		"deadline":            ActionIgnore,
		"reason":              ActionIgnore,
		"daily_cost":          ActionIgnore,
	},
	&database.AuditableGroup{}: {
		"id":              ActionTrack,
		"name":            ActionTrack,
		"organization_id": ActionIgnore, // Never changes.
		"avatar_url":      ActionTrack,
		"quota_allowance": ActionTrack,
		"members":         ActionTrack,
	},
	// We don't show any diff for the APIKey resource
	&database.APIKey{}: {
		"id":               ActionIgnore,
		"hashed_secret":    ActionIgnore,
		"user_id":          ActionIgnore,
		"last_used":        ActionIgnore,
		"expires_at":       ActionIgnore,
		"created_at":       ActionIgnore,
		"updated_at":       ActionIgnore,
		"login_type":       ActionIgnore,
		"lifetime_seconds": ActionIgnore,
		"ip_address":       ActionIgnore,
		"scope":            ActionIgnore,
	},
	// TODO: track an ID here when the below ticket is completed:
	// https://github.com/coder/coder/pull/6012
	&database.License{}: {
		"id":          ActionIgnore,
		"uploaded_at": ActionTrack,
		"jwt":         ActionIgnore,
		"exp":         ActionTrack,
		"uuid":        ActionTrack,
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
