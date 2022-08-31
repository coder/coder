package audit

import (
	"database/sql"
	"fmt"
	"reflect"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/audit"
)

func structName(t reflect.Type) string {
	return t.PkgPath() + "." + t.Name()
}

func diffValues(left, right any, table Table) audit.Map {
	var (
		baseDiff = audit.Map{}

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

		// coerce struct types that would produce bad diffs.
		if leftI, rightI, ok = convertDiffType(leftI, rightI); ok {
			leftF, rightF = reflect.ValueOf(leftI), reflect.ValueOf(rightI)
		}

		// If the field is a pointer, dereference it. Nil pointers are coerced
		// to the zero value of their underlying type.
		if leftF.Kind() == reflect.Ptr && rightF.Kind() == reflect.Ptr {
			leftF, rightF = derefPointer(leftF), derefPointer(rightF)
			leftI, rightI = leftF.Interface(), rightF.Interface()
		}

		if !reflect.DeepEqual(leftI, rightI) {
			switch atype {
			case ActionTrack:
				baseDiff[diffName] = audit.OldNew{Old: leftI, New: rightI}
			case ActionSecret:
				baseDiff[diffName] = audit.OldNew{
					Old:    reflect.Zero(rightF.Type()).Interface(),
					New:    reflect.Zero(rightF.Type()).Interface(),
					Secret: true,
				}
			}
		}
	}

	return baseDiff
}

// convertDiffType converts external struct types to primitive types.
//
//nolint:forcetypeassert
func convertDiffType(left, right any) (newLeft, newRight any, changed bool) {
	switch typedLeft := left.(type) {
	case uuid.UUID:
		typedRight := right.(uuid.UUID)

		// Automatically coerce Nil UUIDs to empty strings.
		outLeft := typedLeft.String()
		if typedLeft == uuid.Nil {
			outLeft = ""
		}

		outRight := typedRight.String()
		if typedRight == uuid.Nil {
			outRight = ""
		}

		return outLeft, outRight, true

	case uuid.NullUUID:
		leftStr, _ := typedLeft.MarshalText()
		rightStr, _ := right.(uuid.NullUUID).MarshalText()
		return string(leftStr), string(rightStr), true

	case sql.NullString:
		leftStr := typedLeft.String
		if !typedLeft.Valid {
			leftStr = "null"
		}

		rightStr := right.(sql.NullString).String
		if !right.(sql.NullString).Valid {
			rightStr = "null"
		}

		return leftStr, rightStr, true

	case sql.NullInt64:
		var leftInt64Ptr *int64
		var rightInt64Ptr *int64
		if !typedLeft.Valid {
			leftInt64Ptr = nil
		} else {
			leftInt64Ptr = ptr(typedLeft.Int64)
		}

		rightInt64Ptr = ptr(right.(sql.NullInt64).Int64)
		if !right.(sql.NullInt64).Valid {
			rightInt64Ptr = nil
		}

		return leftInt64Ptr, rightInt64Ptr, true

	default:
		return left, right, false
	}
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

func ptr[T any](x T) *T {
	return &x
}
