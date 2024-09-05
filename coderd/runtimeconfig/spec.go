package runtimeconfig

import (
	"context"
)

type Initializer interface {
	Initialize(name string)
}

// Manager is just a factory to produce Resolvers.
// The reason a factory is required, is the Manager can act as a caching
// layer for runtime settings.
type Manager interface {
	// DeploymentResolver returns a Resolver scoped to the deployment.
	DeploymentResolver(db Store) Resolver
	// OrganizationResolver returns a Resolver scoped to the organization.
	OrganizationResolver(db Store, orgID string) Resolver
}

type Resolver interface {
	// GetRuntimeSetting gets a runtime setting by name.
	GetRuntimeSetting(ctx context.Context, name string) (string, error)
	// UpsertRuntimeSetting upserts a runtime setting by name.
	UpsertRuntimeSetting(ctx context.Context, name, val string) error
	// DeleteRuntimeSetting deletes a runtime setting by name.
	DeleteRuntimeSetting(ctx context.Context, name string) error
}
