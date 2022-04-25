package audit

import (
	"fmt"
	"reflect"
)

// TODO: this might need to be in the database package.
type Map map[string]interface{}

func Empty[T Auditable]() T {
	var t T
	return t
}

// Diff compares two auditable resources and produces a Map of the changed
// values.
func Diff[T Auditable](left, right T) Map {
	// Values are equal, return an empty diff.
	if reflect.DeepEqual(left, right) {
		return Map{}
	}

	return diffValues(left, right, AuditableResources)
}

func structName(t reflect.Type) string {
	return t.PkgPath() + "." + t.Name()
}

func diffValues[T any](left, right T, table Table) Map {
	var (
		baseDiff = Map{}

		leftV = reflect.ValueOf(left)

		rightV = reflect.ValueOf(right)
		rightT = reflect.TypeOf(right)

		diffKey = table[structName(rightT)]
	)

	if diffKey == nil {
		panic(fmt.Sprintf("dev error: type %q (type %T) attempted audit but not auditable", rightT.Name(), right))
	}

	for i := 0; i < rightT.NumField(); i++ {
		var (
			leftF  = leftV.Field(i)
			rightF = rightV.Field(i)

			leftI  = leftF.Interface()
			rightI = rightF.Interface()

			diffName = rightT.Field(i).Tag.Get("json")
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
		if leftF.Kind() == reflect.Ptr && rightF.Kind() == reflect.Ptr {
			leftF, rightF = derefPointer(leftF), derefPointer(rightF)
			leftI, rightI = leftF.Interface(), rightF.Interface()
		}

		// Recursively walk up nested structs.
		if rightF.Kind() == reflect.Struct {
			baseDiff[diffName] = diffValues(leftI, rightI, table)
			continue
		}

		if !reflect.DeepEqual(leftI, rightI) {
			switch atype {
			case ActionTrack:
				baseDiff[diffName] = rightI
			case ActionSecret:
				baseDiff[diffName] = reflect.Zero(rightF.Type()).Interface()
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
