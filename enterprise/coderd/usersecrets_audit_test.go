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
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestUserSecretAuditDiffRedaction(t *testing.T) {
	// Ensure secret values never appear in plaintext in audit diffs. The
	// enterprise auditor needs to be used because it writes actual diffs.
	// We read straight from the audit_logs table to exercise the full
	// insert, filter, dbauthz read path.
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(
		db,
		entaudit.DefaultFilter,
		backends.NewPostgres(db, true),
	)

	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
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
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	initialDescription := "initial"
	initialValue := "initial-secret-value"
	secret, err := memberClient.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
		Name:        "createDiff-target",
		Description: initialDescription,
		Value:       initialValue,
	})
	require.NoError(t, err)

	newDescription := "after"
	newValue := "new-secret-value"
	_, err = memberClient.UpdateUserSecret(ctx, codersdk.Me, secret.Name, codersdk.UpdateUserSecretRequest{
		Description: &newDescription,
		Value:       &newValue,
	})
	require.NoError(t, err)

	// Read straight from the database. AsSystemRestricted is necessary because
	// the test does not authenticate as an admin when querying the store directly.
	rows, err := db.GetAuditLogsOffset(
		dbauthz.AsSystemRestricted(ctx),
		database.GetAuditLogsOffsetParams{
			ResourceType: string(database.ResourceTypeUserSecret),
			LimitOpt:     10,
		},
	)
	require.NoError(t, err)
	require.Equal(t, len(rows), 2, "expected exactly two rows")
	// GetAuditLogsOffset returns entries sorted by time in descending order.
	createLog := rows[1].AuditLog
	updateLog := rows[0].AuditLog

	var createDiff audit.Map
	require.NoError(t, json.Unmarshal(createLog.Diff, &createDiff))

	// Creation must show both old and new non-secret values verbatim.
	if assert.Contains(t, createDiff, "description", "tracked field missing from createDiff") {
		assert.Equal(t, "", createDiff["description"].Old)
		assert.Equal(t, initialDescription, createDiff["description"].New)
		assert.False(t, createDiff["description"].Secret)
	}

	// Creation must record that it changed but with zero-valued old/new and
	// indicate the value is secret.
	if assert.Contains(t, createDiff, "value", "value field missing from createDiff") {
		assert.True(t, createDiff["value"].Secret, "value field must be marked secret")
		assert.Equal(t, "", createDiff["value"].Old)
		assert.Equal(t, "", createDiff["value"].New)
	}

	// Ensure ignored fields are excluded from the create diff.
	assert.NotContains(t, createDiff, "value_key_id")
	assert.NotContains(t, createDiff, "created_at")
	assert.NotContains(t, createDiff, "updated_at")

	var updateDiff audit.Map
	require.NoError(t, json.Unmarshal(updateLog.Diff, &updateDiff))

	// Update must show both old and new non-secret values verbatim.
	if assert.Contains(t, updateDiff, "description", "tracked field missing from updateDiff") {
		assert.Equal(t, initialDescription, updateDiff["description"].Old)
		assert.Equal(t, newDescription, updateDiff["description"].New)
		assert.False(t, updateDiff["description"].Secret)
	}

	// Update must record that it changed but with zero-valued old/new and
	// indicate the value is secret.
	if assert.Contains(t, updateDiff, "value", "value field missing from updateDiff") {
		assert.True(t, updateDiff["value"].Secret, "value field must be marked secret")
		assert.Equal(t, "", updateDiff["value"].Old)
		assert.Equal(t, "", updateDiff["value"].New)
	}

	// Ensure ignored fields are excluded from update diff.
	assert.NotContains(t, updateDiff, "value_key_id")
	assert.NotContains(t, updateDiff, "created_at")
	assert.NotContains(t, updateDiff, "updated_at")
}
