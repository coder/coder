package wsbuilder_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestPriorityQueue(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		Database:                 db,
		Pubsub:                   ps,
	})
	owner := coderdtest.CreateFirstUser(t, client)

	// Create a template
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

	ctx := testutil.Context(t, testutil.WaitMedium)

	// Test priority setting by directly creating provisioner jobs
	// Create a human-initiated job
	humanJob, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		InitiatorID:    owner.UserID,
		OrganizationID: owner.OrganizationID,
		Provisioner:    database.ProvisionerTypeEcho,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		FileID:         uuid.New(),
		Input:          json.RawMessage(`{}`),
		Tags:           database.StringMap{},
		TraceMetadata:  pqtype.NullRawMessage{},

	})
	require.NoError(t, err)

	// Create a prebuild job
	prebuildJob, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             uuid.New(),
		CreatedAt:      time.Now().Add(time.Millisecond), // Slightly later
		UpdatedAt:      time.Now().Add(time.Millisecond),
		InitiatorID:    database.PrebuildsSystemUserID,
		OrganizationID: owner.OrganizationID,
		Provisioner:    database.ProvisionerTypeEcho,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		FileID:         uuid.New(),
		Input:          json.RawMessage(`{}`),
		Tags:           database.StringMap{},
		TraceMetadata:  pqtype.NullRawMessage{},

	})
	require.NoError(t, err)

	// Verify that jobs have correct initiator IDs
	require.Equal(t, owner.UserID, humanJob.InitiatorID, "Human-initiated job should have user as initiator")
	require.Equal(t, database.PrebuildsSystemUserID, prebuildJob.InitiatorID, "Prebuild job should have system user as initiator")

	// Test job acquisition order - human jobs should be acquired first
	// Even though the prebuild job was created later, the human job should be acquired first due to higher priority
	acquiredJob1, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
		OrganizationID:  owner.OrganizationID,
		StartedAt:       sql.NullTime{Time: time.Now(), Valid: true},
		WorkerID:        uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
		ProvisionerTags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.Equal(t, owner.UserID, acquiredJob1.InitiatorID, "First acquired job should be human-initiated due to higher priority")
	require.Equal(t, humanJob.ID, acquiredJob1.ID, "First acquired job should be the human job")

	acquiredJob2, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
		OrganizationID:  owner.OrganizationID,
		StartedAt:       sql.NullTime{Time: time.Now(), Valid: true},
		WorkerID:        uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
		ProvisionerTags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.Equal(t, database.PrebuildsSystemUserID, acquiredJob2.InitiatorID, "Second acquired job should be prebuild")
	require.Equal(t, prebuildJob.ID, acquiredJob2.ID, "Second acquired job should be the prebuild job")
}
