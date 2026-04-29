package dlppolicy_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/dlppolicy"
	"github.com/coder/coder/v2/testutil"
)

func TestForAgent(t *testing.T) {
	t.Parallel()

	t.Run("AgentWithPolicy", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			OrganizationID: org.ID,
		})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			JobID:          job.ID,
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})

		policyID := uuid.New()
		_, err := db.InsertTemplateVersionDLPPolicy(ctx, database.InsertTemplateVersionDLPPolicyParams{
			ID:                   policyID,
			TemplateVersionID:    tv.ID,
			Name:                 "strict",
			SshAccess:            true,
			WebTerminalAccess:    false,
			PortForwardingAccess: true,
			AllowedApplications:  []string{"code-server"},
			CreatedAt:            time.Now(),
		})
		require.NoError(t, err)

		res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID:  res.ID,
			DlpPolicyID: uuid.NullUUID{UUID: policyID, Valid: true},
		})

		got, err := dlppolicy.ForAgent(context.Background(), db, agent.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, "strict", got.Name)
		require.True(t, got.SshAccess)
		require.False(t, got.WebTerminalAccess)
	})

	t.Run("AgentWithoutPolicy", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: org.ID,
		})
		res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: res.ID,
		})

		got, err := dlppolicy.ForAgent(context.Background(), db, agent.ID)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("AgentNotFound", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		got, err := dlppolicy.ForAgent(context.Background(), db, uuid.New())
		require.NoError(t, err)
		require.Nil(t, got)
	})
}
