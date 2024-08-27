package runtimeconfig

import (
	"context"
	"errors"
	"reflect"

	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
)

// TODO: comment
type Value pflag.Value

type Entry[T Value] struct {
	val T
	key string
}

func New[T Value](key, val string) (out Entry[T], err error) {
	out.Init(key)

	if err = out.Set(val); err != nil {
		return out, err
	}

	return out, nil
}

func MustNew[T Value](key, val string) Entry[T] {
	out, err := New[T](key, val)
	if err != nil {
		panic(err)
	}
	return out
}

func (o *Entry[T]) Init(key string) {
	o.val = create[T]()
	o.key = key
}

func (o *Entry[T]) Set(s string) error {
	if reflect.ValueOf(o.val).IsNil() {
		return xerrors.Errorf("instance of %T is uninitialized", o.val)
	}
	return o.val.Set(s)
}

func (o *Entry[T]) Type() string {
	return o.val.Type()
}

func (o *Entry[T]) String() string {
	return o.val.String()
}

func (o *Entry[T]) StartupValue() T {
	return o.val
}

func (o *Entry[T]) Resolve(r Resolver) (T, error) {
	return o.resolve(r)
}

func (o *Entry[T]) resolve(r Resolver) (T, error) {
	var zero T

	val, err := r.ResolveByKey(nil, o.key)
	if err != nil {
		return zero, err
	}

	inst := create[T]()
	if err = inst.Set(val); err != nil {
		return zero, xerrors.Errorf("instantiate new %T: %w", inst, err)
	}
	return inst, nil
}

func (o *Entry[T]) Save(ctx context.Context, m Mutator, val T) error {
	return m.MutateByKey(ctx, o.key, val.String())
}

func (o *Entry[T]) Coalesce(r Resolver) (T, error) {
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
