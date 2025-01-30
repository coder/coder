package maps

import (
	"sort"

	"golang.org/x/exp/constraints"
)

// Subset returns true if all the keys of a are present
// in b and have the same values.
func Subset[T, U comparable](a, b map[T]U) bool {
	for ka, va := range a {
		if vb, ok := b[ka]; !ok {
			return false
		} else if va != vb {
			return false
		}
	}
	return true
}

// SortedKeys returns the keys of m in sorted order.
func SortedKeys[T constraints.Ordered](m map[T]any) (keys []T) {
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
