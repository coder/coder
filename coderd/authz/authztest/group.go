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
//	Union(all.Wildcard().Abstain(), nilSet),
//
//	Union(all.Site().Positive(), nilSet),
//	Union(all.Site().Negative(), nilSet),
//	Union(all.Site().Abstain(), nilSet),
//
//	Union(all.AllOrgs().Positive(), nilSet),
//	Union(all.AllOrgs().Negative(), nilSet),
//	Union(all.AllOrgs().Abstain(), nilSet),
//
//	Union(all.User().Positive(), nilSet),
//	Union(all.User().Negative(), nilSet),
//	Union(all.User().Abstain(), nilSet),
//)
