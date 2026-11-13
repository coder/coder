package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

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

// TestChatProjectArchiveAudit checks that archiving and unarchiving a
// project are audited as writes whose diff shows archived_at.
func TestChatProjectArchiveAudit(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true))
	ownerClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			DeploymentValues: coderdtest.DeploymentValues(t),
			Database:         db,
			Pubsub:           ps,
			Auditor:          auditor,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAuditLog: 1},
		},
	})
	owner := codersdk.NewExperimentalClient(ownerClient)
	orgID := firstUser.OrganizationID
	ctx := testutil.Context(t, testutil.WaitLong)

	project, err := owner.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{Name: "Audited Project"})
	require.NoError(t, err)
	archived, err := owner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{})
	require.NoError(t, err)
	require.NotNil(t, archived.ArchivedAt)
	_, err = owner.UnarchiveChatProject(ctx, orgID, project.ID)
	require.NoError(t, err)

	rows, err := db.GetAuditLogsOffset(dbauthz.AsSystemRestricted(ctx), database.GetAuditLogsOffsetParams{
		ResourceType: string(database.ResourceTypeChatProject),
		ResourceID:   project.ID,
		Action:       string(database.AuditActionWrite),
		LimitOpt:     10,
	})
	require.NoError(t, err)
	// Logs are newest first: the unarchive, then the archive.
	require.Len(t, rows, 2)
	for i, wantArchived := range []bool{false, true} {
		log := rows[i].AuditLog
		require.EqualValues(t, http.StatusOK, log.StatusCode)
		require.Equal(t, firstUser.UserID, log.UserID)
		// sql.NullTime fields diff as {Time, Valid}.
		type nullTime struct {
			Valid bool `json:"Valid"`
		}
		var diff map[string]struct {
			Old nullTime `json:"Old"`
			New nullTime `json:"New"`
		}
		require.NoError(t, json.Unmarshal(log.Diff, &diff))
		require.Len(t, diff, 1, "only archived_at changes")
		field, ok := diff["archived_at"]
		require.True(t, ok)
		require.Equal(t, wantArchived, field.New.Valid)
		require.Equal(t, !wantArchived, field.Old.Valid)
	}
}
