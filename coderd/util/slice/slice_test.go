package slice_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/util/slice"
)

func TestUnique(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t,
		[]int{1, 2, 3, 4, 5},
		slice.Unique([]int{
			1, 2, 3, 4, 5, 1, 2, 3, 4, 5,
		}))

	require.ElementsMatch(t,
		[]string{"a"},
		slice.Unique([]string{
			"a", "a", "a",
		}))
}

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

func TestOverlap(t *testing.T) {
	t.Parallel()

	assertSetOverlaps(t, true, []int{1, 2, 3, 4, 5}, []int{1, 2, 3, 4, 5})
	assertSetOverlaps(t, true, []int{10}, []int{10})

	assertSetOverlaps(t, false, []int{1, 2, 3, 4, 5}, []int{6, 7, 8, 9})
	assertSetOverlaps(t, false, []int{1, 2, 3, 4, 5}, []int{})
	assertSetOverlaps(t, false, []int{}, []int{})

	assertSetOverlaps(t, true, []string{"hello", "world", "foo", "bar", "baz"}, []string{"hello", "world", "baz"})
	assertSetOverlaps(t, true,
		[]uuid.UUID{uuid.New(), uuid.MustParse("c7c6686d-a93c-4df2-bef9-5f837e9a33d5"), uuid.MustParse("8f3b3e0b-2c3f-46a5-a365-fd5b62bd8818")},
		[]uuid.UUID{uuid.MustParse("c7c6686d-a93c-4df2-bef9-5f837e9a33d5")},
	)
}

func assertSetOverlaps[T comparable](t *testing.T, overlap bool, a []T, b []T) {
	t.Helper()
	for _, e := range a {
		require.True(t, slice.Overlap(a, []T{e}), "elements in set should overlap with itself")
	}
	for _, e := range b {
		require.True(t, slice.Overlap(b, []T{e}), "elements in set should overlap with itself")
	}

	require.Equal(t, overlap, slice.Overlap(a, b))
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
