package dashboard

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/codersdk"
)

// DefaultActions is a table of actions to perform.
// D&D nerds will feel right at home here :-)
// Note that the order of the table is important!
// Entries must be in ascending order.
var DefaultActions RollTable = []RollTableEntry{
	{0, fetchWorkspaces, "fetch workspaces"},
	{1, fetchUsers, "fetch users"},
	{2, fetchTemplates, "fetch templates"},
	{3, authCheckAsOwner, "authcheck owner"},
	{4, authCheckAsNonOwner, "authcheck not owner"},
	{5, fetchAuditLog, "fetch audit log"},
	{6, fetchActiveUsers, "fetch active users"},
	{7, fetchSuspendedUsers, "fetch suspended users"},
	{8, fetchTemplateVersion, "fetch template version"},
	{9, fetchWorkspace, "fetch workspace"},
	{10, fetchTemplate, "fetch template"},
	{11, fetchUserByID, "fetch user by ID"},
	{12, fetchUserByUsername, "fetch user by username"},
	{13, fetchWorkspaceBuild, "fetch workspace build"},
	{14, fetchDeploymentConfig, "fetch deployment config"},
	{15, fetchWorkspaceQuotaForUser, "fetch workspace quota for user"},
	{16, fetchDeploymentStats, "fetch deployment stats"},
	{17, fetchWorkspaceLogs, "fetch workspace logs"},
}

// RollTable is a slice of rollTableEntry.
type RollTable []RollTableEntry

// RollTableEntry is an entry in the roll table.
type RollTableEntry struct {
	// Roll is the minimum number required to perform the action.
	Roll int
	// Fn is the function to call.
	Fn func(ctx context.Context, p *Params) error
	// Label is used for logging.
	Label string
}

// choose returns the first entry in the table that is greater than or equal to n.
func (r RollTable) choose(n int) RollTableEntry {
	for _, entry := range r {
		if entry.Roll >= n {
			return entry
		}
	}
	return RollTableEntry{}
}

// max returns the maximum roll in the table.
// Important: this assumes that the table is sorted in ascending order.
func (r RollTable) max() int {
	return r[len(r)-1].Roll
}

// Params is a set of parameters to pass to the actions in a rollTable.
type Params struct {
	// client is the client to use for performing the action.
	client *codersdk.Client
	// me is the currently authenticated user. Lots of actions require this.
	me codersdk.User
	// For picking random resource IDs, we need to know what resources are
	// present. We store them in a cache to avoid fetching them every time.
	// This may seem counter-intuitive for load testing, but we want to avoid
	// muddying results.
	c *cache
}

// fetchWorkspaces fetches all workspaces.
func fetchWorkspaces(ctx context.Context, p *Params) error {
	ws, err := p.client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	if err != nil {
		// store the workspaces for later use in case they change
		p.c.setWorkspaces(ws.Workspaces)
	}
	return err
}

// fetchUsers fetches all users.
func fetchUsers(ctx context.Context, p *Params) error {
	users, err := p.client.Users(ctx, codersdk.UsersRequest{})
	if err != nil {
		p.c.setUsers(users.Users)
	}
	return err
}

// fetchActiveUsers fetches all active users
func fetchActiveUsers(ctx context.Context, p *Params) error {
	_, err := p.client.Users(ctx, codersdk.UsersRequest{
		Status: codersdk.UserStatusActive,
	})
	return err
}

// fetchSuspendedUsers fetches all suspended users
func fetchSuspendedUsers(ctx context.Context, p *Params) error {
	_, err := p.client.Users(ctx, codersdk.UsersRequest{
		Status: codersdk.UserStatusSuspended,
	})
	return err
}

// fetchTemplates fetches all templates.
func fetchTemplates(ctx context.Context, p *Params) error {
	templates, err := p.client.TemplatesByOrganization(ctx, p.me.OrganizationIDs[0])
	if err != nil {
		p.c.setTemplates(templates)
	}
	return err
}

// fetchTemplateBuild fetches a single template version at random.
func fetchTemplateVersion(ctx context.Context, p *Params) error {
	t := p.c.randTemplate()
	_, err := p.client.TemplateVersion(ctx, t.ActiveVersionID)
	return err
}

// fetchWorkspace fetches a single workspace at random.
func fetchWorkspace(ctx context.Context, p *Params) error {
	w := p.c.randWorkspace()
	_, err := p.client.WorkspaceByOwnerAndName(ctx, w.OwnerName, w.Name, codersdk.WorkspaceOptions{})
	return err
}

// fetchWorkspaceBuild fetches a single workspace build at random.
func fetchWorkspaceBuild(ctx context.Context, p *Params) error {
	w := p.c.randWorkspace()
	_, err := p.client.WorkspaceBuild(ctx, w.LatestBuild.ID)
	return err
}

// fetchTemplate fetches a single template at random.
func fetchTemplate(ctx context.Context, p *Params) error {
	t := p.c.randTemplate()
	_, err := p.client.Template(ctx, t.ID)
	return err
}

// fetchUserByID fetches a single user at random by ID.
func fetchUserByID(ctx context.Context, p *Params) error {
	u := p.c.randUser()
	_, err := p.client.User(ctx, u.ID.String())
	return err
}

// fetchUserByUsername fetches a single user at random by username.
func fetchUserByUsername(ctx context.Context, p *Params) error {
	u := p.c.randUser()
	_, err := p.client.User(ctx, u.Username)
	return err
}

// fetchDeploymentConfig fetches the deployment config.
func fetchDeploymentConfig(ctx context.Context, p *Params) error {
	_, err := p.client.DeploymentConfig(ctx)
	return err
}

// fetchWorkspaceQuotaForUser fetches the workspace quota for a random user.
func fetchWorkspaceQuotaForUser(ctx context.Context, p *Params) error {
	u := p.c.randUser()
	_, err := p.client.WorkspaceQuota(ctx, u.ID.String())
	return err
}

// fetchDeploymentStats fetches the deployment stats.
func fetchDeploymentStats(ctx context.Context, p *Params) error {
	_, err := p.client.DeploymentStats(ctx)
	return err
}

// fetchWorkspaceLogs fetches the logs for a random workspace.
func fetchWorkspaceLogs(ctx context.Context, p *Params) error {
	w := p.c.randWorkspace()
	ch, closer, err := p.client.WorkspaceBuildLogsAfter(ctx, w.LatestBuild.ID, 0)
	if err != nil {
		return err
	}
	defer func() {
		_ = closer.Close()
	}()
	// Drain the channel.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case l, ok := <-ch:
			if !ok {
				return nil
			}
			_ = l
		}
	}
}

// fetchAuditLog fetches the audit log.
// As not all users have access to the audit log, we check first.
func fetchAuditLog(ctx context.Context, p *Params) error {
	res, err := p.client.AuthCheck(ctx, codersdk.AuthorizationRequest{
		Checks: map[string]codersdk.AuthorizationCheck{
			"auditlog": {
				Object: codersdk.AuthorizationObject{
					ResourceType: codersdk.ResourceAuditLog,
				},
				Action: codersdk.ActionRead,
			},
		},
	})
	if err != nil {
		return err
	}
	if !res["auditlog"] {
		return nil // we are not authorized to read the audit log
	}

	// Fetch the first 25 audit log entries.
	_, err = p.client.AuditLogs(ctx, codersdk.AuditLogsRequest{
		Pagination: codersdk.Pagination{
			Offset: 0,
			Limit:  25,
		},
	})
	return err
}

// authCheckAsOwner performs an auth check as the owner of a random
// resource type and action.
func authCheckAsOwner(ctx context.Context, p *Params) error {
	_, err := p.client.AuthCheck(ctx, randAuthReq(
		ownedBy(p.me.ID),
		withAction(randAction()),
		withObjType(randObjectType()),
		inOrg(p.me.OrganizationIDs[0]),
	))
	return err
}

// authCheckAsNonOwner performs an auth check as a non-owner of a random
// resource type and action.
func authCheckAsNonOwner(ctx context.Context, p *Params) error {
	_, err := p.client.AuthCheck(ctx, randAuthReq(
		ownedBy(uuid.New()),
		withAction(randAction()),
		withObjType(randObjectType()),
		inOrg(p.me.OrganizationIDs[0]),
	))
	return err
}

// nolint: gosec
func randAuthReq(mut ...func(*codersdk.AuthorizationCheck)) codersdk.AuthorizationRequest {
	var check codersdk.AuthorizationCheck
	for _, m := range mut {
		m(&check)
	}
	return codersdk.AuthorizationRequest{
		Checks: map[string]codersdk.AuthorizationCheck{
			"check": check,
		},
	}
}

func ownedBy(myID uuid.UUID) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Object.OwnerID = myID.String()
	}
}

func inOrg(orgID uuid.UUID) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Object.OrganizationID = orgID.String()
	}
}

func withObjType(objType codersdk.RBACResource) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Object.ResourceType = objType
	}
}

func withAction(action string) func(check *codersdk.AuthorizationCheck) {
	return func(check *codersdk.AuthorizationCheck) {
		check.Action = action
	}
}

func randAction() string {
	return pick(codersdk.AllRBACActions)
}

func randObjectType() codersdk.RBACResource {
	return pick(codersdk.AllRBACResources)
}
