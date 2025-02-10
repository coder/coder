package maps

import (
	"sort"

	"golang.org/x/exp/constraints"
)

// Subset returns true if all the keys of a are present
// in b and have the same values.
// If the corresponding value of a[k] is the zero value in
// b, Subset will skip comparing that value.
// This allows checking for the presence of map keys.
func Subset[T, U comparable](a, b map[T]U) bool {
	var uz U
	for ka, va := range a {
		ignoreZeroValue := va == uz
		if vb, ok := b[ka]; !ok || (!ignoreZeroValue && va != vb) {
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
