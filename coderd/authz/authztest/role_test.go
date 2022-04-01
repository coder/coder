package authztest_test

import (
	"testing"

	"github.com/coder/coder/coderd/authz/authztest"
	crand "github.com/coder/coder/cryptorand"
	"github.com/stretchr/testify/require"
)

func Test_NewRole(t *testing.T) {
	for i := 0; i < 50; i++ {
		sets := make([]authztest.Iterable, 1+(i%4))
		var total int = 1
		for j := range sets {
			size := 1 + must(crand.Intn(3))
			if i < 5 {
				// Enforce 1 size sets for some cases
				size = 1
			}
			sets[j] = RandomSet(size)
			total *= size
		}

		crossProduct := authztest.NewRole(sets...)
		t.Run("CrossProduct", func(t *testing.T) {
			require.Equal(t, total, crossProduct.Size(), "correct N")
			require.Equal(t, len(sets), crossProduct.ReturnSize(), "return size")
			var c int
			crossProduct.Each(func(set authztest.Set) {
				require.Equal(t, crossProduct.ReturnSize(), len(set), "each set is correct size")
				c++
			})
			require.Equal(t, total, c, "each run N times")

			if crossProduct.Size() > 1 {
				crossProduct.Reset()
				require.Truef(t, crossProduct.Next(), "reset should always make this true")
			}
		})

		t.Run("NestedRoles", func(t *testing.T) {
			merged := authztest.NewRole(sets[0])
			for i := 1; i < len(sets); i++ {
				merged = authztest.NewRole(sets[i], merged)
			}

			require.Equal(t, crossProduct.Size(), merged.Size())
			var c int
			merged.Each(func(set authztest.Set) {
				require.Equal(t, merged.ReturnSize(), len(set), "each set is correct size")
				c++
			})
			require.Equal(t, merged.Size(), c, "each run N times")
		})
	}
}
