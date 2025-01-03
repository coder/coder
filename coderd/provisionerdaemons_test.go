package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

func TestGetProvisionerDaemons(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		daemons, err := memberClient.ProvisionerDaemons(ctx)
		require.NoError(t, err)
		require.Len(t, daemons, 1)
	})
}
