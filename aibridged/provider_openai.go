package aibridged

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"golang.org/x/xerrors"
)

var _ Provider = &OpenAIProvider{}

// OpenAIProvider allows for interactions with the Chat Completions API.
// See https://platform.openai.com/docs/api-reference/chat.
type OpenAIProvider struct {
	baseURL, key string
}

func NewOpenAIProvider(baseURL, key string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: baseURL,
		key:     key,
	}
}

func (p *OpenAIProvider) Identifier() string {
	return ProviderOpenAI
}

func (p *OpenAIProvider) CreateSession(w http.ResponseWriter, r *http.Request, tools ToolRegistry) (Session, error) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, xerrors.Errorf("read body: %w", err)
	}

	switch r.URL.Path {
	case "/v1/chat/completions":
		var req ChatCompletionNewParamsWrapper
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, xerrors.Errorf("unmarshal request body: %w", err)
		}

		if req.Stream {
			return NewOpenAIStreamingChatSession(&req, p.baseURL, p.key), nil
		} else {
			return NewOpenAIBlockingChatSession(&req, p.baseURL, p.key), nil
		}
	}

	return nil, UnknownRoute
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
