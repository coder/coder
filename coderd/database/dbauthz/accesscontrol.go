package dbauthz

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// AccessControlStore fetches access control-related configuration
// that is used when determining whether an actor is authorized
// to interact with an RBAC object.
type AccessControlStore interface {
	GetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID) (TemplateAccessControl, error)
	SetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID, opts TemplateAccessControl) error
}

type TemplateAccessControl struct {
	RequireActiveVersion bool
}

// AGPLTemplateAccessControlStore always returns the defaults for access control
// settings.
type AGPLTemplateAccessControlStore struct{}

var _ AccessControlStore = AGPLTemplateAccessControlStore{}

func (AGPLTemplateAccessControlStore) GetTemplateAccessControl(context.Context, database.Store, uuid.UUID) (TemplateAccessControl, error) {
	return TemplateAccessControl{
		RequireActiveVersion: false,
	}, nil
}

func (AGPLTemplateAccessControlStore) SetTemplateAccessControl(context.Context, database.Store, uuid.UUID, TemplateAccessControl) error {
	return nil
}

// TODO (JonA): This is kind of a no-no since enterprise shouldn't leak into
// the AGPL implementation.
type EnterpriseTemplateAccessControlStore struct{}

func (EnterpriseTemplateAccessControlStore) GetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID) (TemplateAccessControl, error) {
	t, err := store.GetTemplateByID(ctx, id)
	if err != nil {
		return TemplateAccessControl{}, xerrors.Errorf("get template: %w", err)
	}
	return TemplateAccessControl{
		RequireActiveVersion: t.RequireActiveVersion,
	}, nil
}

func (EnterpriseTemplateAccessControlStore) SetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID, opts TemplateAccessControl) error {
	err := store.UpdateTemplateAccessControlByID(ctx, database.UpdateTemplateAccessControlByIDParams{
		ID:                   id,
		RequireActiveVersion: opts.RequireActiveVersion,
	})
	if err != nil {
		return xerrors.Errorf("update template access control: %w", err)
	}
	return nil
}
