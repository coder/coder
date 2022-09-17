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
	UpdateTemplateUserACLByID(ctx context.Context, id uuid.UUID, acl UserACL) error
}

type TemplateUser struct {
	User
	Role TemplateRole `db:"role"`
}

func (q *sqlQuerier) UpdateTemplateUserACLByID(ctx context.Context, id uuid.UUID, acl UserACL) error {
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
