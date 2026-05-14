package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestUserSkillAuditDiffRedaction(t *testing.T) {
	// Ensure raw skill Markdown never appears in plaintext in audit diffs. The
	// enterprise auditor needs to be used because it writes actual diffs.
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
	member := codersdk.NewExperimentalClient(memberClient)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	initialContent := enterpriseUserSkillMarkdown("audit-redaction", "initial", "initial secret body")
	skill, err := member.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
		Content: initialContent,
	})
	require.NoError(t, err)

	newContent := enterpriseUserSkillMarkdown("audit-redaction", "after", "new secret body")
	_, err = member.UpdateUserSkill(ctx, codersdk.Me, skill.Name, codersdk.UpdateUserSkillRequest{
		Content: newContent,
	})
	require.NoError(t, err)

	rows, err := db.GetAuditLogsOffset(
		dbauthz.AsSystemRestricted(ctx),
		database.GetAuditLogsOffsetParams{
			ResourceType: string(database.ResourceTypeUserSkill),
			LimitOpt:     10,
		},
	)
	require.NoError(t, err)
	require.Equal(t, len(rows), 2, "expected exactly two rows")
	createLog := rows[1].AuditLog
	updateLog := rows[0].AuditLog

	var createDiff audit.Map
	require.NoError(t, json.Unmarshal(createLog.Diff, &createDiff))
	if assert.Contains(t, createDiff, "description", "tracked field missing from create diff") {
		assert.Equal(t, "", createDiff["description"].Old)
		assert.Equal(t, "initial", createDiff["description"].New)
		assert.False(t, createDiff["description"].Secret)
	}
	if assert.Contains(t, createDiff, "content", "content field missing from create diff") {
		assert.True(t, createDiff["content"].Secret, "content field must be marked secret")
		assert.Equal(t, "", createDiff["content"].Old)
		assert.Equal(t, "", createDiff["content"].New)
	}
	assert.NotContains(t, string(createLog.Diff), initialContent)

	var updateDiff audit.Map
	require.NoError(t, json.Unmarshal(updateLog.Diff, &updateDiff))
	if assert.Contains(t, updateDiff, "description", "tracked field missing from update diff") {
		assert.Equal(t, "initial", updateDiff["description"].Old)
		assert.Equal(t, "after", updateDiff["description"].New)
		assert.False(t, updateDiff["description"].Secret)
	}
	if assert.Contains(t, updateDiff, "content", "content field missing from update diff") {
		assert.True(t, updateDiff["content"].Secret, "content field must be marked secret")
		assert.Equal(t, "", updateDiff["content"].Old)
		assert.Equal(t, "", updateDiff["content"].New)
	}
	assert.NotContains(t, string(updateLog.Diff), initialContent)
	assert.NotContains(t, string(updateLog.Diff), newContent)
	assert.NotContains(t, updateDiff, "created_at")
	assert.NotContains(t, updateDiff, "updated_at")
}

func enterpriseUserSkillMarkdown(name string, description string, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", name, description, body)
}
