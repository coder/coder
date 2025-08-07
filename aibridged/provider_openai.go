package aibridged

import (
	"encoding/json"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"golang.org/x/xerrors"
)

type OpenAIProvider struct {
	baseURL, key string
}

func NewOpenAIProvider(baseURL, key string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: baseURL,
		key:     key,
	}
}

func (*OpenAIProvider) ParseRequest(payload []byte) (*ChatCompletionNewParamsWrapper, error) {
	var in ChatCompletionNewParamsWrapper
	if err := json.Unmarshal(payload, &in); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal request: %w", err)
	}

	return &in, nil
}

func (p *OpenAIProvider) NewAsynchronousSession(req *ChatCompletionNewParamsWrapper) Session[ChatCompletionNewParamsWrapper] {
	return &OpenAIStreamingSession{}
}
func (p *OpenAIProvider) NewSynchronousSession(req *ChatCompletionNewParamsWrapper) Session[ChatCompletionNewParamsWrapper] {
	panic("not implemented")

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

	opts = append(opts, option.WithMiddleware(LoggingMiddleware))

	return openai.NewClient(opts...)
}
