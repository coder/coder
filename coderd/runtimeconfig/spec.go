package runtimeconfig

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

var (
	// ErrEntryNotFound is returned when a runtime entry is not saved in the
	// store. It is essentially a 'sql.ErrNoRows'.
	ErrEntryNotFound = xerrors.New("entry not found")
	// ErrNameNotSet is returned when a runtime entry is created without a name.
	// This is more likely to happen on DeploymentEntry that has not called
	// Initialize().
	ErrNameNotSet = xerrors.New("name is not set")
)

type Initializer interface {
	Initialize(name string)
}

// TODO: We should probably remove the manager interface and only support
// 1 implementation.
type Manager interface {
	Resolver(db Store) Resolver
	OrganizationResolver(db Store, orgID uuid.UUID) Resolver
}

type Resolver interface {
	// GetRuntimeSetting gets a runtime setting by name.
	GetRuntimeSetting(ctx context.Context, name string) (string, error)
	// UpsertRuntimeSetting upserts a runtime setting by name.
	UpsertRuntimeSetting(ctx context.Context, name, val string) error
	// DeleteRuntimeSetting deletes a runtime setting by name.
	DeleteRuntimeSetting(ctx context.Context, name string) error
}

// Store is a subset of database.Store
type Store interface {
	GetRuntimeConfig(ctx context.Context, key string) (string, error)
	UpsertRuntimeConfig(ctx context.Context, arg database.UpsertRuntimeConfigParams) error
	DeleteRuntimeConfig(ctx context.Context, key string) error
}
