package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestSandboxProvisionerDaemonServe(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		tags  map[string]string
		mixed bool
		valid bool
	}{
		{name: "Local", tags: map[string]string{"sandbox_host": "local"}, valid: true},
		{name: "MissingHost"},
		{name: "OtherHost", tags: map[string]string{"sandbox_host": "other"}},
		{name: "MixedTypes", tags: map[string]string{"sandbox_host": "local"}, mixed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, owner := coderdenttest.New(t, &coderdenttest.Options{
				LicenseOptions: &coderdenttest.LicenseOptions{Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				}},
			})
			ctx := testutil.Context(t, testutil.WaitLong)
			types := []codersdk.ProvisionerType{codersdk.ProvisionerTypeSandbox}
			if tc.mixed {
				types = append(types, codersdk.ProvisionerTypeTerraform)
			}
			daemon, err := client.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{ //nolint:gocritic // Test routing validation with an owner authorized to register daemons.
				Name:         testutil.MustRandString(t, 16),
				Organization: owner.OrganizationID,
				Provisioners: types,
				Tags:         tc.tags,
			})
			if !tc.valid {
				var apiErr *codersdk.Error
				require.ErrorAs(t, err, &apiErr)
				require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
				return
			}
			require.NoError(t, err)
			t.Cleanup(func() { _ = daemon.DRPCConn().Close() })
			daemons, err := client.ProvisionerDaemons(ctx) //nolint:gocritic // Test assertion.
			require.NoError(t, err)
			require.Len(t, daemons, 1)
			require.Equal(t, types, daemons[0].Provisioners)
			require.Equal(t, "local", daemons[0].Tags["sandbox_host"])
		})
	}
}
