package validate

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"golang.org/x/xerrors"
)

const (
	validateTag = "validate" // Used by go-playground/validator
	jsonTag     = "json"     // Stdlib json tag
)

var ErrNotAStruct = xerrors.Errorf("value not a struct")

// FieldsMissingValidation returns a list of fields that are missing appropriate
// validation tags. Any field that is exported, does not have a "-" json tag
// value, and is not a bool or pointer to a bool will be included in the
// returned list of fields if it lacks a "validate" tag.
//
// This will recursively check nested structs, stopping at fields that are
// unexported, or fields that do not unmarshal from json. `v` should be a
// struct.
//
// Nested struct fields that do not have a "validate" tag will be included in
// the returned list.
func FieldsMissingValidation(v interface{}) ([]reflect.StructField, error) {
	_, ok := isStruct(v)
	if !ok {
		return nil, ErrNotAStruct
	}

	fields, err := SelectFields(v,
		SelectAll{
			FieldSelectorFunc(IsExported),
			NegateSelector(FieldSelectorFunc(IsBool)),
			NegateSelector(FieldSelectorFunc(HasSkipJSON)),
			NegateSelector(ValidateTagKeyFieldSelector),
		},
		SelectAny{
			NegateSelector(FieldSelectorFunc(IsExported)),
			FieldSelectorFunc(HasSkipJSON),
			FieldSelectorFunc(HasSkipValidate),
		},
	)
	if err != nil {
		return nil, xerrors.Errorf("select fields: %w", err)
	}

	return fields, nil
}

// FieldsWithValidation returns a list of fields with a "validate" tag.
//
// This will recursively check nested structs, stopping at fields that are
// unexported, or fields that do not unmarshal from json. `v` should be a
// struct.
//
// Nested struct fields that do have a "validate" tag will be included in the
// returned list.
func FieldsWithValidation(v interface{}) ([]reflect.StructField, error) {
	_, ok := isStruct(v)
	if !ok {
		return nil, ErrNotAStruct
	}

	fields, err := SelectFields(v,
		SelectAll{
			FieldSelectorFunc(IsExported),
			NegateSelector(FieldSelectorFunc(IsBool)),
			NegateSelector(FieldSelectorFunc(HasSkipJSON)),
			ValidateTagKeyFieldSelector,
		},
		SelectAny{
			NegateSelector(FieldSelectorFunc(IsExported)),
			FieldSelectorFunc(HasSkipJSON),
			FieldSelectorFunc(HasSkipValidate),
		},
	)
	if err != nil {
		return nil, xerrors.Errorf("select fields: %w", err)
	}

	return fields, nil
}

type FieldSelector interface {
	Matches(field reflect.StructField) bool
}

type SelectAny []FieldSelector

func (fs SelectAny) Matches(field reflect.StructField) bool {
	for _, f := range fs {
		if f.Matches(field) {
			return true
		}
	}
	return false
}

type SelectAll []FieldSelector

func (fs SelectAll) Matches(field reflect.StructField) bool {
	for _, f := range fs {
		if !f.Matches(field) {
			return false
		}
	}
	return true
}

// JSONTagValueFieldSelector selects all fields that has a given json tag value.
type JSONTagValueFieldSelector string

func (fs JSONTagValueFieldSelector) Matches(field reflect.StructField) bool {
	tagVal, ok := field.Tag.Lookup(jsonTag)
	if !ok {
		return false
	}
	for _, s := range strings.Split(tagVal, ",") {
		if s == string(fs) {
			return true
		}
	}
	return false
}

// TagKeyFieldSelector selects all fields with the given tag key.
type TagKeyFieldSelector string

const (
	ValidateTagKeyFieldSelector TagKeyFieldSelector = validateTag
)

func (fs TagKeyFieldSelector) Matches(field reflect.StructField) bool {
	_, ok := field.Tag.Lookup(string(fs))
	return ok
}

type FieldSelectorFunc func(field reflect.StructField) bool

func (fs FieldSelectorFunc) Matches(field reflect.StructField) bool {
	return fs(field)
}

// IsExported checks if the field is exported.
func IsExported(field reflect.StructField) bool {
	// PkgPath is empty for exported fields (noted in doc for PkgPath).
	return field.PkgPath == ""
}

// IsBool checks if the field is a bool, or a *bool.
func IsBool(field reflect.StructField) bool {
	// Field is a bool.
	if field.Type.Kind() == reflect.Bool {
		return true
	}
	// Field is a *bool.
	if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Bool {
		return true
	}
	return false
}

func HasSkipJSON(field reflect.StructField) bool {
	jsonVal, jsonFound := field.Tag.Lookup(jsonTag)
	skipJSON := jsonFound && jsonVal == "-"
	return skipJSON
}

func HasSkipValidate(field reflect.StructField) bool {
	jsonVal, jsonFound := field.Tag.Lookup(validateTag)
	skipJSON := jsonFound && jsonVal == "-"
	return skipJSON
}

func NegateSelector(fs FieldSelector) FieldSelector {
	return FieldSelectorFunc(func(field reflect.StructField) bool {
		return !fs.Matches(field)
	})
}

// Field validates struct `v`, returning just the validation error for a
// field that Matches FieldSelector. If the selector matches more than one
// field, only the first will be checked.
func Field(v interface{}, fs FieldSelector) error {
	_, ok := isStruct(v)
	if !ok {
		return ErrNotAStruct
	}

	err := Validator().Struct(v)
	if err == nil {
		return nil
	}

	var vErrs validator.ValidationErrors
	if xerrors.As(err, &vErrs) {
		fields, _ := SelectFields(v, fs, nil) // Can only error if `v` isn't a struct.
		for _, field := range fields {
			for _, vErr := range vErrs {
				if field.Name == vErr.StructField() {
					return vErr
				}
			}
		}
		// Field selector either matched no fields that failed validation, or
		// all matched fields passed validation.
		return nil
	}

	return xerrors.Errorf("non-validation error when validating: %w", err)
}

// SelectFields selects all fields from struct `v` that match the field
// selector.
//
// This will recurse through nested structs, stopping at fields that are
// selected with `skipFields`. A value of nil for `skipFields` will continue to
// recurse indiscriminately. Infinite recursion is avoided by detecting if a
// field of a struct has the same type as the struct itself.
func SelectFields(v interface{}, fs FieldSelector, skipFields FieldSelector) ([]reflect.StructField, error) {
	return selectFieldsWithVisited(v, fs, skipFields, nil)
}

type fieldType struct {
	pkg  string
	name string
}

func selectFieldsWithVisited(v interface{}, fs FieldSelector, skipFields FieldSelector, visited []*fieldType) ([]reflect.StructField, error) {
	st, ok := isStruct(v)
	if !ok {
		return nil, ErrNotAStruct
	}

	var fields []reflect.StructField

	// Check to make sure we haven't visited this type yet. If we have, there's
	// no need to continue.
	for _, ft := range visited {
		if st.Name() == ft.name && st.PkgPath() == ft.pkg {
			return fields, nil
		}
	}
	visited = append(visited, &fieldType{pkg: st.PkgPath(), name: st.Name()})

	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		if fs.Matches(field) {
			fields = append(fields, field)
		}

		if skipFields != nil && skipFields.Matches(field) {
			continue
		}

		fv := reflect.Zero(field.Type)
		if field.Type.Kind() == reflect.Ptr {
			fv = reflect.Zero(field.Type.Elem())
		}
		if fv.Kind() == reflect.Struct {
			nestedFields, err := selectFieldsWithVisited(fv.Interface(), fs, skipFields, visited)
			if err != nil {
				return nil, xerrors.Errorf("select fields, field: %s: %w", field.Name, err)
			}
			fields = append(fields, nestedFields...)
		}
	}

	return fields, nil
}

// isStruct checks to make sure `v` is either a struct, or a pointer to a
// struct.
func isStruct(v interface{}) (reflect.Type, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return rv.Type(), rv.Kind() == reflect.Struct
}
