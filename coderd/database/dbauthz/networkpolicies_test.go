package dbauthz_test

import (
	"encoding/json"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/testutil"
)

func (s *MethodTestSuite) TestNetworkPolicy() {
	p := database.NetworkPolicy{ID: uuid.New(), OrganizationID: uuid.New()}
	template := database.Template{ID: uuid.New(), OrganizationID: p.OrganizationID}
	attachment := database.NetworkPolicyAttachment{PolicyID: p.ID, TemplateID: template.ID, OrganizationID: p.OrganizationID}
	revision := database.NetworkPolicyRevision{ID: uuid.New(), PolicyID: p.ID}
	target := database.NetworkPolicyTarget{ID: uuid.New(), PolicyID: p.ID}
	run := database.NetworkPolicyRun{ID: uuid.New(), JobID: uuid.New(), PolicyID: p.ID, TemplateIDs: []uuid.UUID{template.ID}}

	for _, name := range []string{
		"GetNetworkPolicyByID", "GetNetworkPolicyByIDForUpdate", "GetNetworkPoliciesByOrganizationID",
		"GetNetworkPolicyAttachmentsByPolicyID", "GetNetworkPolicyRevisionByID", "GetNetworkPolicyRevisionsByPolicyID",
		"GetNetworkPolicyTargetByID", "GetNetworkPolicyTargetByIDForUpdate", "GetNetworkPolicyTargetsByPolicyID",
		"GetNetworkPolicyRunByID", "GetNetworkPolicyRunByJobID", "GetNetworkPolicyRunsByPolicyID",
		"InsertNetworkPolicy", "UpdateNetworkPolicy", "PublishNetworkPolicyRevision", "InsertNetworkPolicyRevision",
		"InsertNetworkPolicyAttachment", "DeleteNetworkPolicyAttachment", "InsertNetworkPolicyTarget",
		"StartNetworkPolicyTargetJob", "UpdateNetworkPolicyTargetState", "CompleteNetworkPolicyTargetJob", "InsertNetworkPolicyRun",
		"RequestNetworkPolicyDeletion", "CompleteNetworkPolicyDeletion", "DestroyUnappliedNetworkPolicyTarget",
	} {
		s.Run(name, s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
			dbm.EXPECT().GetNetworkPolicyByID(gomock.Any(), p.ID).Return(p, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyByIDForUpdate(gomock.Any(), p.ID).Return(p, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPoliciesByOrganizationID(gomock.Any(), p.OrganizationID).Return([]database.NetworkPolicy{p}, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyAttachmentsByPolicyID(gomock.Any(), p.ID).Return([]database.NetworkPolicyAttachment{attachment}, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyRunsByPolicyID(gomock.Any(), p.ID).Return([]database.NetworkPolicyRun{run}, nil).AnyTimes()
			dbm.EXPECT().GetTemplateByID(gomock.Any(), template.ID).Return(template, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyRevisionByID(gomock.Any(), revision.ID).Return(revision, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyRevisionsByPolicyID(gomock.Any(), p.ID).Return([]database.NetworkPolicyRevision{revision}, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyTargetByID(gomock.Any(), target.ID).Return(target, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyTargetByIDForUpdate(gomock.Any(), target.ID).Return(target, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyTargetsByPolicyID(gomock.Any(), p.ID).Return([]database.NetworkPolicyTarget{target}, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyRunByID(gomock.Any(), run.ID).Return(run, nil).AnyTimes()
			dbm.EXPECT().GetNetworkPolicyRunByJobID(gomock.Any(), run.JobID).Return(run, nil).AnyTimes()

			var arg, result any
			var object rbac.Objecter = template
			action := policy.ActionUpdate
			switch name {
			case "GetNetworkPolicyByID", "GetNetworkPolicyByIDForUpdate":
				arg, result = p.ID, p
			case "GetNetworkPoliciesByOrganizationID":
				arg, result = p.OrganizationID, []database.NetworkPolicy{p}
			case "GetNetworkPolicyAttachmentsByPolicyID":
				arg, result = p.ID, []database.NetworkPolicyAttachment{attachment}
			case "GetNetworkPolicyRevisionByID":
				arg, result = revision.ID, revision
			case "GetNetworkPolicyRevisionsByPolicyID":
				arg, result = p.ID, []database.NetworkPolicyRevision{revision}
			case "GetNetworkPolicyTargetByID", "GetNetworkPolicyTargetByIDForUpdate":
				arg, result = target.ID, target
			case "GetNetworkPolicyTargetsByPolicyID":
				arg, result = p.ID, []database.NetworkPolicyTarget{target}
			case "GetNetworkPolicyRunByID":
				arg, result = run.ID, run
			case "GetNetworkPolicyRunByJobID":
				arg, result = run.JobID, run
			case "GetNetworkPolicyRunsByPolicyID":
				arg, result = p.ID, []database.NetworkPolicyRun{run}
			case "InsertNetworkPolicy":
				arg, result = database.InsertNetworkPolicyParams{OrganizationID: p.OrganizationID}, p
				object, action = rbac.ResourceTemplate.InOrg(p.OrganizationID), policy.ActionCreate
				dbm.EXPECT().InsertNetworkPolicy(gomock.Any(), arg).Return(p, nil).AnyTimes()
			case "RequestNetworkPolicyDeletion":
				arg, result = database.RequestNetworkPolicyDeletionParams{ID: p.ID}, p
				dbm.EXPECT().RequestNetworkPolicyDeletion(gomock.Any(), arg).Return(p, nil).AnyTimes()
			case "CompleteNetworkPolicyDeletion":
				arg, result, object = database.CompleteNetworkPolicyDeletionParams{ID: p.ID}, int64(1), rbac.ResourceSystem
				dbm.EXPECT().CompleteNetworkPolicyDeletion(gomock.Any(), arg).Return(int64(1), nil).AnyTimes()
			case "DestroyUnappliedNetworkPolicyTarget":
				arg, result, object = database.DestroyUnappliedNetworkPolicyTargetParams{ID: target.ID}, target, rbac.ResourceSystem
				dbm.EXPECT().DestroyUnappliedNetworkPolicyTarget(gomock.Any(), arg).Return(target, nil).AnyTimes()
			case "UpdateNetworkPolicy":
				arg, result = database.UpdateNetworkPolicyParams{ID: p.ID}, p
				dbm.EXPECT().UpdateNetworkPolicy(gomock.Any(), arg).Return(p, nil).AnyTimes()
			case "PublishNetworkPolicyRevision":
				arg, result = database.PublishNetworkPolicyRevisionParams{ID: p.ID}, p
				dbm.EXPECT().PublishNetworkPolicyRevision(gomock.Any(), arg).Return(p, nil).AnyTimes()
			case "InsertNetworkPolicyRevision":
				arg, result = database.InsertNetworkPolicyRevisionParams{PolicyID: p.ID}, revision
				dbm.EXPECT().InsertNetworkPolicyRevision(gomock.Any(), arg).Return(revision, nil).AnyTimes()
			case "InsertNetworkPolicyAttachment":
				arg, result = database.InsertNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: template.ID}, attachment
				dbm.EXPECT().InsertNetworkPolicyAttachment(gomock.Any(), arg).Return(attachment, nil).AnyTimes()
			case "DeleteNetworkPolicyAttachment":
				arg, result = database.DeleteNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: template.ID}, int64(1)
				dbm.EXPECT().DeleteNetworkPolicyAttachment(gomock.Any(), arg).Return(int64(1), nil).AnyTimes()
			case "InsertNetworkPolicyTarget":
				arg, result, object = database.InsertNetworkPolicyTargetParams{PolicyID: p.ID}, target, rbac.ResourceSystem
				dbm.EXPECT().InsertNetworkPolicyTarget(gomock.Any(), arg).Return(target, nil).AnyTimes()
			case "StartNetworkPolicyTargetJob":
				arg, result, object = database.StartNetworkPolicyTargetJobParams{ID: target.ID}, target, rbac.ResourceSystem
				dbm.EXPECT().StartNetworkPolicyTargetJob(gomock.Any(), arg).Return(target, nil).AnyTimes()
			case "UpdateNetworkPolicyTargetState":
				arg, result, object = database.UpdateNetworkPolicyTargetStateParams{ID: target.ID}, target, rbac.ResourceSystem
				dbm.EXPECT().UpdateNetworkPolicyTargetState(gomock.Any(), arg).Return(target, nil).AnyTimes()
			case "CompleteNetworkPolicyTargetJob":
				arg, result, object = database.CompleteNetworkPolicyTargetJobParams{ID: target.ID}, target, rbac.ResourceSystem
				dbm.EXPECT().CompleteNetworkPolicyTargetJob(gomock.Any(), arg).Return(target, nil).AnyTimes()
			case "InsertNetworkPolicyRun":
				arg, result, object = database.InsertNetworkPolicyRunParams{PolicyID: p.ID}, run, rbac.ResourceSystem
				dbm.EXPECT().InsertNetworkPolicyRun(gomock.Any(), arg).Return(run, nil).AnyTimes()
			}
			check.Args(arg).Asserts(object, action).Returns(result)
			if name == "InsertNetworkPolicyAttachment" {
				check.Asserts(template, policy.ActionUpdate, template, policy.ActionUpdate)
			}
		}))
	}
}

func TestNetworkPolicyAuthorization(t *testing.T) {
	t.Parallel()
	store, _ := dbtestutil.NewDB(t)
	db := dbauthz.New(store, rbac.NewAuthorizer(prometheus.NewRegistry()), testutil.Logger(t), coderdtest.AccessControlStorePointer())
	ctx := testutil.Context(t, testutil.WaitLong)
	org := dbgen.Organization(t, store, database.Organization{})
	otherOrg := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})
	otherUser := dbgen.User(t, store, database.User{})
	ownerCtx := dbauthz.As(ctx, rbac.Subject{ID: user.ID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleOwner()}, Scope: rbac.ScopeAll})
	memberRoles, err := rolestore.Expand(ctx, store, []rbac.RoleIdentifier{rbac.RoleMember(), rbac.ScopedRoleOrgMember(org.ID)})
	require.NoError(t, err)
	memberCtx := dbauthz.As(ctx, rbac.Subject{ID: user.ID.String(), Roles: memberRoles, Scope: rbac.ScopeAll})
	foreignCtx := dbauthz.As(ctx, rbac.Subject{ID: otherUser.ID.String(), Roles: rbac.RoleIdentifiers{rbac.ScopedRoleOrgAdmin(otherOrg.ID)}, Scope: rbac.ScopeAll})
	templateA := dbgen.Template(t, store, database.Template{OrganizationID: org.ID, CreatedBy: user.ID, UserACL: database.TemplateACL{user.ID.String(): {policy.ActionRead, policy.ActionUpdate}}})
	templateB := dbgen.Template(t, store, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	now := dbtime.Now()
	p, err := db.InsertNetworkPolicy(ownerCtx, database.InsertNetworkPolicyParams{ID: uuid.New(), OrganizationID: org.ID, Name: uuid.NewString(), CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)

	// Empty policies require template creation, not merely organization membership.
	_, err = db.GetNetworkPolicyByID(memberCtx, p.ID)
	require.True(t, dbauthz.IsNotAuthorizedError(err))
	for _, template := range []database.Template{templateA, templateB} {
		_, err := db.InsertNetworkPolicyAttachment(ownerCtx, database.InsertNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: template.ID, OrganizationID: org.ID, CreatedAt: now})
		require.NoError(t, err)
	}
	_, err = db.GetNetworkPolicyByID(memberCtx, p.ID)
	require.True(t, dbauthz.IsNotAuthorizedError(err), "updating one attached template is insufficient")
	rows, err := db.GetNetworkPoliciesByOrganizationID(memberCtx, org.ID)
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = db.GetNetworkPolicyByID(foreignCtx, p.ID)
	require.True(t, dbauthz.IsNotAuthorizedError(err))

	err = store.UpdateTemplateACLByID(ctx, database.UpdateTemplateACLByIDParams{ID: templateB.ID, UserACL: database.TemplateACL{user.ID.String(): {policy.ActionUpdate}}, GroupACL: database.TemplateACL{}})
	require.NoError(t, err)
	_, err = db.GetNetworkPolicyByID(memberCtx, p.ID)
	require.NoError(t, err, "full template ACL objects grant composite access")
	_, err = db.InsertNetworkPolicyTarget(memberCtx, database.InsertNetworkPolicyTargetParams{})
	require.True(t, dbauthz.IsNotAuthorizedError(err), "target writes are internal")
	_, err = db.InsertNetworkPolicyRun(memberCtx, database.InsertNetworkPolicyRunParams{})
	require.True(t, dbauthz.IsNotAuthorizedError(err), "run writes are internal")

	file := dbgen.File(t, store, database.File{CreatedBy: user.ID})
	revision, err := db.InsertNetworkPolicyRevision(memberCtx, database.InsertNetworkPolicyRevisionParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: org.ID, RevisionNumber: 1, Hcl: "policy {}", FileID: file.ID, CreatedBy: user.ID, CreatedAt: now})
	require.NoError(t, err)
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	target, err := db.InsertNetworkPolicyTarget(systemCtx, database.InsertNetworkPolicyTargetParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: org.ID, Cluster: "cluster", Context: "context", Namespace: "namespace", Tags: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, store, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeNetworkPolicy})
	target, err = db.StartNetworkPolicyTargetJob(systemCtx, database.StartNetworkPolicyTargetJobParams{ID: target.ID, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, Generation: target.Generation, Status: "planning", UpdatedAt: now})
	require.NoError(t, err)
	run, err := db.InsertNetworkPolicyRun(systemCtx, database.InsertNetworkPolicyRunParams{ID: uuid.New(), JobID: job.ID, PolicyID: p.ID, TargetID: target.ID, RevisionID: revision.ID, OrganizationID: org.ID, Generation: target.Generation, Operation: "plan", CreatedAt: now})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{templateA.ID, templateB.ID}, run.TemplateIDs)
	_, err = db.CompleteNetworkPolicyTargetJob(systemCtx, database.CompleteNetworkPolicyTargetJobParams{ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation, State: []byte{}, Status: "succeeded", UpdatedAt: now})
	require.NoError(t, err)
	deleted, err := db.DeleteNetworkPolicyAttachment(memberCtx, database.DeleteNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: templateB.ID})
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	err = store.UpdateTemplateACLByID(ctx, database.UpdateTemplateACLByIDParams{ID: templateB.ID, UserACL: database.TemplateACL{}, GroupACL: database.TemplateACL{}})
	require.NoError(t, err)
	_, err = db.GetNetworkPolicyRunByID(memberCtx, run.ID)
	require.True(t, dbauthz.IsNotAuthorizedError(err), "detaching cannot erase historical authorization")
	_, err = db.GetNetworkPolicyTargetByID(memberCtx, target.ID)
	require.True(t, dbauthz.IsNotAuthorizedError(err), "target state keeps historical authorization")
	_, err = db.GetNetworkPolicyRevisionByID(memberCtx, revision.ID)
	require.True(t, dbauthz.IsNotAuthorizedError(err), "frozen revisions keep historical authorization")
	_, err = db.GetNetworkPolicyRunByJobID(systemCtx, job.ID)
	require.NoError(t, err, "internal workers can read historical runs")
}
