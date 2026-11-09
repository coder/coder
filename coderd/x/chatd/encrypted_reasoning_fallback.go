package chatd

import (
	"context"
	"errors"
	"sync/atomic"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func isInvalidEncryptedContentError(err error) bool {
	var apiErr *openai.Error
	return errors.As(err, &apiErr) && apiErr.Code == "invalid_encrypted_content"
}

// withEncryptedReasoningFallback wraps OpenAI Responses models in an
// encryptedReasoningFallbackModel that sets rejected when it retries.
func (p *Server) withEncryptedReasoningFallback(model chatprovider.Model, chatID uuid.UUID, rejected *atomic.Bool) chatprovider.Model {
	if !model.Transport().UsesResponses() {
		return model
	}
	return model.WithLanguageModel(&encryptedReasoningFallbackModel{
		LanguageModel: model.LanguageModel(),
		logger:        p.logger.With(slog.F("chat_id", chatID)),
		rejected:      rejected,
	})
}

// encryptedReasoningFallbackModel retries a request without OpenAI reasoning
// when the provider rejects its encrypted content. Encrypted reasoning only
// decrypts for the OpenAI organization that produced it, which can change
// without a provider, model, or credential change, for example when an admin
// replaces the deployment key with one from another organization. The
// rejected reasoning stays in the chat history, so the step stamps its new
// reasoning with the next epoch, and later steps leave out reasoning from
// earlier epochs.
type encryptedReasoningFallbackModel struct {
	fantasy.LanguageModel
	logger   slog.Logger
	rejected *atomic.Bool
}

// Stream retries only when the rejection arrives before any output.
func (m *encryptedReasoningFallbackModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	stream, err := m.LanguageModel.Stream(ctx, call)
	if err != nil {
		return nil, err
	}
	return func(yield func(fantasy.StreamPart) bool) {
		var rejection error
		started := false
		for part := range stream {
			if !started && part.Type == fantasy.StreamPartTypeError && isInvalidEncryptedContentError(part.Error) {
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
		m.rejected.Store(true)
		m.logger.Warn(ctx, "retrying model call without rejected encrypted reasoning", slogError(rejection))
		call.Prompt = dropOpenAIReasoningParts(ctx, m.logger, call.Prompt)
		retry, err := m.LanguageModel.Stream(ctx, call)
		if err != nil {
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: err})
			return
		}
		for part := range retry {
			if !yield(part) {
				return
			}
		}
	}, nil
}
