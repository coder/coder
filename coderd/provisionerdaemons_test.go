package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestGetProvisionerDaemons(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		db, ps, _ := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
		client := coderdtest.New(t, &coderdtest.Options{
			Database:                 db,
			Pubsub:                   ps,
			IncludeProvisionerDaemon: true,
		})
		coderdtest.CreateFirstUser(t, client)

		ctx := testutil.Context(t, testutil.WaitMedium)

		daemons, err := client.ProvisionerDaemons(ctx)
		require.NoError(t, err)
		require.Len(t, daemons, 1)
	})
}
