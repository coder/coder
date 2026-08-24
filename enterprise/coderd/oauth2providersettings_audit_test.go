package coderd_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// TestOAuth2ProviderSettingsAuditDiff guards against a regression where
// disabling dynamic client registration produced an empty audit diff. The
// handler only ever set aReq.New, leaving aReq.Old at its zero value
// (DynamicClientRegistrationEnabled: false). Enabling (false -> true)
// happened to diff correctly since the zero value matched the real prior
// state, masking that disabling (true -> false) diffed the zero value
// against itself and showed no change at all. The mock auditor used in
// coderd's own oauth2_provider_settings_test.go always returns an empty
// diff, so only the real enterprise auditor used here can catch this.
func TestOAuth2ProviderSettingsAuditDiff(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(
		db,
		entaudit.DefaultFilter,
		backends.NewPostgres(db, true),
	)

	ownerClient, _ := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
			Auditor:  auditor,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAuditLog: 1,
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	//nolint:gocritic // Updating OAuth2 provider settings is owner-only.
	_, err := ownerClient.PutOAuth2ProviderSettings(ctx, codersdk.OAuth2ProviderSettings{
		DynamicClientRegistrationEnabled: ptr.Ref(true),
	})
	require.NoError(t, err)

	//nolint:gocritic // Updating OAuth2 provider settings is owner-only.
	_, err = ownerClient.PutOAuth2ProviderSettings(ctx, codersdk.OAuth2ProviderSettings{
		DynamicClientRegistrationEnabled: ptr.Ref(false),
	})
	require.NoError(t, err)

	// Read straight from the database. AsSystemRestricted is necessary
	// because the test does not authenticate as an admin when querying the
	// store directly.
	rows, err := db.GetAuditLogsOffset(
		dbauthz.AsSystemRestricted(ctx),
		database.GetAuditLogsOffsetParams{
			ResourceType: string(database.ResourceTypeOauth2ProviderSettings),
			LimitOpt:     10,
		},
	)
	require.NoError(t, err)
	require.Len(t, rows, 2, "expected exactly two rows")
	// Both updates use the same action, so identify them by the new value.
	var enableDiff, disableDiff audit.Map
	for _, row := range rows {
		var diff audit.Map
		require.NoError(t, json.Unmarshal(row.AuditLog.Diff, &diff))
		require.Contains(t, diff, "dynamic_client_registration_enabled", "tracked field missing from diff")
		enabled, ok := diff["dynamic_client_registration_enabled"].New.(bool)
		require.True(t, ok, "expected bool new value in diff")
		if enabled {
			enableDiff = diff
		} else {
			disableDiff = diff
		}
	}
	require.NotNil(t, enableDiff, "missing audit log for enabling registration")
	require.NotNil(t, disableDiff, "missing audit log for disabling registration")

	assert.Equal(t, false, enableDiff["dynamic_client_registration_enabled"].Old)
	assert.Equal(t, true, enableDiff["dynamic_client_registration_enabled"].New)

	assert.Equal(t, true, disableDiff["dynamic_client_registration_enabled"].Old)
	assert.Equal(t, false, disableDiff["dynamic_client_registration_enabled"].New)
}
