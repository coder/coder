package authz

func AllActions() []Action {
	return []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}
}

// Action represents the allowed actions to be done on an object.
type Action string

const (
	ActionCreate = "create"
	ActionRead   = "read"
	ActionUpdate = "update"
	ActionDelete = "delete"
)
