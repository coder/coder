// Package ptr contains some utility methods related to pointers.
package ptr

import "golang.org/x/exp/constraints"

type number interface {
	constraints.Integer | constraints.Float
}

// Ref returns a reference to v.
func Ref[T any](v T) *T {
	return &v
}

// NilOrEmpty returns true if s is nil or the empty string.
func NilOrEmpty(s *string) bool {
	return s == nil || *s == ""
}

// NilToEmpty coalesces a nil str to the empty string.
func NilToEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// NilOrZero returns true if v is nil or 0.
func NilOrZero[T number](v *T) bool {
	return v == nil || *v == 0
}
