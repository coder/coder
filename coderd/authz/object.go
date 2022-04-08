package authz

type Resource interface {
	ID() string
	ResourceType() ResourceType

	OwnerID() string
	OrgOwnerID() string
}

var _ Resource = (*zObject)(nil)

// zObject is used to create objects for authz checks when you have none in
// hand to run the check on.
// An example is if you want to list all workspaces, you can create a zObject
// that represents the set of workspaces you are trying to get access too.
// Do not export this type, as it can be created from a resource type constant.
type zObject struct {
	id       string
	owner    string
	orgOwner string

	// objectType is "workspace", "project", "devurl", etc
	objectType ResourceType
	// TODO: SharedUsers?
}

func (z zObject) ID() string {
	return z.id
}

func (z zObject) ResourceType() ResourceType {
	return z.objectType
}

func (z zObject) OwnerID() string {
	return z.owner
}

func (z zObject) OrgOwnerID() string {
	return z.orgOwner
}

// Org adds an org OwnerID to the resource
//nolint:revive
func (z zObject) Org(orgID string) zObject {
	z.orgOwner = orgID
	return z
}

// Owner adds an OwnerID to the resource
//nolint:revive
func (z zObject) Owner(id string) zObject {
	z.owner = id
	return z
}

//nolint:revive
func (z zObject) AsID(id string) zObject {
	z.id = id
	return z
}
