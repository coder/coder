package coderd_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestChatRenderTemplateParameters drives the read_template hook directly
// with a plain context, so it must authorize as the owner itself, and checks
// that an owner-derived default resolves to that owner's value.
func TestChatRenderTemplateParameters(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ownerClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   ps,
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	mainTF, err := os.ReadFile("testdata/parameters/public_key/main.tf")
	require.NoError(t, err)
	plan, err := os.ReadFile("testdata/parameters/public_key/plan.json")
	require.NoError(t, err)
	files := echo.WithExtraFiles(map[string][]byte{"main.tf": mainTF})
	files.ProvisionPlan = []*proto.Response{{
		Type: &proto.Response_Plan{Plan: &proto.PlanComplete{Plan: plan}},
	}}
	version := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, files)
	coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, version.ID)
	template := coderdtest.CreateTemplate(t, ownerClient, owner.OrganizationID, version.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	memberKey, err := memberClient.GitSSHKey(ctx, "me")
	require.NoError(t, err)

	params, diags, err := coderd.ChatRenderTemplateParameters(api, ctx, member.ID, version.ID)
	require.NoError(t, err)
	require.Empty(t, diags)
	require.Len(t, params, 1)
	require.Equal(t, "public_key", params[0].Name)
	require.Equal(t, memberKey.PublicKey, params[0].DefaultValue.Value)
	require.Equal(t, 0, api.FileCache.Count(), "renderer must release the template files")

	// A version whose import job has not completed reports not ready rather
	// than failing, so the tool can tell the model to retry.
	pendingJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
		OrganizationID: owner.OrganizationID,
		InitiatorID:    owner.UserID,
	})
	pending := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: owner.OrganizationID,
		CreatedBy:      owner.UserID,
		JobID:          pendingJob.ID,
	})
	_, _, err = coderd.ChatRenderTemplateParameters(api, ctx, member.ID, pending.ID)
	require.ErrorIs(t, err, dynamicparameters.ErrTemplateVersionNotReady)
}
