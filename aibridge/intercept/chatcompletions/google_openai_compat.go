package chatcompletions

import (
	"encoding/json"
	"slices"

	"github.com/openai/openai-go/v3/option"

	"github.com/coder/coder/v2/internal/googleopenai"
)

func (i *interceptionBase) chatCompletionRequestBody() ([]byte, error) {
	body, err := json.Marshal(i.req.ChatCompletionNewParams)
	if err != nil {
		return nil, err
	}
	if !googleopenai.ShouldPatchGoogleUpstreamRequest(i.cfg.BaseURL) {
		return body, nil
	}
	patched, _, err := googleopenai.PatchThoughtSignatures(body)
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
