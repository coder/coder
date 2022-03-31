package authztest

// PermissionSet defines a set of permissions with the same impact.
type PermissionSet string

const (
	SetPositive PermissionSet = "j"
	SetNegative PermissionSet = "j!"
	SetNeutral  PermissionSet = "a"
)

var (
	PermissionSets = []PermissionSet{SetPositive, SetNegative, SetNeutral}
)

var nilSet = Set{nil}

// *.*.*.*
//var PermissionSetWPlus = NewRole(
//	all.Wildcard().Positive(),
//	union(all.Wildcard().Abstain(), nilSet),
//
//	union(all.Site().Positive(), nilSet),
//	union(all.Site().Negative(), nilSet),
//	union(all.Site().Abstain(), nilSet),
//
//	union(all.AllOrgs().Positive(), nilSet),
//	union(all.AllOrgs().Negative(), nilSet),
//	union(all.AllOrgs().Abstain(), nilSet),
//
//	union(all.User().Positive(), nilSet),
//	union(all.User().Negative(), nilSet),
//	union(all.User().Abstain(), nilSet),
//)
