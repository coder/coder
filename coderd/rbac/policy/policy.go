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

	ActionWorkspaceBuild           Action = "build"
	ActionViewWorkspaceBuildParams Action = "build_parameters"

	ActionAssign Action = "assign"
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
	name string
	// Type should be a unique string to identify the
	Type string
	// Actions are a map of actions to some description of what the action
	// should represent. The key in the actions map is the verb to use
	// in the rbac policy.
	Actions map[Action]ActionDefinition
}

func (p PermissionDefinition) Name() string {
	if p.name != "" {
		return p.name
	}
	return p.Type
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

var RBACPermissions = []PermissionDefinition{
	{
		name: "Wildcard",
		Type: WildcardSymbol,
		Actions: map[Action]ActionDefinition{
			WildcardSymbol: {
				Description: "Wildcard gives admin level access to all resources and all actions.",
				Fields:      0,
			},
		},
	},
	{
		Type: "user",
		Actions: map[Action]ActionDefinition{
			// Actions deal with site wide user objects.
			ActionRead:   actDef(0, "read user data"),
			ActionCreate: actDef(0, "create a new user"),
			ActionUpdate: actDef(0, "update an existing user"),
			ActionDelete: actDef(0, "delete an existing user"),

			"read_personal":   actDef(fieldOwner, "read personal user data like password"),
			"update_personal": actDef(fieldOwner, "update personal data"),
			//ActionReadPublic: actDef(fieldOwner, "read public user data"),
		},
	},
	{
		Type: "workspace",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOwner|fieldOrg, "create a new workspace"),
			ActionRead:   actDef(fieldOwner|fieldOrg|fieldACL, "read workspace data to view on the UI"),
			// TODO: Make updates more granular
			ActionUpdate: actDef(fieldOwner|fieldOrg|fieldACL, "edit workspace settings (scheduling, permissions, parameters)"),
			ActionDelete: actDef(fieldOwner|fieldOrg|fieldACL, "delete workspace"),

			// Workspace provisioning
			ActionWorkspaceBuild: actDef(fieldOwner|fieldOrg|fieldACL, "allows starting, stopping, and updating a workspace"),
			// TODO: ActionViewWorkspaceBuildParams is very werid. Seems to be used for autofilling the last params set.
			//		Admins want this so they can update a user's workspace with the old values??
			ActionViewWorkspaceBuildParams: actDef(fieldOwner|fieldOrg|fieldACL, "view workspace build parameters"),

			// Running a workspace
			ActionSSH:                actDef(fieldOwner|fieldOrg|fieldACL, "ssh into a given workspace"),
			ActionApplicationConnect: actDef(fieldOwner|fieldOrg|fieldACL, "connect to workspace apps via browser"),
		},
	},
	{
		Type:    "workspace_dormant",
		Actions: map[Action]ActionDefinition{},
	},
	{
		Type: "workspace_proxy",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create a workspace proxy"),
			ActionDelete: actDef(0, "delete a workspace proxy"),
			ActionUpdate: actDef(0, "update a workspace proxy"),
			ActionRead:   actDef(0, "read and use a workspace proxy"),
		},
	},
	{
		Type: "license",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create a license"),
			ActionRead:   actDef(0, "read licenses"),
			ActionDelete: actDef(0, "delete license"),
			// Licenses are immutable, so update makes no sense
		},
	},
	{
		Type: "audit_log",
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef(0, "read audit logs"),
		},
	},
	{
		Type: "deployment_config",
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef(0, "read deployment config"),
		},
	},
	{
		Type: "deployment_stats",
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef(0, "read deployment stats"),
		},
	},
	{
		Type: "replicas",
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef(0, "read replicas"),
		},
	},
	{
		Type: "template",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOrg, "create a template"),
			// TODO: Create a use permission maybe?
			ActionRead:         actDef(fieldOrg|fieldACL, "read template"),
			ActionUpdate:       actDef(fieldOrg|fieldACL, "update a template"),
			ActionDelete:       actDef(fieldOrg|fieldACL, "delete a template"),
			ActionViewInsights: actDef(fieldOrg|fieldACL, "view insights"),
		},
	},
	{
		Type: "group",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOrg, "create a group"),
			ActionRead:   actDef(fieldOrg, "read groups"),
			ActionDelete: actDef(fieldOrg, "delete a group"),
			ActionUpdate: actDef(fieldOrg, "update a group"),
		},
	},
	{
		Type: "file",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create a file"),
			ActionRead:   actDef(0, "read files"),
		},
	},
	{
		Type: "provisioner_daemon",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOrg, "create a provisioner daemon"),
			// TODO: Move to use?
			ActionRead:   actDef(fieldOrg, "read provisioner daemon"),
			ActionUpdate: actDef(fieldOrg, "update a provisioner daemon"),
			ActionDelete: actDef(fieldOrg, "delete a provisioner daemon"),
		},
	},
	{
		Type: "organization",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create an organization"),
			ActionRead:   actDef(0, "read organizations"),
			ActionDelete: actDef(0, "delete a organization"),
		},
	},
	{
		Type: "organization_member",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOrg, "create an organization member"),
			ActionRead:   actDef(fieldOrg, "read member"),
			ActionUpdate: actDef(fieldOrg, "update a organization member"),
			ActionDelete: actDef(fieldOrg, "delete member"),
		},
	},
	{
		Type: "debug_info",
		Actions: map[Action]ActionDefinition{
			ActionUse: actDef(0, "access to debug routes"),
		},
	},
	{
		Type: "system",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "create system resources"),
			ActionRead:   actDef(0, "view system resources"),
			ActionUpdate: actDef(0, "update system resources"),
			ActionDelete: actDef(0, "delete system resources"),
		},
	},
	{
		Type: "api_key",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(fieldOwner, "create an api key"),
			ActionRead:   actDef(fieldOwner, "read api key details (secrets are not stored)"),
			ActionDelete: actDef(fieldOwner, "delete an api key"),
		},
	},
	{
		Type: "tailnet_coordinator",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, ""),
			ActionRead:   actDef(0, ""),
			ActionUpdate: actDef(0, ""),
			ActionDelete: actDef(0, ""),
		},
	},
	{
		Type: "assign_role",
		Actions: map[Action]ActionDefinition{
			ActionAssign: actDef(0, "ability to assign roles"),
			ActionRead:   actDef(0, "view what roles are assignable"),
			ActionDelete: actDef(0, "ability to delete roles"),
		},
	},
	{
		Type: "assign_org_role",
		Actions: map[Action]ActionDefinition{
			ActionAssign: actDef(0, "ability to assign org scoped roles"),
			ActionDelete: actDef(0, "ability to delete org scoped roles"),
		},
	},
	{
		Type: "oauth2_app",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, "make an OAuth2 app."),
			ActionRead:   actDef(0, "read OAuth2 apps"),
			ActionUpdate: actDef(0, "update the properties of the OAuth2 app."),
			ActionDelete: actDef(0, "delete an OAuth2 app"),
		},
	},
	{
		Type: "oauth2_app_secret",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, ""),
			ActionRead:   actDef(0, ""),
			ActionUpdate: actDef(0, ""),
			ActionDelete: actDef(0, ""),
		},
	},
	{
		Type: "oauth2_app_code_token",
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef(0, ""),
			ActionRead:   actDef(0, ""),
			ActionDelete: actDef(0, ""),
		},
	},
}
