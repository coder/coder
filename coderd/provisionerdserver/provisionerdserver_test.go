package provisionerdserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
)

func TestAcquireJob(t *testing.T) {
	t.Parallel()
	t.Run("NoJobs", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		job, err := srv.AcquireJob(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, &proto.AcquiredJob{}, job)
	})
	t.Run("InitiatorNotFound", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		_, err := srv.Database.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			ID:          uuid.New(),
			InitiatorID: uuid.New(),
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = srv.AcquireJob(context.Background(), nil)
		require.ErrorContains(t, err, "sql: no rows in result set")
	})
	t.Run("WorkspaceBuildJob", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		ctx := context.Background()
		user, err := srv.Database.InsertUser(context.Background(), database.InsertUserParams{
			ID:       uuid.New(),
			Username: "testing",
		})
		require.NoError(t, err)
		template, err := srv.Database.InsertTemplate(ctx, database.InsertTemplateParams{
			ID:   uuid.New(),
			Name: "template",
		})
		require.NoError(t, err)
		version, err := srv.Database.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID: uuid.New(),
			TemplateID: uuid.NullUUID{
				UUID:  template.ID,
				Valid: true,
			},
			JobID: uuid.New(),
		})
		require.NoError(t, err)
		workspace, err := srv.Database.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:         uuid.New(),
			OwnerID:    user.ID,
			TemplateID: template.ID,
			Name:       "workspace",
		})
		require.NoError(t, err)
		build, err := srv.Database.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:                uuid.New(),
			WorkspaceID:       workspace.ID,
			BuildNumber:       1,
			JobID:             uuid.New(),
			TemplateVersionID: version.ID,
			Transition:        database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)

		data, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
			WorkspaceBuildID: build.ID,
		})
		require.NoError(t, err)

		file, err := srv.Database.InsertFile(ctx, database.InsertFileParams{
			ID:   uuid.New(),
			Hash: "something",
			Data: []byte{},
		})
		require.NoError(t, err)

		_, err = srv.Database.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			ID:             build.JobID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			OrganizationID: uuid.New(),
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          data,
		})
		require.NoError(t, err)

		published := make(chan struct{})
		closeSubscribe, err := srv.Pubsub.Subscribe(codersdk.WorkspaceNotifyChannel(workspace.ID), func(_ context.Context, _ []byte) {
			close(published)
		})
		require.NoError(t, err)
		defer closeSubscribe()

		job, err := srv.AcquireJob(ctx, nil)
		require.NoError(t, err)

		<-published

		got, err := json.Marshal(job.Type)
		require.NoError(t, err)

		want, err := json.Marshal(&proto.AcquiredJob_WorkspaceBuild_{
			WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
				WorkspaceBuildId: build.ID.String(),
				WorkspaceName:    workspace.Name,
				ParameterValues:  []*sdkproto.ParameterValue{},
				Metadata: &sdkproto.Provision_Metadata{
					CoderUrl:            srv.AccessURL.String(),
					WorkspaceTransition: sdkproto.WorkspaceTransition_START,
					WorkspaceName:       workspace.Name,
					WorkspaceOwner:      user.Username,
					WorkspaceOwnerEmail: user.Email,
					WorkspaceId:         workspace.ID.String(),
					WorkspaceOwnerId:    user.ID.String(),
				},
			},
		})
		require.NoError(t, err)

		require.JSONEq(t, string(want), string(got))
	})
	t.Run("TemplateVersionDryRun", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		ctx := context.Background()
		user, err := srv.Database.InsertUser(ctx, database.InsertUserParams{
			ID:       uuid.New(),
			Username: "testing",
		})
		require.NoError(t, err)
		version, err := srv.Database.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID: uuid.New(),
		})
		require.NoError(t, err)

		data, err := json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
			TemplateVersionID: version.ID,
			WorkspaceName:     "testing",
			ParameterValues:   []database.ParameterValue{},
		})
		require.NoError(t, err)

		file, err := srv.Database.InsertFile(ctx, database.InsertFileParams{
			ID:   uuid.New(),
			Hash: "something",
			Data: []byte{},
		})
		require.NoError(t, err)

		_, err = srv.Database.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			OrganizationID: uuid.New(),
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
			Input:          data,
		})
		require.NoError(t, err)

		job, err := srv.AcquireJob(ctx, nil)
		require.NoError(t, err)

		got, err := json.Marshal(job.Type)
		require.NoError(t, err)

		want, err := json.Marshal(&proto.AcquiredJob_TemplateDryRun_{
			TemplateDryRun: &proto.AcquiredJob_TemplateDryRun{
				ParameterValues: []*sdkproto.ParameterValue{},
				Metadata: &sdkproto.Provision_Metadata{
					CoderUrl:      srv.AccessURL.String(),
					WorkspaceName: "testing",
				},
			},
		})
		require.NoError(t, err)
		require.JSONEq(t, string(want), string(got))
	})
	t.Run("TemplateVersionImport", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		ctx := context.Background()
		user, err := srv.Database.InsertUser(ctx, database.InsertUserParams{
			ID:       uuid.New(),
			Username: "testing",
		})
		require.NoError(t, err)

		file, err := srv.Database.InsertFile(ctx, database.InsertFileParams{
			ID:   uuid.New(),
			Hash: "something",
			Data: []byte{},
		})
		require.NoError(t, err)

		_, err = srv.Database.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			OrganizationID: uuid.New(),
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          json.RawMessage{},
		})
		require.NoError(t, err)

		job, err := srv.AcquireJob(ctx, nil)
		require.NoError(t, err)

		got, err := json.Marshal(job.Type)
		require.NoError(t, err)

		want, err := json.Marshal(&proto.AcquiredJob_TemplateImport_{
			TemplateImport: &proto.AcquiredJob_TemplateImport{
				Metadata: &sdkproto.Provision_Metadata{
					CoderUrl: srv.AccessURL.String(),
				},
			},
		})
		require.NoError(t, err)
		require.JSONEq(t, string(want), string(got))
	})
}

func TestUpdateJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: "hello",
		})
		require.ErrorContains(t, err, "invalid UUID")

		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: uuid.NewString(),
		})
		require.ErrorContains(t, err, "no rows in result set")
	})
	t.Run("NotRunning", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		job, err := srv.Database.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID: uuid.New(),
		})
		require.NoError(t, err)
		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "job isn't running yet")
	})
	// This test prevents runners from updating jobs they don't own!
	t.Run("NotOwner", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		job, err := srv.Database.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:          uuid.New(),
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = srv.Database.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "you don't own this job")
	})

	setupJob := func(t *testing.T, srv *provisionerdserver.Server) uuid.UUID {
		job, err := srv.Database.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:          uuid.New(),
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = srv.Database.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  srv.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		return job.ID
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		job := setupJob(t, srv)
		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
		})
		require.NoError(t, err)
	})

	t.Run("Logs", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		job := setupJob(t, srv)

		published := make(chan struct{})

		closeListener, err := srv.Pubsub.Subscribe(provisionerdserver.ProvisionerJobLogsNotifyChannel(job), func(_ context.Context, _ []byte) {
			close(published)
		})
		require.NoError(t, err)
		defer closeListener()

		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
			Logs: []*proto.Log{{
				Source: proto.LogSource_PROVISIONER,
				Level:  sdkproto.LogLevel_INFO,
				Output: "hi",
			}},
		})
		require.NoError(t, err)

		<-published
	})
	t.Run("Readme", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		job := setupJob(t, srv)
		version, err := srv.Database.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:    uuid.New(),
			JobID: job,
		})
		require.NoError(t, err)
		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId:  job.String(),
			Readme: []byte("# hello world"),
		})
		require.NoError(t, err)

		version, err = srv.Database.GetTemplateVersionByID(ctx, version.ID)
		require.NoError(t, err)
		require.Equal(t, "# hello world", version.Readme)
	})
}

func TestFailJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		_, err := srv.FailJob(ctx, &proto.FailedJob{
			JobId: "hello",
		})
		require.ErrorContains(t, err, "invalid UUID")

		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: uuid.NewString(),
		})
		require.ErrorContains(t, err, "no rows in result set")
	})
	// This test prevents runners from updating jobs they don't own!
	t.Run("NotOwner", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		job, err := srv.Database.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:          uuid.New(),
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = srv.Database.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		_, err = srv.FailJob(ctx, &proto.FailedJob{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "you don't own this job")
	})
	t.Run("AlreadyCompleted", func(t *testing.T) {
		t.Parallel()
		srv := setup(t)
		job, err := srv.Database.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:          uuid.New(),
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		_, err = srv.Database.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  srv.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		err = srv.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
		})
		require.NoError(t, err)
		_, err = srv.FailJob(ctx, &proto.FailedJob{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "job already completed")
	})
}

func setup(t *testing.T) *provisionerdserver.Server {
	t.Helper()
	db, pubsub := dbtestutil.NewDB(t)

	return &provisionerdserver.Server{
		ID:           uuid.New(),
		Logger:       slogtest.Make(t, nil),
		AccessURL:    &url.URL{},
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
		Database:     db,
		Pubsub:       pubsub,
		Telemetry:    telemetry.NewNoop(),
	}
}
