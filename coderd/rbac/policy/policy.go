package policy

import "strings"

const WildcardSymbol = "*"

type actionFields uint32

// Action represents the allowed actions to be done on an object.
type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"

	ActionUse                Action = "use"
	ActionSSH                Action = "ssh"
	ActionApplicationConnect Action = "application_connect"
	ActionViewInsights       Action = "view_insights"

	ActionWorkspaceBuild Action = "build"

	ActionAssign Action = "assign"

	ActionReadPersonal   Action = "read_personal"
	ActionUpdatePersonal Action = "update_personal"
)

const (
	// What fields are expected for a given action.
	// fieldID: uuid for the resource
	fieldID actionFields = 1 << iota
	// fieldOwner: expects an 'Owner' value
	fieldOwner
	// fieldOrg: expects the resource to be owned by an org
	fieldOrg
	// fieldACL: expects an ACL list to accompany the object
	fieldACL
)

type PermissionDefinition struct {
	// name is optional. Used to override "Type" for function naming.
	Name string
	// Actions are a map of actions to some description of what the action
	// should represent. The key in the actions map is the verb to use
	// in the rbac policy.
	Actions map[Action]ActionDefinition
}

type ActionDefinition struct {
	// Human friendly description to explain the action.
	Description string

	// These booleans enforce these fields are p
	Fields actionFields
}

func actDef(fields actionFields, description string) ActionDefinition {
	return ActionDefinition{
		Description: description,
		Fields:      fields,
	}
}

func (a ActionDefinition) Requires() string {
	fields := make([]string, 0)
	if a.Fields&fieldID != 0 {
		fields = append(fields, "uuid")
	}
	if a.Fields&fieldOwner != 0 {
		fields = append(fields, "owner")
	}
	if a.Fields&fieldOrg != 0 {
		fields = append(fields, "org")
	}
	if a.Fields&fieldACL != 0 {
		fields = append(fields, "acl")
	}

	return strings.Join(fields, ",")
}

// RBACPermissions is indexed by the type
var RBACPermissions = map[string]PermissionDefinition{
	// Wildcard is every object, and the action "*" provides all actions.
	// So can grant all actions on all types.
	WildcardSymbol: {
		Name:    "Wildcard",
		Actions: map[Action]ActionDefinition{},
	},
	"user": {
		Actions: map[Action]ActionDefinition{
			// Actions deal with site wide user objects.
			ActionRead:   actDef(0, "read user data"),
			ActionCreate: actDef(0, "create a new user"),
			ActionUpdate: actDef(0, "update an existing user"),
			ActionDelete: actDef(0, "delete an existing user"),

			ActionReadPersonal:   actDef(fieldOwner, "read personal user data like password"),
			ActionUpdatePersonal: actDef(fieldOwner, "update personal data"),
			// ActionReadPublic: actDef(fieldOwner, "read public user data"),
		},
	},
	"workspace": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOwner|fieldOrg, "create a new workspace"),
			ActionRead:   actDef(fieldOwner|fieldOrg|fieldACL, "read workspace data to view on the UI"),
			// TODO: Make updates more granular
			ActionUpdate: actDef(fieldOwner|fieldOrg|fieldACL, "edit workspace settings (scheduling, permissions, parameters)"),
			ActionDelete: actDef(fieldOwner|fieldOrg|fieldACL, "delete workspace"),

			// Workspace provisioning
			ActionWorkspaceBuild: actDef(fieldOwner|fieldOrg|fieldACL, "allows starting, stopping, and updating a workspace"),

			// Running a workspace
			ActionSSH:                actDef(fieldOwner|fieldOrg|fieldACL, "ssh into a given workspace"),
			ActionApplicationConnect: actDef(fieldOwner|fieldOrg|fieldACL, "connect to workspace apps via browser"),
		},
	},
	"workspace_dormant": {
		Actions: map[Action]ActionDefinition{},
	},
	"workspace_proxy": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create a workspace proxy"),
			ActionDelete: actDef(0, "delete a workspace proxy"),
			ActionUpdate: actDef(0, "update a workspace proxy"),
			ActionRead:   actDef(0, "read and use a workspace proxy"),
		},
	},
	"license": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create a license"),
			ActionRead:   actDef(0, "read licenses"),
			ActionDelete: actDef(0, "delete license"),
			// Licenses are immutable, so update makes no sense
		},
	},
	"audit_log": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef(0, "read audit logs"),
		},
	},
	"deployment_config": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef(0, "read deployment config"),
		},
	},
	"deployment_stats": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef(0, "read deployment stats"),
		},
	},
	"replicas": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef(0, "read replicas"),
		},
	},
	"template": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOrg, "create a template"),
			// TODO: Create a use permission maybe?
			ActionRead:         actDef(fieldOrg|fieldACL, "read template"),
			ActionUpdate:       actDef(fieldOrg|fieldACL, "update a template"),
			ActionDelete:       actDef(fieldOrg|fieldACL, "delete a template"),
			ActionViewInsights: actDef(fieldOrg|fieldACL, "view insights"),
		},
	},
	"group": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOrg, "create a group"),
			ActionRead:   actDef(fieldOrg, "read groups"),
			ActionDelete: actDef(fieldOrg, "delete a group"),
			ActionUpdate: actDef(fieldOrg, "update a group"),
		},
	},
	"file": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create a file"),
			ActionRead:   actDef(0, "read files"),
		},
	},
	"provisioner_daemon": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOrg, "create a provisioner daemon"),
			// TODO: Move to use?
			ActionRead:   actDef(fieldOrg, "read provisioner daemon"),
			ActionUpdate: actDef(fieldOrg, "update a provisioner daemon"),
			ActionDelete: actDef(fieldOrg, "delete a provisioner daemon"),
		},
	},
	"organization": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create an organization"),
			ActionRead:   actDef(0, "read organizations"),
			ActionDelete: actDef(0, "delete a organization"),
		},
	},
	"organization_member": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOrg, "create an organization member"),
			ActionRead:   actDef(fieldOrg, "read member"),
			ActionUpdate: actDef(fieldOrg, "update a organization member"),
			ActionDelete: actDef(fieldOrg, "delete member"),
		},
	},
	"debug_info": {
		Actions: map[Action]ActionDefinition{
			ActionUse: actDef(0, "access to debug routes"),
		},
	},
	"system": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create system resources"),
			ActionRead:   actDef(0, "view system resources"),
			ActionUpdate: actDef(0, "update system resources"),
			ActionDelete: actDef(0, "delete system resources"),
		},
	},
	"api_key": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOwner, "create an api key"),
			ActionRead:   actDef(fieldOwner, "read api key details (secrets are not stored)"),
			ActionDelete: actDef(fieldOwner, "delete an api key"),
		},
	},
	"tailnet_coordinator": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, ""),
			ActionRead:   actDef(0, ""),
			ActionUpdate: actDef(0, ""),
			ActionDelete: actDef(0, ""),
		},
	},
	"assign_role": {
		Actions: map[Action]ActionDefinition{
			ActionAssign: actDef(0, "ability to assign roles"),
			ActionRead:   actDef(0, "view what roles are assignable"),
			ActionDelete: actDef(0, "ability to delete roles"),
		},
	},
	"assign_org_role": {
		Actions: map[Action]ActionDefinition{
			ActionAssign: actDef(0, "ability to assign org scoped roles"),
			ActionRead:   actDef(0, "view what roles are assignable"),
			ActionDelete: actDef(0, "ability to delete org scoped roles"),
		},
	},
	"oauth2_app": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "make an OAuth2 app."),
			ActionRead:   actDef(0, "read OAuth2 apps"),
			ActionUpdate: actDef(0, "update the properties of the OAuth2 app."),
			ActionDelete: actDef(0, "delete an OAuth2 app"),
		},
	},
	"oauth2_app_secret": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, ""),
			ActionRead:   actDef(0, ""),
			ActionUpdate: actDef(0, ""),
			ActionDelete: actDef(0, ""),
		},
	},
	"oauth2_app_code_token": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, ""),
			ActionRead:   actDef(0, ""),
			ActionDelete: actDef(0, ""),
		},
	},
}
