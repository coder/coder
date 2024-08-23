package testutil_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// nolint:paralleltest // this is the whole point
func Test_IsParallel_False(t *testing.T) {
	require.False(t, testutil.IsParallel(t))
}

func Test_IsParallel_True(t *testing.T) {
	t.Parallel()
	require.True(t, testutil.IsParallel(t))
}
