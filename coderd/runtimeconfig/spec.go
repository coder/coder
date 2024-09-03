package runtimeconfig

import "context"

// Resolver is an interface for resolving runtime settings.
type Resolver interface {
	// GetRuntimeSetting gets a runtime setting by key.
	GetRuntimeSetting(ctx context.Context, key string) (string, error)
}

// Mutator is an interface for mutating runtime settings.
type Mutator interface {
	// UpsertRuntimeSetting upserts a runtime setting by key.
	UpsertRuntimeSetting(ctx context.Context, key, val string) error
	// DeleteRuntimeSetting deletes a runtime setting by key.
	DeleteRuntimeSetting(ctx context.Context, key string) error
}
