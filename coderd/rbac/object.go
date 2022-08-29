package rbac

import (
	"github.com/google/uuid"
)

const WildcardSymbol = "*"

// Objecter returns the RBAC object for itself.
type Objecter interface {
	RBACObject() Object
}

// Resources are just typed objects. Making resources this way allows directly
// passing them into an Authorize function and use the chaining api.
var (
	// ResourceWorkspace CRUD. Org + User owner
	//	create/delete = make or delete workspaces
	// 	read = access workspace
	//	update = edit workspace variables
	ResourceWorkspace = Object{
		Type: "workspace",
	}

	// ResourceWorkspaceExecution CRUD. Org + User owner
	//	create = workspace remote execution
	// 	read = ?
	//	update = ?
	// 	delete = ?
	ResourceWorkspaceExecution = Object{
		Type: "workspace_execution",
	}

	// ResourceAuditLog
	// read = access audit log
	ResourceAuditLog = Object{
		Type: "audit_log",
	}

	// ResourceTemplate CRUD. Org owner only.
	//	create/delete = Make or delete a new template
	//	update = Update the template, make new template versions
	//	read = read the template and all versions associated
	ResourceTemplate = Object{
		Type: "template",
	}

	ResourceFile = Object{
		Type: "file",
	}

	ResourceProvisionerDaemon = Object{
		Type: "provisioner_daemon",
	}

	// ResourceOrganization CRUD. Has an org owner on all but 'create'.
	//	create/delete = make or delete organizations
	// 	read = view org information (Can add user owner for read)
	//	update = ??
	ResourceOrganization = Object{
		Type: "organization",
	}

	// ResourceRoleAssignment might be expanded later to allow more granular permissions
	// to modifying roles. For now, this covers all possible roles, so having this permission
	// allows granting/deleting **ALL** roles.
	// Never has an owner or org.
	//	create  = Assign roles
	//	update  = ??
	//	read	= View available roles to assign
	//	delete	= Remove role
	ResourceRoleAssignment = Object{
		Type: "assign_role",
	}

	// ResourceOrgRoleAssignment is just like ResourceRoleAssignment but for organization roles.
	ResourceOrgRoleAssignment = Object{
		Type: "assign_org_role",
	}

	// ResourceAPIKey is owned by a user.
	//	create  = Create a new api key for user
	//	update  = ??
	//	read	= View api key
	//	delete	= Delete api key
	ResourceAPIKey = Object{
		Type: "api_key",
	}

	// ResourceUser is the user in the 'users' table.
	// ResourceUser never has any owners or in an org, as it's site wide.
	// 	create/delete = make or delete a new user.
	// 	read = view all 'user' table data
	// 	update = update all 'user' table data
	ResourceUser = Object{
		Type: "user",
	}

	// ResourceUserData is any data associated with a user. A user has control
	// over their data (profile, password, etc). So this resource has an owner.
	ResourceUserData = Object{
		Type: "user_data",
	}

	// ResourceOrganizationMember is a user's membership in an organization.
	// Has ONLY an organization owner.
	//	create/delete  = Create/delete member from org.
	//	update  = Update organization member
	//	read	= View member
	ResourceOrganizationMember = Object{
		Type: "organization_member",
	}

	// ResourceWildcard represents all resource types
	ResourceWildcard = Object{
		Type: WildcardSymbol,
	}

	// ResourceLicense is the license in the 'licenses' table.
	// ResourceLicense is site wide.
	// 	create/delete = add or remove license from site.
	// 	read = view license claims
	// 	update = not applicable; licenses are immutable
	ResourceLicense = Object{
		Type: "license",
	}

	ResourceMetrics = Object{
		Type: "metrics",
	}
)

// Object is used to create objects for authz checks when you have none in
// hand to run the check on.
// An example is if you want to list all workspaces, you can create a Object
// that represents the set of workspaces you are trying to get access too.
// Do not export this type, as it can be created from a resource type constant.
type Object struct {
	Owner string `json:"owner"`
	// OrgID specifies which org the object is a part of.
	OrgID string `json:"org_owner"`

	// Type is "workspace", "project", "app", etc
	Type string `json:"type"`
	// TODO: SharedUsers?
}

func (z Object) RBACObject() Object {
	return z
}

// All returns an object matching all resources of the same type.
func (z Object) All() Object {
	return Object{
		Owner: "",
		OrgID: "",
		Type:  z.Type,
	}
}

// InOrg adds an org OwnerID to the resource
func (z Object) InOrg(orgID uuid.UUID) Object {
	return Object{
		Owner: z.Owner,
		OrgID: orgID.String(),
		Type:  z.Type,
	}
}

// WithOwner adds an OwnerID to the resource
func (z Object) WithOwner(ownerID string) Object {
	return Object{
		Owner: ownerID,
		OrgID: z.OrgID,
		Type:  z.Type,
	}
}
