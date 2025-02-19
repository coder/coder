package policy

const WildcardSymbol = "*"

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

	ActionWorkspaceStart Action = "start"
	ActionWorkspaceStop  Action = "stop"

	ActionAssign Action = "assign"

	ActionReadPersonal   Action = "read_personal"
	ActionUpdatePersonal Action = "update_personal"
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
}

func (d ActionDefinition) String() string {
	return d.Description
}

func actDef(description string) ActionDefinition {
	return ActionDefinition{
		Description: description,
	}
}

var workspaceActions = map[Action]ActionDefinition{
	ActionCreate: actDef("create a new workspace"),
	ActionRead:   actDef("read workspace data to view on the UI"),
	// TODO: Make updates more granular
	ActionUpdate: actDef("edit workspace settings (scheduling, permissions, parameters)"),
	ActionDelete: actDef("delete workspace"),

	// Workspace provisioning. Start & stop are different so dormant workspaces can be
	// stopped, but not stared.
	ActionWorkspaceStart: actDef("allows starting a workspace"),
	ActionWorkspaceStop:  actDef("allows stopping a workspace"),

	// Running a workspace
	ActionSSH:                actDef("ssh into a given workspace"),
	ActionApplicationConnect: actDef("connect to workspace apps via browser"),
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
			ActionRead:   actDef("read user data"),
			ActionCreate: actDef("create a new user"),
			ActionUpdate: actDef("update an existing user"),
			ActionDelete: actDef("delete an existing user"),

			ActionReadPersonal:   actDef("read personal user data like user settings and auth links"),
			ActionUpdatePersonal: actDef("update personal data"),
		},
	},
	"workspace": {
		Actions: workspaceActions,
	},
	// Dormant workspaces have the same perms as workspaces.
	"workspace_dormant": {
		Actions: workspaceActions,
	},
	"workspace_proxy": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create a workspace proxy"),
			ActionDelete: actDef("delete a workspace proxy"),
			ActionUpdate: actDef("update a workspace proxy"),
			ActionRead:   actDef("read and use a workspace proxy"),
		},
	},
	"license": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create a license"),
			ActionRead:   actDef("read licenses"),
			ActionDelete: actDef("delete license"),
			// Licenses are immutable, so update makes no sense
		},
	},
	"audit_log": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   actDef("read audit logs"),
			ActionCreate: actDef("create new audit log entries"),
		},
	},
	"deployment_config": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   actDef("read deployment config"),
			ActionUpdate: actDef("updating health information"),
		},
	},
	"deployment_stats": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef("read deployment stats"),
		},
	},
	"replicas": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef("read replicas"),
		},
	},
	"template": {
		Actions: map[Action]ActionDefinition{
			ActionCreate:       actDef("create a template"),
			ActionUse:          actDef("use the template to initially create a workspace, then workspace lifecycle permissions take over"),
			ActionRead:         actDef("read template"),
			ActionUpdate:       actDef("update a template"),
			ActionDelete:       actDef("delete a template"),
			ActionViewInsights: actDef("view insights"),
		},
	},
	"group": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create a group"),
			ActionRead:   actDef("read groups"),
			ActionDelete: actDef("delete a group"),
			ActionUpdate: actDef("update a group"),
		},
	},
	"group_member": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef("read group members"),
		},
	},
	"file": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create a file"),
			ActionRead:   actDef("read files"),
		},
	},
	"provisioner_daemon": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create a provisioner daemon key"),
			// TODO: Move to use?
			ActionRead:   actDef("read provisioner daemon"),
			ActionUpdate: actDef("update a provisioner daemon"),
			ActionDelete: actDef("delete a provisioner daemon/key"),
		},
	},
	"provisioner_jobs": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef("read provisioner jobs"),
		},
	},
	"organization": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create an organization"),
			ActionRead:   actDef("read organizations"),
			ActionUpdate: actDef("update an organization"),
			ActionDelete: actDef("delete an organization"),
		},
	},
	"organization_member": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create an organization member"),
			ActionRead:   actDef("read member"),
			ActionUpdate: actDef("update an organization member"),
			ActionDelete: actDef("delete member"),
		},
	},
	"debug_info": {
		Actions: map[Action]ActionDefinition{
			ActionRead: actDef("access to debug routes"),
		},
	},
	"system": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create system resources"),
			ActionRead:   actDef("view system resources"),
			ActionUpdate: actDef("update system resources"),
			ActionDelete: actDef("delete system resources"),
		},
	},
	"api_key": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create an api key"),
			ActionRead:   actDef("read api key details (secrets are not stored)"),
			ActionDelete: actDef("delete an api key"),
			ActionUpdate: actDef("update an api key, eg expires"),
		},
	},
	"tailnet_coordinator": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create a Tailnet coordinator"),
			ActionRead:   actDef("view info about a Tailnet coordinator"),
			ActionUpdate: actDef("update a Tailnet coordinator"),
			ActionDelete: actDef("delete a Tailnet coordinator"),
		},
	},
	"assign_role": {
		Actions: map[Action]ActionDefinition{
			ActionAssign: actDef("ability to assign roles"),
			ActionRead:   actDef("view what roles are assignable"),
			ActionDelete: actDef("ability to unassign roles"),
			ActionCreate: actDef("ability to create/delete/edit custom roles"),
			ActionUpdate: actDef("ability to edit custom roles"),
		},
	},
	"assign_org_role": {
		Actions: map[Action]ActionDefinition{
			ActionAssign: actDef("ability to assign org scoped roles"),
			ActionRead:   actDef("view what roles are assignable"),
			ActionDelete: actDef("ability to delete org scoped roles"),
			ActionCreate: actDef("ability to create/delete custom roles within an organization"),
			ActionUpdate: actDef("ability to edit custom roles within an organization"),
		},
	},
	"oauth2_app": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("make an OAuth2 app"),
			ActionRead:   actDef("read OAuth2 apps"),
			ActionUpdate: actDef("update the properties of the OAuth2 app"),
			ActionDelete: actDef("delete an OAuth2 app"),
		},
	},
	"oauth2_app_secret": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create an OAuth2 app secret"),
			ActionRead:   actDef("read an OAuth2 app secret"),
			ActionUpdate: actDef("update an OAuth2 app secret"),
			ActionDelete: actDef("delete an OAuth2 app secret"),
		},
	},
	"oauth2_app_code_token": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create an OAuth2 app code token"),
			ActionRead:   actDef("read an OAuth2 app code token"),
			ActionDelete: actDef("delete an OAuth2 app code token"),
		},
	},
	"notification_message": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create notification messages"),
			ActionRead:   actDef("read notification messages"),
			ActionUpdate: actDef("update notification messages"),
			ActionDelete: actDef("delete notification messages"),
		},
	},
	"notification_template": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   actDef("read notification templates"),
			ActionUpdate: actDef("update notification templates"),
		},
	},
	"notification_preference": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   actDef("read notification preferences"),
			ActionUpdate: actDef("update notification preferences"),
		},
	},
	"crypto_key": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   actDef("read crypto keys"),
			ActionUpdate: actDef("update crypto keys"),
			ActionDelete: actDef("delete crypto keys"),
			ActionCreate: actDef("create crypto keys"),
		},
	},
	// idpsync_settings should always be org scoped
	"idpsync_settings": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   actDef("read IdP sync settings"),
			ActionUpdate: actDef("update IdP sync settings"),
		},
	},
	"workspace_agent_resource_monitor": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   actDef("read workspace agent resource monitor"),
			ActionCreate: actDef("create workspace agent resource monitor"),
			ActionUpdate: actDef("update workspace agent resource monitor"),
		},
	},
}
