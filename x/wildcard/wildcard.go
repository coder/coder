// Package wildcard provides a tiny generic helper for values that can be
// either a specific value or the wildcard string "*". It is used in APIs,
// query builders, and database types to express "any" semantics without
// introducing pointers or separate optional wrappers.
//
// The zero value of Value[T] represents a wildcard. Use Of to wrap a concrete
// value and Any to construct an explicit wildcard. Value[T] implements
// encoding.TextMarshaler, encoding.TextUnmarshaler, and database/sql.Scanner so
// it can be serialized as either "*" or the underlying value's string form and
// stored/loaded from SQL columns.
//
// Example:
//
//	v := Of("workspace")       // specific value
//	w := Any[string]()          // wildcard
//	_ = v.String()              // "workspace"
//	_ = w.String()              // "*"
package wildcard

import (
	"database/sql"
	"encoding"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// Value wraps either a concrete value of type T or a wildcard ("*").
// Zero value is a wildcard by default (set == false) so it serializes to "*".
type Value[T any] struct {
	v   T
	set bool
}

// Of constructs an Any with a concrete value.
func Of[T any](val T) Value[T] { return Value[T]{v: val, set: true} }

// Any constructs an Any wildcard. (Equivalent to the zero value.)
func Any[T any]() Value[T] { return Value[T]{} }

// IsAny reports whether this is a wildcard.
func (a Value[T]) IsAny() bool { return !a.set }

// Value returns the underlying value and true if it is not a wildcard.
func (a Value[T]) Value() (T, bool) { return a.v, a.set }

// String renders "*" for wildcard; otherwise fmt.Sprint(value).
func (a Value[T]) String() string {
	if !a.set {
		return "*"
	}
	return fmt.Sprint(a.v)
}

// Text marshaling allows Any to encode as a single string ("*" or value string).
var (
	_ encoding.TextMarshaler   = (*Value[struct{}])(nil)
	_ encoding.TextUnmarshaler = (*Value[struct{}])(nil)
	_ sql.Scanner              = (*Value[struct{}])(nil)
)

func (a Value[T]) MarshalText() ([]byte, error) { return []byte(a.String()), nil }

func (a *Value[T]) UnmarshalText(b []byte) error {
	s := string(b)
	if s == "*" || s == "" {
		*a = Any[T]()
		return nil
	}
	var zero T
	tv := reflect.ValueOf(&zero).Elem()
	switch tv.Kind() {
	case reflect.String:
		// Assign to string-like types (including named string)
		nv := reflect.New(tv.Type()).Elem()
		nv.SetString(s)
		v, ok := nv.Interface().(T)
		if !ok {
			return xerrors.Errorf("match.Any: internal type assertion failed for %T", zero)
		}
		a.v = v
		a.set = true
		return nil
	default:
		// Special-case common types
		if tv.Type() == reflect.TypeOf(uuid.UUID{}) {
			u, err := uuid.Parse(s)
			if err != nil {
				return err
			}
			v, ok := any(u).(T)
			if !ok {
				return xerrors.Errorf("match.Any: internal type assertion failed for %T", zero)
			}
			a.v = v
			a.set = true
			return nil
		}
	}

	return xerrors.Errorf("match.Any: unsupported element type %T for UnmarshalText", zero)
}

// Scan implements sql.Scanner; accepts nil, []byte, or string and delegates to UnmarshalText.
func (a *Value[T]) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*a = Any[T]()
		return nil
	case []byte:
		return a.UnmarshalText(v)
	case string:
		return a.UnmarshalText([]byte(v))
	default:
		return xerrors.Errorf("match.Any: unsupported Scan type %T", src)
	}
}
