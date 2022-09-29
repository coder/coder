package coderd_test

import (
	"context"
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceQuota(t *testing.T) {
	t.Parallel()
	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client := coderdenttest.New(t, &coderdenttest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			WorkspaceQuota: true,
		})
		q1, err := client.WorkspaceQuota(ctx, "me")
		require.NoError(t, err)
		require.EqualValues(t, q1.UserWorkspaceLimit, 0)

	})
	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		max := 3
		client := coderdenttest.New(t, &coderdenttest.Options{
			UserWorkspaceQuota: max,
		})
		user := coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			WorkspaceQuota: true,
		})
		q1, err := client.WorkspaceQuota(ctx, "me")
		require.NoError(t, err)
		require.EqualValues(t, q1.UserWorkspaceLimit, max)

		// ensure other user IDs work too
		u2, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "whatever@yo.com",
			Username:       "haha",
			Password:       "laskjdnvkaj",
			OrganizationID: user.OrganizationID,
		})
		q2, err := client.WorkspaceQuota(ctx, u2.ID.String())
		require.NoError(t, err)
		require.EqualValues(t, q1, q2)
	})
}
