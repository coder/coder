package testdata

type permissionSet string

const (
	SetPositive permissionSet = "j"
	SetNegative permissionSet = "j!"
	SetNeutral  permissionSet = "a"
)

var (
	PermissionSets = []permissionSet{SetPositive, SetNegative, SetNeutral}
)
