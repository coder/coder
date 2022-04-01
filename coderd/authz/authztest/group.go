package authztest

// PermissionSet defines a set of permissions with the same impact.
type PermissionSet string

const (
	SetPositive PermissionSet = "j"
	SetNegative PermissionSet = "j!"
	SetNeutral  PermissionSet = "a"
)
