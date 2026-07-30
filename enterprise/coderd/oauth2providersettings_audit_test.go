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
	require.Equal(t, 2, len(rows), "expected exactly two rows")
	// GetAuditLogsOffset returns entries sorted by time in descending order.
	enableLog := rows[1].AuditLog
	disableLog := rows[0].AuditLog

	var enableDiff audit.Map
	require.NoError(t, json.Unmarshal(enableLog.Diff, &enableDiff))
	if assert.Contains(t, enableDiff, "dynamic_client_registration_enabled", "tracked field missing from enableDiff") {
		assert.Equal(t, false, enableDiff["dynamic_client_registration_enabled"].Old)
		assert.Equal(t, true, enableDiff["dynamic_client_registration_enabled"].New)
	}

	var disableDiff audit.Map
	require.NoError(t, json.Unmarshal(disableLog.Diff, &disableDiff))
	if assert.Contains(t, disableDiff, "dynamic_client_registration_enabled", "tracked field missing from disableDiff") {
		assert.Equal(t, true, disableDiff["dynamic_client_registration_enabled"].Old)
		assert.Equal(t, false, disableDiff["dynamic_client_registration_enabled"].New)
	}
}
