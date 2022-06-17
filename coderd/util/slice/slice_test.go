package slice_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/util/slice"
)

func TestContains(t *testing.T) {
	t.Parallel()

	assertSetContains(t, []int{1, 2, 3, 4, 5}, []int{1, 2, 3, 4, 5}, []int{0, 6, -1, -2, 100})
	assertSetContains(t, []string{"hello", "world", "foo", "bar", "baz"}, []string{"hello", "world", "baz"}, []string{"not", "words", "in", "set"})
	assertSetContains(t,
		[]uuid.UUID{uuid.New(), uuid.MustParse("c7c6686d-a93c-4df2-bef9-5f837e9a33d5"), uuid.MustParse("8f3b3e0b-2c3f-46a5-a365-fd5b62bd8818")},
		[]uuid.UUID{uuid.MustParse("c7c6686d-a93c-4df2-bef9-5f837e9a33d5")},
		[]uuid.UUID{uuid.MustParse("1d00e27d-8de6-46f8-80d5-1da0ca83874a")},
	)
}

func assertSetContains[T comparable](t *testing.T, set []T, in []T, out []T) {
	t.Helper()
	for _, e := range set {
		require.True(t, slice.Contains(set, e), "elements in set should be in the set")
	}
	for _, e := range in {
		require.True(t, slice.Contains(set, e), "expect element in set")
	}
	for _, e := range out {
		require.False(t, slice.Contains(set, e), "expect element in set")
	}
}
