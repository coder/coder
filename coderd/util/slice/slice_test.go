package slice_test

import (
	"math/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/slice"
)

func TestSameElements(t *testing.T) {
	t.Parallel()

	// True
	assertSameElements(t, []int{})
	assertSameElements(t, []int{1, 2, 3})
	assertSameElements(t, slice.New("a", "b", "c"))
	assertSameElements(t, slice.New(uuid.New(), uuid.New(), uuid.New()))

	// False
	assert.False(t, slice.SameElements([]int{1, 2, 3}, []int{1, 2, 3, 4}))
	assert.False(t, slice.SameElements([]int{1, 2, 3}, []int{1, 2}))
	assert.False(t, slice.SameElements([]int{1, 2, 3}, []int{}))
	assert.False(t, slice.SameElements([]int{}, []int{1, 2, 3}))
	assert.False(t, slice.SameElements([]int{1, 2, 3}, []int{1, 2, 4}))
	assert.False(t, slice.SameElements([]int{1}, []int{2}))
}

func assertSameElements[T comparable](t *testing.T, elements []T) {
	cpy := make([]T, len(elements))
	copy(cpy, elements)
	rand.Shuffle(len(cpy), func(i, j int) {
		cpy[i], cpy[j] = cpy[j], cpy[i]
	})
	assert.True(t, slice.SameElements(elements, cpy))
}

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

func TestAscending(t *testing.T) {
	t.Parallel()

	assert.Equal(t, -1, slice.Ascending(1, 2))
	assert.Equal(t, 0, slice.Ascending(1, 1))
	assert.Equal(t, 1, slice.Ascending(2, 1))
}

func TestDescending(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1, slice.Descending(1, 2))
	assert.Equal(t, 0, slice.Descending(1, 1))
	assert.Equal(t, -1, slice.Descending(2, 1))
}

func TestOmit(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"a", "b", "f"},
		slice.Omit([]string{"a", "b", "c", "d", "e", "f"}, "c", "d", "e"),
	)
}
