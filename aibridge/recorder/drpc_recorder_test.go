package recorder_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

// tokenUsageClient keeps the last token usage RPC. Its nil embedded client
// makes unexpected calls fail.
type tokenUsageClient struct {
	proto.DRPCRecorderClient
	got *proto.RecordTokenUsageRequest
}

func (c *tokenUsageClient) RecordTokenUsage(_ context.Context, in *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
	c.got = in
	return &proto.RecordTokenUsageResponse{}, nil
}

func TestDRPCRecorderRecordTokenUsage(t *testing.T) {
	t.Parallel()

	client := &tokenUsageClient{}
	rec := recorder.NewDRPCRecorder(func(context.Context) (proto.DRPCRecorderClient, error) {
		return client, nil
	})
	require.NoError(t, rec.RecordTokenUsage(t.Context(), &recorder.TokenUsageRecord{
		InterceptionID: uuid.NewString(),
		Output:         10,
		PromptTokens:   1001,
		CreatedAt:      time.Now(),
	}))
	require.EqualValues(t, 10, client.got.GetOutputTokens())
	require.EqualValues(t, 1001, client.got.GetPromptTokens())
}
