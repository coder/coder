package slice

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
// This is a slow function on large lists.
// TODO: Sort elements and implement a faster search algorithm if we
// really start to use this.
func Unique[T comparable](a []T) []T {
	cpy := make([]T, 0, len(a))
	for _, v := range a {
		v := v
		if !Contains(cpy, v) {
			cpy = append(cpy, v)
		}
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
