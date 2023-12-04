package audit

import (
	"github.com/coder/coder/v2/coderd/database"
)

// Auditable is mostly a marker interface. It contains a definitive list of all
// auditable types. If you want to audit a new type, first define it in
// AuditableResources, then add it to this interface.
type Auditable interface {
	database.APIKey |
		database.Template |
		database.TemplateVersion |
		database.User |
		database.Workspace |
		database.GitSSHKey |
		database.WorkspaceBuild |
		database.AuditableGroup |
		database.License |
		database.WorkspaceProxy |
		database.AuditOAuthConvertState |
		database.HealthSettings
}

// Map is a map of changed fields in an audited resource. It maps field names to
// the old and new value for that field.
type Map map[string]OldNew

// OldNew is a pair of values representing the old value and the new value.
type OldNew struct {
	Old    any
	New    any
	Secret bool
}

// Empty returns a default value of type T.
func Empty[T Auditable]() T {
	var t T
	return t
}

// Diff compares two auditable resources and produces a Map of the changed
// values.
func Diff[T Auditable](a Auditor, left, right T) Map { return a.diff(left, right) }

// Differ is used so the enterprise version can implement the diff function in
// the Auditor feature interface. Only types in the same package as the
// interface can implement unexported methods.
type Differ struct {
	DiffFn func(old, new any) Map
}

//nolint:unused
func (d Differ) diff(old, new any) Map {
	return d.DiffFn(old, new)
}
