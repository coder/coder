package httpapi

import (
	"reflect"

	"golang.org/x/xerrors"
)

func ContainsNilMap(v any) error {
	visited := make(map[uintptr]bool)
	if hasNil, field := findNilMapsRec(reflect.ValueOf(v), visited); hasNil {
		ty := reflect.TypeOf(v)
		extra := ""
		if field != "" {
			extra = " in field " + field
		}
		return xerrors.Errorf("nil maps received in type %q%s", ty.String(), extra)
	}
	return nil
}

func findNilMapsRec(val reflect.Value, visited map[uintptr]bool) (bool, string) {
	if !val.IsValid() {
		return false, ""
	}

	// Handle pointers
	for val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface {
		if val.IsNil() {
			// If someone makes a *map[string]string, this will return early.
			// That is ok, because the typegen will union the type with a null
			// based on the pointer.
			return false, ""
		}
		if val.Kind() == reflect.Interface && !val.CanAddr() {
			return false, ""
		}
		ptr := val.Pointer()
		if visited[ptr] {
			return false, "" // Prevent infinite recursion
		}
		visited[ptr] = true
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			if ok, field := findNilMapsRec(val.Field(i), visited); ok {
				fn := val.Type().Field(i).Name
				if field != "" {
					field = fn + "." + field
				} else {
					field = fn
				}
				return true, field
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			if ok, field := findNilMapsRec(val.Index(i), visited); ok {
				return true, field
			}
		}
	case reflect.Map:
		if val.IsNil() {
			return true, ""
		}
	}

	return false, ""
}
