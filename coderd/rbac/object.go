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
	// ResourceWildcard represents all resource types
	// Try to avoid using this where possible.
	ResourceWildcard = Object{
		Type: WildcardSymbol,
	}

	// ResourceWorkspace CRUD. Org + User owner
	//	create/delete = make or delete workspaces
	// 	read = access workspace
	//	update = edit workspace variables
	ResourceWorkspace = Object{
		Type: "workspace",
	}

	// ResourceWorkspaceBuild refers to permissions necessary to
	// insert a workspace build job.
	// create/delete = ?
	// read = read workspace builds
	// update = insert/update workspace builds.
	ResourceWorkspaceBuild = Object{
		Type: "workspace_build",
	}

	// ResourceWorkspaceDormant is returned if a workspace is dormant.
	// It grants restricted permissions on workspace builds.
	ResourceWorkspaceDormant = Object{
		Type: "workspace_dormant",
	}

	// ResourceWorkspaceProxy CRUD. Org
	//	create/delete = make or delete proxies
	// 	read = read proxy urls
	//	update = edit workspace proxy fields
	ResourceWorkspaceProxy = Object{
		Type: "workspace_proxy",
	}

	// ResourceWorkspaceExecution CRUD. Org + User owner
	//	create = workspace remote execution
	// 	read = ?
	//	update = ?
	// 	delete = ?
	ResourceWorkspaceExecution = Object{
		Type: "workspace_execution",
	}

	// ResourceWorkspaceApplicationConnect CRUD. Org + User owner
	//	create = connect to an application
	// 	read = ?
	//	update = ?
	// 	delete = ?
	ResourceWorkspaceApplicationConnect = Object{
		Type: "application_connect",
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

	// ResourceGroup CRUD. Org admins only.
	//	create/delete = Make or delete a new group.
	//	update = Update the name or members of a group.
	//	read = Read groups and their members.
	ResourceGroup = Object{
		Type: "group",
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

	// ResourceLicense is the license in the 'licenses' table.
	// ResourceLicense is site wide.
	// 	create/delete = add or remove license from site.
	// 	read = view license claims
	// 	update = not applicable; licenses are immutable
	ResourceLicense = Object{
		Type: "license",
	}

	// ResourceDeploymentValues
	ResourceDeploymentValues = Object{
		Type: "deployment_config",
	}

	ResourceDeploymentStats = Object{
		Type: "deployment_stats",
	}

	ResourceReplicas = Object{
		Type: "replicas",
	}

	// ResourceDebugInfo controls access to the debug routes `/api/v2/debug/*`.
	ResourceDebugInfo = Object{
		Type: "debug_info",
	}

	// ResourceSystem is a pseudo-resource only used for system-level actions.
	ResourceSystem = Object{
		Type: "system",
	}

	// ResourceTailnetCoordinator is a pseudo-resource for use by the tailnet coordinator
	ResourceTailnetCoordinator = Object{
		Type: "tailnet_coordinator",
	}

	// ResourceTemplateInsights is a pseudo-resource for reading template insights data.
	ResourceTemplateInsights = Object{
		Type: "template_insights",
	}
)

// ResourceUserObject is a helper function to create a user object for authz checks.
func ResourceUserObject(userID uuid.UUID) Object {
	return ResourceUser.WithID(userID).WithOwner(userID.String())
}

// Object is used to create objects for authz checks when you have none in
// hand to run the check on.
// An example is if you want to list all workspaces, you can create a Object
// that represents the set of workspaces you are trying to get access too.
// Do not export this type, as it can be created from a resource type constant.
type Object struct {
	// ID is the resource's uuid
	ID    string `json:"id"`
	Owner string `json:"owner"`
	// OrgID specifies which org the object is a part of.
	OrgID string `json:"org_owner"`

	// Type is "workspace", "project", "app", etc
	Type string `json:"type"`

	ACLUserList  map[string][]Action ` json:"acl_user_list"`
	ACLGroupList map[string][]Action ` json:"acl_group_list"`
}

func (z Object) Equal(b Object) bool {
	if z.ID != b.ID {
		return false
	}
	if z.Owner != b.Owner {
		return false
	}
	if z.OrgID != b.OrgID {
		return false
	}
	if z.Type != b.Type {
		return false
	}

	if !equalACLLists(z.ACLUserList, b.ACLUserList) {
		return false
	}

	if !equalACLLists(z.ACLGroupList, b.ACLGroupList) {
		return false
	}

	return true
}

func equalACLLists(a, b map[string][]Action) bool {
	if len(a) != len(b) {
		return false
	}

	for k, actions := range a {
		if len(actions) != len(b[k]) {
			return false
		}
		for i, a := range actions {
			if a != b[k][i] {
				return false
			}
		}
	}
	return true
}

func (z Object) RBACObject() Object {
	return z
}

// All returns an object matching all resources of the same type.
func (z Object) All() Object {
	return Object{
		Owner:        "",
		OrgID:        "",
		Type:         z.Type,
		ACLUserList:  map[string][]Action{},
		ACLGroupList: map[string][]Action{},
	}
}

func (z Object) WithIDString(id string) Object {
	return Object{
		ID:           id,
		Owner:        z.Owner,
		OrgID:        z.OrgID,
		Type:         z.Type,
		ACLUserList:  z.ACLUserList,
		ACLGroupList: z.ACLGroupList,
	}
}

func (z Object) WithID(id uuid.UUID) Object {
	return Object{
		ID:           id.String(),
		Owner:        z.Owner,
		OrgID:        z.OrgID,
		Type:         z.Type,
		ACLUserList:  z.ACLUserList,
		ACLGroupList: z.ACLGroupList,
	}
}

// InOrg adds an org OwnerID to the resource
func (z Object) InOrg(orgID uuid.UUID) Object {
	return Object{
		ID:           z.ID,
		Owner:        z.Owner,
		OrgID:        orgID.String(),
		Type:         z.Type,
		ACLUserList:  z.ACLUserList,
		ACLGroupList: z.ACLGroupList,
	}
}

// WithOwner adds an OwnerID to the resource
func (z Object) WithOwner(ownerID string) Object {
	return Object{
		ID:           z.ID,
		Owner:        ownerID,
		OrgID:        z.OrgID,
		Type:         z.Type,
		ACLUserList:  z.ACLUserList,
		ACLGroupList: z.ACLGroupList,
	}
}

// WithACLUserList adds an ACL list to a given object
func (z Object) WithACLUserList(acl map[string][]Action) Object {
	return Object{
		ID:           z.ID,
		Owner:        z.Owner,
		OrgID:        z.OrgID,
		Type:         z.Type,
		ACLUserList:  acl,
		ACLGroupList: z.ACLGroupList,
	}
}

func (z Object) WithGroupACL(groups map[string][]Action) Object {
	return Object{
		ID:           z.ID,
		Owner:        z.Owner,
		OrgID:        z.OrgID,
		Type:         z.Type,
		ACLUserList:  z.ACLUserList,
		ACLGroupList: groups,
	}
}
