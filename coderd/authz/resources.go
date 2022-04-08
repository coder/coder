package authz

// ResourceType defines the list of available resources for authz.
type ResourceType string

const (
	ResourceWorkspace ResourceType = "workspace"
	ResourceProject   ResourceType = "project"
	ResourceDevURL    ResourceType = "devurl"
	ResourceUser      ResourceType = "user"
	ResourceAuditLogs ResourceType = "audit-logs"
)

func (ResourceType) ID() string {
	return ""
}

func (t ResourceType) ResourceType() ResourceType {
	return t
}

func (ResourceType) OwnerID() string    { return "" }
func (ResourceType) OrgOwnerID() string { return "" }

// SetOrg adds an org OwnerID to the resource
//nolint:revive
func (r ResourceType) SetOrg(orgID string) zObject {
	return zObject{
		orgOwner:   orgID,
		objectType: r,
	}
}

// SetOwner adds an OwnerID to the resource
//nolint:revive
func (r ResourceType) SetOwner(id string) zObject {
	return zObject{
		owner:      id,
		objectType: r,
	}
}

// SetID adds a resource ID to the resource
//nolint:revive
func (r ResourceType) SetID(id string) zObject {
	return zObject{
		id:         id,
		objectType: r,
	}
}
