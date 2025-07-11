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

func CountMatchingPairs[A, B any](a []A, b []B, match func(A, B) bool) int {
	count := 0
	for _, a := range a {
		for _, b := range b {
			if match(a, b) {
				count++
				break
			}
		}
	}
	return count
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

// Filter returns all elements that satisfy the condition.
func Filter[T any](haystack []T, cond func(T) bool) []T {
	out := make([]T, 0, len(haystack))
	for _, hay := range haystack {
		if cond(hay) {
			out = append(out, hay)
		}
	}
	return out
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

func CountConsecutive[T comparable](needle T, haystack ...T) int {
	maxLength := 0
	curLength := 0

	for _, v := range haystack {
		if v == needle {
			curLength++
		} else {
			maxLength = max(maxLength, curLength)
			curLength = 0
		}
	}

	return max(maxLength, curLength)
}

// Convert converts a slice of type F to a slice of type T using the provided function f.
func Convert[F any, T any](a []F, f func(F) T) []T {
	if a == nil {
		return []T{}
	}

	tmp := make([]T, 0, len(a))
	for _, v := range a {
		tmp = append(tmp, f(v))
	}
	return tmp
}

func ToMapFunc[T any, K comparable, V any](a []T, cnv func(t T) (K, V)) map[K]V {
	m := make(map[K]V, len(a))

	for i := range a {
		k, v := cnv(a[i])
		m[k] = v
	}
	return m
}
