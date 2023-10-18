package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// AccessControlStore fetches access control-related configuration
// that is used when determining whether an actor is authorized
// to interact with an RBAC object.
type AccessControlStore interface {
	GetTemplateAccessControl(t database.Template) (TemplateAccessControl, error)
	SetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID, opts TemplateAccessControl) error
}

type TemplateAccessControl struct {
	RequireActiveVersion bool
}

// AGPLTemplateAccessControlStore always returns the defaults for access control
// settings.
type AGPLTemplateAccessControlStore struct{}

var _ AccessControlStore = AGPLTemplateAccessControlStore{}

func (AGPLTemplateAccessControlStore) GetTemplateAccessControl(database.Template) (TemplateAccessControl, error) {
	return TemplateAccessControl{
		RequireActiveVersion: false,
	}, nil
}

func (AGPLTemplateAccessControlStore) SetTemplateAccessControl(context.Context, database.Store, uuid.UUID, TemplateAccessControl) error {
	return nil
}
