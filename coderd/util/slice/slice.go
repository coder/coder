package slice

import (
	"golang.org/x/exp/constraints"
)

// ToStrings works for any type where the base type is a string.
func ToStrings[T ~string](a []T) []string {
	tmp := make([]string, 0, len(a))
	for _, v := range a {
		tmp = append(tmp, string(v))
	}
	return tmp
}

func StringEnums[E ~string](a []string) []E {
	if a == nil {
		return nil
	}
	tmp := make([]E, 0, len(a))
	for _, v := range a {
		tmp = append(tmp, E(v))
	}
	return tmp
}

// Omit creates a new slice with the arguments omitted from the list.
func Omit[T comparable](a []T, omits ...T) []T {
	tmp := make([]T, 0, len(a))
	for _, v := range a {
		if Contains(omits, v) {
			continue
		}
		tmp = append(tmp, v)
	}
	return tmp
}

// SameElements returns true if the 2 lists have the same elements in any
// order.
func SameElements[T comparable](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}

	for _, element := range a {
		if !Contains(b, element) {
			return false
		}
	}
	return true
}

func ContainsCompare[T any](haystack []T, needle T, equal func(a, b T) bool) bool {
	for _, hay := range haystack {
		if equal(needle, hay) {
			return true
		}
	}
	return false
}

func Contains[T comparable](haystack []T, needle T) bool {
	return ContainsCompare(haystack, needle, func(a, b T) bool {
		return a == b
	})
}

// Find returns the first element that satisfies the condition.
func Find[T any](haystack []T, cond func(T) bool) (T, bool) {
	for _, hay := range haystack {
		if cond(hay) {
			return hay, true
		}
	}
	var empty T
	return empty, false
}

// Overlap returns if the 2 sets have any overlap (element(s) in common)
func Overlap[T comparable](a []T, b []T) bool {
	return OverlapCompare(a, b, func(a, b T) bool {
		return a == b
	})
}

func UniqueFunc[T any](a []T, equal func(a, b T) bool) []T {
	cpy := make([]T, 0, len(a))

	for _, v := range a {
		if ContainsCompare(cpy, v, equal) {
			continue
		}

		cpy = append(cpy, v)
	}

	return cpy
}

// Unique returns a new slice with all duplicate elements removed.
func Unique[T comparable](a []T) []T {
	cpy := make([]T, 0, len(a))
	seen := make(map[T]struct{}, len(a))

	for _, v := range a {
		if _, ok := seen[v]; ok {
			continue
		}

		seen[v] = struct{}{}
		cpy = append(cpy, v)
	}

	return cpy
}

func OverlapCompare[T any](a []T, b []T, equal func(a, b T) bool) bool {
	// For each element in b, if at least 1 is contained in 'a',
	// return true.
	for _, element := range b {
		if ContainsCompare(a, element, equal) {
			return true
		}
	}
	return false
}

// New is a convenience method for creating []T.
func New[T any](items ...T) []T {
	return items
}

func Ascending[T constraints.Ordered](a, b T) int {
	if a < b {
		return -1
	} else if a == b {
		return 0
	}
	return 1
}

func Descending[T constraints.Ordered](a, b T) int {
	return -Ascending[T](a, b)
}

// SymmetricDifference returns the elements that need to be added and removed
// to get from set 'a' to set 'b'. Note that duplicates are ignored in sets.
// In classical set theory notation, SymmetricDifference returns
// all elements of {add} and {remove} together. It is more useful to
// return them as their own slices.
// Notation: A Δ B = (A\B) ∪ (B\A)
// Example:
//
//	a := []int{1, 3, 4}
//	b := []int{1, 2, 2, 2}
//	add, remove := SymmetricDifference(a, b)
//	fmt.Println(add)    // [2]
//	fmt.Println(remove) // [3, 4]
func SymmetricDifference[T comparable](a, b []T) (add []T, remove []T) {
	f := func(a, b T) bool { return a == b }
	return SymmetricDifferenceFunc(a, b, f)
}

func SymmetricDifferenceFunc[T any](a, b []T, equal func(a, b T) bool) (add []T, remove []T) {
	// Ignore all duplicates
	a, b = UniqueFunc(a, equal), UniqueFunc(b, equal)
	return DifferenceFunc(b, a, equal), DifferenceFunc(a, b, equal)
}

func DifferenceFunc[T any](a []T, b []T, equal func(a, b T) bool) []T {
	tmp := make([]T, 0, len(a))
	for _, v := range a {
		if !ContainsCompare(b, v, equal) {
			tmp = append(tmp, v)
		}
	}
	return tmp
}
