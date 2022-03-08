package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
)

func TestDaemon(t *testing.T) {
	t.Parallel()
	ctx, cancelFunc := context.WithCancel(context.Background())
	go cancelFunc()
	root, _ := clitest.New(t, "daemon")
	err := root.ExecuteContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
