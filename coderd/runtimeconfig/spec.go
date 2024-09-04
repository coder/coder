package runtimeconfig

import (
	"context"
)

type Initializer interface {
	Initialize(name string)
}

type Manager interface {
	// GetRuntimeSetting gets a runtime setting by name.
	GetRuntimeSetting(ctx context.Context, name string) (string, error)
	// UpsertRuntimeSetting upserts a runtime setting by name.
	UpsertRuntimeSetting(ctx context.Context, name, val string) error
	// DeleteRuntimeSetting deletes a runtime setting by name.
	DeleteRuntimeSetting(ctx context.Context, name string) error
	// Scoped returns a new Manager which is responsible for namespacing all runtime keys during CRUD operations.
	// This can be used for scoping runtime settings to organizations, for example.
	Scoped(ns string) Manager
}
