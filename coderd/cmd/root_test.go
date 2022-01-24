package cmd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/cmd"
)

func TestRoot(t *testing.T) {
	t.Parallel()
	ctx, cancelFunc := context.WithCancel(context.Background())
	go cancelFunc()
	err := cmd.Root().ExecuteContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
