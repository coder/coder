package chatdebug

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReuseStep_PreservesExistingHolder(t *testing.T) {
	t.Parallel()

	ctx := ReuseStep(context.Background())
	first, ok := reuseHolderFromContext(ctx)
	require.True(t, ok)

	reused := ReuseStep(ctx)
	second, ok := reuseHolderFromContext(reused)
	require.True(t, ok)
	require.Same(t, first, second)
}
