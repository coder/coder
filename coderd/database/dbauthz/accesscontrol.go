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
	GetTemplateAccessControl(t database.Template) TemplateAccessControl
	SetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID, opts TemplateAccessControl) error
}

type TemplateAccessControl struct {
	RequireActiveVersion bool
	Deprecated           string
}

func (t TemplateAccessControl) IsDeprecated() bool {
	return t.Deprecated != ""
}

// AGPLTemplateAccessControlStore always returns the defaults for access control
// settings.
type AGPLTemplateAccessControlStore struct{}

var _ AccessControlStore = AGPLTemplateAccessControlStore{}

func (AGPLTemplateAccessControlStore) GetTemplateAccessControl(t database.Template) TemplateAccessControl {
	return TemplateAccessControl{
		RequireActiveVersion: false,
		// AGPL cannot set deprecated templates, but it should return
		// existing deprecated templates. This is erroring on the safe side
		// if a license expires, we should not allow deprecated templates
		// to be used for new workspaces.
		Deprecated: t.Deprecated,
	}
}

func (AGPLTemplateAccessControlStore) SetTemplateAccessControl(context.Context, database.Store, uuid.UUID, TemplateAccessControl) error {
	return nil
}
