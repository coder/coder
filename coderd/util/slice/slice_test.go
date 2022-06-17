package slice_test

import (
	"github.com/coder/coder/coderd/util/slice"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContains(t *testing.T) {
	testContains(t, []int{1, 2, 3, 4, 5}, []int{1, 2, 3, 4, 5}, []int{0, 6, -1, -2, 100})
	testContains(t, []string{"hello", "world", "foo", "bar", "baz"}, []string{"hello", "world", "baz"}, []string{"not", "words", "in", "set"})
	testContains(t,
		[]uuid.UUID{uuid.New(), uuid.MustParse("c7c6686d-a93c-4df2-bef9-5f837e9a33d5"), uuid.MustParse("8f3b3e0b-2c3f-46a5-a365-fd5b62bd8818")},
		[]uuid.UUID{uuid.MustParse("c7c6686d-a93c-4df2-bef9-5f837e9a33d5")},
		[]uuid.UUID{uuid.MustParse("1d00e27d-8de6-46f8-80d5-1da0ca83874a")},
	)
}

func testContains[T comparable](t *testing.T, set []T, in []T, out []T) {
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
