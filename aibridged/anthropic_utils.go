package aibridged

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	ant_constant "github.com/anthropics/anthropic-sdk-go/shared/constant"
)

// newAnthropicClient creates an Anthropic client with the given base URL and API key.
func newAnthropicClient(baseURL, key string, opts ...option.RequestOption) anthropic.Client {
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	opts = append(opts, option.WithAPIKey(key))
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return anthropic.NewClient(opts...)
}

func getAnthropicErrorResponse(err error) *AnthropicErrorResponse {
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		return nil
	}

	msg := apierr.Error()

	var detail *anthropic.BetaAPIError
	if field, ok := apierr.JSON.ExtraFields["error"]; ok {
		_ = json.Unmarshal([]byte(field.Raw()), &detail)
	}
	if detail != nil {
		msg = detail.Message
	}

	return &AnthropicErrorResponse{
		BetaErrorResponse: &anthropic.BetaErrorResponse{
			Error: anthropic.BetaErrorUnion{
				Message: msg,
				Type:    string(detail.Type),
			},
			Type: ant_constant.ValueOf[ant_constant.Error](),
		},
		StatusCode: apierr.StatusCode,
	}
}

type AnthropicErrorResponse struct {
	*anthropic.BetaErrorResponse

	StatusCode int `json:"-"`
}
