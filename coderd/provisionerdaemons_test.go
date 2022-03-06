package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestProvisionerDaemons(t *testing.T) {
	// Tests for properly processing specific job types should be placed
	// in their respective files.
	t.Parallel()

	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	_ = coderdtest.NewProvisionerDaemon(t, client)
	require.Eventually(t, func() bool {
		daemons, err := client.ProvisionerDaemonsByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		return len(daemons) > 0
	}, 3*time.Second, 50*time.Millisecond)
}
