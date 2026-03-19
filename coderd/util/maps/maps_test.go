package maps_test

import (
	"strconv"
	"testing"

	"github.com/coder/coder/v2/coderd/util/maps"
)

func TestSortedKeys(t *testing.T) {
	t.Parallel()

	t.Run("StringToStruct", func(t *testing.T) {
		t.Parallel()
		m := map[string]struct{}{
			"banana": {},
			"apple":  {},
			"cherry": {},
		}
		got := maps.SortedKeys(m)
		expected := []string{"apple", "banana", "cherry"}
		if len(got) != len(expected) {
			t.Fatalf("expected %d keys, got %d", len(expected), len(got))
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Errorf("index %d: expected %q, got %q", i, expected[i], got[i])
			}
		}
	})

	t.Run("StringToInt", func(t *testing.T) {
		t.Parallel()
		m := map[string]int{
			"zebra": 1,
			"alpha": 2,
			"mango": 3,
		}
		got := maps.SortedKeys(m)
		expected := []string{"alpha", "mango", "zebra"}
		if len(got) != len(expected) {
			t.Fatalf("expected %d keys, got %d", len(expected), len(got))
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Errorf("index %d: expected %q, got %q", i, expected[i], got[i])
			}
		}
	})

	t.Run("EmptyMap", func(t *testing.T) {
		t.Parallel()
		m := map[string]int{}
		got := maps.SortedKeys(m)
		if len(got) != 0 {
			t.Fatalf("expected 0 keys, got %d", len(got))
		}
	})
}

func TestSubset(t *testing.T) {
	t.Parallel()

	for idx, tc := range []struct {
		a map[string]string
		b map[string]string
		// expected value from Subset
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
		// Zero value
		{
			a:        map[string]string{"a": "1", "b": ""},
			b:        map[string]string{"a": "1", "b": "3"},
			expected: true,
		},
		// Zero value, but the other way round
		{
			a:        map[string]string{"a": "1", "b": "3"},
			b:        map[string]string{"a": "1", "b": ""},
			expected: false,
		},
		// Both zero values
		{
			a:        map[string]string{"a": "1", "b": ""},
			b:        map[string]string{"a": "1", "b": ""},
			expected: true,
		},
	} {
		t.Run("#"+strconv.Itoa(idx), func(t *testing.T) {
			t.Parallel()

			actual := maps.Subset(tc.a, tc.b)
			if actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}
