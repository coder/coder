package authz

type Resource interface {
	ID() string
	ResourceType() ResourceType
}

type UserObject interface {
	Resource
	OwnerID() string
}

type OrgObject interface {
	Resource
	OrgOwnerID() string
}

// zObject is used to create objects for authz checks when you have none in
// hand to run the check on.
// An example is if you want to list all workspaces, you can create a zObject
// that represents the set of workspaces you are trying to get access too.
type zObject struct {
	ObjectID   string `json:"object_id"`
	OwnedBy    string `json:"owner_id"`
	OwnedByOrg string `json:"org_owner_id"`

	// ObjectType is "workspace", "project", "devurl", etc
	ObjectType ResourceType `json:"object_type"`
	// TODO: SharedUsers?
}

func (z zObject) ID() string {
	return z.ObjectID
}

func (z zObject) ResourceType() ResourceType {
	return z.ObjectType
}

func (z zObject) OwnerID() string {
	return z.OwnedBy
}

func (z zObject) OrgOwnerID() string {
	return z.OwnedByOrg
}

// Org adds an org OwnerID to the resource
func (z zObject) Org(orgID string) zObject {
	z.OwnedByOrg = orgID
	return z
}

// Owner adds an OwnerID to the resource
func (z zObject) Owner(id string) zObject {
	z.OwnedBy = id
	return z
}

func (z zObject) AsID(id string) zObject {
	z.ObjectID = id
	return z
}
