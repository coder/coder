package authz

// Object is the resource being accessed
type Object struct {
	ObjectID   string `json:"object_id"`
	OwnerID    string `json:"owner_id"`
	OrgOwnerID string `json:"org_owner_id"`

	// ObjectType is "workspace", "project", "devurl", etc
	ObjectType ResourceType `json:"object_type"`
	// TODO: SharedUsers?
}
