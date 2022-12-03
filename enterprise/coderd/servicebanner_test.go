package coderd_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
)

func TestServiceBanners(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	adminClient := coderdenttest.New(t, &coderdenttest.Options{})

	adminUser := coderdtest.CreateFirstUser(t, adminClient)

	// Even without a license, the banner should return as disabled.
	sb, err := adminClient.ServiceBanner(ctx)
	require.NoError(t, err)
	require.False(t, sb.Enabled)

	coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
		ServiceBanners: true,
	})

	// Default state
	sb, err = adminClient.ServiceBanner(ctx)
	require.NoError(t, err)
	require.False(t, sb.Enabled)

	basicUserClient := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	// Regular user should be unable to set the banner
	sb.Enabled = true
	err = basicUserClient.SetServiceBanner(ctx, sb)
	require.Error(t, err)
	var sdkError *codersdk.Error
	require.True(t, errors.As(err, &sdkError))
	require.Equal(t, http.StatusForbidden, sdkError.StatusCode())

	// But an admin can
	wantBanner := sb
	wantBanner.Enabled = true
	wantBanner.Message = "Hey"
	wantBanner.BackgroundColor = "#00FF00"
	err = adminClient.SetServiceBanner(ctx, wantBanner)
	require.NoError(t, err)
	gotBanner, err := adminClient.ServiceBanner(ctx)
	require.NoError(t, err)
	require.Equal(t, wantBanner, gotBanner)

	// But even an admin can't give a bad color
	wantBanner.BackgroundColor = "#bad color"
	err = adminClient.SetServiceBanner(ctx, wantBanner)
	require.Error(t, err)
}
