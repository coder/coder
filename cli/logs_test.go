package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

func TestLogs(t *testing.T) {
	t.Parallel()

	t.Run("LogsInvalidBuildID", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "logs", "invalid-uuid")
		clitest.SetupConfig(t, client, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancelFunc()

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "invalid build ID"))
	})

	t.Run("LogsNonexistentBuild", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "logs", uuid.New().String())
		clitest.SetupConfig(t, client, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancelFunc()

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "get build logs"))
	})
}
