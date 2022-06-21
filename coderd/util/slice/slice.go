package slice

func Contains[T comparable](haystack []T, needle T) bool {
	for _, hay := range haystack {
		if needle == hay {
			return true
		}
	}
	return false
}

// Overlap returns if the 2 sets have any overlap (element(s) in common)
func Overlap[T comparable](a []T, b []T) bool {
	// For each element in b, if at least 1 is contained in 'a',
	// return true.
	for _, element := range b {
		if Contains(a, element) {
			return true
		}
	}
	return false
}
