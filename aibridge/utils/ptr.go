package utils

// PtrTo returns a reference to v.
func PtrTo[T any](v T) *T {
	return &v
}
