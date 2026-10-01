package aibridged

import (
	"context"

	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
)

// RecordPolicy is the deployment's choice of how AI Gateway records are
// handled before they are sent to coderd.
type RecordPolicy struct {
	// StructuredLogging makes each recorder emit AI Gateway interception
	// records in the format described by [recorder.InterceptionLogMarker].
	StructuredLogging bool
	// DisableContentRecording stops prompts, tool call arguments and model
	// thoughts from being recorded. Interceptions and token usage are still
	// recorded, so AI spend accounting and budget enforcement are unaffected.
	DisableContentRecording bool
}

// RecordPolicyFromConfig returns the record policy the deployment configured.
// Every construction site uses it, so the in-process daemon, the standalone
// gateway and the test harness cannot drift.
//
// It also reports a deployment that drops content records without exporting
// them anywhere: they never reach coderd, so if coderd is the only emitter the
// deployment has silently stopped exporting the very records it is declining to
// store. Nothing else reports this.
func RecordPolicyFromConfig(ctx context.Context, logger slog.Logger, cfg codersdk.AIBridgeConfig) RecordPolicy {
	policy := RecordPolicy{
		StructuredLogging:       cfg.EmitsStructuredLogs(codersdk.AIStructuredLoggingSourceGateway),
		DisableContentRecording: cfg.DisableContentRecording.Value(),
	}

	if policy.DisableContentRecording && !policy.StructuredLogging {
		logger.Warn(ctx, "content recording is disabled but structured logs are emitted by coderd, so prompts, tool calls and model thoughts will not be exported; set --ai-gateway-structured-logging-source to gateway or both to keep exporting them")
	}

	return policy
}

// Recorders creates the [recorder.Recorder] for each API key under one
// [RecordPolicy]. Everything except the API key is fixed when Recorders is
// created, so every caller records the same way.
type Recorders struct {
	logger     slog.Logger
	tracer     trace.Tracer
	clientFn   ClientFunc
	structured bool
	middleware []recorder.Middleware
}

// NewRecorders resolves policy once, so that [Recorders.For] only binds an API
// key. clientFn is called with each record call's context, because recorders
// outlive the request that created them.
func NewRecorders(logger slog.Logger, tracer trace.Tracer, policy RecordPolicy, clientFn ClientFunc) *Recorders {
	var middleware []recorder.Middleware
	if policy.DisableContentRecording {
		middleware = append(middleware, recorder.WithoutRecords(recorder.DisabledRecords{
			PromptUsage:  true,
			ToolUsage:    true,
			ModelThought: true,
		}))
	}
	return &Recorders{
		logger:     logger,
		tracer:     tracer,
		clientFn:   clientFn,
		structured: policy.StructuredLogging,
		middleware: middleware,
	}
}

// For returns the recorder for one API key. Records are logged and validated
// before the policy drops any, and every forwarded record acquires its client
// through the clientFn given to [NewRecorders].
func (r *Recorders) For(apiKeyID string) recorder.Recorder {
	base := recorder.NewDRPCRecorder(apiKeyID, func(ctx context.Context) (proto.DRPCRecorderClient, error) {
		return r.clientFn(ctx)
	})
	return aibridge.NewRecorder(r.logger, r.tracer, apiKeyID, r.structured, base, r.middleware...)
}
