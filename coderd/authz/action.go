package authz

type Action string

const (
	ActionRead   = "read"
	ActionWrite  = "write"
	ActionModify = "modify"
	ActionDelete = "delete"
)
