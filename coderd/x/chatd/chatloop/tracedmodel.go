package chatloop

import (
	"context"
	"errors"

	"charm.land/fantasy"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// TracedModelOptions describes the model calls TraceModelCalls records.
type TracedModelOptions struct {
	StageModel StageModel
	// Identity attributes calls whose context carries none, since some
	// callers run outside any chat context.
	Identity ChatIdentity
	// JSONObjects reports that the client requests GenerateObject output
	// in the provider's native JSON format. Clients that generate objects
	// through a forced tool call or text parsing do not.
	JSONObjects bool
}

// TraceModelCalls wraps model so each non-streaming Generate and
// GenerateObject call records a chat span. Streaming calls pass through:
// GenerateAssistant and compaction trace their own streams.
func (t *StageTracer) TraceModelCalls(model fantasy.LanguageModel, opts TracedModelOptions) fantasy.LanguageModel {
	if t == nil || model == nil {
		return model
	}
	return &tracedModel{LanguageModel: model, tracer: t, opts: opts}
}

type tracedModel struct {
	fantasy.LanguageModel
	tracer *StageTracer
	opts   TracedModelOptions
}

func (m *tracedModel) start(ctx context.Context) (context.Context, *StageSpan) {
	if _, ok := chatIdentityFromContext(ctx); !ok {
		ctx = ContextWithChatIdentity(ctx, m.opts.Identity)
	}
	return m.tracer.StartModelCall(ctx, m.opts.StageModel, false)
}

func (m *tracedModel) Generate(ctx context.Context, call fantasy.Call) (resp *fantasy.Response, err error) {
	ctx, span := m.start(ctx)
	span.setRequestParams(call.MaxOutputTokens, call.Temperature)
	defer func() {
		inference := InferenceResponse{}
		if resp != nil {
			inference.ResponseID = metadataResponseID(resp.ProviderMetadata)
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
	if m.opts.JSONObjects {
		span.SetAttributes(semconv.GenAIOutputTypeJSON)
	}
	defer func() {
		inference := InferenceResponse{}
		var noObject *fantasy.NoObjectGeneratedError
		switch {
		case resp != nil:
			inference.ResponseID = metadataResponseID(resp.ProviderMetadata)
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

// endModelCall records the response of a model call span and ends it.
// The finish reason stays the one the model reported, even when the caller
// rejects the response; err sets the span status and error.type.
func endModelCall(span *StageSpan, inference InferenceResponse, err error) {
	span.SetInferenceResponse(inference)
	span.End(err)
}
