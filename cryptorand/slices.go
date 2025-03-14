package cryptorand
import (
	"fmt"
	"errors"
)
// Element returns a random element of the slice. An error will be returned if
// the slice has no elements in it.
func Element[T any](s []T) (out T, err error) {
	if len(s) == 0 {
		return out, errors.New("slice must have at least one element")
	}
	i, err := Intn(len(s))
	if err != nil {
		return out, fmt.Errorf("generate random integer from 0-%v: %w", len(s), err)
	}
	return s[i], nil
}
