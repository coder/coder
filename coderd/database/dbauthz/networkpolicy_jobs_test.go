package dbauthz_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/testutil"
)

//nolint:tparallel // The Lists group must finish its parallel readers before history is mutated.
func TestNetworkPolicyJobLists(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	store, _ := dbtestutil.NewDB(t)
	db := dbauthz.New(store, rbac.NewAuthorizer(prometheus.NewRegistry()), testutil.Logger(t), coderdtest.AccessControlStorePointer())
	org := dbgen.Organization(t, store, database.Organization{})
	foreignOrg := dbgen.Organization(t, store, database.Organization{})
	full := dbgen.User(t, store, database.User{})
	partial := dbgen.User(t, store, database.User{})
	foreign := dbgen.User(t, store, database.User{})
	actorContext := func(user database.User, organization uuid.UUID) context.Context {
		roles, err := rolestore.Expand(ctx, store, []rbac.RoleIdentifier{rbac.RoleMember(), rbac.ScopedRoleOrgMember(organization)})
		require.NoError(t, err)
		// Generic metadata access must not substitute for update access to all
		// templates affected by a network policy job.
		roles = append(roles, rbac.Role{
			Identifier: rbac.RoleIdentifier{Name: "job-metadata-reader"},
			Site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceProvisionerJobs.Type:   {policy.ActionRead},
				rbac.ResourceProvisionerDaemon.Type: {policy.ActionRead},
			}),
		})
		return dbauthz.As(ctx, rbac.Subject{ID: user.ID.String(), Roles: roles, Scope: rbac.ScopeAll})
	}
	fullCtx, partialCtx, foreignCtx := actorContext(full, org.ID), actorContext(partial, org.ID), actorContext(foreign, foreignOrg.ID)
	templateA := dbgen.Template(t, store, database.Template{OrganizationID: org.ID, CreatedBy: full.ID, UserACL: database.TemplateACL{full.ID.String(): {policy.ActionUpdate}, partial.ID.String(): {policy.ActionUpdate}}})
	templateB := dbgen.Template(t, store, database.Template{OrganizationID: org.ID, CreatedBy: full.ID, UserACL: database.TemplateACL{full.ID.String(): {policy.ActionUpdate}}})
	file := dbgen.File(t, store, database.File{CreatedBy: full.ID})
	now := dbtime.Now()
	p, err := store.InsertNetworkPolicy(ctx, database.InsertNetworkPolicyParams{ID: uuid.New(), OrganizationID: org.ID, Name: uuid.NewString(), CreatedBy: full.ID, CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	for _, id := range []uuid.UUID{templateA.ID, templateB.ID} {
		_, err := store.InsertNetworkPolicyAttachment(ctx, database.InsertNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: id, OrganizationID: org.ID, CreatedAt: now})
		require.NoError(t, err)
	}
	revision, err := store.InsertNetworkPolicyRevision(ctx, database.InsertNetworkPolicyRevisionParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: org.ID, RevisionNumber: 1, Hcl: "policy {}", FileID: file.ID, CreatedBy: full.ID, CreatedAt: now})
	require.NoError(t, err)
	target, err := store.InsertNetworkPolicyTarget(ctx, database.InsertNetworkPolicyTargetParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: org.ID, Cluster: "cluster", Context: "context", Namespace: "namespace", Tags: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	daemon := dbgen.ProvisionerDaemon(t, store, database.ProvisionerDaemon{OrganizationID: org.ID})
	job := dbgen.ProvisionerJob(t, store, nil, database.ProvisionerJob{
		OrganizationID: org.ID, InitiatorID: full.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy,
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour), StartedAt: sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
		WorkerID: uuid.NullUUID{UUID: daemon.ID, Valid: true},
	})
	// dbgen adds a unique acquisition tag to started jobs. Give the daemon the
	// same tags so its eligibility row exercises the policy authorization.
	daemon.Tags = job.Tags
	daemon = dbgen.ProvisionerDaemon(t, store, daemon)
	target, err = store.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{ID: target.ID, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, Generation: target.Generation, Status: "planning", UpdatedAt: now})
	require.NoError(t, err)
	_, err = store.InsertNetworkPolicyRun(ctx, database.InsertNetworkPolicyRunParams{ID: uuid.New(), JobID: job.ID, PolicyID: p.ID, TargetID: target.ID, RevisionID: revision.ID, OrganizationID: org.ID, Generation: target.Generation, Operation: "plan", CreatedAt: now})
	require.NoError(t, err)
	ordinary := dbgen.ProvisionerJob(t, store, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: full.ID, FileID: file.ID, Type: database.ProvisionerJobTypeWorkspaceBuild, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour), Tags: job.Tags})
	orphan := dbgen.ProvisionerJob(t, store, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: full.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour), Tags: job.Tags})
	ids := []uuid.UUID{job.ID, ordinary.ID, orphan.ID}
	jobIDs := func(jobs []database.ProvisionerJob) []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(jobs))
		for _, job := range jobs {
			ids = append(ids, job.ID)
		}
		return ids
	}
	lists := []struct {
		name string
		get  func(context.Context) ([]uuid.UUID, error)
	}{
		{"IDs", func(ctx context.Context) ([]uuid.UUID, error) {
			rows, err := db.GetProvisionerJobsByIDs(ctx, ids)
			return jobIDs(rows), err
		}},
		{"Queue", func(ctx context.Context) ([]uuid.UUID, error) {
			rows, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, database.GetProvisionerJobsByIDsWithQueuePositionParams{IDs: ids, StaleIntervalMS: time.Hour.Milliseconds()})
			var result []uuid.UUID
			for _, row := range rows {
				result = append(result, row.ProvisionerJob.ID)
			}
			return result, err
		}},
		{"Organization", func(ctx context.Context) ([]uuid.UUID, error) {
			rows, err := db.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisioner(ctx, database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams{OrganizationID: org.ID, IDs: ids})
			var result []uuid.UUID
			for _, row := range rows {
				result = append(result, row.ProvisionerJob.ID)
			}
			return result, err
		}},
		{"CreatedAfter", func(ctx context.Context) ([]uuid.UUID, error) {
			rows, err := db.GetProvisionerJobsCreatedAfter(ctx, now.Add(-2*time.Hour))
			return jobIDs(rows), err
		}},
		{"Reaper", func(ctx context.Context) ([]uuid.UUID, error) {
			rows, err := db.GetProvisionerJobsToBeReaped(ctx, database.GetProvisionerJobsToBeReapedParams{PendingSince: now, HungSince: now, MaxJobs: 100})
			return jobIDs(rows), err
		}},
		{"EligibleDaemons", func(ctx context.Context) ([]uuid.UUID, error) {
			rows, err := db.GetEligibleProvisionerDaemonsByProvisionerJobIDs(ctx, ids)
			var result []uuid.UUID
			for _, row := range rows {
				result = append(result, row.JobID)
			}
			return result, err
		}},
	}
	// The group waits for the read-only parallel cases before mutating history.
	t.Run("Lists", func(t *testing.T) {
		for _, actor := range []struct {
			name string
			ctx  context.Context
			want []uuid.UUID
		}{
			{"FullACL", fullCtx, []uuid.UUID{job.ID, ordinary.ID}},
			{"PartialACL", partialCtx, []uuid.UUID{ordinary.ID}},
			{"ForeignOrganization", foreignCtx, []uuid.UUID{ordinary.ID}},
			{"System", dbauthz.AsSystemRestricted(ctx), ids},
			{"Provisionerd", dbauthz.AsProvisionerd(ctx), ids},
		} {
			for _, list := range lists {
				t.Run(actor.name+"/"+list.name, func(t *testing.T) {
					t.Parallel()
					got, err := list.get(actor.ctx)
					require.NoError(t, err)
					want := actor.want
					if actor.name == "Provisionerd" && list.name == "EligibleDaemons" {
						// Provisionerd has job access, not daemon-list access.
						want = nil
					}
					require.ElementsMatch(t, want, got)
				})
			}
		}
	})
	for _, actor := range []struct {
		ctx     context.Context
		visible bool
	}{{fullCtx, true}, {partialCtx, false}, {foreignCtx, false}, {dbauthz.AsSystemRestricted(ctx), true}} {
		rows, err := db.GetProvisionerDaemonsWithStatusByOrganization(actor.ctx, database.GetProvisionerDaemonsWithStatusByOrganizationParams{OrganizationID: org.ID, IDs: []uuid.UUID{daemon.ID}, StaleIntervalMS: time.Hour.Milliseconds()})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, actor.visible, rows[0].CurrentJobID.Valid)
		require.Equal(t, actor.visible, rows[0].CurrentJobStatus.Valid)
		if actor.visible {
			require.Equal(t, job.ID, rows[0].CurrentJobID.UUID)
		}
	}
	_, err = db.GetProvisionerJobsByIDs(ctx, []uuid.UUID{job.ID})
	require.ErrorIs(t, err, dbauthz.ErrNoActor)
	unchanged, err := db.GetProvisionerJobsByIDs(ctx, []uuid.UUID{ordinary.ID})
	require.NoError(t, err, "other job types retain existing authorization behavior")
	require.Len(t, unchanged, 1)

	_, err = store.CompleteNetworkPolicyTargetJob(ctx, database.CompleteNetworkPolicyTargetJobParams{ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation, State: []byte{}, Status: "succeeded", UpdatedAt: now})
	require.NoError(t, err)
	err = store.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{ID: job.ID, CompletedAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now})
	require.NoError(t, err)
	_, err = store.DeleteNetworkPolicyAttachment(ctx, database.DeleteNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: templateB.ID})
	require.NoError(t, err)
	visibleJobs, err := db.GetProvisionerJobsByIDs(partialCtx, []uuid.UUID{job.ID})
	require.NoError(t, err)
	require.Empty(t, visibleJobs, "detaching cannot erase the job's historical template authorization")
	for _, actor := range []struct {
		ctx     context.Context
		visible bool
	}{{fullCtx, true}, {partialCtx, false}} {
		rows, err := db.GetProvisionerDaemonsWithStatusByOrganization(actor.ctx, database.GetProvisionerDaemonsWithStatusByOrganizationParams{OrganizationID: org.ID, IDs: []uuid.UUID{daemon.ID}, StaleIntervalMS: time.Hour.Milliseconds()})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, actor.visible, rows[0].PreviousJobID.Valid)
		require.Equal(t, actor.visible, rows[0].PreviousJobStatus.Valid)
	}
}
