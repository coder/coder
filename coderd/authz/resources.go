package authz

// ResourceType defines the list of available resources for authz.
type ResourceType string

const (
	ResourceWorkspace ResourceType = "workspace"
	ResourceProject   ResourceType = "project"
	ResourceDevURL    ResourceType = "devurl"
)

func (t ResourceType) ID() string {
	return ""
}

func (t ResourceType) ResourceType() ResourceType {
	return t
}

// Org adds an org OwnerID to the resource
func (r ResourceType) Org(orgID string) zObject {
	return zObject{
		OwnedByOrg: orgID,
		ObjectType: r,
	}
}

// Owner adds an OwnerID to the resource
func (r ResourceType) Owner(id string) zObject {
	return zObject{
		OwnedBy:    id,
		ObjectType: r,
	}
}

func (r ResourceType) AsID(id string) zObject {
	return zObject{
		ObjectID:   id,
		ObjectType: r,
	}
}
