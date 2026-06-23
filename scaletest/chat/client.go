package chat

import (
	"context"
	"io"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

type chatClient interface {
	SetLogger(logger slog.Logger)
	SetLogBodies(logBodies bool)
	CreateChat(ctx context.Context, req codersdk.CreateChatRequest) (codersdk.Chat, error)
	StreamChat(ctx context.Context, chatID uuid.UUID, opts *codersdk.StreamChatOptions) (<-chan codersdk.ChatStreamEvent, io.Closer, error)
	CreateChatMessage(ctx context.Context, chatID uuid.UUID, req codersdk.CreateChatMessageRequest) (codersdk.CreateChatMessageResponse, error)
	UpdateChat(ctx context.Context, chatID uuid.UUID, req codersdk.UpdateChatRequest) error
}

var _ chatClient = (*codersdk.ExperimentalClient)(nil)

type chatModelConfigClient interface {
	ListChatModelConfigs(ctx context.Context) ([]codersdk.ChatModelConfig, error)
	CreateChatModelConfig(ctx context.Context, req codersdk.CreateChatModelConfigRequest) (codersdk.ChatModelConfig, error)
}

var _ chatModelConfigClient = (*codersdk.ExperimentalClient)(nil)
