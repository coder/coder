package authz

type Action string

const (
	ReadAction   = "read"
	WriteAction  = "write"
	ModifyAction = "modify"
	DeleteAction = "delete"
)
