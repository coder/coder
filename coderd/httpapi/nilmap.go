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
		return xerrors.Errorf("nil maps recieved in type %q%s", ty.String(), extra)
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
			if ok, _ := findNilMapsRec(val.Field(i), visited); ok {
				return true, val.Field(i).String()
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			if ok, _ := findNilMapsRec(val.Index(i), visited); ok {
				return true, ""
			}
		}
	case reflect.Map:
		if val.IsNil() {
			return true, ""
		}
	}

	return false, ""
}
