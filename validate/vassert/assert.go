package vassert

import (
	"testing"

	"github.com/coder/coder/validate"
)

// Tags asserts that most fields on a struct with a "json" tag also have a
// "validate" tag, unless the json tag value is "-".
//
// This will recursively check nested structs.
//
// Boolean values and boolean pointers do not require a validate tag.
//
// `v` should be a struct.
func Tags(t *testing.T, v interface{}) {
	t.Helper()
	fields, err := validate.FieldsMissingValidation(v)
	if err != nil {
		t.Fatalf("failed to get missing field validations: %s", err)
	}
	if len(fields) > 0 {
		names := make([]string, len(fields))
		for i, f := range fields {
			names[i] = f.Name
		}
		t.Fatalf("the following fields are missing validations: %v", names)
	}
}

// FieldValid asserts that the field with the correspting `jsonField`
// value validates.
//
// `v` should be a struct.
func FieldValid(t *testing.T, v interface{}, jsonField string) {
	t.Helper()
	ensureHasJSONField(t, v, jsonField)
	err := validate.Field(v, validate.JSONTagValueFieldSelector(jsonField))
	if err != nil {
		t.Fatalf("expected field %q to validate: %v", jsonField, err)
	}
}

// FieldInvalid asserts that the field with the correspting `jsonField`
// value does not validate.
//
// `v` should be a struct.
func FieldInvalid(t *testing.T, v interface{}, jsonField string) {
	t.Helper()
	ensureHasJSONField(t, v, jsonField)
	err := validate.Field(v, validate.JSONTagValueFieldSelector(jsonField))
	if err == nil {
		t.Fatalf("expected field %q to be invalid", jsonField)
	}
}

func ensureHasJSONField(t *testing.T, v interface{}, jsonField string) {
	t.Helper()
	fs := validate.JSONTagValueFieldSelector(jsonField)
	fields, err := validate.SelectFields(v, fs, nil)
	if err != nil {
		t.Fatalf("failed to select fields: %v", err)
	}
	if len(fields) == 0 {
		t.Fatalf("%q matches no fields on the struct", jsonField)
	}
}
