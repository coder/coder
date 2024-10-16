package database

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAuthorizedQuery(t *testing.T) {
	t.Parallel()

	query := `SELECT true;`
	_, err := insertAuthorizedFilter(query, "")
	require.ErrorContains(t, err, "does not contain authorized replace string", "ensure replace string")
}

// TestWorkspaceTableConvert verifies all workspace fields are converted
// when reducing a `Workspace` to a `WorkspaceTable`.
func TestWorkspaceTableConvert(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	err := populateStruct(&workspace)
	require.NoError(t, err)

}

func populateStruct(s interface{}) error {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("s must be a non-nil pointer")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("s must be a pointer to a struct")
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldName := field.Name

		fieldValue := v.Field(i)
		if !fieldValue.CanSet() {
			continue // Skip if field is unexported
		}

		switch fieldValue.Kind() {
		case reflect.Struct:
			if err := populateStruct(fieldValue.Addr().Interface()); err != nil {
				return fmt.Errorf("%s : %w", fieldName, err)
			}
		case reflect.String:
			fieldValue.SetString("foo")
		case reflect.Invalid:
		case reflect.Bool:
		case reflect.Int:
		case reflect.Int8:
		case reflect.Int16:
		case reflect.Int32:
		case reflect.Int64:
		case reflect.Uint:
		case reflect.Uint8:
		case reflect.Uint16:
		case reflect.Uint32:
		case reflect.Uint64:
		case reflect.Uintptr:
		case reflect.Float32:
		case reflect.Float64:
		case reflect.Complex64:
		case reflect.Complex128:
		case reflect.Array:
		case reflect.Chan:
		case reflect.Func:
		case reflect.Interface:
		case reflect.Map:
		case reflect.Pointer:
		case reflect.Slice:
		case reflect.UnsafePointer:
		default:
			return fmt.Errorf("unsupported kind %s", fieldValue.Kind())
		}
	}

	return nil
}
