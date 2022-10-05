package database

import (
	"context"
	"encoding/json"
	"fmt"

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
	UpdateTemplateUserACLByID(ctx context.Context, id uuid.UUID, acl TemplateACL) error
	UpdateTemplateGroupACLByID(ctx context.Context, id uuid.UUID, acl TemplateACL) error
	GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]TemplateGroup, error)
	GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]TemplateUser, error)
}

type TemplateUser struct {
	User
	Actions Actions `db:"actions"`
}

func (q *sqlQuerier) UpdateTemplateUserACLByID(ctx context.Context, id uuid.UUID, acl TemplateACL) error {
	raw, err := json.Marshal(acl)
	if err != nil {
		return xerrors.Errorf("marshal user acl: %w", err)
	}

	const query = `
UPDATE
	templates
SET
	user_acl = $2
WHERE
	id = $1`

	_, err = q.db.ExecContext(ctx, query, id.String(), raw)
	if err != nil {
		return xerrors.Errorf("update user acl: %w", err)
	}

	return nil
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

func (q *sqlQuerier) UpdateTemplateGroupACLByID(ctx context.Context, id uuid.UUID, acl TemplateACL) error {
	raw, err := json.Marshal(acl)
	if err != nil {
		return xerrors.Errorf("marshal user acl: %w", err)
	}

	const query = `
UPDATE
	templates
SET
	group_acl = $2
WHERE
	id = $1`

	_, err = q.db.ExecContext(ctx, query, id.String(), raw)
	if err != nil {
		return xerrors.Errorf("update user acl: %w", err)
	}

	return nil
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
}

// GetAuthorizedWorkspaces returns all workspaces that the user is authorized to access.
// This code is copied from `GetWorkspaces` and adds the authorized filter WHERE
// clause.
func (q *sqlQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg GetWorkspacesParams, authorizedFilter rbac.AuthorizeFilter) ([]Workspace, error) {
	// The name comment is for metric tracking
	query := fmt.Sprintf("-- name: GetAuthorizedWorkspaces :many\n%s AND %s", getWorkspaces, authorizedFilter.SQLString(rbac.NoACLConfig()))
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
