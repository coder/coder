package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/testutil"
)

func TestCliGen(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	log := slog.Make(sloghuman.Sink(os.Stderr))
	cb, err := GenerateData(ctx, log, "../../codersdk")
	require.NoError(t, err)
	require.NotNil(t, cb)
}
