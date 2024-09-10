package runtimeconfig

import (
	"reflect"
)

func create[T any]() T {
	var zero T
	//nolint:forcetypeassert
	return reflect.New(reflect.TypeOf(zero).Elem()).Interface().(T)
}
