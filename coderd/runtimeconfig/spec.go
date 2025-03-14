package runtimeconfig

import (
	"errors"
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

var (
	// ErrEntryNotFound is returned when a runtime entry is not saved in the
	// store. It is essentially a 'sql.ErrNoRows'.

	ErrEntryNotFound = errors.New("entry not found")
	// ErrNameNotSet is returned when a runtime entry is created without a name.
	// This is more likely to happen on DeploymentEntry that has not called
	// Initialize().
	ErrNameNotSet = errors.New("name is not set")
)
type Initializer interface {
	Initialize(name string)
}
type Resolver interface {

	// GetRuntimeConfig gets a runtime setting by name.
	GetRuntimeConfig(ctx context.Context, name string) (string, error)
	// UpsertRuntimeConfig upserts a runtime setting by name.
	UpsertRuntimeConfig(ctx context.Context, name, val string) error

	// DeleteRuntimeConfig deletes a runtime setting by name.
	DeleteRuntimeConfig(ctx context.Context, name string) error
}
// Store is a subset of database.Store
type Store interface {
	GetRuntimeConfig(ctx context.Context, key string) (string, error)
	UpsertRuntimeConfig(ctx context.Context, arg database.UpsertRuntimeConfigParams) error
	DeleteRuntimeConfig(ctx context.Context, key string) error
}
