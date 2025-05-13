package httpapi

import "reflect"

func ContainsNilMap(v any) bool {
	visited := make(map[uintptr]bool)
	return findNilMapsRec(reflect.ValueOf(v), visited)
}

func findNilMapsRec(val reflect.Value, visited map[uintptr]bool) bool {
	if !val.IsValid() {
		return false
	}

	// Handle pointers
	for val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return false
		}
		ptr := val.Pointer()
		if visited[ptr] {
			return false // Prevent infinite recursion
		}
		visited[ptr] = true
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			if findNilMapsRec(val.Field(i), visited) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			if findNilMapsRec(val.Index(i), visited) {
				return true
			}
		}
	case reflect.Map:
		if val.IsNil() {
			return true
		}
	}

	return false
}
