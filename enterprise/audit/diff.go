package audit

import (
	"database/sql"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

func structName(t reflect.Type) string {
	return t.PkgPath() + "." + t.Name()
}

func diffValues(left, right any, table Table) audit.Map {
	var (
		baseDiff = audit.Map{}
		rightT   = reflect.TypeOf(right)
		leftV    = reflect.ValueOf(left)
		rightV   = reflect.ValueOf(right)

		diffKey = table[structName(rightT)]
	)

	if diffKey == nil {
		panic(fmt.Sprintf("dev error: type %q (type %T) attempted audit but not auditable", rightT.Name(), right))
	}

	// allFields contains all top level fields of the struct.
	allFields, err := flattenStructFields(leftV, rightV)
	if err != nil {
		// This should never happen. Only structs should be flattened. If an
		// error occurs, an unsupported or non-struct type was passed in.
		panic(fmt.Sprintf("dev error: failed to flatten struct fields: %v", err))
	}

	for _, field := range allFields {
		var (
			leftF  = field.LeftF
			rightF = field.RightF

			leftI  = leftF.Interface()
			rightI = rightF.Interface()
		)

		diffName := field.FieldType.Tag.Get("json")

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
			leftInt64Ptr = ptr.Ref(typedLeft.Int64)
		}

		rightInt64Ptr = ptr.Ref(right.(sql.NullInt64).Int64)
		if !right.(sql.NullInt64).Valid {
			rightInt64Ptr = nil
		}

		return leftInt64Ptr, rightInt64Ptr, true
	case database.TemplateACL:
		return fmt.Sprintf("%+v", left), fmt.Sprintf("%+v", right), true
	case database.CustomRolePermissions:
		// String representation is much easier to visually inspect
		leftArr := make([]string, 0)
		rightArr := make([]string, 0)
		for _, p := range typedLeft {
			leftArr = append(leftArr, p.String())
		}
		for _, p := range right.(database.CustomRolePermissions) {
			rightArr = append(rightArr, p.String())
		}

		return leftArr, rightArr, true
	default:
		return left, right, false
	}
}

// fieldDiff has all the required information to return an audit diff for a
// given field.
type fieldDiff struct {
	FieldType reflect.StructField
	LeftF     reflect.Value
	RightF    reflect.Value
}

// flattenStructFields will return all top level fields for a given structure.
// Only anonymously embedded structs will be recursively flattened such that their
// fields are returned as top level fields. Named nested structs will be returned
// as a single field.
// Conflicting field names need to be handled by the caller.
func flattenStructFields(leftV, rightV reflect.Value) ([]fieldDiff, error) {
	// Dereference pointers if the field is a pointer field.
	if leftV.Kind() == reflect.Ptr {
		leftV = derefPointer(leftV)
		rightV = derefPointer(rightV)
	}

	if leftV.Kind() != reflect.Struct {
		return nil, xerrors.Errorf("%q is not a struct, kind=%s", leftV.String(), leftV.Kind())
	}

	var allFields []fieldDiff
	rightT := rightV.Type()

	// Loop through all top level fields of the struct.
	for i := 0; i < rightT.NumField(); i++ {
		if !rightT.Field(i).IsExported() {
			continue
		}

		var (
			leftF  = leftV.Field(i)
			rightF = rightV.Field(i)
		)

		if rightT.Field(i).Anonymous {
			// Anonymous fields are recursively flattened.
			anonFields, err := flattenStructFields(leftF, rightF)
			if err != nil {
				return nil, xerrors.Errorf("flatten anonymous field %q: %w", rightT.Field(i).Name, err)
			}
			allFields = append(allFields, anonFields...)
			continue
		}

		// Single fields append as is.
		allFields = append(allFields, fieldDiff{
			LeftF:     leftF,
			RightF:    rightF,
			FieldType: rightT.Field(i),
		})
	}
	return allFields, nil
}

// derefPointer deferences a reflect.Value that is a pointer to its underlying
// value. It dereferences recursively until it finds a non-pointer value. If the
// pointer is nil, it will be coerced to the zero value of the underlying type.
func derefPointer(ref reflect.Value) reflect.Value {
	if !ref.IsNil() {
		// Grab the value the pointer references.
		ref = ref.Elem()
	} else {
		// Coerce nil ptrs to zero'd values of their underlying type.
		ref = reflect.Zero(ref.Type().Elem())
	}

	// Recursively deref nested pointers.
	if ref.Kind() == reflect.Ptr {
		return derefPointer(ref)
	}

	return ref
}
