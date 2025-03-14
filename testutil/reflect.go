package testutil
import (
	"fmt"
	"errors"
	"reflect"
	"time"
)
type Random struct {
	String  func() string
	Bool    func() bool
	Int     func() int64
	Uint    func() uint64
	Float   func() float64
	Complex func() complex128
	Time    func() time.Time
}
func NewRandom() *Random {
	// Guaranteed to be random...
	return &Random{
		String:  func() string { return "foo" },
		Bool:    func() bool { return true },
		Int:     func() int64 { return 500 },
		Uint:    func() uint64 { return 126 },
		Float:   func() float64 { return 3.14 },
		Complex: func() complex128 { return 6.24 },
		Time:    func() time.Time { return time.Date(2020, 5, 2, 5, 19, 21, 30, time.UTC) },
	}
}
// PopulateStruct does a best effort to populate a struct with random values.
func PopulateStruct(s interface{}, r *Random) error {
	if r == nil {
		r = NewRandom()
	}
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
		nv, err := populateValue(fieldValue, r)
		if err != nil {
			return fmt.Errorf("%s : %w", fieldName, err)
		}
		v.Field(i).Set(nv)
	}
	return nil
}
func populateValue(v reflect.Value, r *Random) (reflect.Value, error) {
	var err error
	// Handle some special cases
	switch v.Type() {
	case reflect.TypeOf(time.Time{}):
		v.Set(reflect.ValueOf(r.Time()))
		return v, nil
	default:
		// Go to Kind instead
	}
	switch v.Kind() {
	case reflect.Struct:
		if err := PopulateStruct(v.Addr().Interface(), r); err != nil {
			return v, err
		}
	case reflect.String:
		v.SetString(r.String())
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(r.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(r.Uint())
	case reflect.Float32, reflect.Float64:
		v.SetFloat(r.Float())
	case reflect.Complex64, reflect.Complex128:
		v.SetComplex(r.Complex())
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			nv, err := populateValue(v.Index(i), r)
			if err != nil {
				return v, fmt.Errorf("array index %d : %w", i, err)
			}
			v.Index(i).Set(nv)
		}
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		// Set a value in the map
		k := reflect.New(v.Type().Key())
		kv := reflect.New(v.Type().Elem())
		k, err = populateValue(k, r)
		if err != nil {
			return v, fmt.Errorf("map key : %w", err)
		}
		kv, err = populateValue(kv, r)
		if err != nil {
			return v, fmt.Errorf("map value : %w", err)
		}
		m.SetMapIndex(k, kv)
		return m, nil
	case reflect.Pointer:
		return populateValue(v.Elem(), r)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 2, 2)
		sv, err := populateValue(reflect.New(v.Type().Elem()), r)
		if err != nil {
			return v, fmt.Errorf("slice value : %w", err)
		}
		s.Index(0).Set(sv)
		s.Index(1).Set(sv)
		// reflect.AppendSlice(s, sv)
		return s, nil
	case reflect.Uintptr, reflect.UnsafePointer, reflect.Chan, reflect.Func, reflect.Interface:
		// Unsupported
		return v, fmt.Errorf("%s is not supported", v.Kind())
	default:
		return v, fmt.Errorf("unsupported kind %s", v.Kind())
	}
	return v, nil
}
