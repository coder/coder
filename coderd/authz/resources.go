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

// AsID adds a resource ID to the resource
//nolint:revive
func (r ResourceType) AsID(id string) zObject {
	return zObject{
		id:         id,
		objectType: r,
	}
}
