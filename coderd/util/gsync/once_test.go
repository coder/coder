package gsync_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/util/gsync"
)

func TestOnce(t *testing.T) {
	t.Parallel()

	want := 11
	var once gsync.Once[int]

	var performed int

	for i := 0; i < 10; i++ {
		got := once.Do(func() int {
			performed++
			return want
		})
		require.Equal(t, want, got)
		require.Equal(t, 1, performed)
	}
}
