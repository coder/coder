package runtimeconfig

import "context"

type Initializer interface {
	Init(key string)
}

type Resolver interface {
	ResolveByKey(ctx context.Context, key string) (string, error)
}

type Mutator interface {
	MutateByKey(ctx context.Context, key, val string) error
}