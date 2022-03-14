package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
)

func TestStart(t *testing.T) {
	t.Parallel()
	ctx, cancelFunc := context.WithCancel(context.Background())
	go cancelFunc()
	root, _ := clitest.New(t, "start")
	err := root.ExecuteContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
