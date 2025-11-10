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

	ActionAssign   Action = "assign"
	ActionUnassign Action = "unassign"

	ActionReadPersonal   Action = "read_personal"
	ActionUpdatePersonal Action = "update_personal"

	ActionCreateAgent Action = "create_agent"
	ActionDeleteAgent Action = "delete_agent"

	ActionShare Action = "share"
)

type PermissionDefinition struct {
	// name is optional. Used to override "Type" for function naming.
	Name string
	// Actions are a map of actions to some description of what the action
	// should represent. The key in the actions map is the verb to use
	// in the rbac policy.
	Actions map[Action]ActionDefinition
	// Comment is additional text to include in the generated object comment.
	Comment string
}

// Human friendly description to explain the action.
type ActionDefinition string

var workspaceActions = map[Action]ActionDefinition{
	ActionCreate: "create a new workspace",
	ActionRead:   "read workspace data to view on the UI",
	// TODO: Make updates more granular
	ActionUpdate: "edit workspace settings (scheduling, permissions, parameters)",
	ActionDelete: "delete workspace",

	// Workspace provisioning. Start & stop are different so dormant workspaces can be
	// stopped, but not stared.
	ActionWorkspaceStart: "allows starting a workspace",
	ActionWorkspaceStop:  "allows stopping a workspace",

	// Running a workspace
	ActionSSH:                "ssh into a given workspace",
	ActionApplicationConnect: "connect to workspace apps via browser",

	ActionCreateAgent: "create a new workspace agent",
	ActionDeleteAgent: "delete an existing workspace agent",

	// Sharing a workspace
	ActionShare: "share a workspace with other users or groups",
}

var taskActions = map[Action]ActionDefinition{
	ActionCreate: "create a new task",
	ActionRead:   "read task data or output to view on the UI or CLI",
	ActionUpdate: "edit task settings or send input to an existing task",
	ActionDelete: "delete task",
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
			ActionRead:   "read user data",
			ActionCreate: "create a new user",
			ActionUpdate: "update an existing user",
			ActionDelete: "delete an existing user",

			ActionReadPersonal:   "read personal user data like user settings and auth links",
			ActionUpdatePersonal: "update personal data",
		},
	},
	"workspace": {
		Actions: workspaceActions,
	},
	"task": {
		Actions: taskActions,
	},
	// Dormant workspaces have the same perms as workspaces.
	"workspace_dormant": {
		Actions: workspaceActions,
	},
	"prebuilt_workspace": {
		// Prebuilt_workspace actions currently apply only to delete operations.
		// To successfully delete a prebuilt workspace, a user must have the following permissions:
		//   * workspace.read: to read the current workspace state
		//   * update: to modify workspace metadata and related resources during deletion
		//             (e.g., updating the deleted field in the database)
		//   * delete: to perform the actual deletion of the workspace
		// If the user lacks prebuilt_workspace update or delete permissions,
		// the authorization will always fall back to the corresponding permissions on workspace.
		Actions: map[Action]ActionDefinition{
			ActionUpdate: "update prebuilt workspace settings",
			ActionDelete: "delete prebuilt workspace",
		},
	},
	"workspace_proxy": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create a workspace proxy",
			ActionDelete: "delete a workspace proxy",
			ActionUpdate: "update a workspace proxy",
			ActionRead:   "read and use a workspace proxy",
		},
	},
	"license": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create a license",
			ActionRead:   "read licenses",
			ActionDelete: "delete license",
			// Licenses are immutable, so update makes no sense
		},
	},
	"audit_log": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read audit logs",
			ActionCreate: "create new audit log entries",
		},
	},
	"connection_log": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read connection logs",
			ActionUpdate: "upsert connection log entries",
		},
	},
	"deployment_config": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read deployment config",
			ActionUpdate: "updating health information",
		},
	},
	"deployment_stats": {
		Actions: map[Action]ActionDefinition{
			ActionRead: "read deployment stats",
		},
	},
	"replicas": {
		Actions: map[Action]ActionDefinition{
			ActionRead: "read replicas",
		},
	},
	"template": {
		Actions: map[Action]ActionDefinition{
			ActionCreate:       "create a template",
			ActionUse:          "use the template to initially create a workspace, then workspace lifecycle permissions take over",
			ActionRead:         "read template",
			ActionUpdate:       "update a template",
			ActionDelete:       "delete a template",
			ActionViewInsights: "view insights",
		},
	},
	"group": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create a group",
			ActionRead:   "read groups",
			ActionDelete: "delete a group",
			ActionUpdate: "update a group",
		},
	},
	"group_member": {
		Actions: map[Action]ActionDefinition{
			ActionRead: "read group members",
		},
	},
	"file": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create a file",
			ActionRead:   "read files",
		},
	},
	"provisioner_daemon": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create a provisioner daemon/key",
			// TODO: Move to use?
			ActionRead:   "read provisioner daemon",
			ActionUpdate: "update a provisioner daemon",
			ActionDelete: "delete a provisioner daemon/key",
		},
	},
	"provisioner_jobs": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read provisioner jobs",
			ActionUpdate: "update provisioner jobs",
			ActionCreate: "create provisioner jobs",
		},
	},
	"organization": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create an organization",
			ActionRead:   "read organizations",
			ActionUpdate: "update an organization",
			ActionDelete: "delete an organization",
		},
	},
	"organization_member": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create an organization member",
			ActionRead:   "read member",
			ActionUpdate: "update an organization member",
			ActionDelete: "delete member",
		},
	},
	"debug_info": {
		Actions: map[Action]ActionDefinition{
			ActionRead: "access to debug routes",
		},
	},
	"system": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create system resources",
			ActionRead:   "view system resources",
			ActionUpdate: "update system resources",
			ActionDelete: "delete system resources",
		},
		Comment: `
	// DEPRECATED: New resources should be created for new things, rather than adding them to System, which has become
	//             an unmanaged collection of things that don't relate to one another. We can't effectively enforce
	//             least privilege access control when unrelated resources are grouped together.`,
	},
	"api_key": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create an api key",
			ActionRead:   "read api key details (secrets are not stored)",
			ActionDelete: "delete an api key",
			ActionUpdate: "update an api key, eg expires",
		},
	},
	"tailnet_coordinator": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create a Tailnet coordinator",
			ActionRead:   "view info about a Tailnet coordinator",
			ActionUpdate: "update a Tailnet coordinator",
			ActionDelete: "delete a Tailnet coordinator",
		},
	},
	"assign_role": {
		Actions: map[Action]ActionDefinition{
			ActionAssign:   "assign user roles",
			ActionUnassign: "unassign user roles",
			ActionRead:     "view what roles are assignable",
		},
	},
	"assign_org_role": {
		Actions: map[Action]ActionDefinition{
			ActionAssign:   "assign org scoped roles",
			ActionUnassign: "unassign org scoped roles",
			ActionCreate:   "create/delete custom roles within an organization",
			ActionRead:     "view what roles are assignable within an organization",
			ActionUpdate:   "edit custom roles within an organization",
			ActionDelete:   "delete roles within an organization",
		},
	},
	"oauth2_app": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "make an OAuth2 app",
			ActionRead:   "read OAuth2 apps",
			ActionUpdate: "update the properties of the OAuth2 app",
			ActionDelete: "delete an OAuth2 app",
		},
	},
	"oauth2_app_secret": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create an OAuth2 app secret",
			ActionRead:   "read an OAuth2 app secret",
			ActionUpdate: "update an OAuth2 app secret",
			ActionDelete: "delete an OAuth2 app secret",
		},
	},
	"oauth2_app_code_token": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create an OAuth2 app code token",
			ActionRead:   "read an OAuth2 app code token",
			ActionDelete: "delete an OAuth2 app code token",
		},
	},
	"notification_message": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create notification messages",
			ActionRead:   "read notification messages",
			ActionUpdate: "update notification messages",
			ActionDelete: "delete notification messages",
		},
	},
	"notification_template": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read notification templates",
			ActionUpdate: "update notification templates",
		},
	},
	"notification_preference": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read notification preferences",
			ActionUpdate: "update notification preferences",
		},
	},
	"webpush_subscription": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create webpush subscriptions",
			ActionRead:   "read webpush subscriptions",
			ActionDelete: "delete webpush subscriptions",
		},
	},
	"inbox_notification": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create inbox notifications",
			ActionRead:   "read inbox notifications",
			ActionUpdate: "update inbox notifications",
		},
	},
	"crypto_key": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read crypto keys",
			ActionUpdate: "update crypto keys",
			ActionDelete: "delete crypto keys",
			ActionCreate: "create crypto keys",
		},
	},
	// idpsync_settings should always be org scoped
	"idpsync_settings": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read IdP sync settings",
			ActionUpdate: "update IdP sync settings",
		},
	},
	"workspace_agent_resource_monitor": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read workspace agent resource monitor",
			ActionCreate: "create workspace agent resource monitor",
			ActionUpdate: "update workspace agent resource monitor",
		},
	},
	"workspace_agent_devcontainers": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create workspace agent devcontainers",
		},
	},
	"user_secret": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create a user secret",
			ActionRead:   "read user secret metadata and value",
			ActionUpdate: "update user secret metadata and value",
			ActionDelete: "delete a user secret",
		},
	},
	"usage_event": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: "create a usage event",
			ActionRead:   "read usage events",
			ActionUpdate: "update usage events",
		},
	},
	"aibridge_interception": {
		Actions: map[Action]ActionDefinition{
			ActionRead:   "read aibridge interceptions & related records",
			ActionUpdate: "update aibridge interceptions & related records",
			ActionCreate: "create aibridge interceptions & related records",
		},
	},
}
