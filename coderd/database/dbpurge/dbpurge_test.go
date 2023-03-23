package dbpurge_test

import (
	"context"
	"testing"

	"go.uber.org/goleak"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbpurge"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// Ensures no goroutines leak.
func TestPurge(t *testing.T) {
	t.Parallel()
	purger := dbpurge.New(context.Background(), slogtest.Make(t, nil), dbfake.New())
	err := purger.Close()
	require.NoError(t, err)
}
