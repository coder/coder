package coderd_test

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

type captureBackend struct {
	mu   sync.Mutex
	logs []database.AuditLog
}

func (*captureBackend) Decision() entaudit.FilterDecision {
	return entaudit.FilterDecisionStore | entaudit.FilterDecisionExport
}

func (b *captureBackend) Export(_ context.Context, alog database.AuditLog, _ entaudit.BackendDetails) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logs = append(b.logs, alog)
	return nil
}

func (b *captureBackend) AuditLogs() []database.AuditLog {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.logs)
}

func TestUserSecretAuditDiffRedaction(t *testing.T) {
	// Ensure secret values never appear in plaintext in audit diffs. The
	// enterprise auditor needs to be used because it writes actual diffs.
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	backend := &captureBackend{}
	auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, backend)

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

	secret, err := memberClient.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
		Name:        "diff-target",
		Value:       "old-secret-value",
		Description: "before",
	})
	require.NoError(t, err)

	newDescription := "after"
	newValue := "new-secret-value"
	_, err = memberClient.UpdateUserSecret(ctx, codersdk.Me, secret.Name, codersdk.UpdateUserSecretRequest{
		Description: &newDescription,
		Value:       &newValue,
	})
	require.NoError(t, err)

	var writeLog *database.AuditLog
	for i, l := range backend.AuditLogs() {
		if l.ResourceType == database.ResourceTypeUserSecret && l.Action == database.AuditActionWrite {
			writeLog = &backend.AuditLogs()[i]
			break
		}
	}
	require.NotNilf(t, writeLog, "expected a user_secret write log; got %+v", backend.AuditLogs())

	var diff audit.Map
	require.NoError(t, json.Unmarshal(writeLog.Diff, &diff))

	// The diff must show both old and new non-secret values verbatim.
	if assert.Contains(t, diff, "description", "tracked field missing from diff") {
		assert.Equal(t, "before", diff["description"].Old)
		assert.Equal(t, "after", diff["description"].New)
		assert.False(t, diff["description"].Secret)
	}

	// The diff must record that it changed but with zero-valued old/new and
	// indicate the value is secret.
	if assert.Contains(t, diff, "value", "value field missing from diff") {
		assert.True(t, diff["value"].Secret, "value field must be marked secret")
		assert.NotEqual(t, "old-secret-value", diff["value"].Old)
		assert.NotEqual(t, "new-secret-value", diff["value"].New)
	}

	// Ensure ignored fields are not included in the diff.
	assert.NotContains(t, diff, "value_key_id")
	assert.NotContains(t, diff, "created_at")
	assert.NotContains(t, diff, "updated_at")
}
