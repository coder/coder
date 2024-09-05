package runtimeconfig

import (
	"context"
	"fmt"

	"golang.org/x/xerrors"
)

var ErrNameNotSet = xerrors.New("name is not set")

type EntryMarshaller interface {
	fmt.Stringer
}

type EntryValue interface {
	EntryMarshaller
	Set(string) error
}

// RuntimeEntry are **only** runtime configurable. They are stored in the
// database, and have no startup value or default value.
type RuntimeEntry[T EntryValue] struct {
	n string
}

// New creates a new T instance with a defined name and value.
func New[T EntryValue](name string) (out RuntimeEntry[T], err error) {
	out.n = name
	if name == "" {
		return out, ErrNameNotSet
	}

	return out, nil
}

// MustNew is like New but panics if an error occurs.
func MustNew[T EntryValue](name string) RuntimeEntry[T] {
	out, err := New[T](name)
	if err != nil {
		panic(err)
	}
	return out
}

// SetRuntimeValue attempts to update the runtime value of this field in the store via the given Mutator.
func (e *RuntimeEntry[T]) SetRuntimeValue(ctx context.Context, m Manager, val T) error {
	name, err := e.name()
	if err != nil {
		return err
	}

	return m.UpsertRuntimeSetting(ctx, name, val.String())
}

// UnsetRuntimeValue removes the runtime value from the store.
func (e *RuntimeEntry[T]) UnsetRuntimeValue(ctx context.Context, m Manager) error {
	name, err := e.name()
	if err != nil {
		return err
	}

	return m.DeleteRuntimeSetting(ctx, name)
}

// Resolve attempts to resolve the runtime value of this field from the store via the given Resolver.
func (e *RuntimeEntry[T]) Resolve(ctx context.Context, r Manager) (T, error) {
	var zero T

	name, err := e.name()
	if err != nil {
		return zero, err
	}

	val, err := r.GetRuntimeSetting(ctx, name)
	if err != nil {
		return zero, err
	}

	inst := create[T]()
	if err = inst.Set(val); err != nil {
		return zero, xerrors.Errorf("instantiate new %T: %w", inst, err)
	}
	return inst, nil
}

// name returns the configured name, or fails with ErrNameNotSet.
func (e *RuntimeEntry[T]) name() (string, error) {
	if e.n == "" {
		return "", ErrNameNotSet
	}

	return e.n, nil
}
