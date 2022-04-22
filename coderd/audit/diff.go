package audit

import (
	"fmt"
	"reflect"
)

// TODO: this might need to be in the database package.
type DiffMap map[string]interface{}

func Empty[T Auditable]() T {
	var t T
	return t
}

func Diff[T Auditable](old, new T) DiffMap {
	// Values are equal, return an empty diff.
	if reflect.DeepEqual(old, new) {
		return DiffMap{}
	}

	return diffValues(old, new)
}

func diffValues[T any](old, new T) DiffMap {
	var (
		baseDiff = DiffMap{}

		oldV = reflect.ValueOf(old)

		newV = reflect.ValueOf(new)
		newT = reflect.TypeOf(new)

		diffKey = AuditableResources[newT.Name()]
	)

	if diffKey == nil {
		panic(fmt.Sprintf("dev error: type %T attempted audit but not auditable", new))
	}

	for i := 0; i < newT.NumField(); i++ {
		var (
			oldF = oldV.Field(i)
			newF = newV.Field(i)

			oldI = oldF.Interface()
			newI = newF.Interface()

			diffName = newT.Field(i).Tag.Get("json")
		)

		atype, ok := diffKey[diffName]
		if !ok {
			panic(fmt.Sprintf("dev error: field %q lacks audit information", diffName))
		}

		if atype == ActionIgnore {
			continue
		}

		// If the field is a pointer, dereference it. Nil pointers are coerced
		// to the zero value of their underlying type.
		if oldF.Kind() == reflect.Ptr && newF.Kind() == reflect.Ptr {
			oldF, newF = derefPointer(oldF), derefPointer(newF)
			oldI, newI = oldF.Interface(), newF.Interface()
		}

		// Recursively walk up nested structs.
		if newF.Kind() == reflect.Struct {
			baseDiff[diffName] = diffValues(oldI, newI)
			continue
		}

		if !reflect.DeepEqual(oldI, newI) {
			switch atype {
			case ActionTrack:
				baseDiff[diffName] = newI
			case ActionSecret:
				baseDiff[diffName] = reflect.Zero(newF.Type()).Interface()
			}
		}
	}

	return baseDiff
}

// derefPointer deferences a reflect.Value that is a pointer to its underlying
// value. It dereferences recursively until it finds a non-pointer value. If the
// pointer is nil, it will be coerced to the zero value of the underlying type.
func derefPointer(ptr reflect.Value) reflect.Value {
	if !ptr.IsNil() {
		// Grab the value the pointer references.
		ptr = ptr.Elem()
	} else {
		// Coerce nil ptrs to zero'd values of their underlying type.
		ptr = reflect.Zero(ptr.Type().Elem())
	}

	// Recursively deref nested pointers.
	if ptr.Kind() == reflect.Ptr {
		return derefPointer(ptr)
	}

	return ptr
}
