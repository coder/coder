package maps_test

import (
	"strconv"
	"testing"

	"github.com/coder/coder/v2/coderd/util/maps"
)

func TestSubset(t *testing.T) {
	t.Parallel()

	for idx, tc := range []struct {
		a        map[string]string
		b        map[string]string
		expected bool
	}{
		{
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			a:        map[string]string{},
			b:        map[string]string{},
			expected: true,
		},
		{
			a:        map[string]string{"a": "1", "b": "2"},
			b:        map[string]string{"a": "1", "b": "2"},
			expected: true,
		},
		{
			a:        map[string]string{"a": "1", "b": "2"},
			b:        map[string]string{"a": "1"},
			expected: false,
		},
		{
			a:        map[string]string{"a": "1"},
			b:        map[string]string{"a": "1", "b": "2"},
			expected: true,
		},
		{
			a:        map[string]string{"a": "1", "b": "2"},
			b:        map[string]string{},
			expected: false,
		},
		{
			a:        map[string]string{"a": "1", "b": "2"},
			b:        map[string]string{"a": "1", "b": "3"},
			expected: false,
		},
	} {
		tc := tc
		t.Run("#"+strconv.Itoa(idx), func(t *testing.T) {
			t.Parallel()

			actual := maps.Subset(tc.a, tc.b)
			if actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}
