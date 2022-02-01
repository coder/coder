package codersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestProvisioners(t *testing.T) {
	t.Parallel()
	t.Run("ListDaemons", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.ProvisionerDaemons(context.Background())
		require.NoError(t, err)
	})
}
