package aibridged

import (
	"context"

	"storj.io/drpc"

	"github.com/coder/coder/v2/aibridged/proto"
)

// Define a common interface for AI services
type AIServiceClient interface {
	SendRequest(ctx context.Context, payload *proto.JSONPayload) (StreamingResponder[*proto.JSONPayload], error)
}

type StreamingResponder[T any] interface {
	drpc.Stream
	Recv() (T, error)
}

type OpenAIAdapter struct {
	client proto.DRPCOpenAIServiceClient
}

func NewOpenAIAdapter(client proto.DRPCOpenAIServiceClient) *OpenAIAdapter {
	return &OpenAIAdapter{client: client}
}

func (a *OpenAIAdapter) SendRequest(ctx context.Context, payload *proto.JSONPayload) (StreamingResponder[*proto.JSONPayload], error) {
	return a.client.ChatCompletions(ctx, payload)
}

type AnthropicAdapter struct {
	client proto.DRPCAnthropicServiceClient
}

func (a *AnthropicAdapter) SendRequest(ctx context.Context, payload *proto.JSONPayload) (StreamingResponder[*proto.JSONPayload], error) {
	return a.client.Messages(ctx, payload)
}
