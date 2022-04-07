package authz

import "github.com/coder/coder/coderd/authz/rbac"

// type Resource interface {
// ID() string
// ResourceType() rbac.Resource
// }
//
// type UserResource interface {
// Resource
// OwnerID() string
// }
//
// type OrgResource interface {
// Resource
// OrgOwnerID() string
// }
//
// var _ Resource = (*Object)(nil)
// var _ UserResource = (*Object)(nil)
// var _ OrgResource = (*Object)(nil)

// Object is used to create objects for authz checks when you have none in
// hand to run the check on.
// An example is if you want to list all workspaces, you can create a Object
// that represents the set of workspaces you are trying to get access too.
// Do not export this type, as it can be created from a resource type constant.
type Object struct {
	// ID       string
	ID string
	// Owner    string
	Owner string
	// OrgOwner string
	OrgOwner string

	// ObjectType is "workspace", "project", "devurl", etc
	// ObjectType rbac.Resource
	ObjectType rbac.Resource
	// TODO: SharedUsers?
}

// func (z Object) ID() string {
// return z.id
// }
//
// func (z Object) ResourceType() rbac.Resource {
// return z.objectType
// }
//
// func (z Object) OwnerID() string {
// return z.owner
// }
//
// func (z Object) OrgOwnerID() string {
// return z.orgOwner
// }
//
// Org adds an org OwnerID to the resource
// nolint:revive
// func (z Object) Org(orgID string) Object {
// z.orgOwner = orgID
// return z
// }
//
// Owner adds an OwnerID to the resource
// nolint:revive
// func (z Object) Owner(id string) Object {
// z.owner = id
// return z
// }
//
// nolint:revive
// func (z Object) AsID(id string) Object {
// z.id = id
// return z
// }
//
