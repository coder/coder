package provisionerd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/provisionerd"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerdNetworkPolicy(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		operation  proto.NetworkPolicyOperation
		applyError string
	}{
		{name: "Plan", operation: proto.NetworkPolicyOperation_PLAN},
		{name: "Apply", operation: proto.NetworkPolicyOperation_APPLY},
		{name: "Destroy", operation: proto.NetworkPolicyOperation_DESTROY},
		{name: "InterruptedDestroy", operation: proto.NetworkPolicyOperation_DESTROY, applyError: "canceled"},
		{name: "PartialStateOnCancellation", operation: proto.NetworkPolicyOperation_APPLY, applyError: "canceled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			done := make(chan struct{})
			t.Cleanup(func() { close(done) })
			ctx := testutil.Context(t, testutil.WaitLong)
			completed := make(chan *proto.CompletedJob, 1)
			failed := make(chan *proto.FailedJob, 1)
			state := []byte("previous policy state")
			returnedState := []byte("updated policy state")
			transition := sdkproto.WorkspaceTransition_START
			if tt.operation == proto.NetworkPolicyOperation_DESTROY {
				transition = sdkproto.WorkspaceTransition_DESTROY
				returnedState = []byte(`{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`)
			}
			archive := testutil.CreateTar(t, map[string]string{"main.tf": ""})
			variables := []*sdkproto.VariableValue{{Name: "namespace", Value: "tenant"}}
			job := &proto.AcquiredJob{
				JobId: uuid.NewString(), Provisioner: "policy", TemplateSourceArchive: archive,
				Type: &proto.AcquiredJob_NetworkPolicy_{NetworkPolicy: &proto.AcquiredJob_NetworkPolicy{
					Operation: tt.operation, State: state, VariableValues: variables,
					RunId: new(uuid.NewString()), TargetId: new(uuid.NewString()), RevisionId: new(uuid.NewString()),
				}},
			}
			acq := newAcquireOne(t, job)
			var planSession *provisionersdk.Session
			var applied bool
			p := createProvisionerClient(t, done, provisionerTestServer{
				init: func(s *provisionersdk.Session, req *provisionersdk.InitRequest, _ <-chan struct{}) *sdkproto.InitComplete {
					assert.True(t, protobuf.Equal(&sdkproto.Config{}, s.Config))
					assert.True(t, req.OmitModuleFiles)
					assert.Equal(t, archive, req.TemplateSourceArchive)
					assert.Empty(t, req.ModuleArchive)
					return &sdkproto.InitComplete{}
				},
				plan: func(s *provisionersdk.Session, req *sdkproto.PlanRequest, _ <-chan struct{}) *sdkproto.PlanComplete {
					planSession = s
					assert.Equal(t, state, req.State)
					if assert.Len(t, req.VariableValues, 1) {
						assert.True(t, protobuf.Equal(variables[0], req.VariableValues[0]))
					}
					assert.True(t, protobuf.Equal(&sdkproto.Metadata{WorkspaceTransition: transition}, req.Metadata))
					assert.Empty(t, req.RichParameterValues)
					assert.Empty(t, req.ExternalAuthProviders)
					assert.Empty(t, req.PreviousParameterValues)
					// Policy jobs never commit workspace quota, even if the
					// provisioner reports a nonzero cost.
					return &sdkproto.PlanComplete{Plan: []byte(`{"resource_changes":[]}`), DailyCost: 42}
				},
				apply: func(s *provisionersdk.Session, req *sdkproto.ApplyRequest, _ <-chan struct{}) *sdkproto.ApplyComplete {
					applied = true
					assert.Same(t, planSession, s)
					assert.True(t, protobuf.Equal(&sdkproto.Metadata{WorkspaceTransition: transition}, req.Metadata))
					s.ProvisionLog(sdkproto.LogLevel_INFO, "policy apply log")
					return &sdkproto.ApplyComplete{State: returnedState, Error: tt.applyError}
				},
				parse: func(_ *provisionersdk.Session, _ *sdkproto.ParseRequest, _ <-chan struct{}) *sdkproto.ParseComplete {
					t.Error("policy job must not parse workspace parameters")
					return &sdkproto.ParseComplete{}
				},
				graph: func(_ *provisionersdk.Session, _ *sdkproto.GraphRequest, _ <-chan struct{}) *sdkproto.GraphComplete {
					t.Error("policy job must not graph workspace resources")
					return &sdkproto.GraphComplete{}
				},
			})
			daemon := createProvisionerd(t, func(context.Context) (proto.DRPCProvisionerDaemonClient, error) {
				return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
					acquireJobWithCancel: acq.acquireWithCancel,
					updateJob: func(_ context.Context, req *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
						assert.Empty(t, req.TemplateVariables)
						assert.Empty(t, req.WorkspaceTags)
						return &proto.UpdateJobResponse{}, nil
					},
					completeJob: func(_ context.Context, req *proto.CompletedJob) (*proto.Empty, error) {
						completed <- req
						return &proto.Empty{}, nil
					},
					failJob: func(_ context.Context, req *proto.FailedJob) (*proto.Empty, error) {
						failed <- req
						return &proto.Empty{}, nil
					},
					commitQuota: func(context.Context, *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error) {
						t.Error("policy job must not commit workspace quota")
						return &proto.CommitQuotaResponse{}, nil
					},
				}), nil
			}, provisionerd.LocalProvisioners{"policy": p})
			select {
			case result := <-completed:
				require.Empty(t, tt.applyError)
				require.Equal(t, job.JobId, result.JobId)
				policy := result.GetNetworkPolicy()
				require.NotNil(t, policy)
				require.JSONEq(t, `{"resource_changes":[]}`, string(policy.Plan))
				if tt.operation == proto.NetworkPolicyOperation_PLAN {
					require.Equal(t, proto.StateDisposition_UNCHANGED, policy.StateDisposition)
					require.Empty(t, policy.State)
				} else {
					require.Equal(t, proto.StateDisposition_RETURNED, policy.StateDisposition)
					require.Equal(t, returnedState, policy.State)
				}
			case result := <-failed:
				require.NotEmpty(t, tt.applyError, result.Error)
				require.Contains(t, result.Error, tt.applyError)
				require.NotNil(t, result.GetNetworkPolicy())
				require.Equal(t, proto.StateDisposition_UNKNOWN, result.GetNetworkPolicy().StateDisposition)
				require.Equal(t, returnedState, result.GetNetworkPolicy().State)
			case <-ctx.Done():
				t.Fatal("network policy job did not finish")
			}
			require.NoError(t, daemon.Close())
			require.Equal(t, tt.operation != proto.NetworkPolicyOperation_PLAN, applied)
		})
	}
}
