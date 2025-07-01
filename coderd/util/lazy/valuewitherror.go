package lazy

type ValueWithError[T any] struct {
	inner Value[result[T]]
}

type result[T any] struct {
	value T
	err   error
}

// NewWithError allows you to provide a lazy initializer that can fail.
func NewWithError[T any](fn func() (T, error)) *ValueWithError[T] {
	return &ValueWithError[T]{
		inner: Value[result[T]]{fn: func() result[T] {
			value, err := fn()
			return result[T]{value: value, err: err}
		}},
	}
}

func (v *ValueWithError[T]) Load() (T, error) {
	result := v.inner.Load()
	return result.value, result.err
}
