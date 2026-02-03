package chats

import (
	"context"
	"net/http"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
)

type LLMClient interface {
	StreamChat(ctx context.Context, req LLMRequest) (aisdk.DataStream, error)
}

type LLMRequest struct {
	Model    string
	Messages []aisdk.Message
	Tools    []aisdk.Tool
}

type LLMFactory interface {
	New(provider string, httpClient *http.Client) (LLMClient, error)
}

const anthropicMaxTokens = int64(64000)

// EnvLLMFactory constructs LLM clients from environment variables.
//
// OpenAI:
//   - OPENAI_API_KEY
//
// Anthropic:
//   - ANTHROPIC_API_KEY
//
// Model selection is taken from the chat row; the factory does not default it.
type EnvLLMFactory struct{}

func (EnvLLMFactory) New(provider string, httpClient *http.Client) (LLMClient, error) {
	switch provider {
	case "openai":
		if os.Getenv("OPENAI_API_KEY") == "" {
			return nil, xerrors.New("OPENAI_API_KEY is not set")
		}
		c := openai.NewClient(option.WithHTTPClient(httpClient))
		return &openAILLM{client: c}, nil
	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, xerrors.New("ANTHROPIC_API_KEY is not set")
		}
		opts := anthropic.DefaultClientOptions()
		opts = append(opts, anthropicoption.WithAPIKey(key), anthropicoption.WithHTTPClient(httpClient))
		c := anthropic.NewClient(opts...)
		return &anthropicLLM{client: c}, nil
	default:
		return nil, xerrors.Errorf("unsupported provider: %q", provider)
	}
}

type openAILLM struct {
	client openai.Client
}

func (o *openAILLM) StreamChat(ctx context.Context, req LLMRequest) (aisdk.DataStream, error) {
	if req.Model == "" {
		return nil, xerrors.New("openai model is required")
	}
	msgs, err := aisdk.MessagesToOpenAI(req.Messages)
	if err != nil {
		return nil, xerrors.Errorf("convert messages to openai format: %w", err)
	}
	params := openai.ChatCompletionNewParams{
		Model:    req.Model,
		Messages: msgs,
		Tools:    aisdk.ToolsToOpenAI(req.Tools),
	}
	stream := o.client.Chat.Completions.NewStreaming(ctx, params)
	return aisdk.OpenAIToDataStream(stream), nil
}

type anthropicLLM struct {
	client anthropic.Client
}

func (a *anthropicLLM) StreamChat(ctx context.Context, req LLMRequest) (aisdk.DataStream, error) {
	if req.Model == "" {
		return nil, xerrors.New("anthropic model is required")
	}
	messages, system, err := aisdk.MessagesToAnthropic(req.Messages)
	if err != nil {
		return nil, xerrors.Errorf("convert messages to anthropic format: %w", err)
	}
	stream := a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: anthropicMaxTokens,
		System:    system,
		Messages:  messages,
		Tools:     aisdk.ToolsToAnthropic(req.Tools),
	})
	return aisdk.AnthropicToDataStream(stream), nil
}
