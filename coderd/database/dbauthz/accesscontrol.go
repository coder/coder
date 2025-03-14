package dbauthz

import (
	"fmt"
	"errors"

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
func (AGPLTemplateAccessControlStore) SetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID, opts TemplateAccessControl) error {
	// AGPL is allowed to unset deprecated templates.
	if opts.Deprecated == "" {
		// This does require fetching again to ensure other fields are not
		// changed.
		tpl, err := store.GetTemplateByID(ctx, id)
		if err != nil {
			return fmt.Errorf("get template: %w", err)

		}
		if tpl.Deprecated != "" {
			err := store.UpdateTemplateAccessControlByID(ctx, database.UpdateTemplateAccessControlByIDParams{
				ID:                   id,
				RequireActiveVersion: tpl.RequireActiveVersion,
				Deprecated:           opts.Deprecated,
			})
			if err != nil {
				return fmt.Errorf("update template access control: %w", err)
			}

		}
	}
	return nil
}
