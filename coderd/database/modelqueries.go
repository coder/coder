package database

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// customQuerier encompasses all non-generated queries.
// It provides a flexible way to write queries for cases
// where sqlc proves inadequate.
type customQuerier interface {
	templateQuerier
}

type templateQuerier interface {
	UpdateTemplateUserACLByID(ctx context.Context, id uuid.UUID, acl ACL) error
	UpdateTemplateGroupACLByID(ctx context.Context, id uuid.UUID, acl ACL) error
	GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]TemplateGroup, error)
	GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]TemplateUser, error)
}

type TemplateUser struct {
	User
	Role TemplateRole `db:"role"`
}

func (q *sqlQuerier) UpdateTemplateUserACLByID(ctx context.Context, id uuid.UUID, acl ACL) error {
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
		perms.value as role, users.*
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
		users.id::text = perms.key;
	`

	var tus []TemplateUser
	err := q.db.SelectContext(ctx, &tus, query, id.String())
	if err != nil {
		return nil, xerrors.Errorf("select context: %w", err)
	}

	return tus, nil
}

type TemplateGroup struct {
	Group
	Role TemplateRole
}

func (q *sqlQuerier) UpdateTemplateGroupACLByID(ctx context.Context, id uuid.UUID, acl ACL) error {
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
		perms.value as role, group.*
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
							templates.group
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
		return nil, xerrors.Errorf("select context: %w", err)
	}

	return tgs, nil
}
