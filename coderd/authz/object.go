package authz

// Object is used to create objects for authz checks when you have none in
// hand to run the check on.
// An example is if you want to list all workspaces, you can create a Object
// that represents the set of workspaces you are trying to get access too.
// Do not export this type, as it can be created from a resource type constant.
type Object struct {
	ResourceID string `json:"id"`
	Owner      string `json:"owner"`
	// OrgID specifies which org the object is a part of.
	OrgID string `json:"org_owner"`

	// Type is "workspace", "project", "devurl", etc
	Type string `json:"type"`
	// TODO: SharedUsers?
}

func (z Object) All() Object {
	z.OrgID = ""
	z.Owner = ""
	z.ResourceID = ""
	return z
}

// InOrg adds an org OwnerID to the resource
//nolint:revive
func (z Object) InOrg(orgID string) Object {
	z.OrgID = orgID
	return z
}

// WithOwner adds an OwnerID to the resource
//nolint:revive
func (z Object) WithOwner(id string) Object {
	z.Owner = id
	return z
}

//nolint:revive
func (z Object) WithID(id string) Object {
	z.ResourceID = id
	return z
}
