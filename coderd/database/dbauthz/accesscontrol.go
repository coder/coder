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
		// An expired license
		RequireActiveVersion: false,
		// AGPL cannot set deprecated templates, but it should return
		// existing deprecated templates. This is erroring on the safe side
		// if a license expires, we should not allow deprecated templates
		// to be used for new workspaces.
		Deprecated: t.Deprecated,
	}
}

func (AGPLTemplateAccessControlStore) SetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID, opts TemplateAccessControl) error {
	// This does require fetching again to ensure other fields are not changed.
	tpl, err := store.GetTemplateByID(ctx, id)
	if err != nil {
		return xerrors.Errorf("get template: %w", err)
	}

	// AGPL is allowed to unset deprecated templates, anything else is an error
	if opts.Deprecated != "" && tpl.Deprecated != opts.Deprecated {
		return xerrors.Errorf("enterprise license required for deprecation_message")
	}

	// AGPL is allowed to disable require_active_version, anything else is an error
	if opts.RequireActiveVersion && tpl.RequireActiveVersion != opts.RequireActiveVersion {
		return xerrors.Errorf("enterprise license required for require_active_version")
	}

	if opts.Deprecated != tpl.Deprecated || opts.RequireActiveVersion != tpl.RequireActiveVersion {
		err := store.UpdateTemplateAccessControlByID(ctx, database.UpdateTemplateAccessControlByIDParams{
			ID:                   id,
			RequireActiveVersion: opts.RequireActiveVersion,
			Deprecated:           opts.Deprecated,
		})
		if err != nil {
			return xerrors.Errorf("update template access control: %w", err)
		}
	}

	return nil
}
