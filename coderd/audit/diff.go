package audit

import (
	"github.com/coder/coder/coderd/database"
)

// Auditable is mostly a marker interface. It contains a definitive list of all
// auditable types. If you want to audit a new type, first define it in
// AuditableResources, then add it to this interface.
type Auditable interface {
	database.APIKey |
		database.Organization |
		database.OrganizationMember |
		database.Template |
		database.TemplateVersion |
		database.User |
		database.Workspace |
		database.GitSSHKey
}

// Map is a map of changed fields in an audited resource. `any` can be a
// map[string]any in the case of nested structs, or an OldNew struct
// representing a changed value.
type Map map[string]any

// OldNew is a pair of values representing the old value and the new value.
type OldNew struct {
	Old any
	New any
}

// Empty returns a default value of type T.
func Empty[T Auditable]() T {
	var t T
	return t
}

// Diff compares two auditable resources and produces a Map of the changed
// values.
func Diff[T Auditable](a Auditor, left, right T) Map { return a.diff(left, right) }
