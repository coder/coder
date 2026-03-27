package prebuilds_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/files"
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type storeSpy struct {
	database.Store

	claims               *atomic.Int32
	claimParams          *atomic.Pointer[database.ClaimPrebuiltWorkspaceParams]
	claimedWorkspace     *atomic.Pointer[database.ClaimPrebuiltWorkspaceRow]
	insertPrebuildEvents *atomic.Int32
	insertPrebuildInTx   *atomic.Int32
	insertPrebuildEvent  *atomic.Pointer[database.InsertPrebuildEventParams]
	inTx                 bool

	// if claimingErr is not nil - error will be returned when ClaimPrebuiltWorkspace is called
	claimingErr error
	// if insertPrebuildEventErr is not nil - error will be returned when InsertPrebuildEvent is called
	insertPrebuildEventErr error
}

func newStoreSpy(db database.Store, claimingErr, insertPrebuildEventErr error) *storeSpy {
	return &storeSpy{
		Store:                  db,
		claims:                 &atomic.Int32{},
		claimParams:            &atomic.Pointer[database.ClaimPrebuiltWorkspaceParams]{},
		claimedWorkspace:       &atomic.Pointer[database.ClaimPrebuiltWorkspaceRow]{},
		insertPrebuildEvents:   &atomic.Int32{},
		insertPrebuildInTx:     &atomic.Int32{},
		insertPrebuildEvent:    &atomic.Pointer[database.InsertPrebuildEventParams]{},
		claimingErr:            claimingErr,
		insertPrebuildEventErr: insertPrebuildEventErr,
	}
}

func (m *storeSpy) InTx(fn func(store database.Store) error, opts *database.TxOptions) error {
	// Pass spy down into transaction store.
	return m.Store.InTx(func(store database.Store) error {
		spy := newStoreSpy(store, m.claimingErr, m.insertPrebuildEventErr)
		spy.claims = m.claims
		spy.claimParams = m.claimParams
		spy.claimedWorkspace = m.claimedWorkspace
		spy.insertPrebuildEvents = m.insertPrebuildEvents
		spy.insertPrebuildInTx = m.insertPrebuildInTx
		spy.insertPrebuildEvent = m.insertPrebuildEvent
		spy.inTx = true

		return fn(spy)
	}, opts)
}

func (m *storeSpy) ClaimPrebuiltWorkspace(ctx context.Context, arg database.ClaimPrebuiltWorkspaceParams) (database.ClaimPrebuiltWorkspaceRow, error) {
	if m.claimingErr != nil {
		return database.ClaimPrebuiltWorkspaceRow{}, m.claimingErr
	}

	m.claims.Add(1)
	m.claimParams.Store(&arg)
	result, err := m.Store.ClaimPrebuiltWorkspace(ctx, arg)
	if err == nil {
		m.claimedWorkspace.Store(&result)
	}
	return result, err
}

func (m *storeSpy) InsertPrebuildEvent(ctx context.Context, arg database.InsertPrebuildEventParams) error {
	if m.inTx {
		m.insertPrebuildInTx.Add(1)
	} else {
		m.insertPrebuildEvents.Add(1)
	}
	m.insertPrebuildEvent.Store(&arg)
	if m.insertPrebuildEventErr != nil {
		return m.insertPrebuildEventErr
	}
	return m.Store.InsertPrebuildEvent(ctx, arg)
}

func TestClaimPrebuild(t *testing.T) {
	t.Parallel()

	const (
		desiredInstances = 1
		presetCount      = 2
	)

	unexpectedClaimingError := xerrors.New("unexpected claiming error")

	cases := map[string]struct {
		expectPrebuildClaimed  bool
		markPrebuildsClaimable bool
		// if claimingErr is not nil - error will be returned when ClaimPrebuiltWorkspace is called
		claimingErr error
	}{
		"no eligible prebuilds to claim": {
			expectPrebuildClaimed:  false,
			markPrebuildsClaimable: false,
		},
		"claiming an eligible prebuild should succeed": {
			expectPrebuildClaimed:  true,
			markPrebuildsClaimable: true,
		},
		"no claimable prebuilt workspaces error is returned": {
			expectPrebuildClaimed:  false,
			markPrebuildsClaimable: true,
			claimingErr:            agplprebuilds.ErrNoClaimablePrebuiltWorkspaces,
		},
		"AGPL does not support prebuilds error is returned": {
			expectPrebuildClaimed:  false,
			markPrebuildsClaimable: true,
			claimingErr:            agplprebuilds.ErrAGPLDoesNotSupportPrebuiltWorkspaces,
		},
		"unexpected claiming error is returned": {
			expectPrebuildClaimed:  false,
			markPrebuildsClaimable: true,
			claimingErr:            unexpectedClaimingError,
		},
	}

	for name, tc := range cases {
		// Ensure that prebuilt workspaces can be claimed in non-default organizations:
		for _, useDefaultOrg := range []bool{true, false} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				// Setup
				clock := quartz.NewMock(t)
				clock.Set(dbtime.Now())
				ctx := testutil.Context(t, testutil.WaitSuperLong)
				db, pubsub := dbtestutil.NewDB(t)

				spy := newStoreSpy(db, tc.claimingErr, nil)
				expectedPrebuildsCount := desiredInstances * presetCount

				logger := testutil.Logger(t)
				client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
					Options: &coderdtest.Options{
						Database: spy,
						Pubsub:   pubsub,
						Clock:    clock,
					},
					LicenseOptions: &coderdenttest.LicenseOptions{
						Features: license.Features{
							codersdk.FeatureExternalProvisionerDaemons: 1,
						},
					},

					EntitlementsUpdateInterval: time.Second,
				})

				orgID := owner.OrganizationID
				if !useDefaultOrg {
					secondOrg := dbgen.Organization(t, db, database.Organization{})
					orgID = secondOrg.ID
				}

				provisionerCloser := coderdenttest.NewExternalProvisionerDaemon(t, client, orgID, map[string]string{
					provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				})
				defer provisionerCloser.Close()

				cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
				reconciler := prebuilds.NewStoreReconciler(
					spy, pubsub, cache, codersdk.PrebuildsConfig{}, logger,
					quartz.NewMock(t),
					prometheus.NewRegistry(),
					newNoopEnqueuer(),
					newNoopUsageCheckerPtr(),
					noop.NewTracerProvider(),
					10,
					nil,
				)
				var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer()
				api.AGPL.PrebuildsClaimer.Store(&claimer)

				version := coderdtest.CreateTemplateVersion(t, client, orgID, templateWithAgentAndPresetsWithPrebuilds(desiredInstances))
				_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
				coderdtest.CreateTemplate(t, client, orgID, version.ID)
				presets, err := client.TemplateVersionPresets(ctx, version.ID)
				require.NoError(t, err)
				require.Len(t, presets, presetCount)

				userClient, user := coderdtest.CreateAnotherUser(t, client, orgID, rbac.RoleMember())

				// Given: the reconciliation state is snapshot.
				state, err := reconciler.SnapshotState(ctx, spy)
				require.NoError(t, err)
				require.Len(t, state.Presets, presetCount)

				// When: a reconciliation is setup for each preset.
				for _, preset := range presets {
					ps, err := state.FilterByPreset(preset.ID)
					require.NoError(t, err)
					require.NotNil(t, ps)
					actions, err := reconciler.CalculateActions(ctx, *ps)
					require.NoError(t, err)
					require.NotNil(t, actions)

					require.NoError(t, reconciler.ReconcilePreset(ctx, *ps))
				}

				// Given: a set of running, eligible prebuilds eventually starts up.
				runningPrebuilds := make(map[uuid.UUID]database.GetRunningPrebuiltWorkspacesRow, desiredInstances*presetCount)
				require.Eventually(t, func() bool {
					rows, err := spy.GetRunningPrebuiltWorkspaces(ctx)
					if err != nil {
						return false
					}

					for _, row := range rows {
						runningPrebuilds[row.CurrentPresetID.UUID] = row

						if !tc.markPrebuildsClaimable {
							continue
						}

						agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, row.ID)
						if err != nil {
							return false
						}

						// Workspaces are eligible once its agent is marked "ready".
						for _, agent := range agents {
							err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
								ID:             agent.ID,
								LifecycleState: database.WorkspaceAgentLifecycleStateReady,
								StartedAt:      sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
								ReadyAt:        sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
							})
							if err != nil {
								return false
							}
						}
					}

					t.Logf("found %d running prebuilds so far, want %d", len(runningPrebuilds), expectedPrebuildsCount)

					return len(runningPrebuilds) == expectedPrebuildsCount
				}, testutil.WaitSuperLong, testutil.IntervalSlow)

				// When: a user creates a new workspace with a preset for which prebuilds are configured.
				workspaceName := strings.ReplaceAll(testutil.GetRandomName(t), "_", "-")
				params := database.ClaimPrebuiltWorkspaceParams{
					Now:       clock.Now(),
					NewUserID: user.ID,
					NewName:   workspaceName,
					PresetID:  presets[0].ID,
				}
				userWorkspace, err := userClient.CreateUserWorkspace(ctx, user.Username, codersdk.CreateWorkspaceRequest{
					TemplateVersionID:       version.ID,
					Name:                    workspaceName,
					TemplateVersionPresetID: presets[0].ID,
				})

				isNoPrebuiltWorkspaces := errors.Is(tc.claimingErr, agplprebuilds.ErrNoClaimablePrebuiltWorkspaces)
				isUnsupported := errors.Is(tc.claimingErr, agplprebuilds.ErrAGPLDoesNotSupportPrebuiltWorkspaces)

				switch {
				case tc.claimingErr != nil && (isNoPrebuiltWorkspaces || isUnsupported):
					require.NoError(t, err)
					coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, userWorkspace.LatestBuild.ID)

					// Then: the number of running prebuilds hasn't changed because claiming prebuild is failed and we fallback to creating new workspace.
					currentPrebuilds, err := spy.GetRunningPrebuiltWorkspaces(ctx)
					require.NoError(t, err)
					require.Equal(t, expectedPrebuildsCount, len(currentPrebuilds))
					// If there are no prebuilt workspaces to claim, a new workspace is created from scratch
					// and the initiator is set as usual.
					require.Equal(t, user.ID, userWorkspace.LatestBuild.Job.InitiatorID)
					return

				case tc.claimingErr != nil && errors.Is(tc.claimingErr, unexpectedClaimingError):
					// Then: unexpected error happened and was propagated all the way to the caller
					require.Error(t, err)
					require.ErrorContains(t, err, unexpectedClaimingError.Error())

					// Then: the number of running prebuilds hasn't changed because claiming prebuild is failed.
					currentPrebuilds, err := spy.GetRunningPrebuiltWorkspaces(ctx)
					require.NoError(t, err)
					require.Equal(t, expectedPrebuildsCount, len(currentPrebuilds))
					// If a prebuilt workspace claim fails for an unanticipated, erroneous reason,
					// no workspace is created and therefore the initiator is not set.
					require.Equal(t, uuid.Nil, userWorkspace.LatestBuild.Job.InitiatorID)
					return

				default:
					// tc.claimingErr is nil scenario
					require.NoError(t, err)
					build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, userWorkspace.LatestBuild.ID)
					require.Equal(t, build.Job.Status, codersdk.ProvisionerJobSucceeded)
					// Prebuild claims are initiated by the user who requested to create a workspace.
					require.Equal(t, user.ID, userWorkspace.LatestBuild.Job.InitiatorID)
				}

				// at this point we know that tc.claimingErr is nil

				// Then: a prebuild should have been claimed.
				require.EqualValues(t, spy.claims.Load(), 1)
				require.EqualValues(t, *spy.claimParams.Load(), params)

				if !tc.expectPrebuildClaimed {
					require.Nil(t, spy.claimedWorkspace.Load())
					return
				}

				require.NotNil(t, spy.claimedWorkspace.Load())
				claimed := *spy.claimedWorkspace.Load()
				require.NotEqual(t, claimed.ID, uuid.Nil)

				// Then: the claimed prebuild must now be owned by the requester.
				workspace, err := spy.GetWorkspaceByID(ctx, claimed.ID)
				require.NoError(t, err)
				require.Equal(t, user.ID, workspace.OwnerID)

				// Then: the number of running prebuilds has changed since one was claimed.
				currentPrebuilds, err := spy.GetRunningPrebuiltWorkspaces(ctx)
				require.NoError(t, err)
				require.Equal(t, expectedPrebuildsCount-1, len(currentPrebuilds))

				// Then: the claimed prebuild is now missing from the running prebuilds set.
				found := slices.ContainsFunc(currentPrebuilds, func(prebuild database.GetRunningPrebuiltWorkspacesRow) bool {
					return prebuild.ID == claimed.ID
				})
				require.False(t, found, "claimed prebuild should not still be considered a running prebuild")

				// Then: reconciling at this point will provision a new prebuild to replace the claimed one.
				{
					// Given: the reconciliation state is snapshot.
					state, err = reconciler.SnapshotState(ctx, spy)
					require.NoError(t, err)

					// When: a reconciliation is setup for each preset.
					for _, preset := range presets {
						ps, err := state.FilterByPreset(preset.ID)
						require.NoError(t, err)

						// Then: the reconciliation takes place without error.
						require.NoError(t, reconciler.ReconcilePreset(ctx, *ps))
					}
				}

				require.Eventually(t, func() bool {
					rows, err := spy.GetRunningPrebuiltWorkspaces(ctx)
					if err != nil {
						return false
					}

					t.Logf("found %d running prebuilds so far, want %d", len(rows), expectedPrebuildsCount)

					return len(runningPrebuilds) == expectedPrebuildsCount
				}, testutil.WaitSuperLong, testutil.IntervalSlow)

				// Then: when restarting the created workspace (which claimed a prebuild), it should not try and claim a new prebuild.
				// Prebuilds should ONLY be used for net-new workspaces.
				// This is expected by default anyway currently since new workspaces and operations on existing workspaces
				// take different code paths, but it's worth validating.

				spy.claims.Store(0) // Reset counter because we need to check if any new claim requests happen.

				wp, err := userClient.WorkspaceBuildParameters(ctx, userWorkspace.LatestBuild.ID)
				require.NoError(t, err)

				stopBuild, err := userClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
					TemplateVersionID: version.ID,
					Transition:        codersdk.WorkspaceTransitionStop,
				})
				require.NoError(t, err)
				build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, stopBuild.ID)
				require.Equal(t, build.Job.Status, codersdk.ProvisionerJobSucceeded)

				startBuild, err := userClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
					TemplateVersionID:   version.ID,
					Transition:          codersdk.WorkspaceTransitionStart,
					RichParameterValues: wp,
				})
				require.NoError(t, err)
				build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, startBuild.ID)
				require.Equal(t, build.Job.Status, codersdk.ProvisionerJobSucceeded)

				require.Zero(t, spy.claims.Load())
			})
		}
	}
}

func prebuildEventsSince(ctx context.Context, t *testing.T, db database.Store, now time.Time) []database.GetPrebuildEventCountsRow {
	t.Helper()

	// nolint:gocritic // System context is required to read prebuild events.
	rows, err := db.GetPrebuildEventCounts(dbauthz.AsSystemRestricted(ctx), database.GetPrebuildEventCountsParams{
		Since5m:   now.Add(-5 * time.Minute),
		Since10m:  now.Add(-10 * time.Minute),
		Since30m:  now.Add(-30 * time.Minute),
		Since60m:  now.Add(-60 * time.Minute),
		Since120m: now.Add(-120 * time.Minute),
	})
	require.NoError(t, err)
	return rows
}

func TestClaimPrebuildEventRecording(t *testing.T) {
	t.Parallel()

	const desiredInstances = 1

	insertErr := xerrors.New("insert prebuild event failed")

	cases := map[string]struct {
		markPrebuildsClaimable bool
		claimingErr            error
		insertPrebuildEventErr error
		usePresetID            bool
		richParameterValues    []codersdk.WorkspaceBuildParameter
		wantEventType          string
		wantPersistedEventType string
	}{
		"claim success": {
			markPrebuildsClaimable: true,
			usePresetID:            true,
			wantEventType:          string(agplprebuilds.PrebuildEventClaimSucceeded),
			wantPersistedEventType: string(agplprebuilds.PrebuildEventClaimSucceeded),
		},
		"claim miss": {
			markPrebuildsClaimable: true,
			claimingErr:            agplprebuilds.ErrNoClaimablePrebuiltWorkspaces,
			usePresetID:            true,
			wantEventType:          string(agplprebuilds.PrebuildEventClaimMissed),
			wantPersistedEventType: string(agplprebuilds.PrebuildEventClaimMissed),
		},
		"no preset match": {
			markPrebuildsClaimable: true,
			richParameterValues: []codersdk.WorkspaceBuildParameter{{
				Name:  "k1",
				Value: "no-match",
			}},
		},
		"agpl fallback": {
			markPrebuildsClaimable: true,
			claimingErr:            agplprebuilds.ErrAGPLDoesNotSupportPrebuiltWorkspaces,
			usePresetID:            true,
		},
		"event write is best effort": {
			markPrebuildsClaimable: true,
			insertPrebuildEventErr: insertErr,
			usePresetID:            true,
			wantEventType:          string(agplprebuilds.PrebuildEventClaimSucceeded),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			clock := quartz.NewMock(t)
			clock.Set(dbtime.Now())
			ctx := testutil.Context(t, testutil.WaitSuperLong)
			db, pubsub := dbtestutil.NewDB(t)

			spy := newStoreSpy(db, tc.claimingErr, tc.insertPrebuildEventErr)
			logger := testutil.Logger(t)
			client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Database: spy,
					Pubsub:   pubsub,
					Clock:    clock,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureExternalProvisionerDaemons: 1,
					},
				},
				EntitlementsUpdateInterval: time.Second,
			})

			provisionerCloser := coderdenttest.NewExternalProvisionerDaemon(t, client, owner.OrganizationID, map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			})
			defer provisionerCloser.Close()

			cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
			reconciler := prebuilds.NewStoreReconciler(
				spy, pubsub, cache, codersdk.PrebuildsConfig{}, logger,
				quartz.NewMock(t),
				prometheus.NewRegistry(),
				newNoopEnqueuer(),
				newNoopUsageCheckerPtr(),
				noop.NewTracerProvider(),
				10,
				nil,
			)
			var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer()
			api.AGPL.PrebuildsClaimer.Store(&claimer)

			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(desiredInstances))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
			presets, err := client.TemplateVersionPresets(ctx, version.ID)
			require.NoError(t, err)
			require.Len(t, presets, 2)

			userClient, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

			state, err := reconciler.SnapshotState(ctx, spy)
			require.NoError(t, err)
			require.Len(t, state.Presets, len(presets))
			for _, preset := range presets {
				ps, err := state.FilterByPreset(preset.ID)
				require.NoError(t, err)
				require.NotNil(t, ps)
				require.NoError(t, reconciler.ReconcilePreset(ctx, *ps))
			}

			require.Eventually(t, func() bool {
				rows, err := spy.GetRunningPrebuiltWorkspaces(ctx)
				if err != nil {
					return false
				}
				for _, row := range rows {
					if !tc.markPrebuildsClaimable {
						continue
					}
					agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, row.ID)
					if err != nil {
						return false
					}
					for _, agent := range agents {
						err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
							ID:             agent.ID,
							LifecycleState: database.WorkspaceAgentLifecycleStateReady,
							StartedAt:      sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
							ReadyAt:        sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
						})
						if err != nil {
							return false
						}
					}
				}
				return len(rows) == len(presets)*desiredInstances
			}, testutil.WaitSuperLong, testutil.IntervalSlow)

			workspaceName := strings.ReplaceAll(testutil.GetRandomName(t), "_", "-")
			req := codersdk.CreateWorkspaceRequest{
				TemplateVersionID:   version.ID,
				Name:                workspaceName,
				RichParameterValues: tc.richParameterValues,
			}
			if tc.usePresetID {
				req.TemplateVersionPresetID = presets[0].ID
			}

			workspace, err := userClient.CreateUserWorkspace(ctx, user.Username, req)
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, workspace.LatestBuild.ID)

			if tc.wantEventType == "" {
				require.Zero(t, spy.insertPrebuildEvents.Load())
				require.Zero(t, spy.insertPrebuildInTx.Load())
				require.Nil(t, spy.insertPrebuildEvent.Load())
			} else {
				require.EqualValues(t, 1, spy.insertPrebuildEvents.Load())
				require.Zero(t, spy.insertPrebuildInTx.Load())
				inserted := spy.insertPrebuildEvent.Load()
				require.NotNil(t, inserted)
				require.Equal(t, presets[0].ID, inserted.PresetID)
				require.Equal(t, tc.wantEventType, inserted.EventType)
				require.WithinDuration(t, clock.Now(), inserted.CreatedAt, time.Second)
			}

			rows := prebuildEventsSince(ctx, t, db, clock.Now())
			if tc.wantPersistedEventType == "" {
				require.Empty(t, rows)
				return
			}
			require.Len(t, rows, 1)
			require.Equal(t, presets[0].ID, rows[0].PresetID)
			require.Equal(t, tc.wantPersistedEventType, rows[0].EventType)
			require.EqualValues(t, 1, rows[0].Count5m)
		})
	}
}

func templateWithAgentAndPresetsWithPrebuilds(desiredInstances int32) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionGraph: []*proto.Response{
			{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
						Resources: []*proto.Resource{
							{
								Type: "compute",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "smith",
										OperatingSystem: "linux",
										Architecture:    "i386",
									},
								},
							},
						},
						// Make sure immutable params don't break claiming logic
						Parameters: []*proto.RichParameter{
							{
								Name:         "k1",
								Description:  "immutable param",
								Type:         "string",
								DefaultValue: "",
								Required:     false,
								Mutable:      false,
							},
						},
						Presets: []*proto.Preset{
							{
								Name: "preset-a",
								Parameters: []*proto.PresetParameter{
									{
										Name:  "k1",
										Value: "v1",
									},
								},
								Prebuild: &proto.Prebuild{
									Instances: desiredInstances,
								},
							},
							{
								Name: "preset-b",
								Parameters: []*proto.PresetParameter{
									{
										Name:  "k1",
										Value: "v2",
									},
								},
								Prebuild: &proto.Prebuild{
									Instances: desiredInstances,
								},
							},
						},
					},
				},
			},
		},
	}
}
