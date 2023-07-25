package dashboard

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/codersdk"
)

// rollTable is a table of actions to perform.
// D&D nerds will feel right at home here :-)
// Note that the order of the table is important!
// Entries must be in ascending order.
var allActions rollTable = []rollTableEntry{
	{0, fetchWorkspaces, "fetch workspaces"},
	{10, fetchUsers, "fetch users"},
	{20, fetchTemplates, "fetch templates"},
	{30, authCheckAsOwner, "authcheck owner"},
	{40, authCheckAsNonOwner, "authcheck not owner"},
	{50, fetchAuditLog, "fetch audit log"},
}

// rollTable is a slice of rollTableEntry.
type rollTable []rollTableEntry

// rollTableEntry is an entry in the roll table.
type rollTableEntry struct {
	// roll is the minimum number required to perform the action.
	roll int
	// fn is the function to call.
	fn func(ctx context.Context, p *params) error
	// label is used for logging.
	label string
}

// choose returns the first entry in the table that is greater than or equal to n.
func (r rollTable) choose(n int) rollTableEntry {
	for _, entry := range r {
		if entry.roll >= n {
			return entry
		}
	}
	return rollTableEntry{}
}

// max returns the maximum roll in the table.
// Important: this assumes that the table is sorted in ascending order.
func (r rollTable) max() int {
	return r[len(r)-1].roll
}

// params is a set of parameters to pass to the actions in a rollTable.
type params struct {
	// client is the client to use for performing the action.
	client *codersdk.Client
	// me is the currently authenticated user. Lots of actions require this.
	me codersdk.User
}

// fetchWorkspaces fetches all workspaces.
func fetchWorkspaces(ctx context.Context, p *params) error {
	_, err := p.client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	return err
}

// fetchUsers fetches all users.
func fetchUsers(ctx context.Context, p *params) error {
	_, err := p.client.Users(ctx, codersdk.UsersRequest{})
	return err
}

// fetchTemplates fetches all templates.
func fetchTemplates(ctx context.Context, p *params) error {
	_, err := p.client.TemplatesByOrganization(ctx, p.me.OrganizationIDs[0])
	return err
}

// fetchAuditLog fetches the audit log.
// As not all users have access to the audit log, we check first.
func fetchAuditLog(ctx context.Context, p *params) error {
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
func authCheckAsOwner(ctx context.Context, p *params) error {
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
func authCheckAsNonOwner(ctx context.Context, p *params) error {
	_, err := p.client.AuthCheck(ctx, randAuthReq(
		ownedBy(uuid.New()),
		withAction(randAction()),
		withObjType(randObjectType()),
		inOrg(p.me.OrganizationIDs[0]),
	))
	return err
}
