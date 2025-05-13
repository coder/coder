package httpapi_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
)

func TestContainsNilMap(t *testing.T) {
	t.Parallel()

	type SimpleStruct struct {
		M map[string]int
	}

	type NestedStruct struct {
		A string
		B SimpleStruct
	}

	type WithPointer struct {
		P *SimpleStruct
	}

	cases := []struct {
		name     string
		input    any
		expected bool
	}{
		{
			name:     "Nil value",
			input:    nil,
			expected: false,
		},
		{
			name:     "Empty struct, no map",
			input:    struct{}{},
			expected: false,
		},
		{
			name:     "Struct with non-nil map",
			input:    SimpleStruct{M: map[string]int{}},
			expected: false,
		},
		{
			name:     "Struct with nil map",
			input:    SimpleStruct{M: nil},
			expected: true,
		},
		{
			name:     "Nested struct with nil map",
			input:    NestedStruct{A: "hello", B: SimpleStruct{M: nil}},
			expected: true,
		},
		{
			name:     "Nested struct with non-nil map",
			input:    NestedStruct{A: "hello", B: SimpleStruct{M: map[string]int{}}},
			expected: false,
		},
		{
			name:     "Pointer to struct with nil map",
			input:    &SimpleStruct{M: nil},
			expected: true,
		},
		{
			name:     "Struct with pointer to struct with nil map",
			input:    WithPointer{P: &SimpleStruct{M: nil}},
			expected: true,
		},
		{
			name:     "Struct with pointer to struct with non-nil map",
			input:    WithPointer{P: &SimpleStruct{M: map[string]int{}}},
			expected: false,
		},
		{
			name:     "Slice with struct having nil map",
			input:    []SimpleStruct{{M: nil}, {M: map[string]int{}}},
			expected: true,
		},
		{
			name:     "Interface holding struct with nil map",
			input:    interface{}(SimpleStruct{M: nil}),
			expected: true,
		},
		{
			name:     "Interface holding struct ptr with nil map",
			input:    (interface{})(&SimpleStruct{M: nil}),
			expected: true,
		},
		{
			name:     "nil map",
			input:    (map[string]string)(nil),
			expected: true,
		},
		{
			// This is actually allowed because a pty is union'd with a null
			name:     "nil map ptr",
			input:    (*map[string]string)(nil),
			expected: false,
		},
		{
			name:     "nil any ptr",
			input:    (*any)(nil),
			expected: false,
		},
		{
			name:     "Slice with nil map",
			input:    []map[string]string{{}, nil},
			expected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			result := httpapi.ContainsNilMap(c.input) != nil
			if c.expected != result {
				v := reflect.ValueOf(c.input)
				t.Logf("type=%q does not match expected", v.Type().String())
				data, _ := json.Marshal(c.input)
				t.Log(string(data))
			}
			require.Equal(t, c.expected, result)
		})
	}
}
