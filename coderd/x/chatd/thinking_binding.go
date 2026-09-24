package chatd

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"strings"
	"sync"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
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
	providerID uuid.UUID
	model      string
}

// withThinkingDropBlock wraps Anthropic models in a thinkingDropBlockModel.
func (p *Server) withThinkingDropBlock(
	model chatprovider.Model,
	providerID uuid.UUID,
	callConfig codersdk.ChatModelCallConfig,
	chatID uuid.UUID,
) chatprovider.Model {
	if chatprovider.NormalizeProvider(model.Provider()) != fantasyanthropic.Name {
		return model
	}
	// Call headers replace client headers of the same name, so the value
	// must repeat the betas the client already sends.
	betas := []string{thinkingBindingBeta}
	if existing := chatprovider.BetaHeadersFromCallConfig(fantasyanthropic.Name, &callConfig)[chatprovider.HeaderAnthropicBeta]; existing != "" {
		betas = []string{existing, thinkingBindingBeta}
	}
	return model.WithLanguageModel(&thinkingDropBlockModel{
		LanguageModel: model.LanguageModel(),
		logger:        p.logger.With(slog.F("chat_id", chatID)),
		accepted:      &p.thinkingDropBlock,
		key:           thinkingDropBlockKey{providerID: providerID, model: model.ModelID()},
		betas:         strings.Join(betas, ","),
	})
}

// thinkingDropBlockModel makes Anthropic drop stale signed thinking blocks
// instead of rejecting the request. chatd rebuilds the system prompt and
// tools for every request, and some models bind signed thinking blocks to
// that prefix. A request that gets the binding error is sent again once
// with the drop_block control. After a provider and model accept the
// control, every request to them sends it.
type thinkingDropBlockModel struct {
	fantasy.LanguageModel
	logger   slog.Logger
	accepted *sync.Map
	key      thinkingDropBlockKey
	betas    string
}

// Stream retries only when the binding error arrives before any output.
// fantasy reports the HTTP 400 as a stream part, not as the Stream error.
func (m *thinkingDropBlockModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	if _, ok := m.accepted.Load(m.key); ok {
		return m.LanguageModel.Stream(ctx, m.withDropBlock(call))
	}
	stream, err := m.LanguageModel.Stream(ctx, call)
	if err != nil {
		return nil, err
	}
	return func(yield func(fantasy.StreamPart) bool) {
		var bindingErr error
		started := false
		for part := range stream {
			if !started && part.Type == fantasy.StreamPartTypeError && isThinkingBindingError(part.Error) {
				bindingErr = part.Error
				break
			}
			started = started || part.Type != fantasy.StreamPartTypeWarnings
			if !yield(part) {
				return
			}
		}
		if bindingErr != nil {
			m.retryStream(ctx, call, bindingErr, yield)
		}
	}, nil
}

// retryStream streams the call again with drop_block. It records the
// provider and model as accepting drop_block when the retry starts
// without an error.
func (m *thinkingDropBlockModel) retryStream(ctx context.Context, call fantasy.Call, bindingErr error, yield func(fantasy.StreamPart) bool) {
	m.logger.Warn(ctx, "retrying model call with thinking drop_block",
		slog.F("provider_id", m.key.providerID),
		slog.F("model", m.key.model),
		slogError(bindingErr),
	)
	stream, err := m.LanguageModel.Stream(ctx, m.withDropBlock(call))
	if err != nil {
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: err})
		return
	}
	decided := false
	for part := range stream {
		if !decided && part.Type != fantasy.StreamPartTypeWarnings {
			decided = true
			if part.Type != fantasy.StreamPartTypeError {
				m.accepted.Store(m.key, struct{}{})
			}
		}
		if !yield(part) {
			return
		}
	}
}

// withDropBlock adds the drop_block body field and the beta header, which
// must travel together.
func (m *thinkingDropBlockModel) withDropBlock(call fantasy.Call) fantasy.Call {
	options := maps.Clone(call.ProviderOptions)
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
	if anthropicThinkingOmitted(anthropicOptions, m.Model()) {
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
	call.ProviderOptions = options

	headers := maps.Clone(call.Headers)
	if headers == nil {
		headers = map[string]string{}
	}
	headers[chatprovider.HeaderAnthropicBeta] = m.betas
	call.Headers = headers
	return call
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
