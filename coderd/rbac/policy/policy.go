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
	ActionApplicationConnect        = "application_connect"
	ActionViewInsights              = "view_insights"
)

const (
	fieldOwner actionFields = 1 << iota
	fieldOrg
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
			ActionCreate: actDef(fieldOwner|fieldOrg, "create a workspace"),
			ActionRead:   actDef(fieldOwner|fieldOrg|fieldACL, "read workspace data"),
			// TODO: Make updates more granular
			ActionUpdate:             actDef(fieldOwner|fieldOrg|fieldACL, "update a workspace"),
			ActionDelete:             actDef(fieldOwner|fieldOrg|fieldACL, "delete a workspace"),
			ActionSSH:                actDef(fieldOwner|fieldOrg|fieldACL, "ssh into a given workspace"),
			ActionApplicationConnect: actDef(fieldOwner|fieldOrg|fieldACL, "connect to workspace apps via browser"),
		},
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
}
