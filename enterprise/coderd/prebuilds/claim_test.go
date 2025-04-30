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
	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type storeSpy struct {
	database.Store

	claims           *atomic.Int32
	claimParams      *atomic.Pointer[database.ClaimPrebuiltWorkspaceParams]
	claimedWorkspace *atomic.Pointer[database.ClaimPrebuiltWorkspaceRow]

	// if claimingErr is not nil - error will be returned when ClaimPrebuiltWorkspace is called
	claimingErr error
}

func newStoreSpy(db database.Store, claimingErr error) *storeSpy {
	return &storeSpy{
		Store:            db,
		claims:           &atomic.Int32{},
		claimParams:      &atomic.Pointer[database.ClaimPrebuiltWorkspaceParams]{},
		claimedWorkspace: &atomic.Pointer[database.ClaimPrebuiltWorkspaceRow]{},
		claimingErr:      claimingErr,
	}
}

func (m *storeSpy) InTx(fn func(store database.Store) error, opts *database.TxOptions) error {
	// Pass spy down into transaction store.
	return m.Store.InTx(func(store database.Store) error {
		spy := newStoreSpy(store, m.claimingErr)
		spy.claims = m.claims
		spy.claimParams = m.claimParams
		spy.claimedWorkspace = m.claimedWorkspace

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

func TestClaimPrebuild(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

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
		tc := tc

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Setup.
			ctx := testutil.Context(t, testutil.WaitSuperLong)
			db, pubsub := dbtestutil.NewDB(t)

			spy := newStoreSpy(db, tc.claimingErr)
			expectedPrebuildsCount := desiredInstances * presetCount

			logger := testutil.Logger(t)
			client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					IncludeProvisionerDaemon: true,
					Database:                 spy,
					Pubsub:                   pubsub,
				},

				EntitlementsUpdateInterval: time.Second,
			})

			reconciler := prebuilds.NewStoreReconciler(spy, pubsub, codersdk.PrebuildsConfig{}, logger, quartz.NewMock(t), prometheus.NewRegistry())
			var claimer agplprebuilds.Claimer = prebuilds.NewEnterpriseClaimer(spy)
			api.AGPL.PrebuildsClaimer.Store(&claimer)

			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, templateWithAgentAndPresetsWithPrebuilds(desiredInstances))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
			presets, err := client.TemplateVersionPresets(ctx, version.ID)
			require.NoError(t, err)
			require.Len(t, presets, presetCount)

			userClient, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

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
				return

			case tc.claimingErr != nil && errors.Is(tc.claimingErr, unexpectedClaimingError):
				// Then: unexpected error happened and was propagated all the way to the caller
				require.Error(t, err)
				require.ErrorContains(t, err, unexpectedClaimingError.Error())

				// Then: the number of running prebuilds hasn't changed because claiming prebuild is failed.
				currentPrebuilds, err := spy.GetRunningPrebuiltWorkspaces(ctx)
				require.NoError(t, err)
				require.Equal(t, expectedPrebuildsCount, len(currentPrebuilds))
				return

			default:
				// tc.claimingErr is nil scenario
				require.NoError(t, err)
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, userWorkspace.LatestBuild.ID)
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

			stopBuild, err := userClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: version.ID,
				Transition:        codersdk.WorkspaceTransitionStop,
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, stopBuild.ID)

			startBuild, err := userClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: version.ID,
				Transition:        codersdk.WorkspaceTransitionStart,
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, startBuild.ID)

			require.Zero(t, spy.claims.Load())
		})
	}
}

func templateWithAgentAndPresetsWithPrebuilds(desiredInstances int32) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
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
		ProvisionApply: []*proto.Response{
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
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
					},
				},
			},
		},
	}
}
