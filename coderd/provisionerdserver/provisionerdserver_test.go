package provisionerdserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
	"storj.io/drpc"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/usage"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

func testTemplateScheduleStore() *atomic.Pointer[schedule.TemplateScheduleStore] {
	poitr := &atomic.Pointer[schedule.TemplateScheduleStore]{}
	store := schedule.NewAGPLTemplateScheduleStore()
	poitr.Store(&store)
	return poitr
}

func testUserQuietHoursScheduleStore() *atomic.Pointer[schedule.UserQuietHoursScheduleStore] {
	poitr := &atomic.Pointer[schedule.UserQuietHoursScheduleStore]{}
	store := schedule.NewAGPLUserQuietHoursScheduleStore()
	poitr.Store(&store)
	return poitr
}

func testUsageInserter() *atomic.Pointer[usage.Inserter] {
	poitr := &atomic.Pointer[usage.Inserter]{}
	inserter := usage.NewAGPLInserter()
	poitr.Store(&inserter)
	return poitr
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
		t.Log("heartbeat")
		select {
		case <-hbCtx.Done():
			return hbCtx.Err()
		case heartbeatChan <- struct{}{}:
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
		testutil.TryReceive(ctx, t, heartbeatChan)
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
		t.Run(tc.name+"_InitiatorNotFound", func(t *testing.T) {
			t.Parallel()
			srv, db, _, pd := setup(t, false, nil)
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			_, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
				OrganizationID: pd.OrganizationID,
				ID:             uuid.New(),
				InitiatorID:    uuid.New(),
				Provisioner:    database.ProvisionerTypeEcho,
				StorageMethod:  database.ProvisionerStorageMethodFile,
				Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
				Input:          json.RawMessage("{}"),
				Tags:           pd.Tags,
			})
			require.NoError(t, err)
			_, err = tc.acquire(ctx, srv)
			require.ErrorContains(t, err, "sql: no rows in result set")
		})
		for _, prebuiltWorkspaceBuildStage := range []sdkproto.PrebuiltWorkspaceBuildStage{
			sdkproto.PrebuiltWorkspaceBuildStage_NONE,
			sdkproto.PrebuiltWorkspaceBuildStage_CREATE,
			sdkproto.PrebuiltWorkspaceBuildStage_CLAIM,
		} {
			t.Run(tc.name+"_WorkspaceBuildJob_Stage"+prebuiltWorkspaceBuildStage.String(), func(t *testing.T) {
				t.Parallel()
				// Set the max session token lifetime so we can assert we
				// create an API key with an expiration within the bounds of the
				// deployment config.
				dv := &codersdk.DeploymentValues{
					Sessions: codersdk.SessionLifetime{
						MaximumTokenDuration: serpent.Duration(time.Hour),
					},
				}
				gitAuthProvider := &sdkproto.ExternalAuthProviderResource{
					Id: "github",
				}

				srv, db, ps, pd := setup(t, false, &overrides{
					deploymentValues: dv,
					externalAuthConfigs: []*externalauth.Config{{
						ID:                       gitAuthProvider.Id,
						InstrumentedOAuth2Config: &testutil.OAuth2Config{},
					}},
				})
				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				user := dbgen.User(t, db, database.User{})
				group1 := dbgen.Group(t, db, database.Group{
					Name:           "group1",
					OrganizationID: pd.OrganizationID,
				})
				sshKey := dbgen.GitSSHKey(t, db, database.GitSSHKey{
					UserID: user.ID,
				})
				err := db.InsertGroupMember(ctx, database.InsertGroupMemberParams{
					UserID:  user.ID,
					GroupID: group1.ID,
				})
				require.NoError(t, err)
				dbgen.OrganizationMember(t, db, database.OrganizationMember{
					UserID:         user.ID,
					OrganizationID: pd.OrganizationID,
					Roles:          []string{rbac.RoleOrgAuditor()},
				})

				// Add extra erroneous roles
				secondOrg := dbgen.Organization(t, db, database.Organization{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{
					UserID:         user.ID,
					OrganizationID: secondOrg.ID,
					Roles:          []string{rbac.RoleOrgAuditor()},
				})

				link := dbgen.UserLink(t, db, database.UserLink{
					LoginType:        database.LoginTypeOIDC,
					UserID:           user.ID,
					OAuthExpiry:      dbtime.Now().Add(time.Hour),
					OAuthAccessToken: "access-token",
				})
				dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
					ProviderID: gitAuthProvider.Id,
					UserID:     user.ID,
				})
				template := dbgen.Template(t, db, database.Template{
					Name:           "template",
					Provisioner:    database.ProvisionerTypeEcho,
					OrganizationID: pd.OrganizationID,
					CreatedBy:      user.ID,
				})
				file := dbgen.File(t, db, database.File{CreatedBy: user.ID, Hash: "1"})
				versionFile := dbgen.File(t, db, database.File{CreatedBy: user.ID, Hash: "2"})
				version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
					CreatedBy:      user.ID,
					OrganizationID: pd.OrganizationID,
					TemplateID: uuid.NullUUID{
						UUID:  template.ID,
						Valid: true,
					},
					JobID: uuid.New(),
				})
				externalAuthProviders, err := json.Marshal([]database.ExternalAuthProvider{{
					ID:       gitAuthProvider.Id,
					Optional: gitAuthProvider.Optional,
				}})
				require.NoError(t, err)
				err = db.UpdateTemplateVersionExternalAuthProvidersByJobID(ctx, database.UpdateTemplateVersionExternalAuthProvidersByJobIDParams{
					JobID:                 version.JobID,
					ExternalAuthProviders: json.RawMessage(externalAuthProviders),
					UpdatedAt:             dbtime.Now(),
				})
				require.NoError(t, err)
				// Import version job
				_ = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
					OrganizationID: pd.OrganizationID,
					ID:             version.JobID,
					InitiatorID:    user.ID,
					FileID:         versionFile.ID,
					Provisioner:    database.ProvisionerTypeEcho,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeTemplateVersionImport,
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
				workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
					TemplateID:     template.ID,
					OwnerID:        user.ID,
					OrganizationID: pd.OrganizationID,
				})
				buildID := uuid.New()
				dbJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
					OrganizationID: pd.OrganizationID,
					InitiatorID:    user.ID,
					Provisioner:    database.ProvisionerTypeEcho,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					FileID:         file.ID,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID: buildID,
					})),
					Tags: pd.Tags,
				})
				build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID:                buildID,
					WorkspaceID:       workspace.ID,
					BuildNumber:       1,
					JobID:             dbJob.ID,
					TemplateVersionID: version.ID,
					Transition:        database.WorkspaceTransitionStart,
					Reason:            database.BuildReasonInitiator,
				})
				task := dbgen.Task(t, db, database.TaskTable{
					OrganizationID:     pd.OrganizationID,
					OwnerID:            user.ID,
					WorkspaceID:        uuid.NullUUID{Valid: true, UUID: workspace.ID},
					TemplateVersionID:  version.ID,
					TemplateParameters: json.RawMessage("{}"),
					Prompt:             "Build me a REST API",
					CreatedAt:          dbtime.Now(),
					DeletedAt:          sql.NullTime{},
				})

				var agent database.WorkspaceAgent
				if prebuiltWorkspaceBuildStage == sdkproto.PrebuiltWorkspaceBuildStage_CLAIM {
					resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
						JobID: dbJob.ID,
					})
					agent = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
						ResourceID: resource.ID,
						AuthToken:  uuid.New(),
					})
					buildID := uuid.New()
					input := provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID:            buildID,
						PrebuiltWorkspaceBuildStage: prebuiltWorkspaceBuildStage,
					}
					dbJob = database.ProvisionerJob{
						OrganizationID: pd.OrganizationID,
						InitiatorID:    user.ID,
						Provisioner:    database.ProvisionerTypeEcho,
						StorageMethod:  database.ProvisionerStorageMethodFile,
						FileID:         file.ID,
						Type:           database.ProvisionerJobTypeWorkspaceBuild,
						Input:          must(json.Marshal(input)),
						Tags:           pd.Tags,
					}
					dbJob = dbgen.ProvisionerJob(t, db, ps, dbJob)
					// At this point we have an unclaimed workspace and build, now we need to setup the claim
					// build.
					build = database.WorkspaceBuild{
						ID:                buildID,
						WorkspaceID:       workspace.ID,
						BuildNumber:       2,
						JobID:             dbJob.ID,
						TemplateVersionID: version.ID,
						Transition:        database.WorkspaceTransitionStart,
						Reason:            database.BuildReasonInitiator,
						InitiatorID:       user.ID,
					}
					build = dbgen.WorkspaceBuild(t, db, build)
				}

				startPublished := make(chan struct{})
				var closed bool
				closeStartSubscribe, err := ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
					wspubsub.HandleWorkspaceEvent(
						func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
							if err != nil {
								return
							}
							if e.Kind == wspubsub.WorkspaceEventKindStateChange && e.WorkspaceID == workspace.ID {
								if !closed {
									close(startPublished)
									closed = true
								}
							}
						}))
				require.NoError(t, err)
				defer closeStartSubscribe()

				var job *proto.AcquiredJob

				for {
					// Grab jobs until we find the workspace build job. There is also
					// an import version job that we need to ignore.
					job, err = tc.acquire(ctx, srv)
					require.NoError(t, err)
					if job, ok := job.Type.(*proto.AcquiredJob_WorkspaceBuild_); ok {
						// In the case of a prebuild claim, there is a second build, which is the
						// one that we're interested in.
						if prebuiltWorkspaceBuildStage == sdkproto.PrebuiltWorkspaceBuildStage_CLAIM &&
							job.WorkspaceBuild.Metadata.PrebuiltWorkspaceBuildStage != prebuiltWorkspaceBuildStage {
							continue
						}
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
				require.Equal(t, int64(dv.Sessions.MaximumTokenDuration.Value().Seconds()), key.LifetimeSeconds)
				require.WithinDuration(t, time.Now().Add(dv.Sessions.MaximumTokenDuration.Value()), key.ExpiresAt, time.Minute)

				wantedMetadata := &sdkproto.Metadata{
					CoderUrl:                      (&url.URL{}).String(),
					WorkspaceTransition:           sdkproto.WorkspaceTransition_START,
					WorkspaceName:                 workspace.Name,
					WorkspaceOwner:                user.Username,
					WorkspaceOwnerEmail:           user.Email,
					WorkspaceOwnerName:            user.Name,
					WorkspaceOwnerOidcAccessToken: link.OAuthAccessToken,
					WorkspaceOwnerGroups:          []string{"Everyone", group1.Name},
					WorkspaceId:                   workspace.ID.String(),
					WorkspaceOwnerId:              user.ID.String(),
					TemplateId:                    template.ID.String(),
					TemplateName:                  template.Name,
					TemplateVersion:               version.Name,
					TemplateVersionId:             version.ID.String(),
					WorkspaceOwnerSessionToken:    sessionToken,
					WorkspaceOwnerSshPublicKey:    sshKey.PublicKey,
					WorkspaceOwnerSshPrivateKey:   sshKey.PrivateKey,
					WorkspaceBuildId:              build.ID.String(),
					WorkspaceOwnerLoginType:       string(user.LoginType),
					WorkspaceOwnerRbacRoles:       []*sdkproto.Role{{Name: rbac.RoleOrgMember(), OrgId: pd.OrganizationID.String()}, {Name: "member", OrgId: ""}, {Name: rbac.RoleOrgAuditor(), OrgId: pd.OrganizationID.String()}},
					TaskId:                        task.ID.String(),
					TaskPrompt:                    task.Prompt,
				}
				if prebuiltWorkspaceBuildStage == sdkproto.PrebuiltWorkspaceBuildStage_CLAIM {
					// For claimed prebuilds, we expect the prebuild state to be set to CLAIM
					// and we expect tokens from the first build to be set for reuse
					wantedMetadata.PrebuiltWorkspaceBuildStage = prebuiltWorkspaceBuildStage
					wantedMetadata.RunningAgentAuthTokens = append(wantedMetadata.RunningAgentAuthTokens, &sdkproto.RunningAgentAuthToken{
						AgentId: agent.ID.String(),
						Token:   agent.AuthToken.String(),
					})
				}

				slices.SortFunc(wantedMetadata.WorkspaceOwnerRbacRoles, func(a, b *sdkproto.Role) int {
					return strings.Compare(a.Name+a.OrgId, b.Name+b.OrgId)
				})
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
							Id:          gitAuthProvider.Id,
							AccessToken: "access_token",
						}},
						Metadata: wantedMetadata,
					},
				})
				require.NoError(t, err)

				require.JSONEq(t, string(want), string(got))

				stopbuildID := uuid.New()
				stopJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
					ID:            stopbuildID,
					InitiatorID:   user.ID,
					Provisioner:   database.ProvisionerTypeEcho,
					StorageMethod: database.ProvisionerStorageMethodFile,
					FileID:        file.ID,
					Type:          database.ProvisionerJobTypeWorkspaceBuild,
					Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID: stopbuildID,
					})),
					Tags: pd.Tags,
				})
				// Assert that we delete the session token whenever
				// a stop is issued.
				_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID:                stopbuildID,
					WorkspaceID:       workspace.ID,
					BuildNumber:       3,
					JobID:             stopJob.ID,
					TemplateVersionID: version.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonInitiator,
				})

				stopPublished := make(chan struct{})
				closeStopSubscribe, err := ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
					wspubsub.HandleWorkspaceEvent(
						func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
							if err != nil {
								return
							}
							if e.Kind == wspubsub.WorkspaceEventKindStateChange && e.WorkspaceID == workspace.ID {
								close(stopPublished)
							}
						}))
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
		}
		t.Run(tc.name+"_TemplateVersionDryRun", func(t *testing.T) {
			t.Parallel()
			srv, db, ps, pd := setup(t, false, nil)
			ctx := context.Background()

			user := dbgen.User(t, db, database.User{})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedBy:      user.ID,
				OrganizationID: pd.OrganizationID,
			})
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
				Tags: pd.Tags,
			})

			job, err := tc.acquire(ctx, srv)
			require.NoError(t, err)

			// sort
			if wk, ok := job.Type.(*proto.AcquiredJob_WorkspaceBuild_); ok {
				slices.SortFunc(wk.WorkspaceBuild.Metadata.WorkspaceOwnerRbacRoles, func(a, b *sdkproto.Role) int {
					return strings.Compare(a.Name+a.OrgId, b.Name+b.OrgId)
				})
			}

			got, err := json.Marshal(job.Type)
			require.NoError(t, err)

			want, err := json.Marshal(&proto.AcquiredJob_TemplateDryRun_{
				TemplateDryRun: &proto.AcquiredJob_TemplateDryRun{
					Metadata: &sdkproto.Metadata{
						CoderUrl:             (&url.URL{}).String(),
						WorkspaceName:        "testing",
						WorkspaceOwnerGroups: []string{database.EveryoneGroup},
					},
				},
			})
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
		t.Run(tc.name+"_TemplateVersionImport", func(t *testing.T) {
			t.Parallel()
			srv, db, ps, pd := setup(t, false, nil)
			ctx := context.Background()

			user := dbgen.User(t, db, database.User{})
			file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			_ = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				FileID:        file.ID,
				InitiatorID:   user.ID,
				Provisioner:   database.ProvisionerTypeEcho,
				StorageMethod: database.ProvisionerStorageMethodFile,
				Type:          database.ProvisionerJobTypeTemplateVersionImport,
				Tags:          pd.Tags,
			})

			job, err := tc.acquire(ctx, srv)
			require.NoError(t, err)

			got, err := json.Marshal(job.Type)
			require.NoError(t, err)

			want, err := json.Marshal(&proto.AcquiredJob_TemplateImport_{
				TemplateImport: &proto.AcquiredJob_TemplateImport{
					Metadata: &sdkproto.Metadata{
						CoderUrl:             (&url.URL{}).String(),
						WorkspaceOwnerGroups: []string{database.EveryoneGroup},
						TemplateVersionId:    uuid.Nil.String(),
					},
				},
			})
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
		t.Run(tc.name+"_TemplateVersionImportWithUserVariable", func(t *testing.T) {
			t.Parallel()
			srv, db, ps, pd := setup(t, false, nil)

			user := dbgen.User(t, db, database.User{})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedBy:      user.ID,
				OrganizationID: pd.OrganizationID,
			})
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
				Tags: pd.Tags,
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
						CoderUrl:             (&url.URL{}).String(),
						WorkspaceOwnerGroups: []string{database.EveryoneGroup},
						TemplateVersionId:    version.ID.String(),
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
		srv, db, _, pd := setup(t, false, nil)
		user := dbgen.User(t, db, database.User{})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID,
			JobID:          uuid.New(),
		})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            version.JobID,
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
			Input: must(json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
				TemplateVersionID: version.ID,
			})),
			OrganizationID: pd.OrganizationID,
			Tags:           pd.Tags,
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
		srv, db, _, pd := setup(t, false, nil)
		user := dbgen.User(t, db, database.User{})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID,
			JobID:          uuid.New(),
		})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            version.JobID,
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
			Input: must(json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
				TemplateVersionID: version.ID,
			})),
			OrganizationID: pd.OrganizationID,
			Tags:           pd.Tags,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			StartedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			OrganizationID:  pd.OrganizationID,
			ProvisionerTags: must(json.Marshal(job.Tags)),
		})
		require.NoError(t, err)
		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "you don't own this job")
	})

	setupJob := func(t *testing.T, db database.Store, srvID, orgID uuid.UUID, tags database.StringMap) (templateVersionID, jobID uuid.UUID) {
		templateVersionID = uuid.New()
		jobID = uuid.New()
		err := db.InTx(func(db database.Store) error {
			user := dbgen.User(t, db, database.User{})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				ID:             templateVersionID,
				CreatedBy:      user.ID,
				OrganizationID: orgID,
				JobID:          jobID,
			})
			job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
				ID:             version.JobID,
				OrganizationID: orgID,
				Provisioner:    database.ProvisionerTypeEcho,
				Type:           database.ProvisionerJobTypeTemplateVersionImport,
				StorageMethod:  database.ProvisionerStorageMethodFile,
				Input: must(json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
					TemplateVersionID: version.ID,
				})),
				Tags: tags,
			})
			if err != nil {
				return xerrors.Errorf("insert provisioner job: %w", err)
			}
			_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
				WorkerID: uuid.NullUUID{
					UUID:  srvID,
					Valid: true,
				},
				Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
				StartedAt: sql.NullTime{
					Time:  dbtime.Now(),
					Valid: true,
				},
				OrganizationID:  orgID,
				ProvisionerTags: must(json.Marshal(job.Tags)),
			})
			if err != nil {
				return xerrors.Errorf("acquire provisioner job: %w", err)
			}
			return nil
		}, nil)
		require.NoError(t, err)
		return templateVersionID, jobID
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		_, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)
		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
		})
		require.NoError(t, err)
	})

	t.Run("Logs", func(t *testing.T) {
		t.Parallel()
		srv, db, ps, pd := setup(t, false, &overrides{})
		_, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)

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
		templateVersionID, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)
		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId:  job.String(),
			Readme: []byte("# hello world"),
		})
		require.NoError(t, err)

		version, err := db.GetTemplateVersionByID(ctx, templateVersionID)
		require.NoError(t, err)
		require.Equal(t, "# hello world", version.Readme)
	})

	t.Run("TemplateVariables", func(t *testing.T) {
		t.Parallel()

		t.Run("Valid", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			srv, db, _, pd := setup(t, false, &overrides{})
			templateVersionID, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)
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

			templateVariables, err := db.GetTemplateVersionVariables(ctx, templateVersionID)
			require.NoError(t, err)
			require.Len(t, templateVariables, 2)
			require.Equal(t, templateVariables[0].Value, firstTemplateVariable.DefaultValue)
			require.Equal(t, templateVariables[1].Value, "foobar")
		})

		t.Run("Missing required value", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			srv, db, _, pd := setup(t, false, &overrides{})
			templateVersionID, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)
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
			templateVariables, err := db.GetTemplateVersionVariables(ctx, templateVersionID)
			require.NoError(t, err)
			require.Len(t, templateVariables, 2)
			require.Equal(t, templateVariables[0].Value, firstTemplateVariable.DefaultValue)
			require.Equal(t, templateVariables[1].Value, "")
		})
	})

	t.Run("WorkspaceTags", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		srv, db, _, pd := setup(t, false, nil)
		templateVersionID, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)
		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
			WorkspaceTags: map[string]string{
				"bird": "tweety",
				"cat":  "jinx",
			},
		})
		require.NoError(t, err)

		workspaceTags, err := db.GetTemplateVersionWorkspaceTags(ctx, templateVersionID)
		require.NoError(t, err)
		require.Len(t, workspaceTags, 2)
		require.Equal(t, workspaceTags[0].Key, "bird")
		require.Equal(t, workspaceTags[0].Value, "tweety")
		require.Equal(t, workspaceTags[1].Key, "cat")
		require.Equal(t, workspaceTags[1].Value, "jinx")
	})

	t.Run("LogSizeLimit", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		_, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)

		// Create a log message that exceeds the 1MB limit
		largeOutput := strings.Repeat("a", 1048577) // 1MB + 1 byte

		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
			Logs: []*proto.Log{{
				Source: proto.LogSource_PROVISIONER,
				Level:  sdkproto.LogLevel_INFO,
				Output: largeOutput,
			}},
		})
		require.NoError(t, err) // Should succeed but trigger overflow

		// Verify the overflow flag is set
		jobResult, err := db.GetProvisionerJobByID(ctx, job)
		require.NoError(t, err)
		require.True(t, jobResult.LogsOverflowed)
	})

	t.Run("IncrementalLogSizeOverflow", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		_, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)

		// Send logs that together exceed the limit
		mediumOutput := strings.Repeat("b", 524289) // Half a MB + 1 byte

		// First log - should succeed
		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
			Logs: []*proto.Log{{
				Source: proto.LogSource_PROVISIONER,
				Level:  sdkproto.LogLevel_INFO,
				Output: mediumOutput,
			}},
		})
		require.NoError(t, err)

		// Verify overflow flag not yet set
		jobResult, err := db.GetProvisionerJobByID(ctx, job)
		require.NoError(t, err)
		require.False(t, jobResult.LogsOverflowed)

		// Second log - should trigger overflow
		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
			Logs: []*proto.Log{{
				Source: proto.LogSource_PROVISIONER,
				Level:  sdkproto.LogLevel_INFO,
				Output: mediumOutput,
			}},
		})
		require.NoError(t, err)

		// Verify overflow flag is set
		jobResult, err = db.GetProvisionerJobByID(ctx, job)
		require.NoError(t, err)
		require.True(t, jobResult.LogsOverflowed)
	})

	t.Run("LogSizeTracking", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		_, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)

		logOutput := "test log message"
		expectedSize := int32(len(logOutput)) // #nosec G115 - Log length is 16.

		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
			Logs: []*proto.Log{{
				Source: proto.LogSource_PROVISIONER,
				Level:  sdkproto.LogLevel_INFO,
				Output: logOutput,
			}},
		})
		require.NoError(t, err)

		// Verify the logs_length is correctly tracked
		jobResult, err := db.GetProvisionerJobByID(ctx, job)
		require.NoError(t, err)
		require.Equal(t, expectedSize, jobResult.LogsLength)
		require.False(t, jobResult.LogsOverflowed)
	})

	t.Run("LogOverflowStopsProcessing", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		_, job := setupJob(t, db, pd.ID, pd.OrganizationID, pd.Tags)

		// First: trigger overflow
		largeOutput := strings.Repeat("a", 1048577) // 1MB + 1 byte
		_, err := srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
			Logs: []*proto.Log{{
				Source: proto.LogSource_PROVISIONER,
				Level:  sdkproto.LogLevel_INFO,
				Output: largeOutput,
			}},
		})
		require.NoError(t, err)

		// Get the initial log count
		initialLogs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
			JobID:        job,
			CreatedAfter: -1,
		})
		require.NoError(t, err)
		initialCount := len(initialLogs)

		// Second: try to send more logs - should be ignored
		_, err = srv.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.String(),
			Logs: []*proto.Log{{
				Source: proto.LogSource_PROVISIONER,
				Level:  sdkproto.LogLevel_INFO,
				Output: "this should be ignored",
			}},
		})
		require.NoError(t, err)

		// Verify no new logs were added
		finalLogs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
			JobID:        job,
			CreatedAfter: -1,
		})
		require.NoError(t, err)
		require.Equal(t, initialCount, len(finalLogs))
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
		srv, db, _, pd := setup(t, false, nil)
		user := dbgen.User(t, db, database.User{})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID,
			JobID:          uuid.New(),
		})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            version.JobID,
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionImport,
			Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
				TemplateVersionID: version.ID,
			})),
			OrganizationID: pd.OrganizationID,
			Tags:           pd.Tags,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			StartedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			OrganizationID:  pd.OrganizationID,
			ProvisionerTags: must(json.Marshal(job.Tags)),
		})
		require.NoError(t, err)
		_, err = srv.FailJob(ctx, &proto.FailedJob{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "you don't own this job")
	})
	t.Run("AlreadyCompleted", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, nil)
		user := dbgen.User(t, db, database.User{})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID,
			JobID:          uuid.New(),
		})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            version.JobID,
			Provisioner:   database.ProvisionerTypeEcho,
			Type:          database.ProvisionerJobTypeTemplateVersionImport,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
				TemplateVersionID: version.ID,
			})),
			OrganizationID: pd.OrganizationID,
			Tags:           pd.Tags,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			StartedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			OrganizationID:  pd.OrganizationID,
			ProvisionerTags: must(json.Marshal(job.Tags)),
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
		auditor := audit.NewMock()
		srv, db, ps, pd := setup(t, false, &overrides{
			auditor: auditor,
		})
		org := dbgen.Organization(t, db, database.Organization{})
		u := dbgen.User(t, db, database.User{})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      u.ID,
			OrganizationID: org.ID,
		})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			CreatedBy:       u.ID,
			ActiveVersionID: tv.ID,
		})
		workspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:               uuid.New(),
			AutomaticUpdates: database.AutomaticUpdatesNever,
			OrganizationID:   org.ID,
			TemplateID:       tpl.ID,
			OwnerID:          u.ID,
		})
		require.NoError(t, err)
		buildID := uuid.New()
		input, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
			WorkspaceBuildID: buildID,
		})
		require.NoError(t, err)

		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			Input:          input,
			InitiatorID:    workspace.OwnerID,
			OrganizationID: pd.OrganizationID,
			Provisioner:    database.ProvisionerTypeEcho,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Tags:           pd.Tags,
		})
		require.NoError(t, err)
		err = db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:                buildID,
			WorkspaceID:       workspace.ID,
			InitiatorID:       workspace.OwnerID,
			TemplateVersionID: tpl.ActiveVersionID,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
			JobID:             job.ID,
		})
		require.NoError(t, err)

		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			StartedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			OrganizationID:  pd.OrganizationID,
			ProvisionerTags: must(json.Marshal(job.Tags)),
		})
		require.NoError(t, err)

		publishedWorkspace := make(chan struct{})
		closeWorkspaceSubscribe, err := ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
			wspubsub.HandleWorkspaceEvent(
				func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
					if err != nil {
						return
					}
					if e.Kind == wspubsub.WorkspaceEventKindStateChange && e.WorkspaceID == workspace.ID {
						close(publishedWorkspace)
					}
				}))
		require.NoError(t, err)
		defer closeWorkspaceSubscribe()
		publishedLogs := make(chan struct{})
		closeLogsSubscribe, err := ps.Subscribe(provisionersdk.ProvisionerJobLogsNotifyChannel(job.ID), func(_ context.Context, _ []byte) {
			close(publishedLogs)
		})
		require.NoError(t, err)
		defer closeLogsSubscribe()

		auditor.ResetLogs()
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
		require.Len(t, auditor.AuditLogs(), 1)

		// Assert that the workspace_id field get populated
		var additionalFields audit.AdditionalFields
		err = json.Unmarshal(auditor.AuditLogs()[0].AdditionalFields, &additionalFields)
		require.NoError(t, err)
		require.Equal(t, workspace.ID, additionalFields.WorkspaceID)
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
		srv, db, _, pd := setup(t, false, nil)
		user := dbgen.User(t, db, database.User{})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID,
			JobID:          uuid.New(),
		})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             version.JobID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			OrganizationID: pd.OrganizationID,
			Input: must(json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
				TemplateVersionID: version.ID,
			})),
			Tags: pd.Tags,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			OrganizationID: pd.OrganizationID,
			WorkerID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			StartedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			ProvisionerTags: must(json.Marshal(job.Tags)),
		})
		require.NoError(t, err)
		_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
			JobId: job.ID.String(),
		})
		require.ErrorContains(t, err, "you don't own this job")
	})

	// Test for verifying transaction behavior on the extracted methods
	t.Run("TransactionBehavior", func(t *testing.T) {
		t.Parallel()
		// Test TemplateImport transaction
		t.Run("TemplateImportTransaction", func(t *testing.T) {
			t.Parallel()
			srv, db, _, pd := setup(t, false, &overrides{})
			jobID := uuid.New()
			versionID := uuid.New()
			user := dbgen.User(t, db, database.User{})
			err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
				CreatedBy:      user.ID,
				ID:             versionID,
				JobID:          jobID,
				OrganizationID: pd.OrganizationID,
			})
			require.NoError(t, err)
			job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
				OrganizationID: pd.OrganizationID,
				ID:             jobID,
				Provisioner:    database.ProvisionerTypeEcho,
				Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
					TemplateVersionID: versionID,
				})),
				StorageMethod: database.ProvisionerStorageMethodFile,
				Type:          database.ProvisionerJobTypeTemplateVersionImport,
				Tags:          pd.Tags,
			})
			require.NoError(t, err)
			_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
				OrganizationID: pd.OrganizationID,
				WorkerID: uuid.NullUUID{
					UUID:  pd.ID,
					Valid: true,
				},
				Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
				ProvisionerTags: must(json.Marshal(job.Tags)),
				StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
			})
			require.NoError(t, err)

			_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.ID.String(),
				Type: &proto.CompletedJob_TemplateImport_{
					TemplateImport: &proto.CompletedJob_TemplateImport{
						StartResources: []*sdkproto.Resource{{
							Name: "test-resource",
							Type: "aws_instance",
						}},
						Plan: []byte("{}"),
					},
				},
			})
			require.NoError(t, err)

			// Verify job was marked as completed
			completedJob, err := db.GetProvisionerJobByID(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, completedJob.CompletedAt.Valid, "Job should be marked as completed")

			// Verify resources were created
			resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
			require.NoError(t, err)
			require.Len(t, resources, 1, "Expected one resource to be created")
			require.Equal(t, "test-resource", resources[0].Name)
		})

		// Test TemplateDryRun transaction
		t.Run("TemplateDryRunTransaction", func(t *testing.T) {
			t.Parallel()
			srv, db, _, pd := setup(t, false, &overrides{})
			org := dbgen.Organization(t, db, database.Organization{})
			user := dbgen.User(t, db, database.User{})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedBy:      user.ID,
				OrganizationID: org.ID,
				JobID:          uuid.New(),
			})
			job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
				ID:             uuid.New(),
				OrganizationID: org.ID,
				Provisioner:    database.ProvisionerTypeEcho,
				Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
				StorageMethod:  database.ProvisionerStorageMethodFile,
				Input: must(json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
					TemplateVersionID: version.ID,
				})),
				Tags: pd.Tags,
			})
			require.NoError(t, err)
			_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
				WorkerID: uuid.NullUUID{
					UUID:  pd.ID,
					Valid: true,
				},
				OrganizationID:  org.ID,
				Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
				ProvisionerTags: must(json.Marshal(job.Tags)),
				StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
			})
			require.NoError(t, err)

			_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.ID.String(),
				Type: &proto.CompletedJob_TemplateDryRun_{
					TemplateDryRun: &proto.CompletedJob_TemplateDryRun{
						Resources: []*sdkproto.Resource{{
							Name: "test-dry-run-resource",
							Type: "aws_instance",
						}},
					},
				},
			})
			require.NoError(t, err)

			// Verify job was marked as completed
			completedJob, err := db.GetProvisionerJobByID(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, completedJob.CompletedAt.Valid, "Job should be marked as completed")

			// Verify resources were created
			resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
			require.NoError(t, err)
			require.Len(t, resources, 1, "Expected one resource to be created")
			require.Equal(t, "test-dry-run-resource", resources[0].Name)
		})

		// Test WorkspaceBuild transaction
		t.Run("WorkspaceBuildTransaction", func(t *testing.T) {
			t.Parallel()
			srv, db, ps, pd := setup(t, false, &overrides{})

			// Create test data
			user := dbgen.User(t, db, database.User{})
			template := dbgen.Template(t, db, database.Template{
				Name:           "template",
				CreatedBy:      user.ID,
				Provisioner:    database.ProvisionerTypeEcho,
				OrganizationID: pd.OrganizationID,
			})
			file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
			workspaceTable := dbgen.Workspace(t, db, database.WorkspaceTable{
				TemplateID:     template.ID,
				OwnerID:        user.ID,
				OrganizationID: pd.OrganizationID,
			})
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: pd.OrganizationID,
				CreatedBy:      user.ID,
				TemplateID: uuid.NullUUID{
					UUID:  template.ID,
					Valid: true,
				},
				JobID: uuid.New(),
			})
			wsBuildID := uuid.New()
			job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				ID:          uuid.New(),
				FileID:      file.ID,
				InitiatorID: user.ID,
				Type:        database.ProvisionerJobTypeWorkspaceBuild,
				Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
					WorkspaceBuildID: wsBuildID,
				})),
				OrganizationID: pd.OrganizationID,
			})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				ID:                wsBuildID,
				JobID:             job.ID,
				WorkspaceID:       workspaceTable.ID,
				TemplateVersionID: version.ID,
				Transition:        database.WorkspaceTransitionStart,
				Reason:            database.BuildReasonInitiator,
			})
			_, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
				OrganizationID: pd.OrganizationID,
				WorkerID: uuid.NullUUID{
					UUID:  pd.ID,
					Valid: true,
				},
				Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
				ProvisionerTags: must(json.Marshal(job.Tags)),
				StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
			})
			require.NoError(t, err)

			// Add a published channel to make sure the workspace event is sent
			publishedWorkspace := make(chan struct{})
			closeWorkspaceSubscribe, err := ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspaceTable.OwnerID),
				wspubsub.HandleWorkspaceEvent(
					func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
						if err != nil {
							return
						}
						if e.Kind == wspubsub.WorkspaceEventKindStateChange && e.WorkspaceID == workspaceTable.ID {
							close(publishedWorkspace)
						}
					}))
			require.NoError(t, err)
			defer closeWorkspaceSubscribe()

			// The actual test
			_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.ID.String(),
				Type: &proto.CompletedJob_WorkspaceBuild_{
					WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
						State: []byte{},
						Resources: []*sdkproto.Resource{{
							Name: "test-workspace-resource",
							Type: "aws_instance",
						}},
						Timings: []*sdkproto.Timing{
							{
								Stage:    "init",
								Source:   "test-source",
								Resource: "test-resource",
								Action:   "test-action",
								Start:    timestamppb.Now(),
								End:      timestamppb.Now(),
							},
							{
								Stage:    "plan",
								Source:   "test-source2",
								Resource: "test-resource2",
								Action:   "test-action2",
								// Start: omitted
								// End: omitted
							},
							{
								Stage:    "test3",
								Source:   "test-source3",
								Resource: "test-resource3",
								Action:   "test-action3",
								Start:    timestamppb.Now(),
								End:      nil,
							},
							{
								Stage:    "test3",
								Source:   "test-source3",
								Resource: "test-resource3",
								Action:   "test-action3",
								Start:    nil,
								End:      timestamppb.Now(),
							},
							{
								Stage:    "test4",
								Source:   "test-source4",
								Resource: "test-resource4",
								Action:   "test-action4",
								Start:    timestamppb.New(time.Time{}),
								End:      timestamppb.Now(),
							},
							{
								Stage:    "test5",
								Source:   "test-source5",
								Resource: "test-resource5",
								Action:   "test-action5",
								Start:    timestamppb.Now(),
								End:      timestamppb.New(time.Time{}),
							},
							nil, // nil timing should be ignored
						},
					},
				},
			})
			require.NoError(t, err)

			// Wait for workspace notification
			select {
			case <-publishedWorkspace:
				// Success
			case <-time.After(testutil.WaitShort):
				t.Fatal("Workspace event not published")
			}

			// Verify job was marked as completed
			completedJob, err := db.GetProvisionerJobByID(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, completedJob.CompletedAt.Valid, "Job should be marked as completed")

			// Verify resources were created
			resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
			require.NoError(t, err)
			require.Len(t, resources, 1, "Expected one resource to be created")
			require.Equal(t, "test-workspace-resource", resources[0].Name)

			// Verify timings were recorded
			timings, err := db.GetProvisionerJobTimingsByJobID(ctx, job.ID)
			require.NoError(t, err)
			require.Len(t, timings, 1, "Expected one timing entry to be created")
			require.Equal(t, "init", string(timings[0].Stage), "Timing stage should match what was sent")
		})
	})

	t.Run("WorkspaceBuild_BadFormType", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		jobID := uuid.New()
		user := dbgen.User(t, db, database.User{})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID,
			JobID:          jobID,
		})
		template := dbgen.Template(t, db, database.Template{
			CreatedBy:       user.ID,
			OrganizationID:  pd.OrganizationID,
			ActiveVersionID: tv.ID,
		})
		err := db.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
			ID: tv.ID,
			TemplateID: uuid.NullUUID{
				UUID:  template.ID,
				Valid: true,
			},
			UpdatedAt: dbtime.Now(),
			Name:      tv.Name,
			Message:   tv.Message,
		})
		require.NoError(t, err)
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: pd.OrganizationID,
			TemplateID:     template.ID,
		})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             jobID,
			Provisioner:    database.ProvisionerTypeEcho,
			Input:          json.RawMessage("{}"),
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: pd.OrganizationID,
			Tags:           pd.Tags,
		})
		require.NoError(t, err)
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: tv.ID,
			InitiatorID:       user.ID,
			JobID:             jobID,
		})
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			OrganizationID: pd.OrganizationID,
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
			ProvisionerTags: must(json.Marshal(job.Tags)),
			StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
		})
		require.NoError(t, err)

		_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
			JobId: job.ID.String(),
			Type: &proto.CompletedJob_TemplateImport_{
				TemplateImport: &proto.CompletedJob_TemplateImport{
					StartResources: []*sdkproto.Resource{{
						Name: "hello",
						Type: "aws_instance",
					}},
					StopResources: []*sdkproto.Resource{},
					RichParameters: []*sdkproto.RichParameter{
						{
							Name:     "parameter",
							Type:     "string",
							FormType: -1,
						},
					},
					Plan: []byte("{}"),
				},
			},
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "unsupported form type")
	})

	t.Run("TemplateImport_MissingGitAuth", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		jobID := uuid.New()
		versionID := uuid.New()
		user := dbgen.User(t, db, database.User{})
		err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			CreatedBy:      user.ID,
			ID:             versionID,
			JobID:          jobID,
			OrganizationID: pd.OrganizationID,
		})
		require.NoError(t, err)
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:          jobID,
			Provisioner: database.ProvisionerTypeEcho,
			Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
				TemplateVersionID: versionID,
			})),
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			OrganizationID: pd.OrganizationID,
			Tags:           pd.Tags,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			OrganizationID: pd.OrganizationID,
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			StartedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			ProvisionerTags: must(json.Marshal(job.Tags)),
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
						StopResources: []*sdkproto.Resource{},
						ExternalAuthProviders: []*sdkproto.ExternalAuthProviderResource{{
							Id: "github",
						}},
						Plan: []byte("{}"),
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
		user := dbgen.User(t, db, database.User{})
		err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID:             versionID,
			CreatedBy:      user.ID,
			JobID:          jobID,
			OrganizationID: pd.OrganizationID,
		})
		require.NoError(t, err)
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			OrganizationID: pd.OrganizationID,
			ID:             jobID,
			Provisioner:    database.ProvisionerTypeEcho,
			Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
				TemplateVersionID: versionID,
			})),
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeTemplateVersionImport,
			Tags:          pd.Tags,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			OrganizationID: pd.OrganizationID,
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			StartedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			ProvisionerTags: must(json.Marshal(job.Tags)),
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
						ExternalAuthProviders: []*sdkproto.ExternalAuthProviderResource{{Id: "github"}},
						Plan:                  []byte("{}"),
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
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				// Simulate the given time starting from now.
				require.False(t, c.now.IsZero())
				clock := quartz.NewMock(t)
				clock.Set(c.now)
				tss := &atomic.Pointer[schedule.TemplateScheduleStore]{}
				uqhss := &atomic.Pointer[schedule.UserQuietHoursScheduleStore]{}
				auditor := audit.NewMock()
				srv, db, ps, pd := setup(t, false, &overrides{
					clock:                       clock,
					templateScheduleStore:       tss,
					userQuietHoursScheduleStore: uqhss,
					auditor:                     auditor,
				})

				var templateScheduleStore schedule.TemplateScheduleStore = schedule.MockTemplateScheduleStore{
					GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
						return schedule.TemplateScheduleOptions{
							UserAutostartEnabled: false,
							UserAutostopEnabled:  true,
							DefaultTTL:           0,
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
					CreatedBy:      user.ID,
					Name:           "template",
					Provisioner:    database.ProvisionerTypeEcho,
					OrganizationID: pd.OrganizationID,
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
				workspaceTable := dbgen.Workspace(t, db, database.WorkspaceTable{
					TemplateID:     template.ID,
					Ttl:            workspaceTTL,
					OwnerID:        user.ID,
					OrganizationID: pd.OrganizationID,
				})
				version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
					CreatedBy:      user.ID,
					OrganizationID: pd.OrganizationID,
					TemplateID: uuid.NullUUID{
						UUID:  template.ID,
						Valid: true,
					},
					JobID: uuid.New(),
				})
				buildID := uuid.New()
				job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
					FileID:      file.ID,
					InitiatorID: user.ID,
					Type:        database.ProvisionerJobTypeWorkspaceBuild,
					Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID: buildID,
					})),
					OrganizationID: pd.OrganizationID,
				})
				build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID:                buildID,
					JobID:             job.ID,
					WorkspaceID:       workspaceTable.ID,
					InitiatorID:       user.ID,
					TemplateVersionID: version.ID,
					Transition:        c.transition,
					Reason:            database.BuildReasonInitiator,
				})
				_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					OrganizationID: pd.OrganizationID,
					WorkerID: uuid.NullUUID{
						UUID:  pd.ID,
						Valid: true,
					},
					Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
					StartedAt: sql.NullTime{
						Time:  c.now,
						Valid: true,
					},
					ProvisionerTags: must(json.Marshal(job.Tags)),
				})
				require.NoError(t, err)

				publishedWorkspace := make(chan struct{})
				closeWorkspaceSubscribe, err := ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspaceTable.OwnerID),
					wspubsub.HandleWorkspaceEvent(
						func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
							if err != nil {
								return
							}
							if e.Kind == wspubsub.WorkspaceEventKindStateChange && e.WorkspaceID == workspaceTable.ID {
								close(publishedWorkspace)
							}
						}))
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

				workspace, err := db.GetWorkspaceByID(ctx, workspaceTable.ID)
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

				require.Len(t, auditor.AuditLogs(), 1)
				var additionalFields audit.AdditionalFields
				err = json.Unmarshal(auditor.AuditLogs()[0].AdditionalFields, &additionalFields)
				require.NoError(t, err)
				require.Equal(t, workspace.ID, additionalFields.WorkspaceID)
			})
		}
	})
	t.Run("TemplateDryRun", func(t *testing.T) {
		t.Parallel()
		srv, db, _, pd := setup(t, false, &overrides{})
		user := dbgen.User(t, db, database.User{})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID,
			JobID:          uuid.New(),
		})
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:            version.JobID,
			Provisioner:   database.ProvisionerTypeEcho,
			Type:          database.ProvisionerJobTypeTemplateVersionDryRun,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Input: must(json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
				TemplateVersionID: version.ID,
			})),
			OrganizationID: pd.OrganizationID,
			Tags:           pd.Tags,
		})
		require.NoError(t, err)
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			StartedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			OrganizationID:  pd.OrganizationID,
			ProvisionerTags: must(json.Marshal(job.Tags)),
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

	t.Run("Modules", func(t *testing.T) {
		t.Parallel()

		templateVersionID := uuid.New()
		workspaceBuildID := uuid.New()

		cases := []struct {
			name                 string
			job                  *proto.CompletedJob
			expectedResources    []database.WorkspaceResource
			expectedModules      []database.WorkspaceModule
			provisionerJobParams database.InsertProvisionerJobParams
		}{
			{
				name: "TemplateDryRun",
				job: &proto.CompletedJob{
					Type: &proto.CompletedJob_TemplateDryRun_{
						TemplateDryRun: &proto.CompletedJob_TemplateDryRun{
							Resources: []*sdkproto.Resource{{
								Name:       "something",
								Type:       "aws_instance",
								ModulePath: "module.test1",
							}, {
								Name:       "something2",
								Type:       "aws_instance",
								ModulePath: "",
							}},
							Modules: []*sdkproto.Module{
								{
									Key:     "test1",
									Version: "1.0.0",
									Source:  "github.com/example/example",
								},
							},
						},
					},
				},
				expectedResources: []database.WorkspaceResource{{
					Name: "something",
					Type: "aws_instance",
					ModulePath: sql.NullString{
						String: "module.test1",
						Valid:  true,
					},
					Transition: database.WorkspaceTransitionStart,
				}, {
					Name: "something2",
					Type: "aws_instance",
					ModulePath: sql.NullString{
						String: "",
						Valid:  true,
					},
					Transition: database.WorkspaceTransitionStart,
				}},
				expectedModules: []database.WorkspaceModule{{
					Key:        "test1",
					Version:    "1.0.0",
					Source:     "github.com/example/example",
					Transition: database.WorkspaceTransitionStart,
				}},
				provisionerJobParams: database.InsertProvisionerJobParams{
					Type: database.ProvisionerJobTypeTemplateVersionDryRun,
					Input: must(json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
						TemplateVersionID: templateVersionID,
					})),
				},
			},
			{
				name: "TemplateImport",
				job: &proto.CompletedJob{
					Type: &proto.CompletedJob_TemplateImport_{
						TemplateImport: &proto.CompletedJob_TemplateImport{
							StartResources: []*sdkproto.Resource{{
								Name:       "something",
								Type:       "aws_instance",
								ModulePath: "module.test1",
							}},
							StartModules: []*sdkproto.Module{
								{
									Key:     "test1",
									Version: "1.0.0",
									Source:  "github.com/example/example",
								},
							},
							StopResources: []*sdkproto.Resource{{
								Name:       "something2",
								Type:       "aws_instance",
								ModulePath: "module.test2",
							}},
							StopModules: []*sdkproto.Module{
								{
									Key:     "test2",
									Version: "2.0.0",
									Source:  "github.com/example2/example",
								},
							},
							Plan: []byte("{}"),
						},
					},
				},
				provisionerJobParams: database.InsertProvisionerJobParams{
					Type: database.ProvisionerJobTypeTemplateVersionImport,
					Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
						TemplateVersionID: templateVersionID,
					})),
				},
				expectedResources: []database.WorkspaceResource{{
					Name: "something",
					Type: "aws_instance",
					ModulePath: sql.NullString{
						String: "module.test1",
						Valid:  true,
					},
					Transition: database.WorkspaceTransitionStart,
				}, {
					Name: "something2",
					Type: "aws_instance",
					ModulePath: sql.NullString{
						String: "module.test2",
						Valid:  true,
					},
					Transition: database.WorkspaceTransitionStop,
				}},
				expectedModules: []database.WorkspaceModule{{
					Key:        "test1",
					Version:    "1.0.0",
					Source:     "github.com/example/example",
					Transition: database.WorkspaceTransitionStart,
				}, {
					Key:        "test2",
					Version:    "2.0.0",
					Source:     "github.com/example2/example",
					Transition: database.WorkspaceTransitionStop,
				}},
			},
			{
				name: "WorkspaceBuild",
				job: &proto.CompletedJob{
					Type: &proto.CompletedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
							Resources: []*sdkproto.Resource{{
								Name:       "something",
								Type:       "aws_instance",
								ModulePath: "module.test1",
							}, {
								Name:       "something2",
								Type:       "aws_instance",
								ModulePath: "",
							}},
							Modules: []*sdkproto.Module{
								{
									Key:     "test1",
									Version: "1.0.0",
									Source:  "github.com/example/example",
								},
							},
						},
					},
				},
				expectedResources: []database.WorkspaceResource{{
					Name: "something",
					Type: "aws_instance",
					ModulePath: sql.NullString{
						String: "module.test1",
						Valid:  true,
					},
					Transition: database.WorkspaceTransitionStart,
				}, {
					Name: "something2",
					Type: "aws_instance",
					ModulePath: sql.NullString{
						String: "",
						Valid:  true,
					},
					Transition: database.WorkspaceTransitionStart,
				}},
				expectedModules: []database.WorkspaceModule{{
					Key:        "test1",
					Version:    "1.0.0",
					Source:     "github.com/example/example",
					Transition: database.WorkspaceTransitionStart,
				}},
				provisionerJobParams: database.InsertProvisionerJobParams{
					Type: database.ProvisionerJobTypeWorkspaceBuild,
					Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID: workspaceBuildID,
					})),
				},
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				srv, db, _, pd := setup(t, false, &overrides{})
				jobParams := c.provisionerJobParams
				if jobParams.ID == uuid.Nil {
					jobParams.ID = uuid.New()
				}
				if jobParams.Provisioner == "" {
					jobParams.Provisioner = database.ProvisionerTypeEcho
				}
				if jobParams.StorageMethod == "" {
					jobParams.StorageMethod = database.ProvisionerStorageMethodFile
				}
				if jobParams.Tags == nil {
					jobParams.Tags = pd.Tags
				}
				if jobParams.OrganizationID == uuid.Nil {
					jobParams.OrganizationID = pd.OrganizationID
				}
				user := dbgen.User(t, db, database.User{})
				job, err := db.InsertProvisionerJob(ctx, jobParams)
				require.NoError(t, err)

				tpl := dbgen.Template(t, db, database.Template{
					CreatedBy:      user.ID,
					OrganizationID: pd.OrganizationID,
				})
				tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
					ID:             templateVersionID,
					CreatedBy:      user.ID,
					OrganizationID: pd.OrganizationID,
					TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
					JobID:          job.ID,
				})

				if jobParams.Type == database.ProvisionerJobTypeWorkspaceBuild {
					workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
						TemplateID:     tpl.ID,
						OrganizationID: pd.OrganizationID,
						OwnerID:        user.ID,
					})
					_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
						ID:                workspaceBuildID,
						JobID:             job.ID,
						WorkspaceID:       workspace.ID,
						TemplateVersionID: tv.ID,
					})
				}

				require.NoError(t, err)
				_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					WorkerID: uuid.NullUUID{
						UUID:  pd.ID,
						Valid: true,
					},
					OrganizationID:  pd.OrganizationID,
					Types:           []database.ProvisionerType{jobParams.Provisioner},
					ProvisionerTags: must(json.Marshal(job.Tags)),
					StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
				})
				require.NoError(t, err)

				completedJob := c.job
				completedJob.JobId = job.ID.String()

				_, err = srv.CompleteJob(ctx, completedJob)
				require.NoError(t, err)

				resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
				require.NoError(t, err)
				require.Len(t, resources, len(c.expectedResources))

				for _, expectedResource := range c.expectedResources {
					for i, resource := range resources {
						if resource.Name == expectedResource.Name &&
							resource.Type == expectedResource.Type &&
							resource.ModulePath == expectedResource.ModulePath &&
							resource.Transition == expectedResource.Transition {
							resources[i] = database.WorkspaceResource{Name: "matched"}
						}
					}
				}
				// all resources should be matched
				for _, resource := range resources {
					require.Equal(t, "matched", resource.Name)
				}

				modules, err := db.GetWorkspaceModulesByJobID(ctx, job.ID)
				require.NoError(t, err)
				require.Len(t, modules, len(c.expectedModules))

				for _, expectedModule := range c.expectedModules {
					for i, module := range modules {
						if module.Key == expectedModule.Key &&
							module.Version == expectedModule.Version &&
							module.Source == expectedModule.Source &&
							module.Transition == expectedModule.Transition {
							modules[i] = database.WorkspaceModule{Key: "matched"}
						}
					}
				}
				for _, module := range modules {
					require.Equal(t, "matched", module.Key)
				}
			})
		}
	})

	t.Run("ReinitializePrebuiltAgents", func(t *testing.T) {
		t.Parallel()
		type testcase struct {
			name                    string
			shouldReinitializeAgent bool
		}

		for _, tc := range []testcase{
			// Whether or not there are presets and those presets define prebuilds, etc
			// are all irrelevant at this level. Those factors are useful earlier in the process.
			// Everything relevant to this test is determined by the value of `PrebuildClaimedByUser`
			// on the provisioner job. As such, there are only two significant test cases:
			{
				name:                    "claimed prebuild",
				shouldReinitializeAgent: true,
			},
			{
				name:                    "not a claimed prebuild",
				shouldReinitializeAgent: false,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// GIVEN an enqueued provisioner job and its dependencies:

				srv, db, ps, pd := setup(t, false, &overrides{})

				buildID := uuid.New()
				jobInput := provisionerdserver.WorkspaceProvisionJob{
					WorkspaceBuildID: buildID,
				}
				if tc.shouldReinitializeAgent { // This is the key lever in the test
					// GIVEN the enqueued provisioner job is for a workspace being claimed by a user:
					jobInput.PrebuiltWorkspaceBuildStage = sdkproto.PrebuiltWorkspaceBuildStage_CLAIM
				}
				input, err := json.Marshal(jobInput)
				require.NoError(t, err)

				ctx := testutil.Context(t, testutil.WaitShort)
				job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
					ID:             uuid.New(),
					CreatedAt:      dbtime.Now(),
					UpdatedAt:      dbtime.Now(),
					OrganizationID: pd.OrganizationID,
					InitiatorID:    uuid.New(),
					Input:          input,
					Provisioner:    database.ProvisionerTypeEcho,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					Tags:           pd.Tags,
				})
				require.NoError(t, err)

				user := dbgen.User(t, db, database.User{})
				tpl := dbgen.Template(t, db, database.Template{
					OrganizationID: pd.OrganizationID,
					CreatedBy:      user.ID,
				})
				tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
					OrganizationID: pd.OrganizationID,
					TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
					JobID:          job.ID,
					CreatedBy:      user.ID,
				})
				workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
					TemplateID:     tpl.ID,
					OrganizationID: pd.OrganizationID,
					OwnerID:        user.ID,
				})
				_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID:                buildID,
					JobID:             job.ID,
					WorkspaceID:       workspace.ID,
					TemplateVersionID: tv.ID,
				})
				_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					OrganizationID: pd.OrganizationID,
					WorkerID: uuid.NullUUID{
						UUID:  pd.ID,
						Valid: true,
					},
					Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
					ProvisionerTags: must(json.Marshal(job.Tags)),
					StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
				})
				require.NoError(t, err)

				// GIVEN something is listening to process workspace reinitialization:
				reinitChan := make(chan agentsdk.ReinitializationEvent, 1) // Buffered to simplify test structure
				cancel, err := agplprebuilds.NewPubsubWorkspaceClaimListener(ps, testutil.Logger(t)).ListenForWorkspaceClaims(ctx, workspace.ID, reinitChan)
				require.NoError(t, err)
				defer cancel()

				// WHEN the job is completed
				completedJob := proto.CompletedJob{
					JobId: job.ID.String(),
					Type: &proto.CompletedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{},
					},
				}
				_, err = srv.CompleteJob(ctx, &completedJob)
				require.NoError(t, err)

				if tc.shouldReinitializeAgent {
					event := testutil.RequireReceive(ctx, t, reinitChan)
					require.Equal(t, workspace.ID, event.WorkspaceID)
				} else {
					select {
					case <-reinitChan:
						t.Fatal("unexpected reinitialization event published")
					default:
						// OK
					}
				}
			})
		}
	})

	t.Run("PrebuiltWorkspaceClaimWithResourceReplacements", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a mock prebuild orchestrator which stores calls to TrackResourceReplacement.
		done := make(chan struct{})
		orchestrator := &mockPrebuildsOrchestrator{
			ReconciliationOrchestrator: agplprebuilds.DefaultReconciler,
			done:                       done,
		}
		srv, db, ps, pd := setup(t, false, &overrides{
			prebuildsOrchestrator: orchestrator,
		})

		// Given: a workspace build which simulates claiming a prebuild.
		user := dbgen.User(t, db, database.User{})
		template := dbgen.Template(t, db, database.Template{
			CreatedBy:      user.ID,
			Name:           "template",
			Provisioner:    database.ProvisionerTypeEcho,
			OrganizationID: pd.OrganizationID,
		})
		file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
		workspaceTable := dbgen.Workspace(t, db, database.WorkspaceTable{
			TemplateID:     template.ID,
			OwnerID:        user.ID,
			OrganizationID: pd.OrganizationID,
		})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID,
			TemplateID: uuid.NullUUID{
				UUID:  template.ID,
				Valid: true,
			},
			JobID: uuid.New(),
		})
		buildID := uuid.New()
		job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
			FileID:      file.ID,
			InitiatorID: user.ID,
			Type:        database.ProvisionerJobTypeWorkspaceBuild,
			Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
				WorkspaceBuildID:            buildID,
				PrebuiltWorkspaceBuildStage: sdkproto.PrebuiltWorkspaceBuildStage_CLAIM,
			})),
			OrganizationID: pd.OrganizationID,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			ID:                buildID,
			JobID:             job.ID,
			WorkspaceID:       workspaceTable.ID,
			InitiatorID:       user.ID,
			TemplateVersionID: version.ID,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
		})
		_, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			OrganizationID: pd.OrganizationID,
			WorkerID: uuid.NullUUID{
				UUID:  pd.ID,
				Valid: true,
			},
			Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
			ProvisionerTags: must(json.Marshal(job.Tags)),
			StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
		})
		require.NoError(t, err)

		// When: a replacement is encountered.
		replacements := []*sdkproto.ResourceReplacement{
			{
				Resource: "docker_container[0]",
				Paths:    []string{"env"},
			},
		}

		// Then: CompleteJob makes a call to TrackResourceReplacement.
		_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
			JobId: job.ID.String(),
			Type: &proto.CompletedJob_WorkspaceBuild_{
				WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
					State:                []byte{},
					ResourceReplacements: replacements,
				},
			},
		})
		require.NoError(t, err)

		// Then: the replacements are as we expected.
		testutil.RequireReceive(ctx, t, done)
		require.Equal(t, replacements, orchestrator.replacements)
	})

	t.Run("AITasks", func(t *testing.T) {
		t.Parallel()

		// has_ai_task has a default value of nil, but once the template import completes it will have a value;
		// it is set to "true" if the template has any coder_ai_task resources defined.
		t.Run("TemplateImport", func(t *testing.T) {
			type testcase struct {
				name     string
				input    *proto.CompletedJob_TemplateImport
				expected bool
			}

			for _, tc := range []testcase{
				{
					name: "has_ai_task is false by default",
					input: &proto.CompletedJob_TemplateImport{
						// HasAiTasks is not set.
						Plan: []byte("{}"),
					},
					expected: false,
				},
				{
					name: "has_ai_task gets set to true",
					input: &proto.CompletedJob_TemplateImport{
						HasAiTasks: true,
						Plan:       []byte("{}"),
					},
					expected: true,
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					fakeUsageInserter, usageInserterPtr := newFakeUsageInserter()
					srv, db, _, pd := setup(t, false, &overrides{
						usageInserter: usageInserterPtr,
					})

					importJobID := uuid.New()
					tvID := uuid.New()
					templateAdminUser := dbgen.User(t, db, database.User{RBACRoles: []string{codersdk.RoleTemplateAdmin}})
					template := dbgen.Template(t, db, database.Template{
						Name:           "template",
						CreatedBy:      templateAdminUser.ID,
						Provisioner:    database.ProvisionerTypeEcho,
						OrganizationID: pd.OrganizationID,
					})
					version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
						ID:             tvID,
						CreatedBy:      templateAdminUser.ID,
						OrganizationID: pd.OrganizationID,
						TemplateID: uuid.NullUUID{
							UUID:  template.ID,
							Valid: true,
						},
						JobID: importJobID,
					})
					_ = version

					ctx := testutil.Context(t, testutil.WaitShort)
					job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
						ID:             importJobID,
						CreatedAt:      dbtime.Now(),
						UpdatedAt:      dbtime.Now(),
						OrganizationID: pd.OrganizationID,
						InitiatorID:    uuid.New(),
						Input: must(json.Marshal(provisionerdserver.TemplateVersionImportJob{
							TemplateVersionID: tvID,
						})),
						Provisioner:   database.ProvisionerTypeEcho,
						StorageMethod: database.ProvisionerStorageMethodFile,
						Type:          database.ProvisionerJobTypeTemplateVersionImport,
						Tags:          pd.Tags,
					})
					require.NoError(t, err)

					_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
						OrganizationID: pd.OrganizationID,
						WorkerID: uuid.NullUUID{
							UUID:  pd.ID,
							Valid: true,
						},
						Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
						ProvisionerTags: must(json.Marshal(job.Tags)),
						StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
					})
					require.NoError(t, err)

					version, err = db.GetTemplateVersionByID(ctx, tvID)
					require.NoError(t, err)
					require.False(t, version.HasAITask.Valid) // Value should be nil (i.e. valid = false).

					completedJob := proto.CompletedJob{
						JobId: job.ID.String(),
						Type: &proto.CompletedJob_TemplateImport_{
							TemplateImport: tc.input,
						},
					}
					_, err = srv.CompleteJob(ctx, &completedJob)
					require.NoError(t, err)

					version, err = db.GetTemplateVersionByID(ctx, tvID)
					require.NoError(t, err)
					require.True(t, version.HasAITask.Valid) // We ALWAYS expect a value to be set, therefore not nil, i.e. valid = true.
					require.Equal(t, tc.expected, version.HasAITask.Bool)

					// We never expect a usage event to be collected for
					// template imports.
					require.Empty(t, fakeUsageInserter.collectedEvents)
				})
			}
		})

		// has_ai_task has a default value of nil, but once the workspace build completes it will have a value;
		// it is set to "true" if the related template has any coder_ai_task resources defined, and its sidebar app ID
		// will be set as well in that case.
		// HACK(johnstcn): we also set it to "true" if any _previous_ workspace builds ever had it set to "true".
		// This is to avoid tasks "disappearing" when you stop them.
		t.Run("WorkspaceBuild", func(t *testing.T) {
			type testcase struct {
				name             string
				seedFunc         func(context.Context, testing.TB, database.Store) error // If you need to insert other resources
				transition       database.WorkspaceTransition
				input            *proto.CompletedJob_WorkspaceBuild
				isTask           bool
				expectTaskStatus database.TaskStatus
				expectAppID      uuid.NullUUID
				expectHasAiTask  bool
				expectUsageEvent bool
			}

			sidebarAppID := uuid.New()
			for _, tc := range []testcase{
				{
					name:       "has_ai_task is false if task_id is nil",
					transition: database.WorkspaceTransitionStart,
					input:      &proto.CompletedJob_WorkspaceBuild{
						// No AiTasks defined.
					},
					isTask:           false,
					expectHasAiTask:  false,
					expectUsageEvent: false,
				},
				{
					name:       "has_ai_task is false even if there are coder_ai_task resources, but no task_id",
					transition: database.WorkspaceTransitionStart,
					input: &proto.CompletedJob_WorkspaceBuild{
						AiTasks: []*sdkproto.AITask{
							{
								Id:    uuid.NewString(),
								AppId: sidebarAppID.String(),
							},
						},
						Resources: []*sdkproto.Resource{
							{
								Agents: []*sdkproto.Agent{
									{
										Id:   uuid.NewString(),
										Name: "a",
										Apps: []*sdkproto.App{
											{
												Id:   sidebarAppID.String(),
												Slug: "test-app",
											},
										},
									},
								},
							},
						},
					},
					isTask:           false,
					expectHasAiTask:  false,
					expectUsageEvent: false,
				},
				{
					name:       "has_ai_task is set to true",
					transition: database.WorkspaceTransitionStart,
					input: &proto.CompletedJob_WorkspaceBuild{
						AiTasks: []*sdkproto.AITask{
							{
								Id:    uuid.NewString(),
								AppId: sidebarAppID.String(),
							},
						},
						Resources: []*sdkproto.Resource{
							{
								Agents: []*sdkproto.Agent{
									{
										Id:   uuid.NewString(),
										Name: "a",
										Apps: []*sdkproto.App{
											{
												Id:   sidebarAppID.String(),
												Slug: "test-app",
											},
										},
									},
								},
							},
						},
					},
					isTask:           true,
					expectTaskStatus: database.TaskStatusInitializing,
					expectAppID:      uuid.NullUUID{UUID: sidebarAppID, Valid: true},
					expectHasAiTask:  true,
					expectUsageEvent: true,
				},
				{
					name:       "has_ai_task is set to true, with sidebar app id",
					transition: database.WorkspaceTransitionStart,
					input: &proto.CompletedJob_WorkspaceBuild{
						AiTasks: []*sdkproto.AITask{
							{
								Id: uuid.NewString(),
								SidebarApp: &sdkproto.AITaskSidebarApp{
									Id: sidebarAppID.String(),
								},
							},
						},
						Resources: []*sdkproto.Resource{
							{
								Agents: []*sdkproto.Agent{
									{
										Id:   uuid.NewString(),
										Name: "a",
										Apps: []*sdkproto.App{
											{
												Id:   sidebarAppID.String(),
												Slug: "test-app",
											},
										},
									},
								},
							},
						},
					},
					isTask:           true,
					expectTaskStatus: database.TaskStatusInitializing,
					expectAppID:      uuid.NullUUID{UUID: sidebarAppID, Valid: true},
					expectHasAiTask:  true,
					expectUsageEvent: true,
				},
				// Checks regression for https://github.com/coder/coder/issues/18776
				{
					name:       "non-existing app",
					transition: database.WorkspaceTransitionStart,
					input: &proto.CompletedJob_WorkspaceBuild{
						AiTasks: []*sdkproto.AITask{
							{
								Id: uuid.NewString(),
								// Non-existing app ID would previously trigger a FK violation.
								// Now it will trigger a warning instead in the provisioner logs.
								AppId: sidebarAppID.String(),
							},
						},
					},
					isTask:           true,
					expectTaskStatus: database.TaskStatusInitializing,
					// You can still "sort of" use a task in this state, but as we don't have
					// the correct app ID you won't be able to communicate with it via Coder.
					expectHasAiTask:  true,
					expectUsageEvent: true,
				},
				{
					name:       "has_ai_task is set to true, but transition is not start",
					transition: database.WorkspaceTransitionStop,
					input: &proto.CompletedJob_WorkspaceBuild{
						AiTasks: []*sdkproto.AITask{
							{
								Id:    uuid.NewString(),
								AppId: sidebarAppID.String(),
							},
						},
						Resources: []*sdkproto.Resource{
							{
								Agents: []*sdkproto.Agent{
									{
										Id:   uuid.NewString(),
										Name: "a",
										Apps: []*sdkproto.App{
											{
												Id:   sidebarAppID.String(),
												Slug: "test-app",
											},
										},
									},
								},
							},
						},
					},
					isTask:           true,
					expectTaskStatus: database.TaskStatusPaused,
					expectAppID:      uuid.NullUUID{UUID: sidebarAppID, Valid: true},
					expectHasAiTask:  true,
					expectUsageEvent: false,
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					fakeUsageInserter, usageInserterPtr := newFakeUsageInserter()
					srv, db, _, pd := setup(t, false, &overrides{
						usageInserter: usageInserterPtr,
					})

					importJobID := uuid.New()
					tvID := uuid.New()
					templateUser := dbgen.User(t, db, database.User{RBACRoles: []string{codersdk.RoleTemplateAdmin}})
					template := dbgen.Template(t, db, database.Template{
						Name:           "template",
						CreatedBy:      templateUser.ID,
						Provisioner:    database.ProvisionerTypeEcho,
						OrganizationID: pd.OrganizationID,
					})
					version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
						ID:             tvID,
						CreatedBy:      templateUser.ID,
						OrganizationID: pd.OrganizationID,
						TemplateID: uuid.NullUUID{
							UUID:  template.ID,
							Valid: true,
						},
						JobID: importJobID,
					})
					user := dbgen.User(t, db, database.User{})
					workspaceTable := dbgen.Workspace(t, db, database.WorkspaceTable{
						TemplateID:     template.ID,
						OwnerID:        user.ID,
						OrganizationID: pd.OrganizationID,
					})
					var genTask database.Task
					if tc.isTask {
						genTask = dbgen.Task(t, db, database.TaskTable{
							OwnerID:           user.ID,
							OrganizationID:    pd.OrganizationID,
							WorkspaceID:       uuid.NullUUID{UUID: workspaceTable.ID, Valid: true},
							TemplateVersionID: version.ID,
						})
					}

					ctx := testutil.Context(t, testutil.WaitShort)
					if tc.seedFunc != nil {
						require.NoError(t, tc.seedFunc(ctx, t, db))
					}

					buildJobID := uuid.New()
					wsBuildID := uuid.New()
					job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
						ID:             buildJobID,
						CreatedAt:      dbtime.Now(),
						UpdatedAt:      dbtime.Now(),
						OrganizationID: pd.OrganizationID,
						InitiatorID:    user.ID,
						Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
							WorkspaceBuildID: wsBuildID,
							LogLevel:         "DEBUG",
						})),
						Provisioner:   database.ProvisionerTypeEcho,
						StorageMethod: database.ProvisionerStorageMethodFile,
						Type:          database.ProvisionerJobTypeWorkspaceBuild,
						Tags:          pd.Tags,
					})
					require.NoError(t, err)
					var buildNum int32
					if latestBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceTable.ID); err == nil {
						buildNum = latestBuild.BuildNumber
					}
					build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
						ID:                wsBuildID,
						BuildNumber:       buildNum + 1,
						JobID:             buildJobID,
						WorkspaceID:       workspaceTable.ID,
						TemplateVersionID: version.ID,
						InitiatorID:       user.ID,
						Transition:        tc.transition,
					})

					_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
						OrganizationID: pd.OrganizationID,
						WorkerID: uuid.NullUUID{
							UUID:  pd.ID,
							Valid: true,
						},
						Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
						ProvisionerTags: must(json.Marshal(job.Tags)),
						StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
					})
					require.NoError(t, err)

					build, err = db.GetWorkspaceBuildByID(ctx, build.ID)
					require.NoError(t, err)
					require.False(t, build.HasAITask.Valid) // Value should be nil (i.e. valid = false).

					completedJob := proto.CompletedJob{
						JobId: job.ID.String(),
						Type: &proto.CompletedJob_WorkspaceBuild_{
							WorkspaceBuild: tc.input,
						},
					}
					_, err = srv.CompleteJob(ctx, &completedJob)
					require.NoError(t, err)

					build, err = db.GetWorkspaceBuildByID(ctx, build.ID)
					require.NoError(t, err)
					require.True(t, build.HasAITask.Valid) // We ALWAYS expect a value to be set, therefore not nil, i.e. valid = true.
					require.Equal(t, tc.expectHasAiTask, build.HasAITask.Bool)

					task, err := db.GetTaskByID(ctx, genTask.ID)
					if tc.isTask {
						require.NoError(t, err)
						require.Equal(t, tc.expectTaskStatus, task.Status)
					} else {
						require.Error(t, err)
					}

					require.Equal(t, tc.expectAppID, task.WorkspaceAppID)

					if tc.expectUsageEvent {
						// Check that a usage event was collected.
						require.Len(t, fakeUsageInserter.collectedEvents, 1)
						require.Equal(t, usagetypes.DCManagedAgentsV1{
							Count: 1,
						}, fakeUsageInserter.collectedEvents[0])
					} else {
						// Check that no usage event was collected.
						require.Empty(t, fakeUsageInserter.collectedEvents)
					}
				})
			}
		})
	})
}

type mockPrebuildsOrchestrator struct {
	agplprebuilds.ReconciliationOrchestrator

	replacements []*sdkproto.ResourceReplacement
	done         chan struct{}
}

func (m *mockPrebuildsOrchestrator) TrackResourceReplacement(_ context.Context, _, _ uuid.UUID, replacements []*sdkproto.ResourceReplacement) {
	m.replacements = replacements
	m.done <- struct{}{}
}

func TestInsertWorkspacePresetsAndParameters(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		givenPresets []*sdkproto.Preset
	}

	testCases := []testCase{
		{
			name: "no presets",
		},
		{
			name: "one preset with no parameters",
			givenPresets: []*sdkproto.Preset{
				{
					Name: "preset1",
				},
			},
		},
		{
			name: "one preset, no parameters, requesting prebuilds",
			givenPresets: []*sdkproto.Preset{
				{
					Name: "preset1",
					Prebuild: &sdkproto.Prebuild{
						Instances: 1,
					},
				},
			},
		},
		{
			name: "one preset with multiple parameters, requesting 0 prebuilds",
			givenPresets: []*sdkproto.Preset{
				{
					Name: "preset1",
					Parameters: []*sdkproto.PresetParameter{
						{
							Name:  "param1",
							Value: "value1",
						},
					},
					Prebuild: &sdkproto.Prebuild{
						Instances: 0,
					},
				},
			},
		},
		{
			name: "one preset with multiple parameters",
			givenPresets: []*sdkproto.Preset{
				{
					Name: "preset1",
					Parameters: []*sdkproto.PresetParameter{
						{
							Name:  "param1",
							Value: "value1",
						},
						{
							Name:  "param2",
							Value: "value2",
						},
					},
				},
			},
		},
		{
			name: "one preset, multiple parameters, requesting prebuilds",
			givenPresets: []*sdkproto.Preset{
				{
					Name: "preset1",
					Parameters: []*sdkproto.PresetParameter{
						{
							Name:  "param1",
							Value: "value1",
						},
						{
							Name:  "param2",
							Value: "value2",
						},
					},
					Prebuild: &sdkproto.Prebuild{
						Instances: 1,
					},
				},
			},
		},
		{
			name: "multiple presets with parameters",
			givenPresets: []*sdkproto.Preset{
				{
					Name: "preset1",
					Parameters: []*sdkproto.PresetParameter{
						{
							Name:  "param1",
							Value: "value1",
						},
						{
							Name:  "param2",
							Value: "value2",
						},
					},
					Prebuild: &sdkproto.Prebuild{
						Instances: 1,
					},
				},
				{
					Name: "preset2",
					Parameters: []*sdkproto.PresetParameter{
						{
							Name:  "param3",
							Value: "value3",
						},
						{
							Name:  "param4",
							Value: "value4",
						},
					},
				},
			},
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			logger := testutil.Logger(t)
			db, ps := dbtestutil.NewDB(t)
			org := dbgen.Organization(t, db, database.Organization{})
			user := dbgen.User(t, db, database.User{})

			job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				OrganizationID: org.ID,
			})
			templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				JobID:          job.ID,
				OrganizationID: org.ID,
				CreatedBy:      user.ID,
			})

			err := provisionerdserver.InsertWorkspacePresetsAndParameters(
				ctx,
				logger,
				db,
				job.ID,
				templateVersion.ID,
				c.givenPresets,
				time.Now(),
			)
			require.NoError(t, err)

			gotPresets, err := db.GetPresetsByTemplateVersionID(ctx, templateVersion.ID)
			require.NoError(t, err)
			require.Len(t, gotPresets, len(c.givenPresets))

			for _, givenPreset := range c.givenPresets {
				var foundPreset *database.TemplateVersionPreset
				for _, gotPreset := range gotPresets {
					if givenPreset.Name == gotPreset.Name {
						foundPreset = &gotPreset
						break
					}
				}
				require.NotNil(t, foundPreset, "preset %s not found in parameters", givenPreset.Name)

				gotPresetParameters, err := db.GetPresetParametersByPresetID(ctx, foundPreset.ID)
				require.NoError(t, err)
				require.Len(t, gotPresetParameters, len(givenPreset.Parameters))

				for _, givenParameter := range givenPreset.Parameters {
					foundMatch := false
					for _, gotParameter := range gotPresetParameters {
						nameMatches := givenParameter.Name == gotParameter.Name
						valueMatches := givenParameter.Value == gotParameter.Value
						if nameMatches && valueMatches {
							foundMatch = true
							break
						}
					}
					require.True(t, foundMatch, "preset parameter %s not found in parameters", givenParameter.Name)
				}
				if givenPreset.Prebuild == nil {
					require.False(t, foundPreset.DesiredInstances.Valid)
				}
				if givenPreset.Prebuild != nil {
					require.True(t, foundPreset.DesiredInstances.Valid)
					require.Equal(t, givenPreset.Prebuild.Instances, foundPreset.DesiredInstances.Int32)
				}
			}
		})
	}
}

func TestInsertWorkspaceResource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	insert := func(db database.Store, jobID uuid.UUID, resource *sdkproto.Resource) error {
		return provisionerdserver.InsertWorkspaceResource(ctx, db, jobID, database.WorkspaceTransitionStart, resource, &telemetry.Snapshot{})
	}
	t.Run("NoAgents", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
		})
		require.NoError(t, err)
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
		require.NoError(t, err)
		require.Len(t, resources, 1)
	})
	t.Run("InvalidAgentToken", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
				Auth: &sdkproto.Agent_Token{
					Token: "bananas",
				},
			}},
		})
		require.ErrorContains(t, err, "invalid UUID length")
	})
	t.Run("DuplicateApps", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
				Apps: []*sdkproto.App{{
					Slug: "a",
				}, {
					Slug: "a",
				}},
			}},
		})
		require.ErrorContains(t, err, `duplicate app slug, must be unique per template: "a"`)

		db, _ = dbtestutil.NewDB(t)
		job = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err = insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev1",
				Apps: []*sdkproto.App{{
					Slug: "a",
				}},
			}, {
				Name: "dev2",
				Apps: []*sdkproto.App{{
					Slug: "a",
				}},
			}},
		})
		require.ErrorContains(t, err, `duplicate app slug, must be unique per template: "a"`)
	})
	t.Run("AppSlugInvalid", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
				Apps: []*sdkproto.App{{
					Slug: "dev_1",
				}},
			}},
		})
		require.ErrorContains(t, err, `app slug "dev_1" does not match regex`)
		err = insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
				Apps: []*sdkproto.App{{
					Slug: "dev--1",
				}},
			}},
		})
		require.ErrorContains(t, err, `app slug "dev--1" does not match regex`)
		err = insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
				Apps: []*sdkproto.App{{
					Slug: "Dev",
				}},
			}},
		})
		require.ErrorContains(t, err, `app slug "Dev" does not match regex`)
	})
	t.Run("DuplicateAgentNames", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		// case-insensitive-unique
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
			}, {
				Name: "Dev",
			}},
		})
		require.ErrorContains(t, err, "duplicate agent name")
		err = insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
			}, {
				Name: "dev",
			}},
		})
		require.ErrorContains(t, err, "duplicate agent name")
	})
	t.Run("AgentNameInvalid", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "Dev",
			}},
		})
		require.NoError(t, err) // uppercase is still allowed
		err = insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev_1",
			}},
		})
		require.ErrorContains(t, err, `agent name "dev_1" contains underscores`) // custom error for underscores
		err = insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev--1",
			}},
		})
		require.ErrorContains(t, err, `agent name "dev--1" does not match regex`)
	})
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
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
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		require.EqualValues(t, 10, resources[0].DailyCost)
		agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, []uuid.UUID{resources[0].ID})
		require.NoError(t, err)
		require.Len(t, agents, 1)
		agent := agents[0]
		require.Equal(t, uuid.NullUUID{}, agent.ParentID)
		require.Equal(t, "amd64", agent.Architecture)
		require.Equal(t, "linux", agent.OperatingSystem)
		want, err := json.Marshal(map[string]string{
			"something": "test",
			"else":      "I laugh in the face of danger.",
		})
		require.NoError(t, err)
		got, err := json.Marshal(agent.EnvironmentVariables.RawMessage)
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
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
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
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
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
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name:        "dev",
				DisplayApps: &sdkproto.DisplayApps{},
			}},
		})
		require.NoError(t, err)
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
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

	t.Run("ResourcesMonitoring", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name:        "dev",
				DisplayApps: &sdkproto.DisplayApps{},
				ResourcesMonitoring: &sdkproto.ResourcesMonitoring{
					Memory: &sdkproto.MemoryResourceMonitor{
						Enabled:   true,
						Threshold: 80,
					},
					Volumes: []*sdkproto.VolumeResourceMonitor{
						{
							Path:      "/volume1",
							Enabled:   true,
							Threshold: 90,
						},
						{
							Path:      "/volume2",
							Enabled:   true,
							Threshold: 50,
						},
					},
				},
			}},
		})
		require.NoError(t, err)
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, []uuid.UUID{resources[0].ID})
		require.NoError(t, err)
		require.Len(t, agents, 1)

		agent := agents[0]
		memMonitor, err := db.FetchMemoryResourceMonitorsByAgentID(ctx, agent.ID)
		require.NoError(t, err)
		volMonitors, err := db.FetchVolumesResourceMonitorsByAgentID(ctx, agent.ID)
		require.NoError(t, err)

		require.Equal(t, int32(80), memMonitor.Threshold)
		require.Len(t, volMonitors, 2)
		require.Equal(t, int32(90), volMonitors[0].Threshold)
		require.Equal(t, "/volume1", volMonitors[0].Path)
		require.Equal(t, int32(50), volMonitors[1].Threshold)
		require.Equal(t, "/volume2", volMonitors[1].Path)
	})

	t.Run("Devcontainers", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		err := insert(db, job.ID, &sdkproto.Resource{
			Name: "something",
			Type: "aws_instance",
			Agents: []*sdkproto.Agent{{
				Name: "dev",
				Devcontainers: []*sdkproto.Devcontainer{
					{Name: "foo", WorkspaceFolder: "/workspace1"},
					{Name: "bar", WorkspaceFolder: "/workspace2", ConfigPath: "/workspace2/.devcontainer/devcontainer.json"},
				},
			}},
		})
		require.NoError(t, err)
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, []uuid.UUID{resources[0].ID})
		require.NoError(t, err)
		require.Len(t, agents, 1)
		agent := agents[0]
		devcontainers, err := db.GetWorkspaceAgentDevcontainersByAgentID(ctx, agent.ID)
		sort.Slice(devcontainers, func(i, j int) bool {
			return devcontainers[i].Name > devcontainers[j].Name
		})
		require.NoError(t, err)
		require.Len(t, devcontainers, 2)
		require.Equal(t, "foo", devcontainers[0].Name)
		require.Equal(t, "/workspace1", devcontainers[0].WorkspaceFolder)
		require.Equal(t, "", devcontainers[0].ConfigPath)
		require.Equal(t, "bar", devcontainers[1].Name)
		require.Equal(t, "/workspace2", devcontainers[1].WorkspaceFolder)
		require.Equal(t, "/workspace2/.devcontainer/devcontainer.json", devcontainers[1].ConfigPath)
	})
}

func TestNotifications(t *testing.T) {
	t.Parallel()

	t.Run("Workspace deletion", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name               string
			deletionReason     database.BuildReason
			shouldNotify       bool
			shouldSelfInitiate bool
		}{
			{
				name:           "initiated by autodelete",
				deletionReason: database.BuildReasonAutodelete,
				shouldNotify:   true,
			},
			{
				name:               "initiated by self",
				deletionReason:     database.BuildReasonInitiator,
				shouldNotify:       false,
				shouldSelfInitiate: true,
			},
			{
				name:               "initiated by someone else",
				deletionReason:     database.BuildReasonInitiator,
				shouldNotify:       true,
				shouldSelfInitiate: false,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := context.Background()
				notifEnq := &notificationstest.FakeEnqueuer{}

				srv, db, ps, pd := setup(t, false, &overrides{
					notificationEnqueuer: notifEnq,
				})

				user := dbgen.User(t, db, database.User{})
				initiator := user
				if !tc.shouldSelfInitiate {
					initiator = dbgen.User(t, db, database.User{})
				}

				template := dbgen.Template(t, db, database.Template{
					CreatedBy:      user.ID,
					Name:           "template",
					Provisioner:    database.ProvisionerTypeEcho,
					OrganizationID: pd.OrganizationID,
				})
				template, err := db.GetTemplateByID(ctx, template.ID)
				require.NoError(t, err)
				file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
				workspaceTable := dbgen.Workspace(t, db, database.WorkspaceTable{
					TemplateID:     template.ID,
					OwnerID:        user.ID,
					OrganizationID: pd.OrganizationID,
				})
				version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
					CreatedBy:      user.ID,
					OrganizationID: pd.OrganizationID,
					TemplateID: uuid.NullUUID{
						UUID:  template.ID,
						Valid: true,
					},
					JobID: uuid.New(),
				})
				wsBuildID := uuid.New()
				job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
					FileID:      file.ID,
					InitiatorID: initiator.ID,
					Type:        database.ProvisionerJobTypeWorkspaceBuild,
					Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID: wsBuildID,
					})),
					OrganizationID: pd.OrganizationID,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				})
				_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID:                wsBuildID,
					JobID:             job.ID,
					WorkspaceID:       workspaceTable.ID,
					TemplateVersionID: version.ID,
					InitiatorID:       initiator.ID,
					Transition:        database.WorkspaceTransitionDelete,
					Reason:            tc.deletionReason,
				})
				_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					OrganizationID: pd.OrganizationID,
					WorkerID: uuid.NullUUID{
						UUID:  pd.ID,
						Valid: true,
					},
					Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
					ProvisionerTags: must(json.Marshal(job.Tags)),
					StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
				})
				require.NoError(t, err)

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

				workspace, err := db.GetWorkspaceByID(ctx, workspaceTable.ID)
				require.NoError(t, err)
				require.True(t, workspace.Deleted)

				if tc.shouldNotify {
					// Validate that the notification was sent and contained the expected values.
					sent := notifEnq.Sent()
					require.Len(t, sent, 1)
					require.Equal(t, sent[0].UserID, user.ID)
					require.Contains(t, sent[0].Targets, template.ID)
					require.Contains(t, sent[0].Targets, workspace.ID)
					require.Contains(t, sent[0].Targets, workspace.OrganizationID)
					require.Contains(t, sent[0].Targets, user.ID)
					if tc.deletionReason == database.BuildReasonInitiator {
						require.Equal(t, initiator.Username, sent[0].Labels["initiator"])
					}
				} else {
					require.Len(t, notifEnq.Sent(), 0)
				}
			})
		}
	})

	t.Run("Workspace build failed", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string

			buildReason  database.BuildReason
			shouldNotify bool
		}{
			{
				name:         "initiated by owner",
				buildReason:  database.BuildReasonInitiator,
				shouldNotify: false,
			},
			{
				name:         "initiated by autostart",
				buildReason:  database.BuildReasonAutostart,
				shouldNotify: true,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := context.Background()
				notifEnq := &notificationstest.FakeEnqueuer{}

				//	Otherwise `(*Server).FailJob` fails with:
				// audit log - get build {"error": "sql: no rows in result set"}
				ignoreLogErrors := true
				srv, db, ps, pd := setup(t, ignoreLogErrors, &overrides{
					notificationEnqueuer: notifEnq,
				})

				user := dbgen.User(t, db, database.User{})
				initiator := user

				template := dbgen.Template(t, db, database.Template{
					CreatedBy:      user.ID,
					Name:           "template",
					Provisioner:    database.ProvisionerTypeEcho,
					OrganizationID: pd.OrganizationID,
				})
				file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
				workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
					TemplateID:     template.ID,
					OwnerID:        user.ID,
					OrganizationID: pd.OrganizationID,
				})
				version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
					CreatedBy:      user.ID,
					OrganizationID: pd.OrganizationID,
					TemplateID: uuid.NullUUID{
						UUID:  template.ID,
						Valid: true,
					},
					JobID: uuid.New(),
				})
				wsBuildID := uuid.New()
				job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
					ID:          uuid.New(),
					FileID:      file.ID,
					InitiatorID: initiator.ID,
					Type:        database.ProvisionerJobTypeWorkspaceBuild,
					Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
						WorkspaceBuildID: wsBuildID,
					})),
					OrganizationID: pd.OrganizationID,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				})
				_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					ID:                wsBuildID,
					JobID:             job.ID,
					WorkspaceID:       workspace.ID,
					TemplateVersionID: version.ID,
					InitiatorID:       initiator.ID,
					Transition:        database.WorkspaceTransitionDelete,
					Reason:            tc.buildReason,
				})
				_, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					OrganizationID: pd.OrganizationID,
					WorkerID: uuid.NullUUID{
						UUID:  pd.ID,
						Valid: true,
					},
					Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
					ProvisionerTags: must(json.Marshal(job.Tags)),
					StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
				})
				require.NoError(t, err)

				_, err = srv.FailJob(ctx, &proto.FailedJob{
					JobId: job.ID.String(),
					Type: &proto.FailedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{
							State: []byte{},
						},
					},
				})
				require.NoError(t, err)

				if tc.shouldNotify {
					// Validate that the notification was sent and contained the expected values.
					sent := notifEnq.Sent()
					require.Len(t, sent, 1)
					require.Equal(t, sent[0].UserID, user.ID)
					require.Contains(t, sent[0].Targets, template.ID)
					require.Contains(t, sent[0].Targets, workspace.ID)
					require.Contains(t, sent[0].Targets, workspace.OrganizationID)
					require.Contains(t, sent[0].Targets, user.ID)
					require.Equal(t, string(tc.buildReason), sent[0].Labels["reason"])
				} else {
					require.Len(t, notifEnq.Sent(), 0)
				}
			})
		}
	})

	t.Run("Manual build failed, template admins notified", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		// given
		notifEnq := &notificationstest.FakeEnqueuer{}
		srv, db, ps, pd := setup(t, true /* ignoreLogErrors */, &overrides{notificationEnqueuer: notifEnq})

		templateAdmin := dbgen.User(t, db, database.User{RBACRoles: []string{codersdk.RoleTemplateAdmin}})
		_ /* other template admin, should not receive notification */ = dbgen.User(t, db, database.User{RBACRoles: []string{codersdk.RoleTemplateAdmin}})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: templateAdmin.ID, OrganizationID: pd.OrganizationID})
		user := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: pd.OrganizationID})

		template := dbgen.Template(t, db, database.Template{
			CreatedBy: user.ID,
			Name:      "template", DisplayName: "William's Template", Provisioner: database.ProvisionerTypeEcho, OrganizationID: pd.OrganizationID,
		})
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			TemplateID: template.ID, OwnerID: user.ID, OrganizationID: pd.OrganizationID,
		})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			OrganizationID: pd.OrganizationID, TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}, JobID: uuid.New(),
		})
		wsBuildID := uuid.New()
		job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
			FileID:         dbgen.File(t, db, database.File{CreatedBy: user.ID}).ID,
			InitiatorID:    user.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{WorkspaceBuildID: wsBuildID})),
			OrganizationID: pd.OrganizationID,
		})
		build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			ID:          wsBuildID,
			JobID:       job.ID,
			WorkspaceID: workspace.ID, TemplateVersionID: version.ID, InitiatorID: user.ID, Transition: database.WorkspaceTransitionDelete, Reason: database.BuildReasonInitiator,
		})
		_, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			OrganizationID:  pd.OrganizationID,
			WorkerID:        uuid.NullUUID{UUID: pd.ID, Valid: true},
			Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
			ProvisionerTags: must(json.Marshal(job.Tags)),
			StartedAt:       sql.NullTime{Time: job.CreatedAt, Valid: true},
		})
		require.NoError(t, err)

		// when
		_, err = srv.FailJob(ctx, &proto.FailedJob{
			JobId: job.ID.String(), Type: &proto.FailedJob_WorkspaceBuild_{WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{State: []byte{}}},
		})
		require.NoError(t, err)

		// then
		sent := notifEnq.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, sent[0].UserID, templateAdmin.ID)
		assert.Equal(t, sent[0].TemplateID, notifications.TemplateWorkspaceManualBuildFailed)
		assert.Contains(t, sent[0].Targets, template.ID)
		assert.Contains(t, sent[0].Targets, workspace.ID)
		assert.Contains(t, sent[0].Targets, workspace.OrganizationID)
		assert.Contains(t, sent[0].Targets, user.ID)
		assert.Equal(t, workspace.Name, sent[0].Labels["name"])
		assert.Equal(t, template.DisplayName, sent[0].Labels["template_name"])
		assert.Equal(t, version.Name, sent[0].Labels["template_version_name"])
		assert.Equal(t, user.Username, sent[0].Labels["initiator"])
		assert.Equal(t, user.Username, sent[0].Labels["workspace_owner_username"])
		assert.Equal(t, strconv.Itoa(int(build.BuildNumber)), sent[0].Labels["workspace_build_number"])
	})
}

func TestServer_ExpirePrebuildsSessionToken(t *testing.T) {
	t.Parallel()

	// Given: a prebuilt workspace where an API key was previously created for the prebuilds user.
	var (
		ctx             = testutil.Context(t, testutil.WaitShort)
		srv, db, ps, pd = setup(t, false, nil)
		user            = dbgen.User(t, db, database.User{})
		template        = dbgen.Template(t, db, database.Template{
			OrganizationID: pd.OrganizationID,
			CreatedBy:      user.ID,
		})
		version = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
			OrganizationID: pd.OrganizationID,
			CreatedBy:      user.ID,
		})
		workspace = dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: pd.OrganizationID,
			TemplateID:     template.ID,
			OwnerID:        database.PrebuildsSystemUserID,
		})
		workspaceBuildID = uuid.New()
		buildJob         = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
			OrganizationID: pd.OrganizationID,
			FileID:         dbgen.File(t, db, database.File{CreatedBy: user.ID}).ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input: must(json.Marshal(provisionerdserver.WorkspaceProvisionJob{
				WorkspaceBuildID: workspaceBuildID,
			})),
			InitiatorID: database.PrebuildsSystemUserID,
			Tags:        pd.Tags,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			ID:                workspaceBuildID,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: version.ID,
			JobID:             buildJob.ID,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       database.PrebuildsSystemUserID,
		})
		existingKey, _ = dbgen.APIKey(t, db, database.APIKey{
			UserID:    database.PrebuildsSystemUserID,
			TokenName: provisionerdserver.WorkspaceSessionTokenName(database.PrebuildsSystemUserID, workspace.ID),
		})
	)

	// When: the prebuild claim job is acquired
	fs := newFakeStream(ctx)
	err := srv.AcquireJobWithCancel(fs)
	require.NoError(t, err)
	job, err := fs.waitForJob()
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NotNil(t, job.Type, "acquired job type was nil?!")
	workspaceBuildJob := job.Type.(*proto.AcquiredJob_WorkspaceBuild_).WorkspaceBuild
	require.NotNil(t, workspaceBuildJob.Metadata)

	// Assert test invariant: we acquired the expected build job
	require.Equal(t, workspaceBuildID.String(), workspaceBuildJob.WorkspaceBuildId)
	// Then: The session token should be deleted
	_, err = db.GetAPIKeyByID(ctx, existingKey.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "api key for prebuilds user should be deleted")
}

type overrides struct {
	ctx                         context.Context
	deploymentValues            *codersdk.DeploymentValues
	externalAuthConfigs         []*externalauth.Config
	templateScheduleStore       *atomic.Pointer[schedule.TemplateScheduleStore]
	userQuietHoursScheduleStore *atomic.Pointer[schedule.UserQuietHoursScheduleStore]
	usageInserter               *atomic.Pointer[usage.Inserter]
	clock                       *quartz.Mock
	acquireJobLongPollDuration  time.Duration
	heartbeatFn                 func(ctx context.Context) error
	heartbeatInterval           time.Duration
	auditor                     audit.Auditor
	notificationEnqueuer        notifications.Enqueuer
	prebuildsOrchestrator       agplprebuilds.ReconciliationOrchestrator
}

func setup(t *testing.T, ignoreLogErrors bool, ov *overrides) (proto.DRPCProvisionerDaemonServer, database.Store, pubsub.Pubsub, database.ProvisionerDaemon) {
	t.Helper()
	logger := testutil.Logger(t)
	db, ps := dbtestutil.NewDB(t)
	defOrg, err := db.GetDefaultOrganization(context.Background())
	require.NoError(t, err, "default org not found")

	deploymentValues := coderdtest.DeploymentValues(t)
	var externalAuthConfigs []*externalauth.Config
	tss := testTemplateScheduleStore()
	uqhss := testUserQuietHoursScheduleStore()
	usageInserter := testUsageInserter()
	clock := quartz.NewReal()
	pollDur := time.Duration(0)
	if ov == nil {
		ov = &overrides{}
	}
	if ov.ctx == nil {
		ctx, cancel := context.WithCancel(dbauthz.AsProvisionerd(context.Background()))
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
	if ov.usageInserter != nil {
		tUsageInserter := usageInserter.Load()
		// keep the initial test value if the override hasn't set the atomic pointer.
		usageInserter = ov.usageInserter
		if usageInserter.Load() == nil {
			swapped := usageInserter.CompareAndSwap(nil, tUsageInserter)
			require.True(t, swapped)
		}
	}
	if ov.clock != nil {
		clock = ov.clock
	}
	auditPtr := &atomic.Pointer[audit.Auditor]{}
	var auditor audit.Auditor = audit.NewMock()
	if ov.auditor != nil {
		auditor = ov.auditor
	}
	auditPtr.Store(&auditor)
	pollDur = ov.acquireJobLongPollDuration
	var notifEnq notifications.Enqueuer
	if ov.notificationEnqueuer != nil {
		notifEnq = ov.notificationEnqueuer
	} else {
		notifEnq = notifications.NewNoopEnqueuer()
	}

	daemon, err := db.UpsertProvisionerDaemon(ov.ctx, database.UpsertProvisionerDaemonParams{
		Name:           "test",
		CreatedAt:      dbtime.Now(),
		Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
		Tags:           database.StringMap{},
		LastSeenAt:     sql.NullTime{},
		Version:        buildinfo.Version(),
		APIVersion:     proto.CurrentVersion.String(),
		OrganizationID: defOrg.ID,
		KeyID:          codersdk.ProvisionerKeyUUIDBuiltIn,
	})
	require.NoError(t, err)

	prebuildsOrchestrator := ov.prebuildsOrchestrator
	if prebuildsOrchestrator == nil {
		prebuildsOrchestrator = agplprebuilds.DefaultReconciler
	}
	var op atomic.Pointer[agplprebuilds.ReconciliationOrchestrator]
	op.Store(&prebuildsOrchestrator)

	// Use an authz wrapped database for the server to ensure permission checks
	// work.
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	serverDB := dbauthz.New(db, authorizer, logger, coderdtest.AccessControlStorePointer())
	srv, err := provisionerdserver.NewServer(
		ov.ctx,
		proto.CurrentVersion.String(),
		&url.URL{},
		daemon.ID,
		defOrg.ID,
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: ignoreLogErrors}),
		[]database.ProvisionerType{database.ProvisionerTypeEcho},
		provisionerdserver.Tags(daemon.Tags),
		serverDB,
		ps,
		provisionerdserver.NewAcquirer(ov.ctx, logger.Named("acquirer"), db, ps),
		telemetry.NewNoop(),
		trace.NewNoopTracerProvider().Tracer("noop"),
		&atomic.Pointer[proto.QuotaCommitter]{},
		auditPtr,
		tss,
		uqhss,
		usageInserter,
		deploymentValues,
		provisionerdserver.Options{
			ExternalAuthConfigs:   externalAuthConfigs,
			Clock:                 clock,
			OIDCConfig:            &oauth2.Config{},
			AcquireJobLongPollDur: pollDur,
			HeartbeatInterval:     ov.heartbeatInterval,
			HeartbeatFn:           ov.heartbeatFn,
		},
		notifEnq,
		&op,
		provisionerdserver.NewMetrics(logger),
		coderd.ReadExperiments(logger, deploymentValues.Experiments),
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

type fakeUsageInserter struct {
	collectedEvents []usagetypes.Event
}

var _ usage.Inserter = &fakeUsageInserter{}

func newFakeUsageInserter() (*fakeUsageInserter, *atomic.Pointer[usage.Inserter]) {
	poitr := &atomic.Pointer[usage.Inserter]{}
	fake := &fakeUsageInserter{}
	var inserter usage.Inserter = fake
	poitr.Store(&inserter)
	return fake, poitr
}

func (f *fakeUsageInserter) InsertDiscreteUsageEvent(_ context.Context, _ database.Store, event usagetypes.DiscreteEvent) error {
	f.collectedEvents = append(f.collectedEvents, event)
	return nil
}
