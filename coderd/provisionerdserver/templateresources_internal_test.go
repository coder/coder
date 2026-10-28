package provisionerdserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type templateResourcesFixture struct {
	server     *server
	foundation database.Foundation
	job        database.ProvisionerJob
	run        database.TemplateResourceRun
	state      database.TemplateResourceState
	revision   database.TemplateResourceRevision
}

func newTemplateResourcesFixture(t *testing.T, db database.Store, ps pubsub.Pubsub, action string, unbound ...bool) templateResourcesFixture {
	t.Helper()
	ctx := t.Context()
	now := dbtime.Now()
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	binding := uuid.NullUUID{UUID: template.ID, Valid: true}
	if len(unbound) != 0 && unbound[0] {
		binding = uuid.NullUUID{}
	}
	templateFoundation := dbgen.Foundation(t, db, database.Foundation{OrganizationID: template.OrganizationID, TemplateID: binding})
	file := dbgen.File(t, db, database.File{CreatedBy: user.ID, Data: []byte("frozen source archive")})
	_, err := db.InsertTemplateResourceState(ctx, database.InsertTemplateResourceStateParams{
		FoundationID: templateFoundation.ID, OrganizationID: org.ID, UpdatedAt: now,
	})
	require.NoError(t, err)
	revision, err := db.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{
		ID: uuid.New(), FoundationID: templateFoundation.ID, OrganizationID: org.ID, FileID: file.ID, Source: "frozen generic HCL", CreatedBy: user.ID, CreatedAt: now,
	})
	require.NoError(t, err)
	_, err = db.SetTemplateResourceCurrentRevision(ctx, database.SetTemplateResourceCurrentRevisionParams{
		FoundationID: templateFoundation.ID, CurrentRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: now,
	})
	require.NoError(t, err)
	daemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{OrganizationID: org.ID})
	runID := uuid.New()
	input, err := json.Marshal(TemplateResourcesJob{TemplateResourceRunID: runID})
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID,
		Type: database.ProvisionerJobTypeTemplateResources, Input: input,
		StartedAt: sql.NullTime{Time: now, Valid: true}, WorkerID: uuid.NullUUID{UUID: daemon.ID, Valid: true},
	})
	run, err := db.InsertTemplateResourceRun(ctx, database.InsertTemplateResourceRunParams{
		ID: runID, FoundationID: templateFoundation.ID, OrganizationID: org.ID, RevisionID: revision.ID, JobID: job.ID, Action: action, CreatedAt: now,
	})
	require.NoError(t, err)
	state, err := db.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{
		FoundationID: templateFoundation.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, UpdatedAt: now,
	})
	require.NoError(t, err)
	// Establish a previous successful snapshot so plan and failure tests can
	// prove that neither state nor output provenance is accidentally replaced.
	appliedRevision := revision.ID
	if action != "destroy" {
		previous, err := db.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{
			ID: uuid.New(), FoundationID: templateFoundation.ID, OrganizationID: org.ID, FileID: file.ID, Source: "previous generic HCL", CreatedBy: user.ID, CreatedAt: now,
		})
		require.NoError(t, err)
		appliedRevision = previous.ID
	}
	state, err = db.CompleteTemplateResourceRun(ctx, database.CompleteTemplateResourceRunParams{
		FoundationID: templateFoundation.ID, RunID: state.ActiveRunID, Status: "applied",
		State:   templateResourceTestState(`{"old":{"sensitive":false,"value":"previous"}}`, `[]`),
		Outputs: json.RawMessage(`{"old":"previous"}`), AppliedRevisionID: uuid.NullUUID{UUID: appliedRevision, Valid: true},
		CheckpointRevisionID: uuid.NullUUID{UUID: appliedRevision, Valid: true}, UpdatedAt: now,
	})
	require.NoError(t, err)
	state, err = db.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{
		FoundationID: templateFoundation.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, UpdatedAt: now,
	})
	require.NoError(t, err)
	logger := testutil.Logger(t)
	s := &server{
		apiVersion: proto.CurrentVersion.String(), Tags: Tags{provisionersdk.TagTemplateResources: provisionersdk.TemplateResourcesCapabilityVersion},
		ID: daemon.ID, OrganizationID: org.ID, Database: dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), logger, nil),
		Pubsub: ps, Logger: logger, Tracer: trace.NewNoopTracerProvider().Tracer("test"), Clock: quartz.NewMock(t),
		activeJobs: map[uuid.UUID]struct{}{job.ID: {}}, lifecycleCtx: ctx,
	}
	return templateResourcesFixture{foundation: templateFoundation, server: s, job: job, run: run, state: state, revision: revision}
}

func TestTemplateResourcesJobLifecycle(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	applied := templateResourceTestState(`{"secret":{"sensitive":true,"value":"password"},"public":{"value":{"ids":["a","b"]}}}`, `[{"mode":"managed","instances":[{}]}]`)
	destroyed := templateResourceTestState(`{}`, `[]`)
	for _, tc := range []struct {
		name        string
		action      string
		result      templateResourcesJobResult
		cancel      bool
		wantStatus  string
		wantOutputs string
		wantState   []byte
		unknown     bool
	}{
		{name: "Apply", action: "apply", result: templateResourcesJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: applied}, wantStatus: "applied", wantOutputs: `{"secret":"password","public":{"ids":["a","b"]}}`, wantState: applied},
		{name: "Plan", action: "plan", result: templateResourcesJobResult{success: true, disposition: proto.StateDisposition_UNCHANGED, plan: []byte(`{"resource_changes":[]}`)}, wantStatus: "applied"},
		{name: "PlanWithoutPreview", action: "plan", result: templateResourcesJobResult{success: true, disposition: proto.StateDisposition_UNCHANGED}, wantStatus: "applied"},
		{name: "FailedPlan", action: "plan", result: templateResourcesJobResult{error: "plan failed"}, wantStatus: "applied"},
		{name: "PreApplyFailure", action: "apply", result: templateResourcesJobResult{error: "init failed", disposition: proto.StateDisposition_UNCHANGED}, wantStatus: "applied"},
		{name: "FailedApply", action: "apply", result: templateResourcesJobResult{error: "apply failed", state: applied, disposition: proto.StateDisposition_UNKNOWN}, wantStatus: "unknown", wantState: applied, unknown: true},
		{name: "FailedApplyReturnedCheckpoint", action: "apply", result: templateResourcesJobResult{error: "apply failed", state: applied, disposition: proto.StateDisposition_RETURNED}, wantStatus: "failed", wantState: applied},
		{name: "FailedApplyReturnedInvalidCheckpoint", action: "apply", result: templateResourcesJobResult{error: "apply failed", state: []byte(`{}`), disposition: proto.StateDisposition_RETURNED}, wantStatus: "unknown", wantState: []byte(`{}`), unknown: true},
		{name: "FailedApplyReturnedMissingCheckpoint", action: "apply", result: templateResourcesJobResult{error: "apply failed", disposition: proto.StateDisposition_RETURNED}, wantStatus: "unknown", unknown: true},
		{name: "CanceledFailedApply", action: "apply", cancel: true, result: templateResourcesJobResult{error: "apply failed", state: applied, disposition: proto.StateDisposition_RETURNED}, wantStatus: "unknown", wantState: applied, unknown: true},
		{name: "CanceledPreApplyFailure", action: "apply", cancel: true, result: templateResourcesJobResult{error: "canceled before apply", disposition: proto.StateDisposition_UNCHANGED}, wantStatus: "applied"},
		{name: "TransportFailure", action: "apply", result: templateResourcesJobResult{error: "connection lost"}, wantStatus: "unknown", unknown: true},
		{name: "InvalidApplyState", action: "apply", result: templateResourcesJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: []byte(`{}`)}, wantStatus: "unknown", wantState: []byte(`{}`), unknown: true},
		{name: "CanceledApply", action: "apply", cancel: true, result: templateResourcesJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: applied}, wantStatus: "unknown", wantState: applied, unknown: true},
		{name: "Destroy", action: "destroy", result: templateResourcesJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: destroyed}, wantStatus: "destroyed", wantOutputs: `{}`, wantState: destroyed},
		{name: "DestroyManagedInstances", action: "destroy", result: templateResourcesJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: applied}, wantStatus: "failed", wantState: applied},
		{name: "FailedDestroyReturnedCheckpoint", action: "destroy", result: templateResourcesJobResult{error: "destroy failed", disposition: proto.StateDisposition_RETURNED, state: applied}, wantStatus: "failed", wantState: applied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTemplateResourcesFixture(t, db, ps, tc.action)
			ctx := t.Context()
			if tc.cancel {
				require.NoError(t, db.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{ID: f.job.ID, CanceledAt: sql.NullTime{Time: dbtime.Now(), Valid: true}}))
			}
			if tc.result.success {
				_, err := f.server.CompleteJob(ctx, &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_TemplateResources_{TemplateResources: &proto.CompletedJob_TemplateResources{
					State: tc.result.state, StateDisposition: tc.result.disposition, Plan: tc.result.plan,
				}}})
				require.NoError(t, err)
			} else {
				failed := &proto.FailedJob{JobId: f.job.ID.String(), Error: tc.result.error}
				if tc.result.disposition != proto.StateDisposition_UNKNOWN || len(tc.result.state) != 0 {
					failed.Type = &proto.FailedJob_TemplateResources_{TemplateResources: &proto.FailedJob_TemplateResources{State: tc.result.state, StateDisposition: tc.result.disposition}}
				}
				_, err := f.server.FailJob(ctx, failed)
				require.NoError(t, err)
			}
			state, err := db.GetTemplateResourceState(ctx, f.state.FoundationID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, state.Status)
			require.Equal(t, tc.unknown, state.ActiveRunID.Valid)
			wantState := tc.wantState
			if wantState == nil {
				wantState = f.state.State
			}
			require.Equal(t, wantState, state.State)
			wantOutputs := tc.wantOutputs
			if wantOutputs == "" {
				wantOutputs = string(f.state.Outputs)
			}
			metadata := &sdkproto.Metadata{}
			require.NoError(t, populateFoundationOutputMetadata(state.Outputs, metadata))
			require.JSONEq(t, wantOutputs, metadata.TemplateOutputs)
			switch {
			case tc.wantStatus == "destroyed" || (tc.unknown && tc.wantState != nil):
				require.False(t, state.CheckpointRevisionID.Valid)
			case !tc.unknown && tc.result.disposition == proto.StateDisposition_RETURNED:
				require.Equal(t, uuid.NullUUID{UUID: f.revision.ID, Valid: true}, state.CheckpointRevisionID)
			default:
				require.Equal(t, f.state.CheckpointRevisionID, state.CheckpointRevisionID)
			}
			switch {
			case tc.wantStatus == "destroyed":
				require.False(t, state.AppliedRevisionID.Valid)
			case tc.result.success && tc.action == "apply" && !tc.unknown:
				require.Equal(t, uuid.NullUUID{UUID: f.revision.ID, Valid: true}, state.AppliedRevisionID)
			default:
				require.Equal(t, f.state.AppliedRevisionID, state.AppliedRevisionID)
			}
			storedRun, err := db.GetTemplateResourceRunByID(ctx, f.run.ID)
			require.NoError(t, err)
			if tc.action == "plan" && tc.result.success {
				preview, err := json.Marshal(provisionersdk.FoundationPlanPreview(tc.result.plan))
				require.NoError(t, err)
				require.True(t, storedRun.PlanPreview.Valid)
				require.JSONEq(t, string(preview), string(storedRun.PlanPreview.RawMessage))
			} else {
				require.False(t, storedRun.PlanPreview.Valid)
			}
			job, err := db.GetProvisionerJobByID(ctx, f.job.ID)
			require.NoError(t, err)
			require.True(t, job.CompletedAt.Valid)
			require.Equal(t, !tc.result.success || tc.unknown || tc.wantStatus == "failed", job.Error.Valid)
			if tc.unknown {
				require.Equal(t, templateResourcesStateUnknown, job.ErrorCode.String)
				_, err := db.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{FoundationID: state.FoundationID, RunID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, UpdatedAt: dbtime.Now()})
				require.ErrorIs(t, err, sql.ErrNoRows)
			}
			_, err = f.server.CompleteJob(ctx, &proto.CompletedJob{JobId: job.ID.String(), Type: &proto.CompletedJob_TemplateResources_{TemplateResources: &proto.CompletedJob_TemplateResources{State: applied, StateDisposition: proto.StateDisposition_RETURNED}}})
			require.ErrorContains(t, err, "late results are rejected")
			_, err = f.server.FailJob(ctx, &proto.FailedJob{JobId: job.ID.String(), Error: "late failure"})
			require.ErrorContains(t, err, "late results are rejected")
			resources, err := db.GetWorkspaceResourcesByJobID(ctx, job.ID)
			require.NoError(t, err)
			require.Empty(t, resources)
		})
	}
}

func TestTemplateResourcesAcquireFrozenRevision(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newTemplateResourcesFixture(t, db, ps, "apply")
	file := dbgen.File(t, db, database.File{CreatedBy: f.job.InitiatorID, Data: []byte("new source"), Hash: uuid.NewString()})
	revision, err := db.InsertTemplateResourceRevision(t.Context(), database.InsertTemplateResourceRevisionParams{
		ID: uuid.New(), FoundationID: f.run.FoundationID, OrganizationID: f.run.OrganizationID, FileID: file.ID, Source: "new revision", CreatedBy: f.job.InitiatorID, CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	_, err = db.SetTemplateResourceCurrentRevision(t.Context(), database.SetTemplateResourceCurrentRevisionParams{
		FoundationID: f.run.FoundationID, CurrentRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	payload, err := f.server.acquireProtoJob(dbauthz.AsProvisionerd(t.Context()), f.job)
	require.NoError(t, err)
	require.Equal(t, []byte("frozen source archive"), payload.TemplateSourceArchive)
	native := payload.GetTemplateResources()
	require.NotNil(t, native)
	require.Equal(t, f.revision.ID.String(), native.GetRevisionId())
	require.Equal(t, f.run.ID.String(), native.GetRunId())
	require.Equal(t, f.foundation.TemplateID.UUID.String(), native.GetTemplateId())
	require.Equal(t, f.foundation.TemplateID.UUID.String(), native.Metadata.TemplateId)
	require.NotEmpty(t, native.Metadata.TemplateName)
	require.Equal(t, f.foundation.ID.String(), native.FoundationId)
	require.Equal(t, f.foundation.ID.String(), native.Metadata.FoundationId)
	require.NotEqual(t, native.Metadata.FoundationId, native.Metadata.TemplateId)
	require.Equal(t, f.state.State, native.State)
	require.Empty(t, native.VariableValues)
	require.Equal(t, proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, native.Operation)
}

func TestTemplateResourcesAcquireFailureReleasesReservation(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newTemplateResourcesFixture(t, db, ps, "apply")
	state := make([]byte, drpcsdk.MaxMessageSize)
	_, err := db.UpdateTemplateResourceState(t.Context(), database.UpdateTemplateResourceStateParams{
		FoundationID: f.state.FoundationID, RunID: f.state.ActiveRunID, State: state, Status: f.state.Status, UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	_, err = f.server.acquireProtoJob(dbauthz.AsProvisionerd(t.Context()), f.job)
	require.ErrorContains(t, err, "payload was too big")
	current, err := db.GetTemplateResourceState(t.Context(), f.state.FoundationID)
	require.NoError(t, err)
	require.Equal(t, state, current.State)
	require.False(t, current.ActiveRunID.Valid)
	require.Equal(t, f.state.Status, current.Status)
	job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
	require.NoError(t, err)
	require.True(t, job.CompletedAt.Valid)
}

func TestTemplateResourcesFencing(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, tc := range []string{"Reservation", "Owner", "Reaped"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			f := newTemplateResourcesFixture(t, db, ps, "apply")
			wantError := "reservation is stale"
			switch tc {
			case "Reservation":
				_, err := db.CompleteTemplateResourceRun(t.Context(), database.CompleteTemplateResourceRunParams{
					FoundationID: f.state.FoundationID, RunID: f.state.ActiveRunID, Status: f.state.Status, State: f.state.State, Outputs: f.state.Outputs, AppliedRevisionID: f.state.AppliedRevisionID, CheckpointRevisionID: f.state.CheckpointRevisionID, UpdatedAt: dbtime.Now(),
				})
				require.NoError(t, err)
			case "Owner":
				f.server.Database = networkPolicyChangedOwnerStore{Store: f.server.Database}
				wantError = "don't own"
			case "Reaped":
				require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(t.Context(), database.UpdateProvisionerJobWithCompleteByIDParams{
					ID: f.job.ID, CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, UpdatedAt: dbtime.Now(), ErrorCode: sql.NullString{String: templateResourcesStateUnknown, Valid: true},
				}))
				wantError = "late results are rejected"
			}
			_, err := f.server.CompleteJob(t.Context(), &proto.CompletedJob{JobId: f.job.ID.String(), Type: &proto.CompletedJob_TemplateResources_{TemplateResources: &proto.CompletedJob_TemplateResources{
				State: templateResourceTestState(`{}`, `[]`), StateDisposition: proto.StateDisposition_RETURNED,
			}}})
			require.ErrorContains(t, err, wantError)
			_, err = f.server.FailJob(t.Context(), &proto.FailedJob{
				JobId: f.job.ID.String(), Error: "stale",
				Type: &proto.FailedJob_TemplateResources_{TemplateResources: &proto.FailedJob_TemplateResources{
					State: templateResourceTestState(`{}`, `[]`), StateDisposition: proto.StateDisposition_RETURNED,
				}},
			})
			require.ErrorContains(t, err, wantError)
			_, err = f.server.acquireTemplateResourcesJob(dbauthz.AsProvisionerd(t.Context()), f.job)
			require.Error(t, err)
			state, err := db.GetTemplateResourceState(t.Context(), f.state.FoundationID)
			require.NoError(t, err)
			require.Equal(t, f.state.State, state.State)
			require.Equal(t, f.state.Outputs, state.Outputs)
		})
	}
}

func TestTemplateResourcesDeliveryFailure(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newTemplateResourcesFixture(t, db, ps, "apply")
	f.server.failAcquiredJob(f.server.Logger, f.job.ID, "connection lost during delivery", proto.StateDisposition_UNKNOWN)
	state, err := db.GetTemplateResourceState(t.Context(), f.state.FoundationID)
	require.NoError(t, err)
	require.Equal(t, "unknown", state.Status)
	require.Equal(t, f.state.ActiveRunID, state.ActiveRunID)
	require.Equal(t, f.state.State, state.State)
	job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
	require.NoError(t, err)
	require.True(t, job.CompletedAt.Valid)
	require.Equal(t, templateResourcesStateUnknown, job.ErrorCode.String)
}

func TestTemplateResourcesNativeUpdates(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newTemplateResourcesFixture(t, db, ps, "plan")
	_, err := f.server.UpdateJob(t.Context(), &proto.UpdateJobRequest{
		JobId: f.job.ID.String(), Readme: []byte("ignored"), WorkspaceTags: map[string]string{"ignored": "tag"},
		TemplateVariables: []*sdkproto.TemplateVariable{{Name: "ignored"}},
	})
	require.NoError(t, err)
	_, err = f.server.CommitQuota(t.Context(), &proto.CommitQuotaRequest{JobId: f.job.ID.String()})
	require.ErrorContains(t, err, "do not use workspace quota")
}

func TestTemplateResourcesValidationOutputs(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	f := newTemplateResourcesFixture(t, db, ps, "apply")
	ctx := dbauthz.AsProvisionerd(t.Context())
	metadata := &sdkproto.Metadata{}
	require.NoError(t, f.server.templateValidationMetadata(ctx, f.foundation.TemplateID, metadata))
	require.JSONEq(t, string(f.state.Outputs), metadata.TemplateOutputs)
	require.Equal(t, f.state.AppliedRevisionID.UUID.String(), metadata.TemplateResourceRevisionId)
	metadata = &sdkproto.Metadata{}
	require.NoError(t, f.server.templateValidationMetadata(ctx, uuid.NullUUID{}, metadata))
	require.Equal(t, "{}", metadata.TemplateOutputs)
	require.Empty(t, metadata.TemplateResourceRevisionId)
}

// Force the CAS to fail after the reservation check to prove the job and state
// update are rolled back together rather than completing a fenced-out worker.
type templateResourcesCASFailureStore struct{ database.Store }

func (s templateResourcesCASFailureStore) InTx(fn func(database.Store) error, options *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error { return fn(templateResourcesCASFailureStore{Store: tx}) }, options)
}

func (templateResourcesCASFailureStore) CompleteTemplateResourceRun(context.Context, database.CompleteTemplateResourceRunParams) (database.TemplateResourceState, error) {
	return database.TemplateResourceState{}, sql.ErrNoRows
}

func TestTemplateResourcesCASFailure(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	for _, action := range []string{"apply", "plan"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			f := newTemplateResourcesFixture(t, db, ps, action)
			f.server.Database = templateResourcesCASFailureStore{Store: f.server.Database}
			result := templateResourcesJobResult{success: true, disposition: proto.StateDisposition_RETURNED, state: templateResourceTestState(`{}`, `[]`)}
			if action == "plan" {
				result = templateResourcesJobResult{success: true, disposition: proto.StateDisposition_UNCHANGED, plan: []byte(`{"resource_changes":[]}`)}
			}
			err := f.server.finalizeTemplateResourcesJob(dbauthz.AsProvisionerd(t.Context()), f.job.ID, result)
			require.ErrorIs(t, err, sql.ErrNoRows)
			job, err := db.GetProvisionerJobByID(t.Context(), f.job.ID)
			require.NoError(t, err)
			require.False(t, job.CompletedAt.Valid)
			state, err := db.GetTemplateResourceState(t.Context(), f.state.FoundationID)
			require.NoError(t, err)
			require.Equal(t, f.state, state)
			run, err := db.GetTemplateResourceRunByID(t.Context(), f.run.ID)
			require.NoError(t, err)
			require.False(t, run.PlanPreview.Valid, "preview and terminal state must commit together")
		})
	}
}
