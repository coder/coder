package chatloop

import (
	"context"
	"errors"

	"charm.land/fantasy"
)

// TraceModelCalls wraps model so each non-streaming Generate and
// GenerateObject call records a chat span. Streaming calls pass through:
// GenerateAssistant and compaction trace their own streams. identity
// attributes calls whose context carries none, since some callers run
// outside any chat context.
func (t *StageTracer) TraceModelCalls(model fantasy.LanguageModel, stageModel StageModel, identity ChatIdentity) fantasy.LanguageModel {
	if t == nil || model == nil {
		return model
	}
	return &tracedModel{LanguageModel: model, tracer: t, stageModel: stageModel, identity: identity}
}

type tracedModel struct {
	fantasy.LanguageModel
	tracer     *StageTracer
	stageModel StageModel
	identity   ChatIdentity
}

func (m *tracedModel) start(ctx context.Context) (context.Context, *StageSpan) {
	if _, ok := chatIdentityFromContext(ctx); !ok {
		ctx = ContextWithChatIdentity(ctx, m.identity)
	}
	return m.tracer.StartModelCall(ctx, m.stageModel, false)
}

func (m *tracedModel) Generate(ctx context.Context, call fantasy.Call) (resp *fantasy.Response, err error) {
	ctx, span := m.start(ctx)
	span.setRequestParams(call.MaxOutputTokens, call.Temperature)
	defer func() {
		inference := InferenceResponse{}
		if resp != nil {
			inference.FinishReason = resp.FinishReason
			inference.Usage = resp.Usage
		}
		endModelCall(span, inference, err)
	}()
	return m.LanguageModel.Generate(ctx, call)
}

func (m *tracedModel) GenerateObject(ctx context.Context, call fantasy.ObjectCall) (resp *fantasy.ObjectResponse, err error) {
	ctx, span := m.start(ctx)
	span.setRequestParams(call.MaxOutputTokens, call.Temperature)
	defer func() {
		inference := InferenceResponse{}
		var noObject *fantasy.NoObjectGeneratedError
		switch {
		case resp != nil:
			inference.FinishReason = resp.FinishReason
			inference.Usage = resp.Usage
		case errors.As(err, &noObject):
			// The model responded and billed tokens, but the output did
			// not parse.
			inference.FinishReason = noObject.FinishReason
			inference.Usage = noObject.Usage
		}
		endModelCall(span, inference, err)
	}()
	return m.LanguageModel.GenerateObject(ctx, call)
}

// endModelCall records the response of a model call span and ends it. The
// finish reason follows the call's outcome, so a response the caller
// rejects is not reported as a clean stop.
func endModelCall(span *StageSpan, inference InferenceResponse, err error) {
	if err != nil && inference.FinishReason != fantasy.FinishReasonContentFilter {
		inference.FinishReason = fantasy.FinishReasonError
	}
	span.SetInferenceResponse(inference)
	span.End(err)
}
