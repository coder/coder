package authz

// ResourceType defines the list of available resources for authz.
type ResourceType string

const (
	ResourceWorkspace ResourceType = "workspace"
	ResourceProject   ResourceType = "project"
	ResourceDevURL    ResourceType = "devurl"
	ResourceUser      ResourceType = "user"
)

func (ResourceType) ID() string {
	return ""
}

func (t ResourceType) ResourceType() ResourceType {
	return t
}

// Org adds an org OwnerID to the resource
//nolint:revive
func (r ResourceType) Org(orgID string) zObject {
	return zObject{
		orgOwner:   orgID,
		objectType: r,
	}
}

// Owner adds an OwnerID to the resource
//nolint:revive
func (r ResourceType) Owner(id string) zObject {
	return zObject{
		owner:      id,
		objectType: r,
	}
}

//nolint:revive
func (r ResourceType) AsID(id string) zObject {
	return zObject{
		id:         id,
		objectType: r,
	}
}
