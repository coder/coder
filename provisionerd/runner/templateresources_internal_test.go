package runner

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func TestTemplateResourcesForcedFailureStateDisposition(t *testing.T) {
	t.Parallel()
	for _, operation := range []proto.TemplateResourceOperation{
		proto.TemplateResourceOperation_TEMPLATE_RESOURCE_PLAN,
		proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY,
		proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY,
	} {
		t.Run(operation.String(), func(t *testing.T) {
			t.Parallel()
			for _, applyStarted := range []bool{false, true} {
				updater := &forcedPolicyUpdater{}
				r := New(t.Context(), &proto.AcquiredJob{
					JobId: uuid.NewString(),
					Type:  &proto.AcquiredJob_TemplateResources_{TemplateResources: &proto.AcquiredJob_TemplateResources{Operation: operation}},
				}, Options{Updater: updater})
				// Re-evaluate disposition when delivering an earlier failure.
				failed := r.failedJobf("lost server connection")
				r.templateResourcesApplyStarted.Store(applyStarted)
				require.NoError(t, r.Fail(context.Background(), failed))
				result := updater.failed.GetTemplateResources()
				require.NotNil(t, result)
				want := proto.StateDisposition_UNCHANGED
				if applyStarted {
					want = proto.StateDisposition_UNKNOWN
				}
				require.Equal(t, want, result.StateDisposition)
				require.Empty(t, result.State)
				require.ErrorIs(t, r.notCanceled.Err(), context.Canceled)
			}
		})
	}
}

func TestTemplateResourcesRequestTransport(t *testing.T) {
	t.Parallel()
	for _, operation := range []proto.TemplateResourceOperation{
		proto.TemplateResourceOperation_TEMPLATE_RESOURCE_PLAN,
		proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY,
		proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY,
	} {
		t.Run(operation.String(), func(t *testing.T) {
			t.Parallel()
			metadata := &sdkproto.Metadata{
				TemplateId: uuid.NewString(), TemplateName: "shared-infrastructure",
				TemplateOutputs: `{"nested":{"items":[1,true,"a=b"]}}`, TemplateResourceRevisionId: uuid.NewString(),
				WorkspaceTransition: sdkproto.WorkspaceTransition_STOP,
			}
			original := protobuf.Clone(metadata)
			state := []byte(`{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`)
			variables := []*sdkproto.VariableValue{{Name: "region", Value: "somewhere"}, {Name: "credential", Value: "private", Sensitive: true}}
			job := &proto.AcquiredJob{
				JobId: uuid.NewString(), TemplateSourceArchive: []byte("arbitrary source archive"),
				Type: &proto.AcquiredJob_TemplateResources_{TemplateResources: &proto.AcquiredJob_TemplateResources{
					Operation: operation, State: state, VariableValues: variables, Metadata: metadata,
				}},
			}
			sess := &templateResourceSession{state: state}
			r := New(t.Context(), job, Options{Tracer: noop.NewTracerProvider().Tracer(t.Name())})
			defer r.cancel()
			defer r.stop()
			r.session = sess
			completed, failed := r.runTemplateResources(t.Context())
			require.Nil(t, failed)
			require.NotNil(t, completed)
			require.True(t, protobuf.Equal(original, metadata), "must not mutate acquired metadata")
			wantRequests := 4
			if operation == proto.TemplateResourceOperation_TEMPLATE_RESOURCE_PLAN {
				wantRequests = 3
			}
			require.Len(t, sess.requests, wantRequests, "no graph or parse requests")
			require.NotNil(t, sess.requests[0].GetConfig())
			init := sess.requests[1].GetInit()
			require.Equal(t, job.TemplateSourceArchive, init.TemplateSourceArchive)
			require.True(t, init.OmitModuleFiles)
			plan := sess.requests[2].GetPlan()
			require.Equal(t, state, plan.State)
			require.Equal(t, variables, plan.VariableValues)
			wantMetadata := protobuf.Clone(metadata).(*sdkproto.Metadata)
			wantMetadata.WorkspaceTransition = sdkproto.WorkspaceTransition_START
			if operation == proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY {
				wantMetadata.WorkspaceTransition = sdkproto.WorkspaceTransition_DESTROY
			}
			require.True(t, protobuf.Equal(wantMetadata, plan.Metadata))
			if wantRequests == 4 {
				require.True(t, protobuf.Equal(wantMetadata, sess.requests[3].GetApply().Metadata))
			}
		})
	}
}

func TestTemplateResourcesCanceledAfterPlan(t *testing.T) {
	t.Parallel()
	for _, force := range []bool{false, true} {
		name := "Cancel"
		if force {
			name = "Fail"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			updater := &forcedPolicyUpdater{}
			r := New(t.Context(), &proto.AcquiredJob{
				JobId: uuid.NewString(),
				Type: &proto.AcquiredJob_TemplateResources_{TemplateResources: &proto.AcquiredJob_TemplateResources{
					Operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY,
				}},
			}, Options{Updater: updater, Tracer: noop.NewTracerProvider().Tracer(t.Name())})
			defer r.cancel()
			defer r.stop()
			sess := &templateResourceSession{onPlanComplete: func() {
				if force {
					require.NoError(t, r.Fail(t.Context(), r.failedJobf("forced failure")))
				} else {
					r.Cancel()
				}
			}}
			r.session = sess
			completed, failed := r.runTemplateResources(t.Context())
			require.Nil(t, completed)
			require.Contains(t, failed.Error, "canceled before apply")
			require.Equal(t, proto.StateDisposition_UNCHANGED, failed.GetTemplateResources().StateDisposition)
			require.False(t, r.templateResourcesApplyStarted.Load())
			for _, request := range sess.requests {
				require.Nil(t, request.GetApply())
			}
		})
	}
}

func TestTemplateResourcesCanceledApplyResponse(t *testing.T) {
	t.Parallel()
	for _, operation := range []proto.TemplateResourceOperation{
		proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY,
		proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY,
	} {
		t.Run(operation.String(), func(t *testing.T) {
			t.Parallel()
			for _, applyError := range []string{"", "apply failed"} {
				t.Run("Error="+applyError, func(t *testing.T) {
					t.Parallel()
					state := []byte(`{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`)
					r := New(t.Context(), &proto.AcquiredJob{
						JobId: uuid.NewString(),
						Type: &proto.AcquiredJob_TemplateResources_{TemplateResources: &proto.AcquiredJob_TemplateResources{
							Operation: operation, State: state,
						}},
					}, Options{Tracer: noop.NewTracerProvider().Tracer(t.Name())})
					defer r.cancel()
					defer r.stop()
					r.session = &templateResourceSession{state: state, applyError: applyError, onApplyComplete: r.Cancel}
					completed, failed := r.runTemplateResources(t.Context())
					require.Nil(t, completed)
					require.NotNil(t, failed)
					require.Equal(t, proto.StateDisposition_UNKNOWN, failed.GetTemplateResources().StateDisposition)
					require.Equal(t, state, failed.GetTemplateResources().State)
				})
			}
		})
	}
}

func TestTemplateOutputSnapshotsInImportAndDryRun(t *testing.T) {
	t.Parallel()
	for _, dryRun := range []bool{false, true} {
		name := "Import"
		if dryRun {
			name = "DryRun"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			metadata := &sdkproto.Metadata{
				TemplateId: uuid.NewString(), TemplateName: "shared-infrastructure",
				TemplateOutputs: `{"network_id":"pinned","credential":"private-marker"}`, TemplateResourceRevisionId: uuid.NewString(),
				FoundationId: uuid.NewString(), FoundationName: "network",
				FoundationOutputMetadata: `{"network_id":{"type":"string","sensitive":false},"credential":{"type":"string","sensitive":true}}`,
				FoundationSnapshotId:     uuid.NewString(),
			}
			metadata.FoundationTemplateIds = []string{metadata.TemplateId}
			original := protobuf.Clone(metadata)
			job := &proto.AcquiredJob{JobId: uuid.NewString()}
			job.Type = &proto.AcquiredJob_TemplateImport_{TemplateImport: &proto.AcquiredJob_TemplateImport{Metadata: metadata}}
			if dryRun {
				job.Type = &proto.AcquiredJob_TemplateDryRun_{TemplateDryRun: &proto.AcquiredJob_TemplateDryRun{Metadata: metadata}}
			}
			sess := &templateResourceSession{}
			r := New(t.Context(), job, Options{
				Tracer: noop.NewTracerProvider().Tracer(t.Name()), Updater: templateSnapshotUpdater{}, LogDebounceInterval: time.Hour,
			})
			defer r.cancel()
			defer r.stop()
			defer func() {
				r.mutex.Lock()
				defer r.mutex.Unlock()
				if r.flushLogsTimer != nil {
					r.flushLogsTimer.Stop()
				}
			}()
			r.session = sess
			var completed *proto.CompletedJob
			var failed *proto.FailedJob
			if dryRun {
				completed, failed = r.runTemplateDryRun(t.Context())
			} else {
				completed, failed = r.runTemplateImport(t.Context())
			}
			require.Nil(t, failed)
			require.NotNil(t, completed)
			var transitions []sdkproto.WorkspaceTransition
			for _, request := range sess.requests {
				var actual *sdkproto.Metadata
				if plan := request.GetPlan(); plan != nil {
					actual = plan.Metadata
					transitions = append(transitions, actual.WorkspaceTransition)
				} else if graph := request.GetGraph(); graph != nil {
					actual = graph.Metadata
				} else {
					continue
				}
				require.Equal(t, metadata.TemplateOutputs, actual.TemplateOutputs)
				require.Equal(t, metadata.TemplateResourceRevisionId, actual.TemplateResourceRevisionId)
				require.Equal(t, metadata.TemplateId, actual.TemplateId)
				require.Equal(t, metadata.TemplateName, actual.TemplateName)
				require.Equal(t, metadata.FoundationId, actual.FoundationId)
				require.Equal(t, metadata.FoundationName, actual.FoundationName)
				require.Equal(t, metadata.FoundationTemplateIds, actual.FoundationTemplateIds)
				require.Equal(t, metadata.FoundationOutputMetadata, actual.FoundationOutputMetadata)
				require.Equal(t, metadata.FoundationSnapshotId, actual.FoundationSnapshotId)
			}
			wantTransitions := []sdkproto.WorkspaceTransition{sdkproto.WorkspaceTransition_START, sdkproto.WorkspaceTransition_STOP}
			if dryRun {
				wantTransitions = []sdkproto.WorkspaceTransition{sdkproto.WorkspaceTransition_START}
			}
			require.Equal(t, wantTransitions, transitions)
			if !dryRun {
				require.True(t, protobuf.Equal(original, metadata), "must not mutate acquired metadata")
			}
		})
	}
}

type templateSnapshotUpdater struct{ JobUpdater }

func (templateSnapshotUpdater) UpdateJob(context.Context, *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	return &proto.UpdateJobResponse{}, nil
}

type templateResourceSession struct {
	sdkproto.DRPCProvisioner_SessionClient
	state           []byte
	requests        []*sdkproto.Request
	onPlanComplete  func()
	onApplyComplete func()
	applyError      string
}

func (s *templateResourceSession) Send(req *sdkproto.Request) error {
	if req.GetCancel() != nil {
		return nil
	}
	s.requests = append(s.requests, req)
	return nil
}

func (s *templateResourceSession) Recv() (*sdkproto.Response, error) {
	switch s.requests[len(s.requests)-1].Type.(type) {
	case *sdkproto.Request_Init:
		return &sdkproto.Response{Type: &sdkproto.Response_Init{Init: &sdkproto.InitComplete{}}}, nil
	case *sdkproto.Request_Plan:
		if s.onPlanComplete != nil {
			s.onPlanComplete()
		}
		return &sdkproto.Response{Type: &sdkproto.Response_Plan{Plan: &sdkproto.PlanComplete{Plan: []byte("plan")}}}, nil
	case *sdkproto.Request_Apply:
		if s.onApplyComplete != nil {
			s.onApplyComplete()
		}
		return &sdkproto.Response{Type: &sdkproto.Response_Apply{Apply: &sdkproto.ApplyComplete{State: s.state, StateIsAuthoritative: true, Error: s.applyError}}}, nil
	case *sdkproto.Request_Parse:
		return &sdkproto.Response{Type: &sdkproto.Response_Parse{Parse: &sdkproto.ParseComplete{}}}, nil
	case *sdkproto.Request_Graph:
		return &sdkproto.Response{Type: &sdkproto.Response_Graph{Graph: &sdkproto.GraphComplete{}}}, nil
	default:
		return nil, xerrors.New("unexpected provisioner request")
	}
}
