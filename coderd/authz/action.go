package authz

// Action represents the allowed actions to be done on an object.
type Action string

const (
	ReadAction   = "read"
	WriteAction  = "write"
	ModifyAction = "modify"
	DeleteAction = "delete"
)
