package cmd_test

import (
	"context"
	"testing"

	"github.com/coder/coder/coderd/cmd"
	"github.com/stretchr/testify/require"
)

func TestRoot(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	go cancelFunc()
	err := cmd.Root().ExecuteContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
