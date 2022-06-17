package slice

func Contains[T comparable](haystack []T, needle T) bool {
	for _, hay := range haystack {
		if needle == hay {
			return true
		}
	}
	return false
}
