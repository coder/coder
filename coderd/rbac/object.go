package rbac

import (
	"github.com/google/uuid"
)

const WildcardSymbol = "*"

// Resources are just typed objects. Making resources this way allows directly
// passing them into an Authorize function and use the chaining api.
var (
	ResourceWorkspace = Object{
		Type: "workspace",
	}

	ResourceTemplate = Object{
		Type: "template",
	}

	// ResourceWildcard represents all resource types
	ResourceWildcard = Object{
		Type: WildcardSymbol,
	}
)

// Object is used to create objects for authz checks when you have none in
// hand to run the check on.
// An example is if you want to list all workspaces, you can create a Object
// that represents the set of workspaces you are trying to get access too.
// Do not export this type, as it can be created from a resource type constant.
type Object struct {
	ResourceID string `json:"id"`
	Owner      string `json:"owner"`
	// OrgID specifies which org the object is a part of.
	OrgID string `json:"org_owner"`

	// Type is "workspace", "project", "devurl", etc
	Type string `json:"type"`
	// TODO: SharedUsers?
}

// All returns an object matching all resources of the same type.
func (z Object) All() Object {
	return Object{
		ResourceID: "",
		Owner:      "",
		OrgID:      "",
		Type:       z.Type,
	}
}

// InOrg adds an org OwnerID to the resource
func (z Object) InOrg(orgID uuid.UUID) Object {
	return Object{
		ResourceID: z.ResourceID,
		Owner:      z.Owner,
		OrgID:      orgID.String(),
		Type:       z.Type,
	}
}

// WithOwner adds an OwnerID to the resource
func (z Object) WithOwner(ownerID string) Object {
	return Object{
		ResourceID: z.ResourceID,
		Owner:      ownerID,
		OrgID:      z.OrgID,
		Type:       z.Type,
	}
}

// WithID adds a ResourceID to the resource
func (z Object) WithID(resourceID string) Object {
	return Object{
		ResourceID: resourceID,
		Owner:      z.Owner,
		OrgID:      z.OrgID,
		Type:       z.Type,
	}
}
