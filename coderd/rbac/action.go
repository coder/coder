package rbac

// Action represents the allowed actions to be done on an object.
type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// AllActions is a helper function to return all the possible actions types.
func AllActions() []Action {
	return []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}
}
