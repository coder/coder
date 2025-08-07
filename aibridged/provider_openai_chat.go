package aibridged

import (
	"encoding/json"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"golang.org/x/xerrors"
)

var _ Provider[ChatCompletionNewParamsWrapper] = &OpenAIChatProvider{}

// OpenAIChatProvider allows for interactions with the Chat Completions API.
// See https://platform.openai.com/docs/api-reference/chat.
type OpenAIChatProvider struct {
	baseURL, key string
}

func NewOpenAIChatProvider(baseURL, key string) *OpenAIChatProvider {
	return &OpenAIChatProvider{
		baseURL: baseURL,
		key:     key,
	}
}

func (*OpenAIChatProvider) ParseRequest(payload []byte) (*ChatCompletionNewParamsWrapper, error) {
	var in ChatCompletionNewParamsWrapper
	if err := json.Unmarshal(payload, &in); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal request: %w", err)
	}

	return &in, nil
}

// NewStreamingSession creates a new session which handles streaming chat completions.
// See https://platform.openai.com/docs/api-reference/chat-streaming
func (*OpenAIChatProvider) NewStreamingSession(req *ChatCompletionNewParamsWrapper) Session {
	return NewOpenAIStreamingChatSession(req)
}

// NewBlockingSession creates a new session which handles non-streaming chat completions.
// See https://platform.openai.com/docs/api-reference/chat
func (*OpenAIChatProvider) NewBlockingSession(req *ChatCompletionNewParamsWrapper) Session {
	return NewOpenAIBlockingChatSession(req)

}

func newOpenAIClient(baseURL, key string) openai.Client {
	var opts []option.RequestOption
	if key == "" {
		key = os.Getenv("OPENAI_API_KEY")
	}
	opts = append(opts, option.WithAPIKey(key))
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return openai.NewClient(opts...)
}
