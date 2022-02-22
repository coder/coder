package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestProvisionerDaemons(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.NewProvisionerDaemon(t, client)
	require.Eventually(t, func() bool {
		daemons, err := client.ProvisionerDaemons(context.Background())
		require.NoError(t, err)
		return len(daemons) > 0
	}, 3*time.Second, 50*time.Millisecond)
}
