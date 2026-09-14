package chatcompletions

import (
	"encoding/json"
	"slices"

	"github.com/openai/openai-go/v3/option"

	"github.com/coder/coder/v2/coderd/x/googleopenai"
)

func (i *interceptionBase) chatCompletionRequestBody() ([]byte, error) {
	body, err := json.Marshal(i.req.ChatCompletionNewParams)
	if err != nil {
		return nil, err
	}
	if !googleopenai.ShouldPatchGoogleUpstreamRequest(i.cfg.BaseURL) {
		return body, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	// Reattach the extra_body passthrough dropped by the typed params so
	// Gemini settings such as thinking_config reach Google.
	if len(i.req.ExtraBody) > 0 {
		payload["extra_body"] = i.req.ExtraBody
	}
	googleopenai.AddThoughtSignaturesToLatestTurn(payload)
	patched, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return patched, nil
}

func (i *interceptionBase) chatCompletionRequestOptions(opts []option.RequestOption) ([]option.RequestOption, bool, error) {
	if !googleopenai.ShouldPatchGoogleUpstreamRequest(i.cfg.BaseURL) {
		return opts, false, nil
	}
	body, err := i.chatCompletionRequestBody()
	if err != nil {
		return nil, false, err
	}
	updated := slices.Clone(opts)
	return append(updated, option.WithRequestBody("application/json", body)), true, nil
}
