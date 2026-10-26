package chatd

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
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

// Anthropic reports "block_binding: Extra inputs are not permitted" or names
// the rejected beta in an "Unexpected value(s)" error. Bedrock reports
// unsupported betas as "invalid beta flag".
var dropBlockRejections = []string{"block_binding", thinkingBindingBeta, "invalid beta flag"}

// isThinkingBindingError reports whether Anthropic rejected a replayed
// signed thinking block because the system prompt or tools changed since
// the block was produced.
func isThinkingBindingError(err error) bool {
	text, ok := badRequestText(err)
	return ok && strings.Contains(text, "bound to a different conversation")
}

func isDropBlockRejectedError(err error) bool {
	text, ok := badRequestText(err)
	return ok && slices.ContainsFunc(dropBlockRejections, func(pattern string) bool {
		return strings.Contains(text, pattern)
	})
}

func badRequestText(err error) (string, bool) {
	var pe *fantasy.ProviderError
	if !errors.As(err, &pe) || pe.StatusCode != http.StatusBadRequest {
		return "", false
	}
	return strings.ToLower(pe.Message + string(pe.ResponseBody)), true
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
		hints:         &p.thinkingDropBlock,
		key:           thinkingDropBlockKey{providerID: providerID, model: model.ModelID()},
		betas:         strings.Join(betas, ","),
	})
}

// thinkingDropBlockModel makes Anthropic drop stale signed thinking blocks.
// A hint for each provider and model picks whether to send drop_block first.
// When hinted, only a drop_block rejection retries, so other errors are not
// hidden behind a binding error.
type thinkingDropBlockModel struct {
	fantasy.LanguageModel
	logger slog.Logger
	hints  *sync.Map
	key    thinkingDropBlockKey
	betas  string
}

// Stream retries only when the rejection arrives before any output.
// fantasy reports the HTTP 400 as a stream part, not as the Stream error.
func (m *thinkingDropBlockModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	first, fallback := call, m.withDropBlock(call)
	retryable, updateHint := isThinkingBindingError, func() { m.hints.Store(m.key, struct{}{}) }
	_, hinted := m.hints.Load(m.key)
	if hinted {
		first, fallback = fallback, first
		retryable, updateHint = isDropBlockRejectedError, func() { m.hints.Delete(m.key) }
	}
	stream, err := m.LanguageModel.Stream(ctx, first)
	if err != nil {
		return nil, err
	}
	return func(yield func(fantasy.StreamPart) bool) {
		var rejection error
		started := false
		for part := range stream {
			if !started && part.Type == fantasy.StreamPartTypeError && retryable(part.Error) {
				rejection = part.Error
				break
			}
			started = started || part.Type != fantasy.StreamPartTypeWarnings
			if !yield(part) {
				return
			}
		}
		if rejection == nil {
			return
		}
		m.logger.Warn(ctx, "retrying model call with thinking drop_block toggled",
			slog.F("provider_id", m.key.providerID),
			slog.F("model", m.key.model),
			slog.F("drop_block", !hinted),
			slogError(rejection),
		)
		m.retryStream(ctx, fallback, updateHint, yield)
	}, nil
}

// retryStream streams the fallback request and calls updateHint when the
// fallback starts without an error.
func (m *thinkingDropBlockModel) retryStream(ctx context.Context, fallback fantasy.Call, updateHint func(), yield func(fantasy.StreamPart) bool) {
	stream, err := m.LanguageModel.Stream(ctx, fallback)
	if err != nil {
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: err})
		return
	}
	decided := false
	for part := range stream {
		if !decided && part.Type != fantasy.StreamPartTypeWarnings {
			decided = true
			if part.Type != fantasy.StreamPartTypeError {
				updateHint()
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
