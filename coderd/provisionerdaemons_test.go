package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestProvisionerDaemons(t *testing.T) {
	// Tests for properly processing specific job
	// types should be placed in their respective
	// resource location.
	//
	// eg. project import is a project-related job
	t.Parallel()

	client := coderdtest.New(t)
	_ = coderdtest.NewProvisionerDaemon(t, client)
	require.Eventually(t, func() bool {
		daemons, err := client.ProvisionerDaemons(context.Background())
		require.NoError(t, err)
		return len(daemons) > 0
	}, time.Second, 25*time.Millisecond)
}
