package database

import (
	"context"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"

	"github.com/lib/pq"
)

// AuthorizedGetWorkspaces returns all workspaces that the user is authorized to access.
func (q *sqlQuerier) AuthorizedGetWorkspaces(ctx context.Context, arg GetWorkspacesParams, authorizedFilter rbac.AuthorizeFilter) ([]Workspace, error) {
	query := fmt.Sprintf("%s AND %s", getWorkspaces, authorizedFilter.SQLString(rbac.SQLConfig{
		VariableRenames: map[string]string{
			"input.object.org_owner": "organization_id::text",
			"input.object.owner":     "owner_id::text",
		},
	}))
	rows, err := q.db.QueryContext(ctx, query,
		arg.Deleted,
		arg.OwnerID,
		arg.OwnerUsername,
		arg.TemplateName,
		pq.Array(arg.TemplateIds),
		arg.Name,
	)
	if err != nil {
		return nil, xerrors.Errorf("get authorized workspaces: %w", err)
	}
	defer rows.Close()
	var items []Workspace
	for rows.Next() {
		var i Workspace
		if err := rows.Scan(
			&i.ID,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.OwnerID,
			&i.OrganizationID,
			&i.TemplateID,
			&i.Deleted,
			&i.Name,
			&i.AutostartSchedule,
			&i.Ttl,
			&i.LastUsedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
