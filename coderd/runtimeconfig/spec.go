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

type Manager interface {
	Resolver
	Mutator
}

type NoopManager struct {}

func NewNoopManager() *NoopManager {
	return &NoopManager{}
}

func (n NoopManager) GetRuntimeSetting(context.Context, string) (string, error) {
	return "", EntryNotFound
}

func (n NoopManager) UpsertRuntimeSetting(context.Context, string, string) error {
	return EntryNotFound
}

func (n NoopManager) DeleteRuntimeSetting(context.Context, string) error {
	return EntryNotFound
}
