package config

import (
	"errors"
	"reflect"

	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
)

// TODO: comment
type Value pflag.Value

type Runtime[T Value] struct {
	val T
	key string
}

func (o *Runtime[T]) Init(key string) {
	o.val = create[T]()
	o.key = key
}

func (o *Runtime[T]) Set(s string) error {
	if reflect.ValueOf(o.val).IsNil() {
		return xerrors.Errorf("instance of %T is uninitialized", o.val)
	}
	return o.val.Set(s)
}

func (o *Runtime[T]) Type() string {
	return o.val.Type()
}

func (o *Runtime[T]) String() string {
	return o.val.String()
}

func (o *Runtime[T]) StartupValue() T {
	return o.val
}

func (o *Runtime[T]) Resolve(r Resolver) (T, error) {
	return o.resolve(r)
}

func (o *Runtime[T]) resolve(r Resolver) (T, error) {
	var zero T

	val, err := r.ResolveByKey(o.key)
	if err != nil {
		return zero, err
	}

	inst := create[T]()
	if err = inst.Set(val); err != nil {
		return zero, xerrors.Errorf("instantiate new %T: %w", inst, err)
	}
	return inst, nil
}

func (o *Runtime[T]) Save(m Mutator, val T) error {
	return m.MutateByKey(o.key, val.String())
}

func (o *Runtime[T]) Coalesce(r Resolver) (T, error) {
	var zero T
	resolved, err := o.resolve(r)
	if err != nil {
		if errors.Is(err, EntryNotFound) {
			return o.StartupValue(), nil
		}
		return zero, err
	}

	return resolved, nil
}
