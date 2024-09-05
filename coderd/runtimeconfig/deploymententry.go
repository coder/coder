package runtimeconfig

import (
	"context"
	"errors"
	"reflect"

	"github.com/spf13/pflag"
)

// Ensure serpent values satisfy the ConfigValue interface for easier usage.
var _ pflag.Value = SerpentEntry(nil)
var _ pflag.Value = &DeploymentEntry[SerpentEntry]{}

type SerpentEntry interface {
	EntryValue
	Type() string
}

// DeploymentEntry extends a runtime entry with a startup value.
// This allows for a single entry to source its value from startup or runtime.
type DeploymentEntry[T SerpentEntry] struct {
	RuntimeEntry[T]
	startupValue T
}

// Initialize sets the entry's name, and initializes the value.
func (e *DeploymentEntry[T]) Initialize(name string) {
	e.n = name
	e.val()
}

// SetStartupValue sets the value of the wrapped field. This ONLY sets the value locally, not in the store.
// See SetRuntimeValue.
func (e *DeploymentEntry[T]) SetStartupValue(s string) error {
	return e.val().Set(s)
}

// StartupValue returns the wrapped type T which represents the state as of the definition of this Entry.
// This function would've been named Value, but this conflicts with a field named Value on some implementations of T in
// the serpent library; plus it's just more clear.
func (e *DeploymentEntry[T]) StartupValue() T {
	return e.val()
}

// Coalesce attempts to resolve the runtime value of this field from the store via the given Manager. Should no runtime
// value be found, the startup value will be used.
func (e *DeploymentEntry[T]) Coalesce(ctx context.Context, r Resolver) (T, error) {
	var zero T

	resolved, err := e.Resolve(ctx, r)
	if err != nil {
		if errors.Is(err, EntryNotFound) {
			return e.StartupValue(), nil
		}
		return zero, err
	}

	return resolved, nil
}

// Functions to implement pflag.Value for serpent usage

// Set is an alias of SetStartupValue. Implemented to match the serpent interface
// such that we can use this Go type in the OptionSet.
func (e *DeploymentEntry[T]) Set(s string) error {
	return e.SetStartupValue(s)
}

// Type returns the wrapped value's type.
func (e *DeploymentEntry[T]) Type() string {
	return e.val().Type()
}

// String returns the wrapper value's string representation.
func (e *DeploymentEntry[T]) String() string {
	return e.val().String()
}

// val fronts the T value in the struct, and initializes it should the value be nil.
func (e *DeploymentEntry[T]) val() T {
	if reflect.ValueOf(e.startupValue).IsNil() {
		e.startupValue = create[T]()
	}
	return e.startupValue
}
