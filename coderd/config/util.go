package config

import (
	"fmt"
	"reflect"

	"github.com/google/uuid"
)

func create[T any]() T {
	var zero T
	return reflect.New(reflect.TypeOf(zero).Elem()).Interface().(T)
}

func orgKey(orgID uuid.UUID, key string) string {
	return fmt.Sprintf("%s:%s", orgID.String(), key)
}