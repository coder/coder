package validate

import (
	"github.com/go-playground/validator/v10"
	"golang.org/x/xerrors"
)

func init() {
	validate = validator.New()
	mustRegisterValidation(validate, "longid", validateLongID)
}

func mustRegisterValidation(v *validator.Validate, tag string, fn validator.Func) {
	if err := v.RegisterValidation(tag, fn); err != nil {
		panic(xerrors.Errorf("register validation: %w", err))
	}
}

// Global validation struct
//
// Custom validators should be added to this struct if needed (see
// https://github.com/go-playground/validator/blob/master/_examples/custom-validation/main.go
// for an example).
var validate *validator.Validate

// Validator returns a copy of the global validator.
func Validator() *validator.Validate {
	v := *validate
	return &v
}

// validateLongID validates that a field is a string, and that the string does
// not exceed the max length of a long ID.
//
// Additional formatting checks are omitted as there's a lot of tests that don't
// use actual long IDs when generating test requests, and the system admin's ID
// also does not follow the long ID format.
func validateLongID(fl validator.FieldLevel) bool {
	f := fl.Field().Interface()
	s, ok := f.(string)
	if !ok {
		return false
	}
	const longIDLen = 33 // The format string for a long id is "%08x-%024x"
	return len(s) <= longIDLen
}
