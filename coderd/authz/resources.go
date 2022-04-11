package authz

// ResourceType defines the list of available resources for authz.
type ResourceType string

const (
	ResourceWorkspace ResourceType = "workspace"
	ResourceTemplate  ResourceType = "template"
	ResourceUser      ResourceType = "user"
)

func (z ResourceType) All() Object {
	return Object{
		ObjectType: z,
	}
}

// InOrg adds an org OwnerID to the resource
//nolint:revive
func (r ResourceType) InOrg(orgID string) Object {
	return Object{
		OrgID:      orgID,
		ObjectType: r,
	}
}

// WithOwner adds an OwnerID to the resource
//nolint:revive
func (r ResourceType) WithOwner(id string) Object {
	return Object{
		Owner:      id,
		ObjectType: r,
	}
}

// WithID adds a resource ID to the resource
//nolint:revive
func (r ResourceType) WithID(id string) Object {
	return Object{
		ID:         id,
		ObjectType: r,
	}
}
