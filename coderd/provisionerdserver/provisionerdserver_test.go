package provisionerdserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func mockAuditor() *atomic.Pointer[audit.Auditor] {
	ptr := &atomic.Pointer[audit.Auditor]{}
	mock := audit.Auditor(audit.NewMock())
	ptr.Store(&mock)
	return ptr
}

func testTemplateScheduleStore() *atomic.Pointer[schedule.TemplateScheduleStore] {
	ptr := &atomic.Pointer[schedule.TemplateScheduleStore]{}
	store := schedule.NewAGPLTemplateScheduleStore()
	ptr.Store(&store)
	return ptr
}

func testUserQuietHoursScheduleStore() *atomic.Pointer[schedule.UserQuietHoursScheduleStore] {
	ptr := &atomic.Pointer[schedule.UserQuietHoursScheduleStore]{}
	store := schedule.NewAGPLUserQuietHoursScheduleStore()
	ptr.Store(&store)
	return ptr
}

func TestAcquireJob_LongPoll(t *testing.T) {
	t.Parallel()
	//nolint:dogsled
	srv, _, _, _ := setup(t, false, &overrides{acquireJobLongPollDuration: time.Microsecond})
	job, err := srv.AcquireJob(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, &proto.AcquiredJob{}, job)
}

func TestAcquireJobWithCancel_Cancel(t *testing.T) {
	t.Parallel()
	//nolint:dogsled
	srv, _, _, _ := setup(t, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	fs := newFakeStream(ctx)
	errCh := make(chan error)
	go func() {
		errCh <- srv.AcquireJobWithCancel(fs)
	}()
	fs.cancel()
	select {
	case <-ctx.Done():
		t.Fatal("timed out waiting for AcquireJobWithCancel")
	case err := <-errCh:
		require.NoError(t, err)
	}
	job, err := fs.waitForJob()
	require.NoError(t, err)
	require.NotNil(t, job)
	require.Equal(t, "", job.JobId)
}

func TestHeartbeat(t *testing.T) {
	t.Parallel()

	numBeats := 3
	ctx := testutil.Context(t, testutil.WaitShort)
	heartbeatChan := make(chan struct{})
	heartbeatFn := func(hbCtx context.Context) error {
		t.Logf("heartbeat")
		select {
		case <-hbCtx.Done():
			return hbCtx.Err()
		default:
			heartbeatChan <- struct{}{}
			return nil
		}
	}
	//nolint:dogsled
	_, _, _, _ = setup(t, false, &overrides{
		ctx:               ctx,
		heartbeatFn:       heartbeatFn,
		heartbeatInterval: testutil.IntervalFast,
	})

	for i := 0; i < numBeats; i++ {
		testutil.RequireRecvCtx(ctx, t, heartbeatChan)
	}
	// goleak.VerifyTestMain ensures that the heartbeat goroutine does not leak
}

func TestAcquireJob(t *testing.T) {
	t.Parallel()

	// These test acquiring a single job without canceling, and tests both AcquireJob (deprecated) and
	// AcquireJobWithCancel as the way to get the job.
	cases := []struct {
		name    string
		acquire func(context.Context, proto.DRPCProvisionerDaemonServer) (*proto.AcquiredJob, error)
	}{
		{name: "Deprecated", acquire: func(ctx context.Context, srv proto.DRPCProvisionerDaemonServer) (*proto.AcquiredJob, error) {
			return srv.AcquireJob(ctx, nil)
		}},
		{name: "WithCancel", acquire: func(ctx context.Context, srv proto.DRPCProvisionerDaemonServer) (*proto.AcquiredJob, error) {
			fs := newFakeStream(ctx)
			err := srv.AcquireJobWithCancel(fs)
			if err != nil {
				return nil, err
			}
			return fs.waitForJob()
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"_InitiatorNotFound", func(t *testing.T) {
			t.Parallel()
			srv, db, _, _ := setup(t, false, nil)
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()
			_, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
				ID:            uuid.New(),
				InitiatorID:   uuid.New(),
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
			})
			require.NoError(t, err)
			_, err = tc.acquire(ctx, srv)
			require.ErrorContains(t, err, "sql: no rows in result set")
		})
		t.Run(tc.name+"_WorkspaceBuildJob", func(t *testing.T) {
			t.Parallel()
			// Set the max session token lifetime so we can assert we
			// create an API key with an expiration within the bounds of the
			// deployment config.
			dv := &codersdk.DeploymentValues{MaxTokenLifetime: clibase.Duration(time.Hour)}
			gitAuthProvider := "github"
			srv, db, ps, _ := setup(t, false, &overrides{
				deploymentValues: dv,
				externalAuthConfigs: []*externalauth.Config{{
					ID:                       gitAuthProvider,
					InstrumentedOAuth2Config: &testutil.OAuth2Config{},
				}},
			})
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			user := dbgen.User(t, db, database.User{})
			link := dbgen.UserLink(t, db, database.UserLink{
				LoginType:        database.LoginTypeOIDC,
				UserID:           user.ID,
				OAuthExpiry:      dbtime.Now().Add(time.Hour),
				OAuthAccessToken: "access-token",
			})
			dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
				ProviderID: gitAuthProvider,
				UserID:     user.ID,
			})
			template := dbgen.Template(t, db, database.Template{
				Name:        "template",
				Provisioner: database.ProvisionerTypeEcho,
			})
			file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			versionFile := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				TemplateID: uuid.NullUUID{
					UUID:  template.ID,
					Valid: true,
				},
				JobID: uuid.New(),
			})
			err := db.UpdateTemplateVersionExternalAuthProvidersByJobID(ctx, database.UpdateTemplateVersionExternalAuthProvidersByJobIDParams{
				JobID:                 version.JobID,
				ExternalAuthProviders: []string{gitAuthProvider},
				UpdatedAt:             dbtime.Now(),
			})
			require.NoError(t, err)
			// Import version job
			_ = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				ID:            version.JobID,
				InitiatorID:   user.ID,
				FileID:        versionFile.ID,
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				Type:          database.ProvisionerJobTypeTemplateVersionImport,
				Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
					TemplateVersionID: version.ID,
					UserVariableValues: []codersdk.VariableValue{
						{Name: "second", Value: "bah"},
					},
				})),
			})
			_ = dbgen.TemplateVersionVariable(t, db, database.TemplateVersionVariable{
				TemplateVersionID: version.ID,
				Name:              "first",
				Value:             "first_value",
				DefaultValue:      "default_value",
				Sensitive:         true,
			})
			_ = dbgen.TemplateVersionVariable(t, db, database.TemplateVersionVariable{
				TemplateVersionID: version.ID,
				Name:              "second",
				Value:             "second_value",
				DefaultValue:      "default_value",
				Required:          true,
				Sensitive:         false,
			})
			workspace := dbgen.Workspace(t, db, database.Workspace{
				TemplateID: template.ID,
				OwnerID:    user.ID,
			})
			build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				WorkspaceID:       workspace.ID,
				BuildNumber:       1,
				JobID:             uuid.New(),
				TemplateVersionID: version.ID,
				Transition:        database.WorkspaceTransitionStart,
				Reason:            database.BuildReasonInitiator,
			})
			_ = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				ID:            build.ID,
				InitiatorID:   user.ID,
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				FileID:        file.ID,
				Type:          database.ProvisionerJobTypeWorkspaceBuild,
				Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
					WorkspaceBuildID: build.ID,
				})),
			})

			startPublished := make(chan struct{})
			var closed bool
			closeStartSubscribe, err := ps.Subscribe(codersdk.WorkspaceNotifyChannel(workspace.ID), func(_ context.Context, _ []byte) {
				if !closed {
					close(startPublished)
					closed = true
				}
			})
			require.NoError(t, err)
			defer closeStartSubscribe()

			var job *proto.AcquiredJob

			for {
				// Grab jobs until we find the workspace build job. There is also
				// an import version job that we need to ignore.
				job, err = tc.acquire(ctx, srv)
				require.NoError(t, err)
				if _, ok := job.Type.(*proto.AcquiredJob_WorkspaceBuild_); ok {
					break
				}
			}

			<-startPublished

			got, err := json.Marshal(job.Type)
			require.NoError(t, err)

			// Validate that a session token is generated during the job.
			sessionToken := job.Type.(*proto.AcquiredJob_WorkspaceBuild_).WorkspaceBuild.Metadata.WorkspaceOwnerSessionToken
			require.NotEmpty(t, sessionToken)
			toks := strings.Split(sessionToken, "-")
			require.Len(t, toks, 2, "invalid api key")
			key, err := db.GetAPIKeyByID(ctx, toks[0])
			require.NoError(t, err)
			require.Equal(t, int64(dv.MaxTokenLifetime.Value().Seconds()), key.LifetimeSeconds)
			require.WithinDuration(t, time.Now().Add(dv.MaxTokenLifetime.Value()), key.ExpiresAt, time.Minute)

			want, err := json.Marshal(&proto.AcquiredJob_WorkspaceBuild_{
				WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
					WorkspaceBuildId: build.ID.String(),
					WorkspaceName:    workspace.Name,
					VariableValues: []*sdkproto.VariableValue{
						{
							Name:      "first",
							Value:     "first_value",
							Sensitive: true,
						},
						{
							Name:  "second",
							Value: "second_value",
						},
					},
					ExternalAuthProviders: []*sdkproto.ExternalAuthProvider{{
						Id:          gitAuthProvider,
						AccessToken: "access_token",
					}},
					Metadata: &sdkproto.Metadata{
						CoderUrl:                      (&url.URL{}).String(),
						WorkspaceTransition:           sdkproto.WorkspaceTransition_START,
						WorkspaceName:                 workspace.Name,
						WorkspaceOwner:                user.Username,
						WorkspaceOwnerEmail:           user.Email,
						WorkspaceOwnerName:            user.Name,
						WorkspaceOwnerOidcAccessToken: link.OAuthAccessToken,
						WorkspaceId:                   workspace.ID.String(),
						WorkspaceOwnerId:              user.ID.String(),
						TemplateId:                    template.ID.String(),
						TemplateName:                  template.Name,
						TemplateVersion:               version.Name,
						WorkspaceOwnerSessionToken:    sessionToken,
					},
				},
			})
			require.NoError(t, err)

			require.JSONEq(t, string(want), string(got))

			// Assert that we delete the session token whenever
			// a stop is issued.
			stopbuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				WorkspaceID:       workspace.ID,
				BuildNumber:       2,
				JobID:             uuid.New(),
				TemplateVersionID: version.ID,
				Transition:        database.WorkspaceTransitionStop,
				Reason:            database.BuildReasonInitiator,
			})
			_ = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				ID:            stopbuild.ID,
				InitiatorID:   user.ID,
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				FileID:        file.ID,
				Type:          database.ProvisionerJobTypeWorkspaceBuild,
				Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
					WorkspaceBuildID: stopbuild.ID,
				})),
			})

			stopPublished := make(chan struct{})
			closeStopSubscribe, err := ps.Subscribe(codersdk.WorkspaceNotifyChannel(workspace.ID), func(_ context.Context, _ []byte) {
				close(stopPublished)
			})
			require.NoError(t, err)
			defer closeStopSubscribe()

			// Grab jobs until we find the workspace build job. There is also
			// an import version job that we need to ignore.
			job, err = tc.acquire(ctx, srv)
			require.NoError(t, err)
			_, ok := job.Type.(*proto.AcquiredJob_WorkspaceBuild_)
			require.True(t, ok, "acquired job not a workspace build?")

			<-stopPublished

			// Validate that a session token is deleted during a stop job.
			sessionToken = job.Type.(*proto.AcquiredJob_WorkspaceBuild_).WorkspaceBuild.Metadata.WorkspaceOwnerSessionToken
			require.Empty(t, sessionToken)
			_, err = db.GetAPIKeyByID(ctx, key.ID)
			require.ErrorIs(t, err, sql.ErrNoRows)
		})

		t.Run(tc.name+"_TemplateVersionDryRun", func(t *testing.T) {
			t.Parallel()
			srv, db, ps, _ := setup(t, false, nil)
			ctx := context.Background()

			user := dbgen.User(t, db, database.User{})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{})
			file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			_ = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				InitiatorID:   user.ID,
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				FileID:        file.ID,
				Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
				Input: must(json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
					TemplateVersionID: version.ID,
					WorkspaceName:     "testing",
				})),
			})

			job, err := tc.acquire(ctx, srv)
			require.NoError(t, err)

			got, err := json.Marshal(job.Type)
			require.NoError(t, err)

			want, err := json.Marshal(&proto.AcquiredJob_TemplateDryRun_{
				TemplateDryRun: &proto.AcquiredJob_TemplateDryRun{
					Metadata: &sdkproto.Metadata{
						CoderUrl:      (&url.URL{}).String(),
						WorkspaceName: "testing",
					},
				},
			})
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
		t.Run(tc.name+"_TemplateVersionImport", func(t *testing.T) {
			t.Parallel()
			srv, db, ps, _ := setup(t, false, nil)
			ctx := context.Background()

			user := dbgen.User(t, db, database.User{})
			file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			_ = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				FileID:        file.ID,
				InitiatorID:   user.ID,
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				Type:          database.ProvisionerJobTypeTemplateVersionImport,
			})

			job, err := tc.acquire(ctx, srv)
			require.NoError(t, err)

			got, err := json.Marshal(job.Type)
			require.NoError(t, err)

			want, err := json.Marshal(&proto.AcquiredJob_TemplateImport_{
				TemplateImport: &proto.AcquiredJob_TemplateImport{
					Metadata: &sdkproto.Metadata{
						CoderUrl: (&url.URL{}).String(),
					},
				},
			})
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
		t.Run(tc.name+"_TemplateVersionImportWithUserVariable", func(t *testing.T) {
			t.Parallel()
			srv, db, ps, _ := setup(t, false, nil)

			user := dbgen.User(t, db, database.User{})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{})
			file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			_ = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				FileID:        file.ID,
				InitiatorID:   user.ID,
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				Type:          database.ProvisionerJobTypeTemplateVersionImport,
				Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
					TemplateVersionID: version.ID,
					UserVariableValues: []codersdk.VariableValue{
						{Name: "first", Value: "first_value"},
					},
				})),
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			job, err := tc.acquire(ctx, srv)
			require.NoError(t, err)

			got, err := json.Marshal(job.Type)
			require.NoError(t, err)

			want, err := json.Marshal(&proto.AcquiredJob_TemplateImport_{
				TemplateImport: &proto.AcquiredJob_TemplateImport{
					UserVariableValues: []*sdkproto.VariableValue{
						{Name: "first", Sensitive: true, Value: "first_value"},
					},
					Metadata: &sdkproto.Metadata{
						CoderUrl: (&url.URL{}).String(),
					},
				},
			})
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}

func TestUpdateJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		srv, _, _, _ := setup(t, false, nil)
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
		srv, db, _, _ := setup(t, false, nil)
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
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
		srv, db, _, _ := setup(t, false, nil)
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
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

	setupJob := func(t *testing.T, db database.Store, srvID uuid.UUID) uuid.UUID {
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			Type:          database.ProvisionerJobTypeTemplateVersionImport,
			StorageMethod: database.ProvisionerStorageMethodFile,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  srvID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		return job.ID
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		job := setupJob(t, db, pd.ID)
		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
		})
		require.NoError(t, err)
	})

	t.Run("Logs", func(t *testing.T) {
		t.Parallel()
		srv, db, ps, pd := setup(t, false, &overrides{})
		job := setupJob(t, db, pd.ID)

		published := make(chan struct{})

		closeListener, err := ps.Subscribe(provisionersdk.ProvisionerJobLogsNotifyChannel(job), func(_ context.Context, _ []byte) {
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
		srv, db, _, pd := setup(t, false, &overrides{})
		job := setupJob(t, db, pd.ID)
		versionID := uuid.New()
		err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:    versionID,
			JobID: job,
		})
		require.NoError(t, err)
		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId:  job.String(),
			Readme: []byte("# hello world"),
		})
		require.NoError(t, err)

		version, err := db.GetTemplateVersionByID(ctx, versionID)
		require.NoError(t, err)
		require.Equal(t, "# hello world", version.Readme)
	})

	t.Run("TemplateVariables", func(t *testing.T) {
		t.Parallel()

		t.Run("Valid", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			srv, db, _, pd := setup(t, false, &overrides{})
			job := setupJob(t, db, pd.ID)
			versionID := uuid.New()
			err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
				ID:    versionID,
				JobID: job,
			})
			require.NoError(t, err)
			firstTemplateVariable := &sdkproto.TemplateVariable{
				Name:         "first",
				Type:         "string",
				DefaultValue: "default_value",
				Sensitive:    true,
			}
			secondTemplateVariable := &sdkproto.TemplateVariable{
				Name:      "second",
				Type:      "string",
				Required:  true,
				Sensitive: true,
			}
			response, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
				JobId: job.String(),
				TemplateVariables: []*sdkproto.TemplateVariable{
					firstTemplateVariable,
					secondTemplateVariable,
				},
				UserVariableValues: []*sdkproto.VariableValue{
					{
						Name:  "second",
						Value: "foobar",
					},
				},
			})
			require.NoError(t, err)
			require.Len(t, response.VariableValues, 2)

			templateVariables, err := db.GetTemplateVersionVariables(ctx, versionID)
			require.NoError(t, err)
			require.Len(t, templateVariables, 2)
			require.Equal(t, templateVariables[0].Value, firstTemplateVariable.DefaultValue)
			require.Equal(t, templateVariables[1].Value, "foobar")
		})

		t.Run("Missing required value", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			srv, db, _, pd := setup(t, false, &overrides{})
			job := setupJob(t, db, pd.ID)
			versionID := uuid.New()
			err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
				ID:    versionID,
				JobID: job,
			})
			require.NoError(t, err)
			firstTemplateVariable := &sdkproto.TemplateVariable{
				Name:         "first",
				Type:         "string",
				DefaultValue: "default_value",
				Sensitive:    true,
			}
			secondTemplateVariable := &sdkproto.TemplateVariable{
				Name:      "second",
				Type:      "string",
				Required:  true,
				Sensitive: true,
			}
			response, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
				JobId: job.String(),
				TemplateVariables: []*sdkproto.TemplateVariable{
					firstTemplateVariable,
					secondTemplateVariable,
				},
			})
			require.Error(t, err) // required template variables need values
			require.Nil(t, response)

			// Even though there is an error returned, variables are stored in the database
			// to show the schema in the site UI.
			templateVariables, err := db.GetTemplateVersionVariables(ctx, versionID)
			require.NoError(t, err)
			require.Len(t, templateVariables, 2)
			require.Equal(t, templateVariables[0].Value, firstTemplateVariable.DefaultValue)
			require.Equal(t, templateVariables[1].Value, "")
		})
	})
}

func TestFailJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		srv, _, _, _ := setup(t, false, nil)
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
		srv, db, _, _ := setup(t, false, nil)
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionImport,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
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
		srv, db, _, pd := setup(t, false, &overrides{})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			Type:          database.ProvisionerJobTypeTemplateVersionImport,
			StorageMethod: database.ProvisionerStorageMethodFile,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
		})
		require.NoError(t, err)
		_, err = srv.FailJob(ctx, &proto.FailedJob{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "job already completed")
	})
	t.Run("WorkspaceBuild", func(t *testing.T) {
		t.Parallel()
		// Ignore log errors because we get:
		//
		//	(*Server).FailJob       audit log - get build {"error": "sql: no rows in result set"}
		ignoreLogErrors := true
		srv, db, ps, pd := setup(t, ignoreLogErrors, &overrides{})
		workspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:               uuid.New(),
			AutomaticUpdates: database.AutomaticUpdatesNever,
		})
		require.NoError(t, err)
		buildID := uuid.New()
		err = db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:          buildID,
			WorkspaceID: workspace.ID,
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
		})
		require.NoError(t, err)
		input, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
			WorkspaceBuildID: buildID,
		})
		require.NoError(t, err)

		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Input:         input,
			Provisioner:   database.ProvisionerTypeEcho,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod: database.ProvisionerStorageMethodFile,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)

		publishedWorkspace := make(chan struct{})
		closeWorkspaceSubscribe, err := ps.Subscribe(codersdk.WorkspaceNotifyChannel(workspace.ID), func(_ context.Context, _ []byte) {
			close(publishedWorkspace)
		})
		require.NoError(t, err)
		defer closeWorkspaceSubscribe()
		publishedLogs := make(chan struct{})
		closeLogsSubscribe, err := ps.Subscribe(provisionersdk.ProvisionerJobLogsNotifyChannel(job.ID), func(_ context.Context, _ []byte) {
			close(publishedLogs)
		})
		require.NoError(t, err)
		defer closeLogsSubscribe()

		_, err = srv.FailJob(ctx, &proto.FailedJob{
			JobId: job.ID.String(),
			Type: &proto.FailedJob_WorkspaceBuild_{
				WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{
					State: []byte("some state"),
				},
			},
		})
		require.NoError(t, err)
		<-publishedWorkspace
		<-publishedLogs
		build, err := db.GetWorkspaceBuildByID(ctx, buildID)
		require.NoError(t, err)
		require.Equal(t, "some state", string(build.ProvisionerState))
	})
}

func TestCompleteJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		srv, _, _, _ := setup(t, false, nil)
		_, err := srv.CompleteJob(ctx, &proto.CompletedJob{
			JobId: "hello",
		})
		require.ErrorContains(t, err, "invalid UUID")

		_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
			JobId: uuid.NewString(),
		})
		require.ErrorContains(t, err, "no rows in result set")
	})
	// This test prevents runners from updating jobs they don't own!
	t.Run("NotOwner", func(t *testing.T) {
		t.Parallel()
		srv, db, _, _ := setup(t, false, nil)
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "you don't own this job")
	})

	t.Run("TemplateImport_MissingGitAuth", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		jobID := uuid.New()
		versionID := uuid.New()
		err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:    versionID,
			JobID: jobID,
		})
		require.NoError(t, err)
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            jobID,
			Provisioner:   database.ProvisionerTypeEcho,
			Input:         []byte(`{"template_version_id": "` + versionID.String() + `"}`),
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		completeJob := func() {
			_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.ID.String(),
				Type: &proto.CompletedJob_TemplateImport_{
					TemplateImport: &proto.CompletedJob_TemplateImport{
						StartResources: []*sdkproto.Resource{{
							Name: "hello",
							Type: "aws_instance",
						}},
						StopResources:         []*sdkproto.Resource{},
						ExternalAuthProviders: []string{"github"},
					},
				},
			})
			require.NoError(t, err)
		}
		completeJob()
		job, err = db.GetProvisionerJobByID(ctx, job.ID)
		require.NoError(t, err)
		require.Contains(t, job.Error.String, `external auth provider "github" is not configured`)
	})

	t.Run("TemplateImport_WithGitAuth", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{
			externalAuthConfigs: []*externalauth.Config{{
				ID: "github",
			}},
		})
		jobID := uuid.New()
		versionID := uuid.New()
		err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:    versionID,
			JobID: jobID,
		})
		require.NoError(t, err)
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            jobID,
			Provisioner:   database.ProvisionerTypeEcho,
			Input:         []byte(`{"template_version_id": "` + versionID.String() + `"}`),
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		completeJob := func() {
			_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.ID.String(),
				Type: &proto.CompletedJob_TemplateImport_{
					TemplateImport: &proto.CompletedJob_TemplateImport{
						StartResources: []*sdkproto.Resource{{
							Name: "hello",
							Type: "aws_instance",
						}},
						StopResources:         []*sdkproto.Resource{},
						ExternalAuthProviders: []string{"github"},
					},
				},
			})
			require.NoError(t, err)
		}
		completeJob()
		job, err = db.GetProvisionerJobByID(ctx, job.ID)
		require.NoError(t, err)
		require.False(t, job.Error.Valid)
	})

	// TODO(@dean): remove this legacy test for MaxTTL
	t.Run("WorkspaceBuildLegacy", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name                  string
			templateAllowAutostop bool
			templateDefaultTTL    time.Duration
			templateMaxTTL        time.Duration
			workspaceTTL          time.Duration
			transition            database.WorkspaceTransition
			// The TTL is actually a deadline time on the workspace_build row,
			// so during the test this will be compared to be within 15 seconds
			// of the expected value.
			expectedTTL    time.Duration
			expectedMaxTTL time.Duration
		}{
			{
				name:                  "OK",
				templateAllowAutostop: true,
				templateDefaultTTL:    0,
				templateMaxTTL:        0,
				workspaceTTL:          0,
				transition:            database.WorkspaceTransitionStart,
				expectedTTL:           0,
				expectedMaxTTL:        0,
			},
			{
				name:                  "Delete",
				templateAllowAutostop: true,
				templateDefaultTTL:    0,
				templateMaxTTL:        0,
				workspaceTTL:          0,
				transition:            database.WorkspaceTransitionDelete,
				expectedTTL:           0,
				expectedMaxTTL:        0,
			},
			{
				name:                  "WorkspaceTTL",
				templateAllowAutostop: true,
				templateDefaultTTL:    0,
				templateMaxTTL:        0,
				workspaceTTL:          time.Hour,
				transition:            database.WorkspaceTransitionStart,
				expectedTTL:           time.Hour,
				expectedMaxTTL:        0,
			},
			{
				name:                  "TemplateDefaultTTLIgnored",
				templateAllowAutostop: true,
				templateDefaultTTL:    time.Hour,
				templateMaxTTL:        0,
				workspaceTTL:          0,
				transition:            database.WorkspaceTransitionStart,
				expectedTTL:           0,
				expectedMaxTTL:        0,
			},
			{
				name:                  "WorkspaceTTLOverridesTemplateDefaultTTL",
				templateAllowAutostop: true,
				templateDefaultTTL:    2 * time.Hour,
				templateMaxTTL:        0,
				workspaceTTL:          time.Hour,
				transition:            database.WorkspaceTransitionStart,
				expectedTTL:           time.Hour,
				expectedMaxTTL:        0,
			},
			{
				name:                  "TemplateMaxTTL",
				templateAllowAutostop: true,
				templateDefaultTTL:    0,
				templateMaxTTL:        time.Hour,
				workspaceTTL:          0,
				transition:            database.WorkspaceTransitionStart,
				expectedTTL:           time.Hour,
				expectedMaxTTL:        time.Hour,
			},
			{
				name:                  "TemplateMaxTTLOverridesWorkspaceTTL",
				templateAllowAutostop: true,
				templateDefaultTTL:    0,
				templateMaxTTL:        2 * time.Hour,
				workspaceTTL:          3 * time.Hour,
				transition:            database.WorkspaceTransitionStart,
				expectedTTL:           2 * time.Hour,
				expectedMaxTTL:        2 * time.Hour,
			},
			{
				name:                  "TemplateMaxTTLOverridesTemplateDefaultTTL",
				templateAllowAutostop: true,
				templateDefaultTTL:    3 * time.Hour,
				templateMaxTTL:        2 * time.Hour,
				workspaceTTL:          0,
				transition:            database.WorkspaceTransitionStart,
				expectedTTL:           2 * time.Hour,
				expectedMaxTTL:        2 * time.Hour,
			},
			{
				name:                  "TemplateBlockWorkspaceTTL",
				templateAllowAutostop: false,
				templateDefaultTTL:    3 * time.Hour,
				templateMaxTTL:        6 * time.Hour,
				workspaceTTL:          4 * time.Hour,
				transition:            database.WorkspaceTransitionStart,
				expectedTTL:           3 * time.Hour,
				expectedMaxTTL:        6 * time.Hour,
			},
		}

		for _, c := range cases {
			c := c

			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				tss := &atomic.Pointer[schedule.TemplateScheduleStore]{}
				srv, db, ps, pd := setup(t, false, &overrides{templateScheduleStore: tss})

				var store schedule.TemplateScheduleStore = schedule.MockTemplateScheduleStore{
					GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
						return schedule.TemplateScheduleOptions{
							UserAutostartEnabled: false,
							UserAutostopEnabled:  c.templateAllowAutostop,
							DefaultTTL:           c.templateDefaultTTL,
							MaxTTL:               c.templateMaxTTL,
							UseMaxTTL:            true,
						}, nil
					},
				}
				tss.Store(&store)

				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				template := dbgen.Template(t, db, database.Template{
					Name:           "template",
					Provisioner:    database.ProvisionerTypeEcho,
					OrganizationID: org.ID,
				})
				version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
					TemplateID: uuid.NullUUID{
						UUID:  template.ID,
						Valid: true,
					},
					JobID: uuid.New(),
				})
				err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
					ID:                 template.ID,
					UpdatedAt:          dbtime.Now(),
					AllowUserAutostart: c.templateAllowAutostop,
					DefaultTTL:         int64(c.templateDefaultTTL),
					MaxTTL:             int64(c.templateMaxTTL),
				})
				require.NoError(t, err)
				file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
				workspaceTTL := sql.NullInt64{}
				if c.workspaceTTL != 0 {
					workspaceTTL = sql.NullInt64{
						Int64: int64(c.workspaceTTL),
						Valid: true,
					}
				}
				workspace := dbgen.Workspace(t, db, database.Workspace{
					TemplateID: template.ID,
					Ttl:        workspaceTTL,
				})
				build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					WorkspaceID:       workspace.ID,
					TemplateVersionID: version.ID,
					Transition:        c.transition,
					Reason:            database.BuildReasonInitiator,
				})
				job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
					FileID: file.ID,
					Type:   database.ProvisionerJobTypeWorkspaceBuild,
					Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID: build.ID,
					})),
				})
				_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					WorkerID: uuid.NullUUID{
						UUID:  pd.ID,
						Valid: true,
					},
					Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
				})
				require.NoError(t, err)

				publishedWorkspace := make(chan struct{})
				closeWorkspaceSubscribe, err := ps.Subscribe(codersdk.WorkspaceNotifyChannel(build.WorkspaceID), func(_ context.Context, _ []byte) {
					close(publishedWorkspace)
				})
				require.NoError(t, err)
				defer closeWorkspaceSubscribe()
				publishedLogs := make(chan struct{})
				closeLogsSubscribe, err := ps.Subscribe(provisionersdk.ProvisionerJobLogsNotifyChannel(job.ID), func(_ context.Context, _ []byte) {
					close(publishedLogs)
				})
				require.NoError(t, err)
				defer closeLogsSubscribe()

				_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
					JobId: job.ID.String(),
					Type: &proto.CompletedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
							State: []byte{},
							Resources: []*sdkproto.Resource{{
								Name: "example",
								Type: "aws_instance",
							}},
						},
					},
				})
				require.NoError(t, err)

				<-publishedWorkspace
				<-publishedLogs

				workspace, err = db.GetWorkspaceByID(ctx, workspace.ID)
				require.NoError(t, err)
				require.Equal(t, c.transition == database.WorkspaceTransitionDelete, workspace.Deleted)

				workspaceBuild, err := db.GetWorkspaceBuildByID(ctx, build.ID)
				require.NoError(t, err)

				if c.expectedTTL == 0 {
					require.True(t, workspaceBuild.Deadline.IsZero())
				} else {
					require.WithinDuration(t, time.Now().Add(c.expectedTTL), workspaceBuild.Deadline, 15*time.Second, "deadline does not match expected")
				}
				if c.expectedMaxTTL == 0 {
					require.True(t, workspaceBuild.MaxDeadline.IsZero())
				} else {
					require.WithinDuration(t, time.Now().Add(c.expectedMaxTTL), workspaceBuild.MaxDeadline, 15*time.Second, "max deadline does not match expected")
					require.GreaterOrEqual(t, workspaceBuild.MaxDeadline.Unix(), workspaceBuild.Deadline.Unix(), "max deadline is smaller than deadline")
				}
			})
		}
	})

	t.Run("WorkspaceBuild", func(t *testing.T) {
		t.Parallel()

		now := time.Now()

		// NOTE: if you're looking for more in-depth deadline/max_deadline
		// calculation testing, see the schedule package. The provsiionerdserver
		// package calls `schedule.CalculateAutostop()` to generate the deadline
		// and max_deadline.

		// Wednesday the 8th of February 2023 at midnight. This date was
		// specifically chosen as it doesn't fall on a applicable week for both
		// fortnightly and triweekly autostop requirements.
		wednesdayMidnightUTC := time.Date(2023, 2, 8, 0, 0, 0, 0, time.UTC)

		sydneyQuietHours := "CRON_TZ=Australia/Sydney 0 0 * * *"
		sydneyLoc, err := time.LoadLocation("Australia/Sydney")
		require.NoError(t, err)
		// 12am on Saturday the 11th of February 2023 in Sydney.
		saturdayMidnightSydney := time.Date(2023, 2, 11, 0, 0, 0, 0, sydneyLoc)

		t.Log("now", now)
		t.Log("wednesdayMidnightUTC", wednesdayMidnightUTC)
		t.Log("saturdayMidnightSydney", saturdayMidnightSydney)

		cases := []struct {
			name         string
			now          time.Time
			workspaceTTL time.Duration
			transition   database.WorkspaceTransition

			// These fields are only used when testing max deadline.
			userQuietHoursSchedule      string
			templateAutostopRequirement schedule.TemplateAutostopRequirement

			expectedDeadline    time.Time
			expectedMaxDeadline time.Time
		}{
			{
				name:                        "OK",
				now:                         now,
				templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
				workspaceTTL:                0,
				transition:                  database.WorkspaceTransitionStart,
				expectedDeadline:            time.Time{},
				expectedMaxDeadline:         time.Time{},
			},
			{
				name:                        "Delete",
				now:                         now,
				templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
				workspaceTTL:                0,
				transition:                  database.WorkspaceTransitionDelete,
				expectedDeadline:            time.Time{},
				expectedMaxDeadline:         time.Time{},
			},
			{
				name:                        "WorkspaceTTL",
				now:                         now,
				templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
				workspaceTTL:                time.Hour,
				transition:                  database.WorkspaceTransitionStart,
				expectedDeadline:            now.Add(time.Hour),
				expectedMaxDeadline:         time.Time{},
			},
			{
				name:                   "TemplateAutostopRequirement",
				now:                    wednesdayMidnightUTC,
				userQuietHoursSchedule: sydneyQuietHours,
				templateAutostopRequirement: schedule.TemplateAutostopRequirement{
					DaysOfWeek: 0b00100000, // Saturday
					Weeks:      0,          // weekly
				},
				workspaceTTL: 0,
				transition:   database.WorkspaceTransitionStart,
				// expectedDeadline is copied from expectedMaxDeadline.
				expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
			},
		}

		for _, c := range cases {
			c := c

			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				// Simulate the given time starting from now.
				require.False(t, c.now.IsZero())
				start := time.Now()
				tss := &atomic.Pointer[schedule.TemplateScheduleStore]{}
				uqhss := &atomic.Pointer[schedule.UserQuietHoursScheduleStore]{}
				srv, db, ps, pd := setup(t, false, &overrides{
					timeNowFn: func() time.Time {
						return c.now.Add(time.Since(start))
					},
					templateScheduleStore:       tss,
					userQuietHoursScheduleStore: uqhss,
				})

				var templateScheduleStore schedule.TemplateScheduleStore = schedule.MockTemplateScheduleStore{
					GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
						return schedule.TemplateScheduleOptions{
							UserAutostartEnabled: false,
							UserAutostopEnabled:  true,
							DefaultTTL:           0,
							UseMaxTTL:            false,
							AutostopRequirement:  c.templateAutostopRequirement,
						}, nil
					},
				}
				tss.Store(&templateScheduleStore)

				var userQuietHoursScheduleStore schedule.UserQuietHoursScheduleStore = schedule.MockUserQuietHoursScheduleStore{
					GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.UserQuietHoursScheduleOptions, error) {
						if c.userQuietHoursSchedule == "" {
							return schedule.UserQuietHoursScheduleOptions{
								Schedule: nil,
							}, nil
						}

						sched, err := cron.Daily(c.userQuietHoursSchedule)
						if !assert.NoError(t, err) {
							return schedule.UserQuietHoursScheduleOptions{}, err
						}

						return schedule.UserQuietHoursScheduleOptions{
							Schedule: sched,
							UserSet:  false,
						}, nil
					},
				}
				uqhss.Store(&userQuietHoursScheduleStore)

				user := dbgen.User(t, db, database.User{
					QuietHoursSchedule: c.userQuietHoursSchedule,
				})
				template := dbgen.Template(t, db, database.Template{
					Name:        "template",
					Provisioner: database.ProvisionerTypeEcho,
				})
				err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
					ID:                            template.ID,
					UpdatedAt:                     dbtime.Now(),
					AllowUserAutostart:            false,
					AllowUserAutostop:             true,
					DefaultTTL:                    0,
					AutostopRequirementDaysOfWeek: int16(c.templateAutostopRequirement.DaysOfWeek),
					AutostopRequirementWeeks:      c.templateAutostopRequirement.Weeks,
				})
				require.NoError(t, err)
				template, err = db.GetTemplateByID(ctx, template.ID)
				require.NoError(t, err)
				file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
				workspaceTTL := sql.NullInt64{}
				if c.workspaceTTL != 0 {
					workspaceTTL = sql.NullInt64{
						Int64: int64(c.workspaceTTL),
						Valid: true,
					}
				}
				workspace := dbgen.Workspace(t, db, database.Workspace{
					TemplateID: template.ID,
					Ttl:        workspaceTTL,
					OwnerID:    user.ID,
				})
				version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
					TemplateID: uuid.NullUUID{
						UUID:  template.ID,
						Valid: true,
					},
					JobID: uuid.New(),
				})
				build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					WorkspaceID:       workspace.ID,
					TemplateVersionID: version.ID,
					Transition:        c.transition,
					Reason:            database.BuildReasonInitiator,
				})
				job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
					FileID: file.ID,
					Type:   database.ProvisionerJobTypeWorkspaceBuild,
					Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID: build.ID,
					})),
				})
				_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					WorkerID: uuid.NullUUID{
						UUID:  pd.ID,
						Valid: true,
					},
					Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
				})
				require.NoError(t, err)

				publishedWorkspace := make(chan struct{})
				closeWorkspaceSubscribe, err := ps.Subscribe(codersdk.WorkspaceNotifyChannel(build.WorkspaceID), func(_ context.Context, _ []byte) {
					close(publishedWorkspace)
				})
				require.NoError(t, err)
				defer closeWorkspaceSubscribe()
				publishedLogs := make(chan struct{})
				closeLogsSubscribe, err := ps.Subscribe(provisionersdk.ProvisionerJobLogsNotifyChannel(job.ID), func(_ context.Context, _ []byte) {
					close(publishedLogs)
				})
				require.NoError(t, err)
				defer closeLogsSubscribe()

				_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
					JobId: job.ID.String(),
					Type: &proto.CompletedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
							State: []byte{},
							Resources: []*sdkproto.Resource{{
								Name: "example",
								Type: "aws_instance",
							}},
						},
					},
				})
				require.NoError(t, err)

				<-publishedWorkspace
				<-publishedLogs

				workspace, err = db.GetWorkspaceByID(ctx, workspace.ID)
				require.NoError(t, err)
				require.Equal(t, c.transition == database.WorkspaceTransitionDelete, workspace.Deleted)

				workspaceBuild, err := db.GetWorkspaceBuildByID(ctx, build.ID)
				require.NoError(t, err)

				// If the max deadline is set, the deadline should also be set.
				// Default to the max deadline if the deadline is not set.
				if c.expectedDeadline.IsZero() {
					c.expectedDeadline = c.expectedMaxDeadline
				}

				if c.expectedDeadline.IsZero() {
					require.True(t, workspaceBuild.Deadline.IsZero())
				} else {
					require.WithinDuration(t, c.expectedDeadline, workspaceBuild.Deadline, 15*time.Second, "deadline does not match expected")
				}
				if c.expectedMaxDeadline.IsZero() {
					require.True(t, workspaceBuild.MaxDeadline.IsZero())
				} else {
					require.WithinDuration(t, c.expectedMaxDeadline, workspaceBuild.MaxDeadline, 15*time.Second, "max deadline does not match expected")
					require.GreaterOrEqual(t, workspaceBuild.MaxDeadline.Unix(), workspaceBuild.Deadline.Unix(), "max deadline is smaller than deadline")
				}
			})
		}
	})
	t.Run("TemplateDryRun", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
			StorageMethod: database.ProvisionerStorageMethodFile,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)

		_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
			JobId: job.ID.String(),
			Type: &proto.CompletedJob_TemplateDryRun_{
				TemplateDryRun: &proto.CompletedJob_TemplateDryRun{
					Resources: []*sdkproto.Resource{{
						Name: "something",
						Type: "aws_instance",
					}},
				},
			},
		})
		require.NoError(t, err)
	})
}

func TestInsertWorkspaceResource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	insert := func(db database.Store, jobID uuid.UUID, resource *sdkproto.Resource) error {
		return provisionerdserver.InsertWorkspaceResource(ctx, db, jobID, database.WorkspaceTransitionStart, resource, &telemetry.Snapshot{})
	}
	t.Run("NoAgents", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		job := uuid.New()
		err := insert(db, job, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
		})
		require.NoError(t, err)
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job)
		require.NoError(t, err)
		require.Len(t, resources, 1)
	})
	t.Run("InvalidAgentToken", func(t *testing.T) {
		t.Parallel()
		err := insert(dbmem.New(), uuid.New(), &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Auth: &sdkproto.Agent_Token{
					Token: "bananas",
				},
			}},
		})
		require.ErrorContains(t, err, "invalid UUID length")
	})
	t.Run("DuplicateApps", func(t *testing.T) {
		t.Parallel()
		err := insert(dbmem.New(), uuid.New(), &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Apps: []*sdkproto.App{{
					Slug: "a",
				}, {
					Slug: "a",
				}},
			}},
		})
		require.ErrorContains(t, err, "duplicate app slug")
	})
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		job := uuid.New()
		err := insert(db, job, &sdkproto.Resource{
			Name:      "something",
			Type:      "aws_instance",
			DailyCost: 10,
			Agents: []*sdkproto.Agent{{
				Name: "dev",
				Env: map[string]string{
					"something": "test",
				},
				OperatingSystem: "linux",
				Architecture:    "amd64",
				Auth: &sdkproto.Agent_Token{
					Token: uuid.NewString(),
				},
				Apps: []*sdkproto.App{{
					Slug: "a",
				}},
				ExtraEnvs: []*sdkproto.Env{
					{
						Name:  "something", // Duplicate, already set by Env.
						Value: "I should be discarded!",
					},
					{
						Name:  "else",
						Value: "I laugh in the face of danger.",
					},
				},
				Scripts: []*sdkproto.Script{{
					DisplayName: "Startup",
					Icon:        "/test.png",
				}},
				DisplayApps: &sdkproto.DisplayApps{
					Vscode:               true,
					PortForwardingHelper: true,
					SshHelper:            true,
				},
			}},
		})
		require.NoError(t, err)
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		require.EqualValues(t, 10, resources[0].DailyCost)
		agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, []uuid.UUID{resources[0].ID})
		require.NoError(t, err)
		require.Len(t, agents, 1)
		agent := agents[0]
		require.Equal(t, "amd64", agent.Architecture)
		require.Equal(t, "linux", agent.OperatingSystem)
		want, err := json.Marshal(map[string]string{
			"something": "test",
			"else":      "I laugh in the face of danger.",
		})
		require.NoError(t, err)
		got, err := agent.EnvironmentVariables.RawMessage.MarshalJSON()
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.ElementsMatch(t, []database.DisplayApp{
			database.DisplayAppPortForwardingHelper,
			database.DisplayAppSSHHelper,
			database.DisplayAppVscode,
		}, agent.DisplayApps)
	})

	t.Run("AllDisplayApps", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		job := uuid.New()
		err := insert(db, job, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				DisplayApps: &sdkproto.DisplayApps{
					Vscode:               true,
					VscodeInsiders:       true,
					SshHelper:            true,
					PortForwardingHelper: true,
					WebTerminal:          true,
				},
			}},
		})
		require.NoError(t, err)
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, []uuid.UUID{resources[0].ID})
		require.NoError(t, err)
		require.Len(t, agents, 1)
		agent := agents[0]
		require.ElementsMatch(t, database.AllDisplayAppValues(), agent.DisplayApps)
	})

	t.Run("DisableDefaultApps", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		job := uuid.New()
		err := insert(db, job, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				DisplayApps: &sdkproto.DisplayApps{},
			}},
		})
		require.NoError(t, err)
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, []uuid.UUID{resources[0].ID})
		require.NoError(t, err)
		require.Len(t, agents, 1)
		agent := agents[0]
		// An empty array (as opposed to nil) should be returned to indicate
		// that all apps are disabled.
		require.Equal(t, []database.DisplayApp{}, agent.DisplayApps)
	})
}

type overrides struct {
	ctx                         context.Context
	deploymentValues            *codersdk.DeploymentValues
	externalAuthConfigs         []*externalauth.Config
	templateScheduleStore       *atomic.Pointer[schedule.TemplateScheduleStore]
	userQuietHoursScheduleStore *atomic.Pointer[schedule.UserQuietHoursScheduleStore]
	timeNowFn                   func() time.Time
	acquireJobLongPollDuration  time.Duration
	heartbeatFn                 func(ctx context.Context) error
	heartbeatInterval           time.Duration
}

func setup(t *testing.T, ignoreLogErrors bool, ov *overrides) (proto.DRPCProvisionerDaemonServer, database.Store, pubsub.Pubsub, database.ProvisionerDaemon) {
	t.Helper()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	db := dbmem.New()
	ps := pubsub.NewInMemory()
	deploymentValues := &codersdk.DeploymentValues{}
	var externalAuthConfigs []*externalauth.Config
	tss := testTemplateScheduleStore()
	uqhss := testUserQuietHoursScheduleStore()
	var timeNowFn func() time.Time
	pollDur := time.Duration(0)
	if ov == nil {
		ov = &overrides{}
	}
	if ov.ctx == nil {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		ov.ctx = ctx
	}
	if ov.heartbeatInterval == 0 {
		ov.heartbeatInterval = testutil.IntervalMedium
	}
	if ov.deploymentValues != nil {
		deploymentValues = ov.deploymentValues
	}
	if ov.externalAuthConfigs != nil {
		externalAuthConfigs = ov.externalAuthConfigs
	}
	if ov.templateScheduleStore != nil {
		ttss := tss.Load()
		// keep the initial test value if the override hasn't set the atomic pointer.
		tss = ov.templateScheduleStore
		if tss.Load() == nil {
			swapped := tss.CompareAndSwap(nil, ttss)
			require.True(t, swapped)
		}
	}
	if ov.userQuietHoursScheduleStore != nil {
		tuqhss := uqhss.Load()
		// keep the initial test value if the override hasn't set the atomic pointer.
		uqhss = ov.userQuietHoursScheduleStore
		if uqhss.Load() == nil {
			swapped := uqhss.CompareAndSwap(nil, tuqhss)
			require.True(t, swapped)
		}
	}
	if ov.timeNowFn != nil {
		timeNowFn = ov.timeNowFn
	}
	pollDur = ov.acquireJobLongPollDuration

	daemon, err := db.UpsertProvisionerDaemon(ov.ctx, database.UpsertProvisionerDaemonParams{
		Name:         "test",
		CreatedAt:    dbtime.Now(),
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
		Tags:         database.StringMap{},
		LastSeenAt:   sql.NullTime{},
		Version:      buildinfo.Version(),
		APIVersion:   provisionersdk.VersionCurrent.String(),
	})
	require.NoError(t, err)

	srv, err := provisionerdserver.NewServer(
		ov.ctx,
		&url.URL{},
		daemon.ID,
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: ignoreLogErrors}),
		[]database.ProvisionerType{database.ProvisionerTypeEcho},
		provisionerdserver.Tags(daemon.Tags),
		db,
		ps,
		provisionerdserver.NewAcquirer(ov.ctx, logger.Named("acquirer"), db, ps),
		telemetry.NewNoop(),
		trace.NewNoopTracerProvider().Tracer("noop"),
		&atomic.Pointer[proto.QuotaCommitter]{},
		mockAuditor(),
		tss,
		uqhss,
		deploymentValues,
		provisionerdserver.Options{
			ExternalAuthConfigs:   externalAuthConfigs,
			TimeNowFn:             timeNowFn,
			OIDCConfig:            &oauth2.Config{},
			AcquireJobLongPollDur: pollDur,
			HeartbeatInterval:     ov.heartbeatInterval,
			HeartbeatFn:           ov.heartbeatFn,
		},
	)
	require.NoError(t, err)
	return srv, db, ps, daemon
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

var (
	errUnimplemented = xerrors.New("unimplemented")
	errClosed        = xerrors.New("closed")
)

type fakeStream struct {
	ctx        context.Context
	c          *sync.Cond
	closed     bool
	canceled   bool
	sendCalled bool
	job        *proto.AcquiredJob
}

func newFakeStream(ctx context.Context) *fakeStream {
	return &fakeStream{
		ctx: ctx,
		c:   sync.NewCond(&sync.Mutex{}),
	}
}

func (s *fakeStream) Send(j *proto.AcquiredJob) error {
	s.c.L.Lock()
	defer s.c.L.Unlock()
	s.sendCalled = true
	s.job = j
	s.c.Broadcast()
	return nil
}

func (s *fakeStream) Recv() (*proto.CancelAcquire, error) {
	s.c.L.Lock()
	defer s.c.L.Unlock()
	for !(s.canceled || s.closed) {
		s.c.Wait()
	}
	if s.canceled {
		return &proto.CancelAcquire{}, nil
	}
	return nil, io.EOF
}

// Context returns the context associated with the stream. It is canceled
// when the Stream is closed and no more messages will ever be sent or
// received on it.
func (s *fakeStream) Context() context.Context {
	return s.ctx
}

// MsgSend sends the Message to the remote.
func (*fakeStream) MsgSend(drpc.Message, drpc.Encoding) error {
	return errUnimplemented
}

// MsgRecv receives a Message from the remote.
func (*fakeStream) MsgRecv(drpc.Message, drpc.Encoding) error {
	return errUnimplemented
}

// CloseSend signals to the remote that we will no longer send any messages.
func (*fakeStream) CloseSend() error {
	return errUnimplemented
}

// Close closes the stream.
func (s *fakeStream) Close() error {
	s.c.L.Lock()
	defer s.c.L.Unlock()
	s.closed = true
	s.c.Broadcast()
	return nil
}

func (s *fakeStream) waitForJob() (*proto.AcquiredJob, error) {
	s.c.L.Lock()
	defer s.c.L.Unlock()
	for !(s.sendCalled || s.closed) {
		s.c.Wait()
	}
	if s.sendCalled {
		return s.job, nil
	}
	return nil, errClosed
}

func (s *fakeStream) cancel() {
	s.c.L.Lock()
	defer s.c.L.Unlock()
	s.canceled = true
	s.c.Broadcast()
}
