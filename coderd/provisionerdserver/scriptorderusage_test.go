package provisionerdserver_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestCompleteJobPersistsScriptOrderCounts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	srv, db, _, daemon := setup(t, false, nil)
	user := dbgen.User(t, db, database.User{})

	// GIVEN: A template version with an acquired import job.
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		CreatedBy:      user.ID,
		OrganizationID: daemon.OrganizationID,
		JobID:          uuid.New(),
	})
	input, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
		TemplateVersionID: version.ID,
	})
	require.NoError(t, err)
	job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             version.JobID,
		OrganizationID: daemon.OrganizationID,
		InitiatorID:    user.ID,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
		Input:          input,
		Tags:           daemon.Tags,
	})
	require.NoError(t, err)
	tags, err := json.Marshal(job.Tags)
	require.NoError(t, err)
	_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
		OrganizationID:  daemon.OrganizationID,
		WorkerID:        uuid.NullUUID{UUID: daemon.ID, Valid: true},
		Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
		StartedAt:       sql.NullTime{Time: dbtime.Now(), Valid: true},
		ProvisionerTags: tags,
	})
	require.NoError(t, err)

	// This test calls CompleteJob directly without running Terraform or the
	// provisioner. Use a captured START plan and reproduce the provisioner's
	// decode/re-encode step to verify literal rule blocks survive serialization
	// and their counts are persisted.
	planJSON, err := os.ReadFile("../../provisioner/terraform/testdata/script-order-plan.json")
	require.NoError(t, err)
	var plan tfjson.Plan
	require.NoError(t, json.Unmarshal(planJSON, &plan))
	planJSON, err = json.Marshal(&plan)
	require.NoError(t, err)

	// WHEN: The import completes with the serialized START plan.
	_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
		JobId: job.ID.String(),
		Type: &proto.CompletedJob_TemplateImport_{
			TemplateImport: &proto.CompletedJob_TemplateImport{
				Plan: planJSON,
			},
		},
	})
	require.NoError(t, err)

	// THEN: Counts are persisted but excluded from the telemetry summary.
	version, err = db.GetTemplateVersionByID(ctx, version.ID)
	require.NoError(t, err)
	require.Equal(t, int32(1), version.ScriptOrderDataSourceCount)
	require.Equal(t, int32(1), version.ScriptOrderRuleCount)

	usage, err := db.GetScriptOrderTelemetrySummary(ctx)
	require.NoError(t, err)
	require.Equal(t, database.GetScriptOrderTelemetrySummaryRow{}, usage)

	// WHEN: The version becomes a template's active version.
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  daemon.OrganizationID,
		CreatedBy:       user.ID,
		ActiveVersionID: version.ID,
	})
	err = db.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
		ID:         version.ID,
		TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true},
		UpdatedAt:  dbtime.Now(),
		Name:       version.Name,
		Message:    version.Message,
	})
	require.NoError(t, err)

	// THEN: Its counts are included in the telemetry summary.
	usage, err = db.GetScriptOrderTelemetrySummary(ctx)
	require.NoError(t, err)
	require.Equal(t, database.GetScriptOrderTelemetrySummaryRow{
		TemplatesWithScriptOrder: 1,
		DataSourceCount:          1,
		RuleCount:                1,
	}, usage)
}

// Script-order telemetry is best effort: missing or malformed plan
// configuration must not fail an otherwise successful template import.
func TestCompleteJobWithoutScriptOrderConfiguration(t *testing.T) {
	t.Parallel()

	srv, db, _, daemon := setup(t, false, nil)
	user := dbgen.User(t, db, database.User{})

	for _, tt := range []struct {
		name string
		plan string
	}{
		{name: "MissingPlan"},
		{name: "MissingConfiguration", plan: "{}"},
		{name: "InvalidConfiguration", plan: `{"configuration":[]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			// GIVEN: A template version with an acquired import job.
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedBy:      user.ID,
				OrganizationID: daemon.OrganizationID,
				JobID:          uuid.New(),
			})
			input, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
				TemplateVersionID: version.ID,
			})
			require.NoError(t, err)
			job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				ID:             version.JobID,
				OrganizationID: daemon.OrganizationID,
				InitiatorID:    user.ID,
				Type:           database.ProvisionerJobTypeTemplateVersionImport,
				Input:          input,
				Tags:           daemon.Tags,
				StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
				WorkerID:       uuid.NullUUID{UUID: daemon.ID, Valid: true},
			})

			// WHEN: The import completes without usable plan configuration.
			_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.ID.String(),
				Type: &proto.CompletedJob_TemplateImport_{
					TemplateImport: &proto.CompletedJob_TemplateImport{
						Plan: []byte(tt.plan),
					},
				},
			})
			require.NoError(t, err)

			// THEN: The import succeeds without script-order usage counts.
			job, err = db.GetProvisionerJobByID(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, job.CompletedAt.Valid)
			require.Empty(t, job.Error.String)

			version, err = db.GetTemplateVersionByID(ctx, version.ID)
			require.NoError(t, err)
			require.Zero(t, version.ScriptOrderDataSourceCount)
			require.Zero(t, version.ScriptOrderRuleCount)
		})
	}
}
