package maps_test

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/coder/coder/v2/coderd/util/maps"
)

func TestSortedKeys(t *testing.T) {
	t.Parallel()

	for idx, tc := range []struct {
		name     string
		input    map[string]int
		expected []string
	}{
		{
			name: "SortsAlphabetically",
			input: map[string]int{
				"banana": 1,
				"apple":  2,
				"cherry": 3,
			},
			expected: []string{"apple", "banana", "cherry"},
		},
		{
			name: "AlreadySorted",
			input: map[string]int{
				"alpha": 1,
				"mango": 2,
				"zebra": 3,
			},
			expected: []string{"alpha", "mango", "zebra"},
		},
		{
			name:     "EmptyMap",
			input:    map[string]int{},
			expected: nil,
		},
	} {
		t.Run("#"+strconv.Itoa(idx)+"_"+tc.name, func(t *testing.T) {
			t.Parallel()
			got := maps.SortedKeys(tc.input)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Fatalf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
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
