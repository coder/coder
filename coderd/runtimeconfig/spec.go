package runtimeconfig

import "context"

type Initializer interface {
	Initialize(name string)
}

// Resolver is an interface for resolving runtime settings.
type Resolver interface {
	// GetRuntimeSetting gets a runtime setting by name.
	GetRuntimeSetting(ctx context.Context, name string) (string, error)
}

// Mutator is an interface for mutating runtime settings.
type Mutator interface {
	// UpsertRuntimeSetting upserts a runtime setting by name.
	UpsertRuntimeSetting(ctx context.Context, name, val string) error
	// DeleteRuntimeSetting deletes a runtime setting by name.
	DeleteRuntimeSetting(ctx context.Context, name string) error
}
