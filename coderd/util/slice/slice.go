package slice

import (
	"golang.org/x/exp/constraints"
)

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

// Overlap returns if the 2 sets have any overlap (element(s) in common)
func Overlap[T comparable](a []T, b []T) bool {
	return OverlapCompare(a, b, func(a, b T) bool {
		return a == b
	})
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
