package audit

import (
	"fmt"
	"os"
	"reflect"
	"runtime"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
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
	"APIKey":          {codersdk.AuditActionLogin, codersdk.AuditActionLogout, codersdk.AuditActionRegister, codersdk.AuditActionCreate, codersdk.AuditActionDelete},
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
// which fields are auditable. All resource types must be valid audit.Auditable
// types.
var AuditableResources = auditMap(auditableResourcesTypes)

var auditableResourcesTypes = map[any]map[string]Action{
	&database.GitSSHKey{}: {
		"user_id":     ActionTrack,
		"created_at":  ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":  ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"private_key": ActionSecret, // We don't want to expose private keys in diffs.
		"public_key":  ActionTrack,  // Public keys are ok to expose in a diff.
	},
	&database.Template{}: {
		"id":                                ActionTrack,
		"created_at":                        ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":                        ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"organization_id":                   ActionIgnore, /// Never changes.
		"deleted":                           ActionIgnore, // Changes, but is implicit when a delete event is fired.
		"name":                              ActionTrack,
		"display_name":                      ActionTrack,
		"provisioner":                       ActionTrack,
		"active_version_id":                 ActionTrack,
		"description":                       ActionTrack,
		"icon":                              ActionTrack,
		"default_ttl":                       ActionTrack,
		"max_ttl":                           ActionTrack,
		"autostart_block_days_of_week":      ActionTrack,
		"autostop_requirement_days_of_week": ActionTrack,
		"autostop_requirement_weeks":        ActionTrack,
		"created_by":                        ActionTrack,
		"created_by_username":               ActionIgnore,
		"created_by_avatar_url":             ActionIgnore,
		"group_acl":                         ActionTrack,
		"user_acl":                          ActionTrack,
		"allow_user_autostart":              ActionTrack,
		"allow_user_autostop":               ActionTrack,
		"allow_user_cancel_workspace_jobs":  ActionTrack,
		"failure_ttl":                       ActionTrack,
		"time_til_dormant":                  ActionTrack,
		"time_til_dormant_autodelete":       ActionTrack,
		"require_active_version":            ActionTrack,
		"deprecated":                        ActionTrack,
	},
	&database.TemplateVersion{}: {
		"id":                      ActionTrack,
		"template_id":             ActionTrack,
		"organization_id":         ActionIgnore, // Never changes.
		"created_at":              ActionIgnore, // Never changes, but is implicit and not helpful in a diff.
		"updated_at":              ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"name":                    ActionTrack,
		"message":                 ActionIgnore, // Never changes after creation.
		"readme":                  ActionTrack,
		"job_id":                  ActionIgnore, // Not helpful in a diff because jobs aren't tracked in audit logs.
		"created_by":              ActionTrack,
		"external_auth_providers": ActionIgnore, // Not helpful because this can only change when new versions are added.
		"created_by_avatar_url":   ActionIgnore,
		"created_by_username":     ActionIgnore,
		"archived":                ActionTrack,
	},
	&database.User{}: {
		"id":                   ActionTrack,
		"email":                ActionTrack,
		"username":             ActionTrack,
		"hashed_password":      ActionSecret, // Do not expose a users hashed password.
		"created_at":           ActionIgnore, // Never changes.
		"updated_at":           ActionIgnore, // Changes, but is implicit and not helpful in a diff.
		"status":               ActionTrack,
		"rbac_roles":           ActionTrack,
		"login_type":           ActionTrack,
		"avatar_url":           ActionIgnore,
		"last_seen_at":         ActionIgnore,
		"deleted":              ActionTrack,
		"quiet_hours_schedule": ActionTrack,
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
		"dormant_at":         ActionTrack,
		"deleting_at":        ActionTrack,
		"automatic_updates":  ActionTrack,
	},
	&database.WorkspaceBuild{}: {
		"id":                      ActionIgnore,
		"created_at":              ActionIgnore,
		"updated_at":              ActionIgnore,
		"workspace_id":            ActionIgnore,
		"template_version_id":     ActionTrack,
		"build_number":            ActionIgnore,
		"transition":              ActionIgnore,
		"initiator_id":            ActionIgnore,
		"provisioner_state":       ActionIgnore,
		"job_id":                  ActionIgnore,
		"deadline":                ActionIgnore,
		"reason":                  ActionIgnore,
		"daily_cost":              ActionIgnore,
		"max_deadline":            ActionIgnore,
		"initiator_by_avatar_url": ActionIgnore,
		"initiator_by_username":   ActionIgnore,
	},
	&database.AuditableGroup{}: {
		"id":              ActionTrack,
		"name":            ActionTrack,
		"display_name":    ActionTrack,
		"organization_id": ActionIgnore, // Never changes.
		"avatar_url":      ActionTrack,
		"quota_allowance": ActionTrack,
		"members":         ActionTrack,
		"source":          ActionIgnore,
	},
	&database.APIKey{}: {
		"id":               ActionIgnore,
		"hashed_secret":    ActionIgnore,
		"user_id":          ActionTrack,
		"last_used":        ActionTrack,
		"expires_at":       ActionTrack,
		"created_at":       ActionTrack,
		"updated_at":       ActionIgnore,
		"login_type":       ActionIgnore,
		"lifetime_seconds": ActionIgnore,
		"ip_address":       ActionIgnore,
		"scope":            ActionIgnore,
		"token_name":       ActionIgnore,
	},
	&database.AuditOAuthConvertState{}: {
		"created_at":      ActionTrack,
		"expires_at":      ActionTrack,
		"from_login_type": ActionTrack,
		"to_login_type":   ActionTrack,
		"user_id":         ActionTrack,
	},
	&database.HealthSettings{}: {
		"id":                     ActionIgnore,
		"dismissed_healthchecks": ActionTrack,
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
	&database.WorkspaceProxy{}: {
		"id":                  ActionTrack,
		"name":                ActionTrack,
		"display_name":        ActionTrack,
		"icon":                ActionTrack,
		"url":                 ActionTrack,
		"wildcard_hostname":   ActionTrack,
		"created_at":          ActionTrack,
		"updated_at":          ActionIgnore,
		"deleted":             ActionIgnore,
		"token_hashed_secret": ActionSecret,
		"derp_enabled":        ActionTrack,
		"derp_only":           ActionTrack,
		"region_id":           ActionTrack,
		"version":             ActionTrack,
	},
}

// auditMap converts a map of struct pointers to a map of struct names as
// strings. It's a convenience wrapper so that structs can be passed in by value
// instead of manually typing struct names as strings.
func auditMap(m map[any]map[string]Action) Table {
	out := make(Table, len(m))

	for k, v := range m {
		tableKey, tableValue := entry(k, v)
		out[tableKey] = tableValue
	}

	return out
}

// entry is a helper function that checks the json tags to make sure all fields
// are tracked. And no excess fields are tracked.
func entry(v any, f map[string]Action) (string, map[string]Action) {
	vt := reflect.TypeOf(v)
	for vt.Kind() == reflect.Ptr {
		vt = vt.Elem()
	}

	// This should never happen because audit.Audible only allows structs in
	// its union.
	if vt.Kind() != reflect.Struct {
		panic(fmt.Sprintf("audit table entry value must be a struct, got %T", v))
	}

	name := structName(vt)

	// Use the flattenStructFields to recurse anonymously embedded structs
	vv := reflect.ValueOf(v)
	diffs, err := flattenStructFields(vv, vv)
	if err != nil {
		panic(fmt.Sprintf("audit table entry type %T failed to flatten", v))
	}

	fcpy := make(map[string]Action, len(f))
	for k, v := range f {
		fcpy[k] = v
	}
	for _, d := range diffs {
		jsonTag := d.FieldType.Tag.Get("json")
		if jsonTag == "-" {
			// This field is explicitly ignored.
			continue
		}
		if _, ok := fcpy[jsonTag]; !ok {
			_, _ = fmt.Fprintf(os.Stderr, "ERROR: Audit table entry missing action for field %q in type %q\nPlease update the auditable resource types in: %s\n", d.FieldType.Name, name, self())
			//nolint:revive
			os.Exit(1)
		}
		delete(fcpy, jsonTag)
	}

	// If there are any fields left in fcpy, they are extra fields that don't
	// exist in the struct. Don't track them.
	if len(fcpy) > 0 {
		panic(fmt.Sprintf("audit table entry has extra actions for type %q: %v", name, fcpy))
	}

	return structName(vt), f
}

func (t Action) String() string {
	return string(t)
}

func self() string {
	//nolint:dogsled
	_, file, _, _ := runtime.Caller(1)
	return file
}
