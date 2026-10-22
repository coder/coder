package provisionerdserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type networkPolicyFixture struct {
	server   *server
	job      database.ProvisionerJob
	run      database.NetworkPolicyRun
	target   database.NetworkPolicyTarget
	revision database.NetworkPolicyRevision
}

func newNetworkPolicyFixture(t *testing.T, db database.Store, ps pubsub.Pubsub, operation string) networkPolicyFixture {
	t.Helper()
	ctx := t.Context()
	now := dbtime.Now()
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	file := dbgen.File(t, db, database.File{CreatedBy: user.ID, Data: []byte("frozen source archive")})
	policy, err := db.InsertNetworkPolicy(ctx, database.InsertNetworkPolicyParams{
		ID: uuid.New(), OrganizationID: org.ID, Name: uuid.NewString(), CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	_, err = db.InsertNetworkPolicyAttachment(ctx, database.InsertNetworkPolicyAttachmentParams{
		PolicyID: policy.ID, TemplateID: template.ID, OrganizationID: org.ID, CreatedAt: now,
	})
	require.NoError(t, err)
	revision, err := db.InsertNetworkPolicyRevision(ctx, database.InsertNetworkPolicyRevisionParams{
		ID: uuid.New(), PolicyID: policy.ID, OrganizationID: org.ID, RevisionNumber: 1,
		FileID: file.ID, Hcl: "frozen HCL", CreatedBy: user.ID, CreatedAt: now,
	})
	require.NoError(t, err)
	target, err := db.InsertNetworkPolicyTarget(ctx, database.InsertNetworkPolicyTargetParams{
		ID: uuid.New(), PolicyID: policy.ID, OrganizationID: org.ID, Cluster: "cluster", Context: "context",
		Namespace: "namespace", Tags: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	daemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{OrganizationID: org.ID})
	runID := uuid.New()
	input, err := json.Marshal(NetworkPolicyJob{NetworkPolicyRunID: runID})
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID,
		Type: database.ProvisionerJobTypeNetworkPolicy, Input: input,
		StartedAt: sql.NullTime{Time: now, Valid: true}, WorkerID: uuid.NullUUID{UUID: daemon.ID, Valid: true},
	})
	target, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
		ID: target.ID, JobID: uuid.NullUUID{UUID: job.ID, Valid: true},
		DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, Generation: target.Generation,
		Status: "pending", UpdatedAt: now,
	})
	require.NoError(t, err)
	target, err = db.UpdateNetworkPolicyTargetState(ctx, database.UpdateNetworkPolicyTargetStateParams{
		ID: target.ID, JobID: target.ActiveJobID, Generation: target.Generation, State: []byte("previous state"), UpdatedAt: now,
	})
	require.NoError(t, err)
	run, err := db.InsertNetworkPolicyRun(ctx, database.InsertNetworkPolicyRunParams{
		ID: runID, JobID: job.ID, PolicyID: policy.ID, TargetID: target.ID, RevisionID: revision.ID,
		OrganizationID: org.ID, Generation: target.Generation, Operation: operation, CreatedAt: now,
	})
	require.NoError(t, err)
	logger := testutil.Logger(t)
	s := &server{
		ID: daemon.ID, OrganizationID: org.ID, Database: dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), logger, nil),
		Pubsub: ps, Logger: logger, Tracer: trace.NewNoopTracerProvider().Tracer("test"), Clock: quartz.NewMock(t),
		activeJobs: map[uuid.UUID]struct{}{job.ID: {}}, lifecycleCtx: ctx,
	}
	return networkPolicyFixture{server: s, job: job, run: run, target: target, revision: revision}
}

func TestNetworkPolicyJobLifecycle(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, tc := range []struct {
		name         string
		operation    string
		result       networkPolicyJobResult
		cancel       bool
		wantState    string
		wantApplied  bool
		wantReserved bool
		wantStatus   string
		wantDrifted  bool
	}{
		{name: "Plan", operation: "plan", result: networkPolicyJobResult{success: true, disposition: proto.StateDisposition_UNCHANGED, plan: []byte(`{"resource_changes":[]}`)}, wantState: "previous state", wantStatus: "succeeded"},
		{name: "PlanDetectsDrift", operation: "plan", result: networkPolicyJobResult{success: true, disposition: proto.StateDisposition_UNCHANGED, plan: []byte(`{"resource_drift":[{"change":{"actions":["update"]}}]}`)}, wantState: "previous state", wantStatus: "succeeded", wantDrifted: true},
		{name: "Apply", operation: "apply", result: networkPolicyJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte("applied state")}, wantState: "applied state", wantApplied: true, wantStatus: "succeeded"},
		{name: "PlanFailure", operation: "plan", result: networkPolicyJobResult{error: "plan failed", disposition: proto.StateDisposition_UNCHANGED}, wantState: "previous state", wantStatus: "failed"},
		{name: "GenericPlanFailure", operation: "plan", result: networkPolicyJobResult{error: "transport failed"}, wantState: "previous state", wantStatus: "failed"},
		{name: "BeforeApplyFailure", operation: "apply", result: networkPolicyJobResult{error: "init failed", disposition: proto.StateDisposition_UNCHANGED}, wantState: "previous state", wantStatus: "failed"},
		{name: "PartialApplyFailure", operation: "apply", result: networkPolicyJobResult{error: "apply failed", disposition: proto.StateDisposition_UNKNOWN, state: []byte("partial state")}, wantState: "partial state", wantStatus: "pending", wantReserved: true},
		{name: "KilledApplyReturnsOldCheckpoint", operation: "apply", result: networkPolicyJobResult{error: "apply killed", disposition: proto.StateDisposition_UNKNOWN, state: []byte("previous state")}, wantState: "previous state", wantStatus: "pending", wantReserved: true},
		{name: "ApplyTransportFailure", operation: "apply", result: networkPolicyJobResult{error: "transport failed"}, wantState: "previous state", wantReserved: true, wantStatus: "pending"},
		{name: "CanceledPartialApply", operation: "apply", cancel: true, result: networkPolicyJobResult{error: "canceled", disposition: proto.StateDisposition_UNKNOWN, state: []byte("partial canceled state")}, wantState: "partial canceled state", wantStatus: "pending", wantReserved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newNetworkPolicyFixture(t, db, ps, tc.operation)
			ctx := t.Context()
			if tc.cancel {
				require.NoError(t, db.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{
					ID: f.job.ID, CanceledAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
				}))
			}
			var err error
			if tc.result.success {
				_, err = f.server.CompleteJob(ctx, &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_NetworkPolicy_{
					NetworkPolicy: &proto.CompletedJob_NetworkPolicy{State: tc.result.state, StateDisposition: tc.result.disposition, Plan: tc.result.plan},
				}})
			} else {
				failed := &proto.FailedJob{JobId: f.job.ID.String(), Error: tc.result.error}
				if tc.result.disposition != proto.StateDisposition_UNKNOWN || len(tc.result.state) > 0 {
					failed.Type = &proto.FailedJob_NetworkPolicy_{NetworkPolicy: &proto.FailedJob_NetworkPolicy{State: tc.result.state, StateDisposition: tc.result.disposition}}
				}
				_, err = f.server.FailJob(ctx, failed)
			}
			require.NoError(t, err)
			target, err := db.GetNetworkPolicyTargetByID(ctx, f.target.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantState, string(target.State))
			require.Equal(t, tc.wantApplied, target.AppliedRevisionID.Valid)
			if tc.wantApplied {
				require.Equal(t, f.revision.ID, target.AppliedRevisionID.UUID)
			}
			require.Equal(t, tc.wantStatus, target.Status)
			require.Equal(t, tc.wantDrifted, target.Drifted)
			require.Equal(t, tc.wantReserved, target.ActiveJobID.Valid)
			job, err := db.GetProvisionerJobByID(ctx, f.job.ID)
			require.NoError(t, err)
			require.True(t, job.CompletedAt.Valid)
			require.Equal(t, !tc.result.success, job.Error.Valid)
			if !tc.result.success && !tc.wantReserved {
				require.Equal(t, "NETWORK_POLICY_STATE_UNCHANGED", job.ErrorCode.String)
			}
			if tc.wantReserved {
				require.Equal(t, "NETWORK_POLICY_STATE_UNKNOWN", job.ErrorCode.String)
				require.Contains(t, job.Error.String, "reconciliation required")
				_, err := f.server.CompleteJob(ctx, &proto.CompletedJob{JobId: job.ID.String(), Type: &proto.CompletedJob_NetworkPolicy_{
					NetworkPolicy: &proto.CompletedJob_NetworkPolicy{State: []byte("late state"), StateDisposition: proto.StateDisposition_RETURNED},
				}})
				require.ErrorContains(t, err, "late results are rejected")
				_, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{
					ID: target.ID, JobID: target.ActiveJobID, DesiredRevisionID: target.DesiredRevisionID, Generation: target.Generation, Status: "pending", UpdatedAt: dbtime.Now(),
				})
				require.ErrorIs(t, err, sql.ErrNoRows)
			}
			resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
			require.NoError(t, err)
			require.Empty(t, resources)
		})
	}
}

func TestNetworkPolicyAcquire(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "apply")
	// Publishing a different revision after queueing must not change the
	// archive or revision delivered to this job.
	otherFile := dbgen.File(t, db, database.File{CreatedBy: f.job.InitiatorID, Hash: uuid.NewString(), Data: []byte("new published archive")})
	otherRevision, err := db.InsertNetworkPolicyRevision(t.Context(), database.InsertNetworkPolicyRevisionParams{
		ID: uuid.New(), PolicyID: f.run.PolicyID, OrganizationID: f.job.OrganizationID, RevisionNumber: 2,
		FileID: otherFile.ID, Hcl: "different HCL", CreatedBy: f.job.InitiatorID, CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	_, err = db.PublishNetworkPolicyRevision(t.Context(), database.PublishNetworkPolicyRevisionParams{
		ID: f.run.PolicyID, PublishedRevisionID: uuid.NullUUID{UUID: otherRevision.ID, Valid: true}, UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	ctx := dbauthz.AsProvisionerd(t.Context())
	payload, err := f.server.acquireProtoJob(ctx, f.job)
	require.NoError(t, err)
	require.Equal(t, []byte("frozen source archive"), payload.TemplateSourceArchive)
	native := payload.GetNetworkPolicy()
	require.NotNil(t, native)
	require.Equal(t, proto.NetworkPolicyOperation_APPLY, native.Operation)
	require.Equal(t, f.target.State, native.State)
	require.Equal(t, f.run.ID.String(), native.GetRunId())
	require.Equal(t, f.target.ID.String(), native.GetTargetId())
	require.Equal(t, f.revision.ID.String(), native.GetRevisionId())
	require.NotNil(t, native.Metadata)
	require.Equal(t, sdkproto.WorkspaceTransition_START, native.Metadata.WorkspaceTransition)
	values := map[string]string{}
	for _, variable := range native.VariableValues {
		values[variable.Name] = variable.Value
	}
	templateIDs, err := json.Marshal(f.run.TemplateIDs)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"policy_id": f.run.PolicyID.String(), "namespace": f.target.Namespace,
		"kubeconfig_context": f.target.Context, "template_ids": string(templateIDs),
	}, values)
}

func TestNetworkPolicyDuplicateCompletion(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "apply")
	completed := &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_NetworkPolicy_{
		NetworkPolicy: &proto.CompletedJob_NetworkPolicy{State: []byte("applied"), StateDisposition: proto.StateDisposition_RETURNED},
	}}
	var group errgroup.Group
	for range 2 {
		group.Go(func() error {
			_, err := f.server.CompleteJob(t.Context(), completed)
			return err
		})
	}
	require.NoError(t, group.Wait())
	// An RPC retry cannot replace the already committed state, even when its
	// payload differs or a failure RPC arrives after the successful completion.
	completed.GetNetworkPolicy().State = []byte("different")
	_, err := f.server.CompleteJob(t.Context(), completed)
	require.NoError(t, err)
	_, err = f.server.FailJob(t.Context(), &proto.FailedJob{JobId: f.job.ID.String(), Error: "late failure"})
	require.NoError(t, err)
	target, err := db.GetNetworkPolicyTargetByID(t.Context(), f.target.ID)
	require.NoError(t, err)
	require.Equal(t, []byte("applied"), target.State)
	require.Equal(t, f.revision.ID, target.AppliedRevisionID.UUID)
}

func TestNetworkPolicyInvalidPayload(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, tc := range []struct {
		name      string
		operation string
		payload   *proto.CompletedJob_NetworkPolicy
	}{
		{name: "EmptyApplyState", operation: "apply", payload: &proto.CompletedJob_NetworkPolicy{StateDisposition: proto.StateDisposition_RETURNED}},
		{name: "UnknownApplySuccess", operation: "apply", payload: &proto.CompletedJob_NetworkPolicy{}},
		{name: "PlanReturnsState", operation: "plan", payload: &proto.CompletedJob_NetworkPolicy{State: []byte("bad"), StateDisposition: proto.StateDisposition_RETURNED}},
		{name: "BinaryPlan", operation: "plan", payload: &proto.CompletedJob_NetworkPolicy{Plan: []byte("binary plan"), StateDisposition: proto.StateDisposition_UNCHANGED}},
		{name: "OversizedState", operation: "apply", payload: &proto.CompletedJob_NetworkPolicy{State: make([]byte, drpcsdk.MaxMessageSize), StateDisposition: proto.StateDisposition_RETURNED}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newNetworkPolicyFixture(t, db, ps, tc.operation)
			_, err := f.server.CompleteJob(t.Context(), &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_NetworkPolicy_{NetworkPolicy: tc.payload}})
			require.Error(t, err)
			job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
			require.NoError(t, err)
			require.False(t, job.CompletedAt.Valid)
			target, err := db.GetNetworkPolicyTargetByID(t.Context(), f.target.ID)
			require.NoError(t, err)
			require.Equal(t, f.target.State, target.State)
			require.Equal(t, f.target.ActiveJobID, target.ActiveJobID)
		})
	}
}

func TestNetworkPolicyPreDeliveryFailure(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "BeforeSend", true: "SendFailed"}[uncertain], func(t *testing.T) {
			t.Parallel()
			f := newNetworkPolicyFixture(t, db, ps, "apply")
			disposition := proto.StateDisposition_UNCHANGED
			if uncertain {
				disposition = proto.StateDisposition_UNKNOWN
			}
			f.server.failAcquiredJob(testutil.Logger(t), f.job.ID, "connection broken", disposition)
			target, err := db.GetNetworkPolicyTargetByID(t.Context(), f.target.ID)
			require.NoError(t, err)
			require.Equal(t, uncertain, target.ActiveJobID.Valid)
			require.Equal(t, f.target.State, target.State)
			job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
			require.NoError(t, err)
			require.True(t, job.CompletedAt.Valid)
		})
	}
}

func TestNetworkPolicyRejectsOtherJobPayloads(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "apply")
	_, err := f.server.FailJob(t.Context(), &proto.FailedJob{JobId: f.job.ID.String(), Type: &proto.FailedJob_WorkspaceBuild_{WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{State: []byte("wrong state")}}})
	require.ErrorContains(t, err, "does not match")
	_, err = f.server.CompleteJob(t.Context(), &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_WorkspaceBuild_{WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{}}})
	require.ErrorContains(t, err, "payload must be provided")
	_, err = f.server.FailJob(t.Context(), &proto.FailedJob{JobId: f.job.ID.String(), Error: "apply failed", Type: &proto.FailedJob_NetworkPolicy_{
		NetworkPolicy: &proto.FailedJob_NetworkPolicy{State: []byte("possibly stale"), StateDisposition: proto.StateDisposition_RETURNED},
	}})
	require.ErrorContains(t, err, "authoritative network policy state requires successful apply")
	_, err = f.server.CommitQuota(t.Context(), &proto.CommitQuotaRequest{JobId: f.job.ID.String()})
	require.ErrorContains(t, err, "do not use workspace quota")
	job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
	require.NoError(t, err)
	require.False(t, job.CompletedAt.Valid)
}

// networkPolicyFailCompletionStore injects a final write failure inside the
// real transaction, after the target has already been updated.
type networkPolicyFailCompletionStore struct{ database.Store }

func (s networkPolicyFailCompletionStore) InTx(fn func(database.Store) error, options *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error { return fn(networkPolicyFailCompletionStore{Store: tx}) }, options)
}

func (networkPolicyFailCompletionStore) UpdateProvisionerJobWithCompleteByID(context.Context, database.UpdateProvisionerJobWithCompleteByIDParams) error {
	return xerrors.New("injected completion failure")
}

func TestNetworkPolicyFinalizationRollback(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "apply")
	f.server.Database = networkPolicyFailCompletionStore{Store: f.server.Database}
	_, err := f.server.CompleteJob(t.Context(), &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_NetworkPolicy_{
		NetworkPolicy: &proto.CompletedJob_NetworkPolicy{State: []byte("new state"), StateDisposition: proto.StateDisposition_RETURNED},
	}})
	require.ErrorContains(t, err, "injected completion failure")
	target, err := db.GetNetworkPolicyTargetByID(t.Context(), f.target.ID)
	require.NoError(t, err)
	require.Equal(t, f.target.State, target.State)
	require.Equal(t, f.target.ActiveJobID, target.ActiveJobID)
	require.False(t, target.AppliedRevisionID.Valid)
	job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
	require.NoError(t, err)
	require.False(t, job.CompletedAt.Valid)
}

func TestNetworkPolicyLogs(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newNetworkPolicyFixture(t, db, ps, "plan")
	_, err := f.server.UpdateJob(t.Context(), &proto.UpdateJobRequest{
		JobId: f.job.ID.String(), Logs: []*proto.Log{{CreatedAt: time.Now().UnixMilli(), Level: sdkproto.LogLevel_INFO, Source: proto.LogSource_PROVISIONER, Stage: "Planning", Output: "native plan log"}},
		Readme: []byte("ignored"), WorkspaceTags: map[string]string{"ignored": "true"},
	})
	require.NoError(t, err)
	logs, err := db.GetProvisionerLogsAfterID(t.Context(), database.GetProvisionerLogsAfterIDParams{JobID: f.job.ID})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, "native plan log", logs[0].Output)
}
