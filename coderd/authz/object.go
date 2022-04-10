package authz

//type Resource interface {
//	ID() string
//	ResourceType() ResourceType
//
//	OwnerID() string
//	OrgOwnerID() string
//}

//var _ Resource = (*Object)(nil)

// Object is used to create objects for authz checks when you have none in
// hand to run the check on.
// An example is if you want to list all workspaces, you can create a Object
// that represents the set of workspaces you are trying to get access too.
// Do not export this type, as it can be created from a resource type constant.
type Object struct {
	ID       string
	Owner    string
	OrgOwner string

	// ObjectType is "workspace", "project", "devurl", etc
	ObjectType ResourceType
	// TODO: SharedUsers?
}

// SetOrg adds an org OwnerID to the resource
//nolint:revive
func (z Object) SetOrg(orgID string) Object {
	z.OrgOwner = orgID
	return z
}

// SetOwner adds an OwnerID to the resource
//nolint:revive
func (z Object) SetOwner(id string) Object {
	z.Owner = id
	return z
}

//nolint:revive
func (z Object) SetID(id string) Object {
	z.ID = id
	return z
}
