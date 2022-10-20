package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// customQuerier encompasses all non-generated queries.
// It provides a flexible way to write queries for cases
// where sqlc proves inadequate.
type customQuerier interface {
	templateQuerier
	workspaceQuerier
}

type templateQuerier interface {
	GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]TemplateGroup, error)
	GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]TemplateUser, error)
}

type TemplateUser struct {
	User
	Actions Actions `db:"actions"`
}

func (q *sqlQuerier) GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]TemplateUser, error) {
	const query = `
	SELECT
		perms.value as actions, users.*
	FROM
		users
	JOIN
		(
			SELECT
				*
			FROM
				jsonb_each_text(
					(
						SELECT
							templates.user_acl
						FROM
							templates
						WHERE
							id = $1
					)
				)
		) AS perms
	ON
		users.id::text = perms.key
	WHERE
		users.deleted = false
	AND
		users.status = 'active';
	`

	var tus []TemplateUser
	err := q.db.SelectContext(ctx, &tus, query, id.String())
	if err != nil {
		return nil, xerrors.Errorf("select user actions: %w", err)
	}

	return tus, nil
}

type TemplateGroup struct {
	Group
	Actions Actions `db:"actions"`
}

func (q *sqlQuerier) GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]TemplateGroup, error) {
	const query = `
	SELECT
		perms.value as actions, groups.*
	FROM
		groups
	JOIN
		(
			SELECT
				*
			FROM
				jsonb_each_text(
					(
						SELECT
							templates.group_acl
						FROM
							templates
						WHERE
							id = $1
					)
				)
		) AS perms
	ON
		groups.id::text = perms.key;
	`

	var tgs []TemplateGroup
	err := q.db.SelectContext(ctx, &tgs, query, id.String())
	if err != nil {
		return nil, xerrors.Errorf("select group roles: %w", err)
	}

	return tgs, nil
}

type workspaceQuerier interface {
	GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, authorizedFilter rbac.AuthorizeFilter) ([]Workspace, error)
	GetAuthorizedWorkspaceCount(ctx context.Context, arg GetWorkspaceCountParams, authorizedFilter rbac.AuthorizeFilter) (int64, error)
}

// GetAuthorizedWorkspaces returns all workspaces that the user is authorized to access.
// This code is copied from `GetWorkspaces` and adds the authorized filter WHERE
// clause.
func (q *sqlQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, authorizedFilter rbac.AuthorizeFilter) ([]Workspace, error) {
	// In order to properly use ORDER BY, OFFSET, and LIMIT, we need to inject the
	// authorizedFilter between the end of the where clause and those statements.
	filter := strings.Replace(getWorkspaces, "-- @authorize_filter", fmt.Sprintf(" AND %s", authorizedFilter.SQLString(rbac.NoACLConfig())), 1)
	// The name comment is for metric tracking
	query := fmt.Sprintf("-- name: GetAuthorizedWorkspaces :many\n%s", filter)
	rows, err := q.db.QueryContext(ctx, query,
		arg.Deleted,
		arg.Status,
		arg.OwnerID,
		arg.OwnerUsername,
		arg.TemplateName,
		pq.Array(arg.TemplateIds),
		arg.Name,
		arg.Offset,
		arg.Limit,
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

func (q *sqlQuerier) GetAuthorizedWorkspaceCount(ctx context.Context, arg GetWorkspaceCountParams, authorizedFilter rbac.AuthorizeFilter) (int64, error) {
	// In order to properly use ORDER BY, OFFSET, and LIMIT, we need to inject the
	// authorizedFilter between the end of the where clause and those statements.
	filter := strings.Replace(getWorkspaceCount, "-- @authorize_filter", fmt.Sprintf(" AND %s", authorizedFilter.SQLString(rbac.NoACLConfig())), 1)
	// The name comment is for metric tracking
	query := fmt.Sprintf("-- name: GetAuthorizedWorkspaceCount :one\n%s", filter)
	row := q.db.QueryRowContext(ctx, query,
		arg.Deleted,
		arg.Status,
		arg.OwnerID,
		arg.OwnerUsername,
		arg.TemplateName,
		pq.Array(arg.TemplateIds),
		arg.Name,
	)
	var count int64
	err := row.Scan(&count)
	return count, err
}
