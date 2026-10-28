package provisionerdserver

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func TestFoundationUnattachedWorkerLifecycle(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, action := range []string{"plan", "apply", "destroy"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			f := newTemplateResourcesFixture(t, db, ps, action, true)
			job, err := f.server.acquireProtoJob(dbauthz.AsProvisionerd(t.Context()), f.job)
			require.NoError(t, err)
			payload := job.GetTemplateResources()
			require.Equal(t, f.foundation.ID.String(), payload.FoundationId)
			require.Equal(t, f.foundation.ID.String(), payload.Metadata.FoundationId)
			require.Equal(t, f.foundation.Name, payload.Metadata.FoundationName)
			require.Empty(t, payload.GetTemplateId())
			require.Empty(t, payload.Metadata.TemplateId)
			require.Empty(t, payload.Metadata.FoundationTemplateIds)
			result := templateResourcesJobResult{success: true, disposition: proto.StateDisposition_UNCHANGED}
			if action != "plan" {
				result.disposition = proto.StateDisposition_RETURNED
				result.state = templateResourceTestState(`{}`, `[]`)
			}
			require.NoError(t, f.server.finalizeTemplateResourcesJob(dbauthz.AsProvisionerd(t.Context()), f.job.ID, result))
		})
	}
}

func TestFoundationMetadataRequiresCapableWorkerWithoutOutputs(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newTemplateResourcesFixture(t, db, ps, "plan")
	f.server.apiVersion = "1.23"
	metadata := &sdkproto.Metadata{TemplateOutputs: "{}"}
	err := f.server.workspaceFoundationMetadata(dbauthz.AsProvisionerd(t.Context()), f.state.AppliedRevisionID, metadata)
	require.ErrorContains(t, err, "v1.25")
	require.Equal(t, f.foundation.ID.String(), metadata.FoundationId)
	require.NotEqual(t, f.foundation.TemplateID.UUID.String(), metadata.FoundationId)

	template := dbgen.Template(t, db, database.Template{OrganizationID: f.foundation.OrganizationID, CreatedBy: f.job.InitiatorID})
	binding := uuid.NullUUID{UUID: template.ID, Valid: true}
	unconfigured := dbgen.Foundation(t, db, database.Foundation{OrganizationID: f.foundation.OrganizationID, TemplateID: binding})
	metadata = &sdkproto.Metadata{}
	err = f.server.templateValidationMetadata(dbauthz.AsProvisionerd(t.Context()), binding, metadata)
	require.ErrorContains(t, err, "v1.25", "even an unconfigured Foundation needs metadata support")
	require.Equal(t, unconfigured.ID.String(), metadata.FoundationId)
	f.server.apiVersion = "1.25"
	require.NoError(t, f.server.templateValidationMetadata(dbauthz.AsProvisionerd(t.Context()), binding, metadata))
	require.Equal(t, unconfigured.ID.String(), metadata.FoundationId)
	require.Equal(t, unconfigured.Name, metadata.FoundationName)
	require.Equal(t, []string{template.ID.String()}, metadata.FoundationTemplateIds)
	require.Equal(t, template.ID.String(), metadata.TemplateId)
	require.Empty(t, metadata.TemplateResourceRevisionId)
	require.JSONEq(t, `{}`, metadata.TemplateOutputs)

	// Starts expose the attached Foundation even before its first apply.
	metadata = &sdkproto.Metadata{TemplateId: template.ID.String(), TemplateOutputs: "{}", WorkspaceTransition: sdkproto.WorkspaceTransition_START}
	require.NoError(t, f.server.workspaceFoundationMetadata(dbauthz.AsProvisionerd(t.Context()), uuid.NullUUID{}, metadata))
	require.Equal(t, unconfigured.ID.String(), metadata.FoundationId)
	require.Equal(t, unconfigured.Name, metadata.FoundationName)
	require.Equal(t, []string{template.ID.String()}, metadata.FoundationTemplateIds)
	f.server.apiVersion = "1.23"
	require.ErrorContains(t, f.server.workspaceFoundationMetadata(dbauthz.AsProvisionerd(t.Context()), uuid.NullUUID{}, metadata), "v1.25")
	f.server.apiVersion = "1.25"

	// Cleanup without a pinned revision must not inherit the current binding.
	for _, transition := range []sdkproto.WorkspaceTransition{sdkproto.WorkspaceTransition_STOP, sdkproto.WorkspaceTransition_DESTROY} {
		metadata = &sdkproto.Metadata{TemplateId: template.ID.String(), TemplateOutputs: "{}", WorkspaceTransition: transition}
		require.NoError(t, f.server.workspaceFoundationMetadata(dbauthz.AsProvisionerd(t.Context()), uuid.NullUUID{}, metadata))
		require.Empty(t, metadata.FoundationId)
		require.Empty(t, metadata.FoundationName)
		require.Empty(t, metadata.FoundationTemplateIds)
	}

	// Historical stop/delete metadata must not switch to a replacement binding.
	_, err = db.DeleteFoundation(t.Context(), database.DeleteFoundationParams{ID: f.foundation.ID, UpdatedAt: f.foundation.UpdatedAt})
	require.NoError(t, err)
	replacement := dbgen.Foundation(t, db, database.Foundation{OrganizationID: f.foundation.OrganizationID, TemplateID: f.foundation.TemplateID})
	metadata = &sdkproto.Metadata{TemplateOutputs: "{}"}
	require.NoError(t, f.server.workspaceFoundationMetadata(dbauthz.AsProvisionerd(t.Context()), f.state.AppliedRevisionID, metadata))
	require.Equal(t, f.foundation.ID.String(), metadata.FoundationId)
	require.NotEqual(t, replacement.ID.String(), metadata.FoundationId)
}
