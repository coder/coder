package runtimeconfig

import "context"

type Initializer interface {
	Init(key string)
}

type Resolver interface {
	GetRuntimeSetting(ctx context.Context, key string) (string, error)
}

type Mutator interface {
	UpsertRuntimeSetting(ctx context.Context, key, val string) error
	DeleteRuntimeSetting(ctx context.Context, key string) error
}
