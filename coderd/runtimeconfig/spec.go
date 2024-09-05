package runtimeconfig

import (
	"context"

	"github.com/google/uuid"
)

type Initializer interface {
	Initialize(name string)
}

// TODO: We should probably remove the manager interface and only support
// 1 implementation.
type Manager interface {
	DeploymentResolver(db Store) Resolver
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
