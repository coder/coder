package aibridged

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"

	bridge "github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

type recorderClientStub struct {
	requests []*proto.RecordInterceptionRequest
}

func (*recorderClientStub) DRPCConn() drpc.Conn { return nil }
func (s *recorderClientStub) RecordInterception(_ context.Context, in *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
	s.requests = append(s.requests, in)
	return &proto.RecordInterceptionResponse{}, nil
}
func (*recorderClientStub) RecordInterceptionEnded(context.Context, *proto.RecordInterceptionEndedRequest) (*proto.RecordInterceptionEndedResponse, error) {
	panic("unexpected call")
}
func (*recorderClientStub) RecordTokenUsage(context.Context, *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
	panic("unexpected call")
}
func (*recorderClientStub) RecordPromptUsage(context.Context, *proto.RecordPromptUsageRequest) (*proto.RecordPromptUsageResponse, error) {
	panic("unexpected call")
}
func (*recorderClientStub) RecordToolUsage(context.Context, *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
	panic("unexpected call")
}
func (*recorderClientStub) RecordModelThought(context.Context, *proto.RecordModelThoughtRequest) (*proto.RecordModelThoughtResponse, error) {
	panic("unexpected call")
}

func TestDRPCRecorderAttribution(t *testing.T) {
	t.Parallel()

	client := &recorderClientStub{}
	recorder := &DRPCRecorder{apiKeyID: "test-key", client: client}
	workspaceID := uuid.New()
	record := &bridge.InterceptionRecord{ID: uuid.NewString(), InitiatorID: uuid.NewString()}

	// With workspace attribution.
	require.NoError(t, recorder.RecordInterception(
		aibridge.WithAttribution(t.Context(), aibridge.Attribution{WorkspaceID: workspaceID}),
		record,
	))
	// Without attribution.
	require.NoError(t, recorder.RecordInterception(t.Context(), record))

	require.Len(t, client.requests, 2)

	// Workspace ID is forwarded when attribution is set.
	require.Equal(t, workspaceID.String(), client.requests[0].GetWorkspaceId())

	// No workspace ID when attribution is absent.
	require.Empty(t, client.requests[1].GetWorkspaceId())
}
