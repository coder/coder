package runtimeconfig

import (
	"context"
	"errors"
	"reflect"

	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
)

var ErrKeyNotSet = xerrors.New("key is not set")

// TODO: comment
type Value pflag.Value

type Entry[T Value] struct {
	k string
	v T
}

func New[T Value](key, val string) (out Entry[T], err error) {
	out.k = key

	if err = out.SetStartupValue(val); err != nil {
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

func (e *Entry[T]) val() T {
	if reflect.ValueOf(e.v).IsNil() {
		e.v = create[T]()
	}
	return e.v
}

func (e *Entry[T]) key() (string, error) {
	if e.k == "" {
		return "", ErrKeyNotSet
	}

	return e.k, nil
}

func (e *Entry[T]) SetKey(k string) {
	e.k = k
}

func (e *Entry[T]) SetStartupValue(s string) error {
	return e.val().Set(s)
}

func (e *Entry[T]) MustSet(s string) {
	err := e.val().Set(s)
	if err != nil {
		panic(err)
	}
}

func (e *Entry[T]) Type() string {
	return e.val().Type()
}

func (e *Entry[T]) String() string {
	return e.val().String()
}

func (e *Entry[T]) StartupValue() T {
	return e.val()
}

func (e *Entry[T]) SetRuntimeValue(ctx context.Context, m Mutator, val T) error {
	key, err := e.key()
	if err != nil {
		return err
	}

	return m.MutateByKey(ctx, key, val.String())
}

func (e *Entry[T]) Resolve(ctx context.Context, r Resolver) (T, error) {
	var zero T

	key, err := e.key()
	if err != nil {
		return zero, err
	}

	val, err := r.ResolveByKey(ctx, key)
	if err != nil {
		return zero, err
	}

	inst := create[T]()
	if err = inst.Set(val); err != nil {
		return zero, xerrors.Errorf("instantiate new %T: %w", inst, err)
	}
	return inst, nil
}

func (e *Entry[T]) Coalesce(ctx context.Context, r Resolver) (T, error) {
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
