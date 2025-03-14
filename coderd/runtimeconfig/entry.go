package runtimeconfig
import (
	"errors"
	"context"
	"encoding/json"
	"fmt"
)
// EntryMarshaller requires all entries to marshal to and from a string.
// The final store value is a database `text` column.
// This also is compatible with serpent values.
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
func (e RuntimeEntry[T]) SetRuntimeValue(ctx context.Context, m Resolver, val T) error {
	name, err := e.name()
	if err != nil {
		return fmt.Errorf("set runtime: %w", err)
	}
	return m.UpsertRuntimeConfig(ctx, name, val.String())
}
// UnsetRuntimeValue removes the runtime value from the store.
func (e RuntimeEntry[T]) UnsetRuntimeValue(ctx context.Context, m Resolver) error {
	name, err := e.name()
	if err != nil {
		return fmt.Errorf("unset runtime: %w", err)
	}
	return m.DeleteRuntimeConfig(ctx, name)
}
// Resolve attempts to resolve the runtime value of this field from the store via the given Resolver.
func (e RuntimeEntry[T]) Resolve(ctx context.Context, r Resolver) (T, error) {
	var zero T
	name, err := e.name()
	if err != nil {
		return zero, fmt.Errorf("resolve, name issue: %w", err)
	}
	val, err := r.GetRuntimeConfig(ctx, name)
	if err != nil {
		return zero, fmt.Errorf("resolve runtime: %w", err)
	}
	inst := create[T]()
	if err = inst.Set(val); err != nil {
		return zero, fmt.Errorf("instantiate new %T: %w", inst, err)
	}
	return inst, nil
}
// name returns the configured name, or fails with ErrNameNotSet.
func (e RuntimeEntry[T]) name() (string, error) {
	if e.n == "" {
		return "", ErrNameNotSet
	}
	return e.n, nil
}
func JSONString(v any) string {
	s, err := json.Marshal(v)
	if err != nil {
		return "decode failed: " + err.Error()
	}
	return string(s)
}
