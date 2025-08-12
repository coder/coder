package aibridged

import (
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	ant_constant "github.com/anthropics/anthropic-sdk-go/shared/constant"
)

func getAnthropicErrorResponse(err error) *AnthropicErrorResponse {
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		return nil
	}

	msg := apierr.Error()
	typ := string(ant_constant.ValueOf[ant_constant.APIError]())

	var detail *anthropic.BetaAPIError
	if field, ok := apierr.JSON.ExtraFields["error"]; ok {
		_ = json.Unmarshal([]byte(field.Raw()), &detail)
	}
	if detail != nil {
		msg = detail.Message
		typ = string(detail.Type)
	}

	return &AnthropicErrorResponse{
		BetaErrorResponse: &anthropic.BetaErrorResponse{
			Error: anthropic.BetaErrorUnion{
				Message: msg,
				Type:    typ,
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
