package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/testutil"
)

func TestCliGen(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	cb, err := GenerateData(ctx, "../../codersdk")
	require.NoError(t, err)
	require.NotNil(t, cb)
}
