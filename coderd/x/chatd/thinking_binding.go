package chatd

import (
	"errors"
	"maps"
	"net/http"
	"strings"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const (
	thinkingBindingBeta          = "thinking-binding-controls-2026-08-01"
	thinkingBindingDropBlock     = "drop_block"
	thinkingBindingDropBlockPath = "thinking.block_binding.prefix_mismatch_behavior"
)

// isThinkingBindingError reports whether Anthropic rejected a replayed
// signed thinking block because the system prompt or tools changed since
// the block was produced.
func isThinkingBindingError(err error) bool {
	var pe *fantasy.ProviderError
	if !errors.As(err, &pe) || pe.StatusCode != http.StatusBadRequest {
		return false
	}
	text := strings.ToLower(pe.Message + string(pe.ResponseBody))
	return strings.Contains(text, "bound to a different conversation")
}

type thinkingDropBlockKey struct {
	chatID        uuid.UUID
	modelConfigID uuid.UUID
}

// thinkingDropBlockEnabled reports whether earlier generations of the chat
// hit a thinking binding error with the given model config.
func (p *Server) thinkingDropBlockEnabled(chatID, modelConfigID uuid.UUID) bool {
	_, ok := p.thinkingDropBlock.Load(thinkingDropBlockKey{chatID: chatID, modelConfigID: modelConfigID})
	return ok
}

// enableThinkingDropBlock turns on drop_block for the chat and model config.
// It returns false when drop_block was already on, so each chat retries a
// binding error at most once per model config.
func (p *Server) enableThinkingDropBlock(chatID, modelConfigID uuid.UUID) bool {
	_, loaded := p.thinkingDropBlock.LoadOrStore(thinkingDropBlockKey{chatID: chatID, modelConfigID: modelConfigID}, struct{}{})
	return !loaded
}

// applyThinkingDropBlock makes Anthropic drop stale signed thinking blocks
// instead of rejecting the request. The body field and the beta header must
// travel together.
func (r *resolvedModelCall) applyThinkingDropBlock() {
	if !r.model.Valid() || chatprovider.NormalizeProvider(r.model.Provider()) != fantasyanthropic.Name {
		return
	}

	options := maps.Clone(r.providerOptions)
	if options == nil {
		options = fantasy.ProviderOptions{}
	}
	anthropicOptions := &fantasyanthropic.ProviderOptions{}
	if existing, ok := options[fantasyanthropic.Name].(*fantasyanthropic.ProviderOptions); ok && existing != nil {
		copied := *existing
		anthropicOptions = &copied
	}
	anthropicOptions.ExtraBody = maps.Clone(anthropicOptions.ExtraBody)
	if anthropicOptions.ExtraBody == nil {
		anthropicOptions.ExtraBody = map[string]any{}
	}
	if anthropicThinkingOmitted(anthropicOptions, r.model.ModelID()) {
		// Claude 5+ models think by default when the request omits the
		// thinking field, so an explicit adaptive config keeps behavior.
		anthropicOptions.ExtraBody["thinking"] = map[string]any{
			"type": "adaptive",
			"block_binding": map[string]any{
				"prefix_mismatch_behavior": thinkingBindingDropBlock,
			},
		}
	} else {
		anthropicOptions.ExtraBody[thinkingBindingDropBlockPath] = thinkingBindingDropBlock
	}
	options[fantasyanthropic.Name] = anthropicOptions
	r.providerOptions = options

	// Call headers replace client headers of the same name, so the value
	// must repeat the betas the client already sends.
	betas := []string{thinkingBindingBeta}
	if existing := chatprovider.BetaHeadersFromCallConfig(fantasyanthropic.Name, &r.clientCallConfig)[chatprovider.HeaderAnthropicBeta]; existing != "" {
		betas = []string{existing, thinkingBindingBeta}
	}
	headers := maps.Clone(r.headers)
	if headers == nil {
		headers = map[string]string{}
	}
	headers[chatprovider.HeaderAnthropicBeta] = strings.Join(betas, ",")
	r.headers = headers
}

// anthropicThinkingOmitted mirrors when fantasy leaves the thinking field
// out of an Anthropic request for a model that thinks by default.
func anthropicThinkingOmitted(options *fantasyanthropic.ProviderOptions, modelID string) bool {
	if options.Thinking != nil {
		return false
	}
	model := strings.ToLower(modelID)
	if options.Effort == nil {
		return !strings.Contains(model, "claude-mythos-preview")
	}
	return *options.Effort == fantasyanthropic.EffortNone &&
		(strings.Contains(model, "claude-mythos") || strings.Contains(model, "claude-fable"))
}
