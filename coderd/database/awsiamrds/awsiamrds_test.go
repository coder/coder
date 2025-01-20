package awsiamrds_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd/database/awsiamrds"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestDriver(t *testing.T) {
	t.Parallel()
	// Be sure to set AWS_DEFAULT_REGION to the database region as well.
	// Example:
	// export AWS_DEFAULT_REGION=us-east-2;
	// export DBAWSIAMRDS_TEST_URL="postgres://user@host:5432/dbname";
	url := os.Getenv("DBAWSIAMRDS_TEST_URL")
	if url == "" {
		t.Log("skipping test; no DBAWSIAMRDS_TEST_URL set")
		t.Skip()
	}

	logger := testutil.Logger(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	sqlDriver, err := awsiamrds.Register(ctx, "postgres")
	require.NoError(t, err)

	db, err := cli.ConnectToPostgres(ctx, testutil.Logger(t), sqlDriver, url, migrations.Up)
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	i, err := db.QueryContext(ctx, "select 1;")
	require.NoError(t, err)
	defer func() {
		_ = i.Close()
	}()

	require.True(t, i.Next())
	var one int
	require.NoError(t, i.Scan(&one))
	require.Equal(t, 1, one)

	ps, err := pubsub.New(ctx, logger, db, url)
	require.NoError(t, err)
	defer ps.Close()

	gotChan := make(chan struct{})
	subCancel, err := ps.Subscribe("test", func(_ context.Context, _ []byte) {
		close(gotChan)
	})
	require.NoError(t, err)
	defer subCancel()

	err = ps.Publish("test", []byte("hello"))
	require.NoError(t, err)

	select {
	case <-gotChan:
	case <-ctx.Done():
		require.Fail(t, "timed out waiting for message")
	}
}
