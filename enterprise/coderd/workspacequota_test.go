package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func verifyQuota(ctx context.Context, t *testing.T, client *codersdk.Client, organizationID string, consumed, total int) {
	t.Helper()

	got, err := client.WorkspaceQuota(ctx, organizationID, codersdk.Me)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceQuota{
		Budget:          total,
		CreditsConsumed: consumed,
	}, got)

	// Remove this check when the deprecated endpoint is removed.
	// This just makes sure the deprecated endpoint is still working
	// as intended. It will only work for the default organization.
	deprecatedGot, err := deprecatedQuotaEndpoint(ctx, client, codersdk.Me)
	require.NoError(t, err, "deprecated endpoint")
	// Only continue to check if the values differ
	if deprecatedGot.Budget != got.Budget || deprecatedGot.CreditsConsumed != got.CreditsConsumed {
		org, err := client.OrganizationByName(ctx, organizationID)
		if err != nil {
			return
		}
		if org.IsDefault {
			require.Equal(t, got, deprecatedGot)
		}
	}
}

func TestWorkspaceQuota(t *testing.T) {
	t.Parallel()

	// This first test verifies the behavior of creating and deleting workspaces.
	// It also tests multi-group quota stacking and the everyone group.
	t.Run("CreateDelete", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		max := 1
		client, _, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			UserWorkspaceQuota: max,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		coderdtest.NewProvisionerDaemon(t, api.AGPL)

		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 0)

		// Patch the 'Everyone' group to verify its quota allowance is being accounted for.
		_, err := client.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(1),
		})
		require.NoError(t, err)
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 1)

		// Add user to two groups, granting them a total budget of 4.
		group1, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "test-1",
			QuotaAllowance: 1,
		})
		require.NoError(t, err)

		group2, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "test-2",
			QuotaAllowance: 2,
		})
		require.NoError(t, err)

		_, err = client.PatchGroup(ctx, group1.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user.UserID.String()},
		})
		require.NoError(t, err)

		_, err = client.PatchGroup(ctx, group2.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user.UserID.String()},
		})
		require.NoError(t, err)

		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 4)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name:      "example",
							Type:      "aws_instance",
							DailyCost: 1,
							Agents: []*proto.Agent{{
								Id:   uuid.NewString(),
								Name: "example",
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Spin up three workspaces fine
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				workspace := coderdtest.CreateWorkspace(t, client, template.ID)
				build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
				assert.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
			}()
		}
		wg.Wait()
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)

		// Next one must fail
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Consumed shouldn't bump
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		require.Contains(t, build.Job.Error, "quota")

		// Delete one random workspace, then quota should recover.
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		for _, w := range workspaces.Workspaces {
			if w.LatestBuild.Status != codersdk.WorkspaceStatusRunning {
				continue
			}
			build, err := client.CreateWorkspaceBuild(ctx, w.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionDelete,
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
			verifyQuota(ctx, t, client, user.OrganizationID.String(), 3, 4)
			break
		}

		// Next one should now succeed
		workspace = coderdtest.CreateWorkspace(t, client, template.ID)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
	})

	t.Run("StartStop", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		max := 1
		client, _, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			UserWorkspaceQuota: max,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		coderdtest.NewProvisionerDaemon(t, api.AGPL)

		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 0)

		// Patch the 'Everyone' group to verify its quota allowance is being accounted for.
		_, err := client.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(4),
		})
		require.NoError(t, err)
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 0, 4)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlanMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_START: planWithCost(2),
				proto.WorkspaceTransition_STOP:  planWithCost(1),
			},
			ProvisionApplyMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_START: applyWithCost(2),
				proto.WorkspaceTransition_STOP:  applyWithCost(1),
			},
		})

		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Spin up two workspaces.
		var wg sync.WaitGroup
		var workspaces []codersdk.Workspace
		for i := 0; i < 2; i++ {
			workspace := coderdtest.CreateWorkspace(t, client, template.ID)
			workspaces = append(workspaces, workspace)
			build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
			assert.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		}
		wg.Wait()
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)

		// Next one must fail
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		require.Contains(t, build.Job.Error, "quota")

		// Consumed shouldn't bump
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)

		build = coderdtest.CreateWorkspaceBuild(t, client, workspaces[0], database.WorkspaceTransitionStop)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Quota goes down one
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 3, 4)
		require.Equal(t, codersdk.WorkspaceStatusStopped, build.Status)

		build = coderdtest.CreateWorkspaceBuild(t, client, workspaces[0], database.WorkspaceTransitionStart)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Quota goes back up
		verifyQuota(ctx, t, client, user.OrganizationID.String(), 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
	})

	// Ensures allowance from everyone groups only counts if you are an org member.
	// This was a bug where the group "Everyone" was being counted for all users,
	// regardless of membership.
	t.Run("AllowanceEveryone", func(t *testing.T) {
		t.Parallel()

		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		member, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

		// Create a second organization
		second := coderdenttest.CreateOrganization(t, owner, coderdenttest.CreateOrganizationOptions{})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// update everyone quotas
		//nolint:gocritic // using owner for simplicity
		_, err := owner.PatchGroup(ctx, first.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(30),
		})
		require.NoError(t, err)

		_, err = owner.PatchGroup(ctx, second.ID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(15),
		})
		require.NoError(t, err)

		verifyQuota(ctx, t, member, first.OrganizationID.String(), 0, 30)

		// Verify org scoped quota limits
		verifyQuota(ctx, t, owner, first.OrganizationID.String(), 0, 30)
		verifyQuota(ctx, t, owner, second.ID.String(), 0, 15)
	})
}

// DB=ci DB_FROM=cikggwjxbths
func TestWorkspaceSerialization(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		panic("We should only run this test with postgres")
	}

	db, ps := dbtestutil.NewDB(t)
	//c := &coderd.Committer{
	//	Log:      slogtest.Make(t, nil),
	//	Database: db,
	//}
	var _ = ps

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tplVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID: uuid.NullUUID{
			UUID:  tpl.ID,
			Valid: true,
		},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})

	seed := database.WorkspaceBuild{
		TemplateVersionID: tplVersion.ID,
	}
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     tpl.ID,
	})

	workspaceResp := dbfake.WorkspaceBuild(t, db, workspace).Seed(seed).Do()

	workspaceTwo := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     tpl.ID,
	})
	workspaceTwoResp := dbfake.WorkspaceBuild(t, db, workspaceTwo).Seed(seed).Do()

	// TX mixing tests. **DO NOT** run these in parallel.
	// The goal here is to mess around with different ordering of
	// transactions and queries.

	// UpdateBuildDeadline bumps a workspace deadline while doing a quota
	// commit.
	// pq: could not serialize access due to concurrent update
	//
	// Note: This passes if the interrupt is run before 'GetQuota()'
	// Passing orders:
	//	- BeginTX -> Bump! -> GetQuota -> GetAllowance -> UpdateCost -> EndTx
	//  - BeginTX -> GetQuota -> GetAllowance -> UpdateCost -> Bump! -> EndTx
	t.Run("UpdateBuildDeadline", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		ctx = dbauthz.AsSystemRestricted(ctx)

		bumpDeadline := func() {
			err := db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
				Deadline:    dbtime.Now(),
				MaxDeadline: dbtime.Now(),
				UpdatedAt:   dbtime.Now(),
				ID:          workspaceResp.Build.ID,
			})
			require.NoError(t, err)
		}

		// Start TX
		// Run order

		quota := newCommitter(t, db, workspace, workspaceResp.Build)
		quota.GetQuota(ctx, t)                         // Step 1
		bumpDeadline()                                 // Interrupt
		quota.GetAllowance(ctx, t)                     // Step 2
		quota.UpdateWorkspaceBuildCostByID(ctx, t, 10) // Step 3
		// End commit
		require.NoError(t, quota.Done())
	})

	t.Run("ReadCost", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		ctx = dbauthz.AsSystemRestricted(ctx)

		readCost := func() {
			_, err := db.GetQuotaConsumedForUser(ctx, database.GetQuotaConsumedForUserParams{
				OwnerID:        workspace.OwnerID,
				OrganizationID: workspace.OrganizationID,
			})
			require.NoError(t, err)
		}

		// Start TX
		// Run order

		quota := newCommitter(t, db, workspace, workspaceResp.Build)
		quota.GetQuota(ctx, t)                         // Step 1
		readCost()                                     // Interrupt
		quota.GetAllowance(ctx, t)                     // Step 2
		quota.UpdateWorkspaceBuildCostByID(ctx, t, 10) // Step 3

		// End commit
		require.NoError(t, quota.Done())
	})

	t.Run("AutoBuild", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		ctx = dbauthz.AsSystemRestricted(ctx)

		auto := newautobuild(t, db, workspace, workspaceResp.Build)
		quota := newCommitter(t, db, workspace, workspaceResp.Build)

		// Run order
		auto.DoAllReads(ctx, t)

		quota.GetQuota(ctx, t) // Step 1
		auto.DoAllReads(ctx, t)

		quota.GetAllowance(ctx, t) // Step 2
		auto.DoAllReads(ctx, t)

		quota.UpdateWorkspaceBuildCostByID(ctx, t, 10) // Step 3
		auto.DoAllReads(ctx, t)
		auto.DoAllWrites(ctx, t)

		// End commit
		require.NoError(t, auto.Done())
		require.NoError(t, quota.Done())
	})

	t.Run("DoubleCommit", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		ctx = dbauthz.AsSystemRestricted(ctx)

		one := newCommitter(t, db, workspace, workspaceResp.Build)
		two := newCommitter(t, db, workspaceTwo, workspaceTwoResp.Build)

		var _, _ = one, two
		// Run order
		two.GetQuota(ctx, t)
		two.GetAllowance(ctx, t)

		one.GetQuota(ctx, t)
		one.GetAllowance(ctx, t)

		one.UpdateWorkspaceBuildCostByID(ctx, t, 10)
		two.UpdateWorkspaceBuildCostByID(ctx, t, 10)

		// End commit
		require.NoError(t, one.Done())
		require.NoError(t, two.Done())
	})
}

func deprecatedQuotaEndpoint(ctx context.Context, client *codersdk.Client, userID string) (codersdk.WorkspaceQuota, error) {
	res, err := client.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspace-quota/%s", userID), nil)
	if err != nil {
		return codersdk.WorkspaceQuota{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.WorkspaceQuota{}, codersdk.ReadBodyAsError(res)
	}
	var quota codersdk.WorkspaceQuota
	return quota, json.NewDecoder(res.Body).Decode(&quota)
}

func planWithCost(cost int32) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Resources: []*proto.Resource{{
					Name:      "example",
					Type:      "aws_instance",
					DailyCost: cost,
				}},
			},
		},
	}}
}

func applyWithCost(cost int32) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Apply{
			Apply: &proto.ApplyComplete{
				Resources: []*proto.Resource{{
					Name:      "example",
					Type:      "aws_instance",
					DailyCost: cost,
				}},
			},
		},
	}}
}

type autobuilder struct {
	DBTx *dbtestutil.DBTx
	w    database.WorkspaceTable
	b    database.WorkspaceBuild
	step int

	// some stuff in mem
	lastestBuild database.WorkspaceBuild
	newJob       database.ProvisionerJob
}

func newautobuild(t *testing.T, db database.Store, workspace database.WorkspaceTable, b database.WorkspaceBuild) *autobuilder {
	quotaTX := dbtestutil.StartTx(t, db, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})
	return &autobuilder{DBTx: quotaTX, w: workspace, b: b}
}

func (c *autobuilder) DoAllWrites(ctx context.Context, t *testing.T) {
	c.InsertProvisionerJob(ctx, t)
	c.InsertWorkspaceBuild(ctx, t)
}

func (c *autobuilder) InsertProvisionerJob(ctx context.Context, t *testing.T) {
	now := dbtime.Now()
	job, err := c.DBTx.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             uuid.New(),
		CreatedAt:      now,
		UpdatedAt:      now,
		OrganizationID: c.w.OrganizationID,
		InitiatorID:    c.w.OwnerID,
		Provisioner:    database.ProvisionerTypeTerraform,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		FileID:         uuid.New(),
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		Input:          []byte("{}"),
		Tags:           nil,
		TraceMetadata:  pqtype.NullRawMessage{},
	})
	require.NoError(t, err)
	c.newJob = job
}

func (c *autobuilder) InsertWorkspaceBuild(ctx context.Context, t *testing.T) {
	now := dbtime.Now()
	err := c.DBTx.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
		ID:                uuid.New(),
		CreatedAt:         now,
		UpdatedAt:         now,
		WorkspaceID:       c.w.ID,
		TemplateVersionID: c.b.TemplateVersionID,
		BuildNumber:       c.lastestBuild.BuildNumber + 1,
		Transition:        database.WorkspaceTransitionStart,
		InitiatorID:       c.w.OwnerID,
		JobID:             c.newJob.ID,
		ProvisionerState:  nil,
		Deadline:          dbtime.Now().Add(time.Hour),
		MaxDeadline:       dbtime.Now().Add(time.Hour),
		Reason:            database.BuildReasonAutostart,
	})
	require.NoError(t, err)
}

func (c *autobuilder) DoAllReads(ctx context.Context, t *testing.T) {
	c.GetWorkspace(ctx, t)
	c.GetUser(ctx, t)
	c.GetLatestWorkspaceBuildByWorkspaceID(ctx, t)
	c.GetProvisionerJobByID(ctx, t)
	c.GetTemplateByID(ctx, t)
}

func (c *autobuilder) Next(ctx context.Context, t *testing.T) {
	list := c.steps()
	list[c.step%len(list)](ctx, t)
	c.step++
}

func (c *autobuilder) steps() []func(context.Context, *testing.T) {
	return []func(context.Context, *testing.T){
		noReturn(c.GetWorkspace),
		noReturn(c.GetUser),
		noReturn(c.GetLatestWorkspaceBuildByWorkspaceID),
		noReturn(c.GetProvisionerJobByID),
		noReturn(c.GetTemplateByID),
	}
}

func (c *autobuilder) GetWorkspace(ctx context.Context, t *testing.T) database.Workspace {
	workspace, err := c.DBTx.GetWorkspaceByID(ctx, c.w.ID)
	require.NoError(t, err)
	return workspace
}

func (c *autobuilder) GetUser(ctx context.Context, t *testing.T) database.User {
	user, err := c.DBTx.GetUserByID(ctx, c.w.OwnerID)
	require.NoError(t, err)
	return user
}

func (c *autobuilder) GetLatestWorkspaceBuildByWorkspaceID(ctx context.Context, t *testing.T) database.WorkspaceBuild {
	build, err := c.DBTx.GetLatestWorkspaceBuildByWorkspaceID(ctx, c.w.ID)
	require.NoError(t, err)
	c.lastestBuild = build
	return build
}

func (c *autobuilder) GetProvisionerJobByID(ctx context.Context, t *testing.T) database.ProvisionerJob {
	job, err := c.DBTx.GetProvisionerJobByID(ctx, c.lastestBuild.JobID)
	require.NoError(t, err)
	return job
}

func (c *autobuilder) GetTemplateByID(ctx context.Context, t *testing.T) database.Template {
	tpl, err := c.DBTx.GetTemplateByID(ctx, c.w.TemplateID)
	require.NoError(t, err)
	return tpl
}

func (c *autobuilder) Done() error {
	return c.DBTx.Done()
}

// committer does what the CommitQuota does, but allows
// stepping through the actions in the tx and controlling the
// timing.
type committer struct {
	DBTx *dbtestutil.DBTx
	w    database.WorkspaceTable
	b    database.WorkspaceBuild
}

func newCommitter(t *testing.T, db database.Store, workspace database.WorkspaceTable, build database.WorkspaceBuild) *committer {
	quotaTX := dbtestutil.StartTx(t, db, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	})
	return &committer{DBTx: quotaTX, w: workspace, b: build}
}

func (c *committer) GetQuota(ctx context.Context, t *testing.T) int64 {
	consumed, err := c.DBTx.GetQuotaConsumedForUser(ctx, database.GetQuotaConsumedForUserParams{
		OwnerID:        c.w.OwnerID,
		OrganizationID: c.w.OrganizationID,
	})
	require.NoError(t, err)
	return consumed
}

func (c *committer) GetAllowance(ctx context.Context, t *testing.T) int64 {
	allowance, err := c.DBTx.GetQuotaAllowanceForUser(ctx, database.GetQuotaAllowanceForUserParams{
		UserID:         c.w.OwnerID,
		OrganizationID: c.w.OrganizationID,
	})
	require.NoError(t, err)
	return allowance
}

func (c *committer) UpdateWorkspaceBuildCostByID(ctx context.Context, t *testing.T, cost int32) {
	err := c.DBTx.UpdateWorkspaceBuildCostByID(ctx, database.UpdateWorkspaceBuildCostByIDParams{
		ID:        c.b.ID,
		DailyCost: cost,
	})
	require.NoError(t, err)
}

func (c *committer) Done() error {
	return c.DBTx.Done()
}

func noReturn[T any](f func(context.Context, *testing.T) T) func(context.Context, *testing.T) {
	return func(ctx context.Context, t *testing.T) {
		f(ctx, t)
	}
}
