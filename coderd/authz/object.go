package authz

type Object interface {
	ID() string
	ResourceType() ResourceType
}

type UserObject interface {
	Object
	OwnerID() string
}

type OrgObject interface {
	Object
	OrgOwnerID() string
}

// ZObject is the resource being accessed
type ZObject struct {
	ObjectID string `json:"object_id"`
	Owner    string `json:"owner_id"`
	OrgOwner string `json:"org_owner_id"`

	// ObjectType is "workspace", "project", "devurl", etc
	ObjectType ResourceType `json:"object_type"`
	// TODO: SharedUsers?
}

var _ Object = (*ZObject)(nil)

func (z ZObject) ID() string {
	return z.ObjectID
}

func (z ZObject) ResourceType() ResourceType {
	return z.ObjectType
}

func (z ZObject) OwnerID() string {
	return z.Owner
}

func (z ZObject) OrgOwnerID() string {
	return z.OrgOwner
}
