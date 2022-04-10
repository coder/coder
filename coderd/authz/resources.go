package authz

// ResourceType defines the list of available resources for authz.
type ResourceType string

const (
	ResourceWorkspace ResourceType = "workspace"
	ResourceTemplate  ResourceType = "template"
	ResourceDevURL    ResourceType = "devurl"
	ResourceUser      ResourceType = "user"
	ResourceAuditLogs ResourceType = "audit-logs"
)

func (z ResourceType) All() Object {
	return Object{
		ObjectType: z,
	}
}

// SetOrg adds an org OwnerID to the resource
//nolint:revive
func (r ResourceType) SetOrg(orgID string) Object {
	return Object{
		OrgOwner:   orgID,
		ObjectType: r,
	}
}

// SetOwner adds an OwnerID to the resource
//nolint:revive
func (r ResourceType) SetOwner(id string) Object {
	return Object{
		Owner:      id,
		ObjectType: r,
	}
}

// SetID adds a resource ID to the resource
//nolint:revive
func (r ResourceType) SetID(id string) Object {
	return Object{
		ID:         id,
		ObjectType: r,
	}
}
