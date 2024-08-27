package config

type RuntimeConfigInitializer interface {
	Init(key string)
}

type RuntimeConfigResolver[T Value] interface {
	StartupValue() T
	Resolve(r Resolver) (T, error)
	Coalesce(r Resolver) (T, error)
}

type RuntimeConfigMutator[T Value] interface {
	Save(m Mutator, val T) error
}

type Resolver interface {
	ResolveByKey(key string) (string, error)
}

type Mutator interface {
	MutateByKey(key, val string) error
}