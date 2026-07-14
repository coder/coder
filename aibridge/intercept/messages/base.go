package messages

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibconfig "github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

// bedrockSupportedBetaFlags is the set of Anthropic-Beta flags that AWS Bedrock
// accepts. Flags not in this set cause a 400 "invalid beta flag" error.
//
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages-request-response.html
var bedrockSupportedBetaFlags = map[string]bool{
	// Supported on Claude 3.7 Sonnet.
	"computer-use-2025-01-24": true,
	// Supported on Claude 3.7 Sonnet and Claude 4+.
	"token-efficient-tools-2025-02-19": true,
	// Supported on Claude 4+ models.
	"interleaved-thinking-2025-05-14": true,
	// Supported on Claude 3.7 Sonnet.
	"output-128k-2025-02-19": true,
	// Supported on Claude 4+ models. Requires account team access.
	"dev-full-thinking-2025-05-14": true,
	// Supported on Claude Sonnet 4.
	"context-1m-2025-08-07": true,
	// Supported on Claude Sonnet 4.5 and Claude Haiku 4.5.
	// Enables context_management body field for thinking block clearing.
	"context-management-2025-06-27": true,
	// Supported on Claude Opus 4.5.
	// Enables output_config body field for effort control.
	"effort-2025-11-24": true,
	// Supported on Claude Opus 4.5.
	"tool-search-tool-2025-10-19": true,
	// Supported on Claude Opus 4.5.
	"tool-examples-2025-10-29": true,
}

// BedrockPRMUserAgent is Coder's AWS Partner Revenue Measurement (PRM)
// attribution marker for outbound Bedrock requests.
//
// It is appended to Bedrock User-Agent headers so AWS can recognize the
// traffic as Coder-associated Bedrock usage.
const BedrockPRMUserAgent = "sdk-ua-app-id/APN_1.1%2Fpc_cdfmjwn8i6u8l9fwz8h82e4w3%24"

// bedrockMantleSigningService is the AWS SigV4 service name for mantle.
const bedrockMantleSigningService = "bedrock-mantle"

func appendBedrockPRMUserAgent(req *http.Request) {
	if ua := req.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua+" "+BedrockPRMUserAgent)
	}
}

// BedrockRuntime carries everything a Bedrock-backed interception needs: the
// static Bedrock config plus the AWS credentials provider.
type BedrockRuntime struct {
	Cfg   aibconfig.AWSBedrock
	Creds aws.CredentialsProvider
}

type interceptionBase struct {
	id         uuid.UUID
	reqPayload RequestPayload

	cfg  intercept.Config
	cred intercept.Credential
	// bedrock is nil for non-Bedrock providers.
	bedrock *BedrockRuntime

	// clientHeaders are the original HTTP headers from the client request.
	clientHeaders http.Header

	logger slog.Logger
	tracer trace.Tracer

	recorder recorder.Recorder
	mcpProxy mcp.ServerProxier
}

func (i *interceptionBase) ID() uuid.UUID {
	return i.id
}

// Credential returns the credential resolved for this interception.
func (i *interceptionBase) Credential() intercept.Credential {
	return i.cred
}

func (i *interceptionBase) Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier) {
	i.logger = logger
	i.recorder = rec
	i.mcpProxy = mcpProxy
}

func (i *interceptionBase) CorrelatingToolCallID() *string {
	return i.reqPayload.correlatingToolCallID()
}

// isBedrockMantle reports whether the interception targets the Bedrock mantle
// protocol.
func (i *interceptionBase) isBedrockMantle() bool {
	return i.bedrock != nil && i.bedrock.Cfg.Protocol == aibconfig.BedrockProtocolMantle
}

// isBedrockInvokeModel reports whether the interception targets the Bedrock
// InvokeModel protocol.
func (i *interceptionBase) isBedrockInvokeModel() bool {
	return i.bedrock != nil &&
		(i.bedrock.Cfg.Protocol == "" || i.bedrock.Cfg.Protocol == aibconfig.BedrockProtocolInvokeModel)
}

func (i *interceptionBase) Model() string {
	if len(i.reqPayload) == 0 {
		return "coder-aibridge-unknown"
	}

	// InvokeModel is the only protocol that remaps the model: it replaces the
	// client's model with the operator-configured one. Every other case (mantle
	// passthrough, non-Bedrock providers) returns the model the client sent in
	// the body.
	if i.isBedrockInvokeModel() {
		model := i.bedrock.Cfg.Model
		if i.isSmallFastModel() {
			model = i.bedrock.Cfg.SmallFastModel
		}
		return model
	}

	return i.reqPayload.model()
}

func (i *interceptionBase) baseTraceAttributes(r *http.Request, streaming bool) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(tracing.RequestPath, r.URL.Path),
		attribute.String(tracing.InterceptionID, i.id.String()),
		attribute.String(tracing.InitiatorID, aibcontext.ActorIDFromContext(r.Context())),
		attribute.String(tracing.Provider, i.cfg.ProviderName),
		attribute.String(tracing.Model, i.Model()),
		attribute.Bool(tracing.Streaming, streaming),
		attribute.Bool(tracing.IsBedrock, i.bedrock != nil),
	}
}

func (i *interceptionBase) injectTools() {
	if i.mcpProxy == nil || !i.hasInjectableTools() {
		return
	}

	i.disableParallelToolCalls()

	// Inject tools.
	var injectedTools []anthropic.ToolUnionParam
	for _, tool := range i.mcpProxy.ListTools() {
		injectedTools = append(injectedTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: tool.Params,
					Required:   tool.Required,
				},
				Name:        tool.ID,
				Description: anthropic.String(tool.Description),
				Type:        anthropic.ToolTypeCustom,
			},
		})
	}

	// Prepend the injected tools in order to maintain any configured cache breakpoints.
	// The order of injected tools is expected to be stable, and therefore will not cause
	// any cache invalidation when prepended.
	updated, err := i.reqPayload.injectTools(injectedTools)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to set inject tools in request payload", slog.Error(err))
		return
	}
	i.reqPayload = updated
}

func (i *interceptionBase) disableParallelToolCalls() {
	// Note: Parallel tool calls are disabled to avoid tool_use/tool_result block mismatches.
	// https://github.com/coder/aibridge/issues/2
	updated, err := i.reqPayload.disableParallelToolCalls()
	if err != nil {
		i.logger.Warn(context.Background(), "failed to set tool_choice in request payload", slog.Error(err))
		return
	}
	i.reqPayload = updated
}

// extractModelThoughts returns any thinking blocks that were returned in the response.
func (*interceptionBase) extractModelThoughts(msg *anthropic.Message) []*recorder.ModelThoughtRecord {
	if msg == nil {
		return nil
	}

	var thoughtRecords []*recorder.ModelThoughtRecord
	for _, block := range msg.Content {
		// anthropic.RedactedThinkingBlock also exists, but there's nothing useful we can capture.
		variant, ok := block.AsAny().(anthropic.ThinkingBlock)
		if !ok || variant.Thinking == "" {
			continue
		}
		thoughtRecords = append(thoughtRecords, &recorder.ModelThoughtRecord{
			Content:  variant.Thinking,
			Metadata: recorder.Metadata{"source": recorder.ThoughtSourceThinking},
		})
	}
	return thoughtRecords
}

// IsSmallFastModel checks if the model is a small/fast model (Haiku 3.5).
// These models are optimized for tasks like code autocomplete and other small, quick operations.
// See `ANTHROPIC_SMALL_FAST_MODEL`: https://docs.anthropic.com/en/docs/claude-code/settings#environment-variables
// https://docs.claude.com/en/docs/claude-code/costs#background-token-usage
func (i *interceptionBase) isSmallFastModel() bool {
	return strings.Contains(i.reqPayload.model(), "haiku")
}

// newMessagesService builds the SDK service used for upstream calls.
func (i *interceptionBase) newMessagesService(ctx context.Context, opts ...option.RequestOption) (anthropic.MessageService, error) {
	// Only BYOK sets its credential here. Centralized keys are injected
	// per-attempt in the failover loop.
	if byok, ok := intercept.AsBYOK(i.cred); ok {
		i.logger.Debug(ctx, "using byok auth",
			slog.F("auth_header", byok.Header), slog.F("key_hint", byok.Hint()),
		)
		switch byok.Header {
		case intercept.AuthHeaderAuthorization:
			opts = append(opts, option.WithAuthToken(byok.Secret))
		case intercept.AuthHeaderXAPIKey:
			opts = append(opts, option.WithAPIKey(byok.Secret))
		default:
			return anthropic.MessageService{}, xerrors.Errorf("unexpected byok auth header: %q", byok.Header)
		}
	}
	opts = append(opts, option.WithBaseURL(i.cfg.BaseURL))

	// Forward client headers to upstream. This middleware runs after the SDK
	// has built the request, and replaces the outgoing headers with the sanitized
	// client headers plus provider auth.
	if i.clientHeaders != nil {
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			req.Header = intercept.BuildUpstreamHeaders(req.Header, i.clientHeaders, i.cred.AuthHeader())
			return next(req)
		}))
	}

	// Add API dump middleware if configured
	if mw := apidump.NewBridgeMiddleware(i.cfg.APIDumpDir, i.cfg.ProviderName, i.Model(), i.id, i.logger, quartz.NewReal()); mw != nil {
		opts = append(opts, option.WithMiddleware(mw))
	}

	// bedrockCredentialResolutionTimeout bounds the credential
	// resolution (STS/IRSA) shared by both Bedrock protocols.
	const bedrockCredentialResolutionTimeout = 30 * time.Second

	if i.isBedrockInvokeModel() {
		ctx, cancel := context.WithTimeout(ctx, bedrockCredentialResolutionTimeout)
		defer cancel()
		bedrockOpts, err := i.withBedrockInvokeModelOptions(ctx)
		if err != nil {
			return anthropic.MessageService{}, err
		}
		opts = append(opts, bedrockOpts...)
		i.augmentRequestForBedrockInvokeModel()
	}

	if i.isBedrockMantle() {
		ctx, cancel := context.WithTimeout(ctx, bedrockCredentialResolutionTimeout)
		defer cancel()
		bedrockOpts, err := i.withBedrockMantleOptions(ctx)
		if err != nil {
			return anthropic.MessageService{}, err
		}
		opts = append(opts, bedrockOpts...)
	}

	return anthropic.NewMessageService(opts...), nil
}

// withBody returns a per-request option that sends the current raw request
// payload as the request body. This is called for each API request so that the
// latest payload (including any messages appended during the agentic tool loop)
// is always sent.
func (i *interceptionBase) withBody() option.RequestOption {
	return option.WithRequestBody("application/json", []byte(i.reqPayload))
}

// withBedrockInvokeModelOptions returns request options for the AWS Bedrock
// InvokeModel protocol.
//
// Credentials come from i.bedrock.Creds. It is a shared credentials cache, so the per-request Retrieve()
// below is served from that cache and does not re-resolve or re-assume on every request.
func (i *interceptionBase) withBedrockInvokeModelOptions(ctx context.Context) ([]option.RequestOption, error) {
	if i.bedrock == nil {
		return nil, xerrors.New("nil bedrock runtime")
	}
	cfg := i.bedrock.Cfg
	if err := cfg.Validate(); err != nil {
		return nil, xerrors.Errorf("bedrock invoke-model config: %w", err)
	}

	// Fail fast: ensure credentials can be resolved before signing. Served from
	// the shared cache on most requests (no network); on the cold or refresh
	// path this performs the actual STS/IMDS call.
	if _, err := i.bedrock.Creds.Retrieve(ctx); err != nil {
		return nil, xerrors.Errorf("resolve AWS credentials: %w", err)
	}

	awsCfg := aws.Config{
		Region:      cfg.Region,
		Credentials: i.bedrock.Creds,
	}

	var out []option.RequestOption
	out = append(out, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		appendBedrockPRMUserAgent(req)
		return next(req)
	}))
	out = append(out, bedrock.WithConfig(awsCfg))

	// If a custom base URL is set, override the default endpoint constructed by the bedrock middleware.
	if cfg.BaseURL != "" {
		out = append(out, option.WithBaseURL(cfg.BaseURL))
	}

	return out, nil
}

// withBedrockMantleOptions returns request options for the AWS Bedrock mantle
// endpoint (bedrock-mantle.{region}.api.aws/anthropic/v1/messages). It speaks
// the native Messages wire format, so this middleware only SigV4-signs the
// request (service "bedrock-mantle") and forwards it; the response is plain
// SSE.
func (i *interceptionBase) withBedrockMantleOptions(ctx context.Context) ([]option.RequestOption, error) {
	if i.bedrock == nil {
		return nil, xerrors.New("nil bedrock runtime")
	}
	cfg := i.bedrock.Cfg
	if err := cfg.Validate(); err != nil {
		return nil, xerrors.Errorf("bedrock mantle config: %w", err)
	}

	// Fail fast: ensure credentials can be resolved before signing. Served from
	// the shared cache on most requests (no network); on the cold or refresh
	// path this performs the actual STS/IMDS call.
	if _, err := i.bedrock.Creds.Retrieve(ctx); err != nil {
		return nil, xerrors.Errorf("resolve AWS credentials: %w", err)
	}

	signer := v4.NewSigner()
	var out []option.RequestOption
	out = append(out, option.WithBaseURL(cfg.BaseURL))
	// Appended last so it runs innermost (right before the HTTP send) and signs
	// the request after all other headers are set.
	out = append(out, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		appendBedrockPRMUserAgent(req)

		creds, err := i.bedrock.Creds.Retrieve(req.Context())
		if err != nil {
			return nil, xerrors.Errorf("mantle SigV4: resolve AWS credentials: %w", err)
		}

		// SigV4 requires a payload hash, so read the body to hash it and then
		// restore it for the downstream HTTP client to send.
		var body []byte
		if req.Body != nil {
			var err error
			body, err = io.ReadAll(req.Body)
			if err != nil {
				return nil, xerrors.Errorf("mantle SigV4: read request body: %w", err)
			}
			_ = req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
		}

		hash := sha256.Sum256(body)
		if err := signer.SignHTTP(req.Context(), creds, req, hex.EncodeToString(hash[:]), bedrockMantleSigningService, cfg.Region, time.Now()); err != nil {
			return nil, xerrors.Errorf("mantle SigV4: sign request: %w", err)
		}
		return next(req)
	}))

	return out, nil
}

// augmentRequestForBedrockInvokeModel changes the model used for the request since AWS Bedrock doesn't support
// Anthropics' model names. It also converts adaptive thinking to enabled with a budget for models that
// don't support adaptive thinking natively, or enabled thinking to adaptive for models that only support
// adaptive (Opus 4.7+).
func (i *interceptionBase) augmentRequestForBedrockInvokeModel() {
	if i.bedrock == nil {
		return
	}

	model := i.Model()
	updated, err := i.reqPayload.withModel(model)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to set model in request payload for Bedrock", slog.Error(err))
		return
	}
	i.reqPayload = updated

	switch {
	case bedrockModelRequiresAdaptiveThinking(model):
		// Symmetric conversion for adaptive-only models (Opus 4.7+): rewrite
		// thinking.type "enabled" with budget_tokens to the "adaptive" shape,
		// since Bedrock returns 400 for these models when the legacy shape is
		// used. Claude Code falls back to the legacy shape when it cannot
		// read the upstream model's capability metadata (which is the case
		// when AI Gateway is in the path).
		updated, err = i.reqPayload.convertEnabledThinkingForBedrock()
		if err != nil {
			i.logger.Warn(context.Background(), "failed to convert enabled thinking for Bedrock", slog.Error(err))
			return
		}
		i.reqPayload = updated
	case !bedrockModelSupportsAdaptiveThinking(model):
		updated, err = i.reqPayload.convertAdaptiveThinkingForBedrock()
		if err != nil {
			i.logger.Warn(context.Background(), "failed to convert adaptive thinking for Bedrock", slog.Error(err))
			return
		}
		i.reqPayload = updated
	}

	// Filter Anthropic-Beta header to only include Bedrock-supported flags
	// that the current model supports.
	if i.clientHeaders != nil {
		filterBedrockBetaFlags(i.clientHeaders, model)
	}

	// Strip body fields that Bedrock does not accept. Adaptive-only models
	// (Opus 4.7+) support output_config natively without a beta flag, so
	// keep it for those models even when the effort-2025-11-24 flag is
	// absent from the request.
	var exemptFields []string
	if bedrockModelRequiresAdaptiveThinking(model) {
		exemptFields = append(exemptFields, messagesReqPathOutputConfig)
	}
	updated, err = i.reqPayload.removeUnsupportedBedrockFields(i.clientHeaders, exemptFields...)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to remove unsupported fields for Bedrock", slog.Error(err))
		return
	}
	i.reqPayload = updated

	// Adaptive-only models accept output_config but reject some of its
	// sub-fields (currently: output_config.format). Strip those after the
	// top-level pass has decided to keep output_config.
	if bedrockModelRequiresAdaptiveThinking(model) {
		updated, err = i.reqPayload.removeBedrockUnsupportedOutputConfigSubFields()
		if err != nil {
			i.logger.Warn(context.Background(), "failed to strip unsupported output_config sub-fields for Bedrock", slog.Error(err))
			return
		}
		i.reqPayload = updated
	}
}

// bedrockModelSupportsAdaptiveThinking returns true if the given Bedrock model ID
// supports the "adaptive" thinking type natively (i.e. Claude 4.6 models, and
// adaptive-only models such as Opus 4.7+).
// See https://docs.aws.amazon.com/bedrock/latest/userguide/claude-messages-adaptive-thinking.html
func bedrockModelSupportsAdaptiveThinking(model string) bool {
	return strings.Contains(model, "anthropic.claude-opus-4-6") ||
		strings.Contains(model, "anthropic.claude-sonnet-4-6") ||
		bedrockModelRequiresAdaptiveThinking(model)
}

// bedrockModelRequiresAdaptiveThinking returns true if the given Bedrock model
// ID only supports the "adaptive" thinking type and rejects the legacy
// "enabled" + budget_tokens shape with a 400. Claude Opus 4.7 was the first
// model in this category.
//
// See https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-4-7.html
func bedrockModelRequiresAdaptiveThinking(model string) bool {
	return strings.Contains(model, "anthropic.claude-opus-4-7") ||
		strings.Contains(model, "anthropic.claude-opus-4-8")
}

// filterBedrockBetaFlags removes unsupported beta flags from the Anthropic-Beta
// header and also removes model-gated flags the current model doesn't support.
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages-request-response.html
func filterBedrockBetaFlags(headers http.Header, model string) {
	// Collect all flags regardless of whether the client sent them as a single
	// comma-separated value (eg. Claude Code sends them in that format)
	// or as multiple separate header lines.
	// https://httpwg.org/specs/rfc9110.html#rfc.section.5.3
	var flags []string
	for _, v := range headers.Values("Anthropic-Beta") {
		flags = append(flags, strings.Split(v, ",")...)
	}

	if len(flags) == 0 {
		return
	}

	var keep []string
	for _, flag := range flags {
		trimmed := strings.TrimSpace(flag)
		if !bedrockSupportedBetaFlags[trimmed] {
			continue
		}

		// effort is only supported in Opus 4.5 on Bedrock.
		if trimmed == "effort-2025-11-24" && !strings.Contains(model, "anthropic.claude-opus-4-5") {
			continue
		}

		// context_management is only supported in Sonnet 4.5 and Haiku 4.5 models on Bedrock.
		if trimmed == "context-management-2025-06-27" &&
			!strings.Contains(model, "anthropic.claude-sonnet-4-5") &&
			!strings.Contains(model, "anthropic.claude-haiku-4-5") {
			continue
		}

		keep = append(keep, trimmed)
	}

	headers.Del("Anthropic-Beta")
	for _, flag := range keep {
		headers.Add("Anthropic-Beta", flag)
	}
}

// writeUpstreamError marshals and writes a given error.
func (i *interceptionBase) writeUpstreamError(w http.ResponseWriter, antErr *ResponseError) {
	if antErr == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Set Retry-After when a cooldown is configured.
	if antErr.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(antErr.RetryAfter.Seconds()))))
	}
	w.WriteHeader(antErr.StatusCode)

	out, err := json.Marshal(antErr)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to marshal upstream error", slog.Error(err), slog.F("error_payload", fmt.Sprintf("%+v", antErr)))
		// Response has to match expected format.
		// See https://docs.claude.com/en/api/errors#error-shapes.
		_, _ = w.Write([]byte(fmt.Sprintf(`{
	"type":"error",
	"error": {
		"type": "error",
		"message":"error marshaling upstream error"
	},
	"request_id": "%s"
}`, i.ID().String())))
	} else {
		_, _ = w.Write(out)
	}
}

func (i *interceptionBase) hasInjectableTools() bool {
	return i.mcpProxy != nil && len(i.mcpProxy.ListTools()) > 0
}

// accumulateUsage accumulates usage statistics from source into dest.
// It handles both [anthropic.Usage] and [anthropic.MessageDeltaUsage] types through [any].
// The function uses reflection to handle the differences between the types:
// - [anthropic.Usage] has CacheCreation field with ephemeral tokens
// - [anthropic.MessageDeltaUsage] doesn't have CacheCreation field
func accumulateUsage(dest, src any) {
	switch d := dest.(type) {
	case *anthropic.Usage:
		if d == nil {
			return
		}
		switch s := src.(type) {
		case anthropic.Usage:
			// Usage -> Usage
			d.CacheCreation.Ephemeral1hInputTokens += s.CacheCreation.Ephemeral1hInputTokens
			d.CacheCreation.Ephemeral5mInputTokens += s.CacheCreation.Ephemeral5mInputTokens
			d.CacheCreationInputTokens += s.CacheCreationInputTokens
			d.CacheReadInputTokens += s.CacheReadInputTokens
			d.InputTokens += s.InputTokens
			d.OutputTokens += s.OutputTokens
			d.ServerToolUse.WebSearchRequests += s.ServerToolUse.WebSearchRequests
		case anthropic.MessageDeltaUsage:
			// MessageDeltaUsage -> Usage
			d.CacheCreationInputTokens += s.CacheCreationInputTokens
			d.CacheReadInputTokens += s.CacheReadInputTokens
			d.InputTokens += s.InputTokens
			d.OutputTokens += s.OutputTokens
			d.ServerToolUse.WebSearchRequests += s.ServerToolUse.WebSearchRequests
		}
	case *anthropic.MessageDeltaUsage:
		if d == nil {
			return
		}
		switch s := src.(type) {
		case anthropic.Usage:
			// Usage -> MessageDeltaUsage (only common fields)
			d.CacheCreationInputTokens += s.CacheCreationInputTokens
			d.CacheReadInputTokens += s.CacheReadInputTokens
			d.InputTokens += s.InputTokens
			d.OutputTokens += s.OutputTokens
			d.ServerToolUse.WebSearchRequests += s.ServerToolUse.WebSearchRequests
		case anthropic.MessageDeltaUsage:
			// MessageDeltaUsage -> MessageDeltaUsage
			d.CacheCreationInputTokens += s.CacheCreationInputTokens
			d.CacheReadInputTokens += s.CacheReadInputTokens
			d.InputTokens += s.InputTokens
			d.OutputTokens += s.OutputTokens
			d.ServerToolUse.WebSearchRequests += s.ServerToolUse.WebSearchRequests
		}
	}
}

// For centralized requests, markKeyOnError extracts an
// Anthropic SDK error from err and marks the key based on
// its status code. Returns true if the status was a key-specific
// failover trigger so callers can retry with the next key.
func (i *interceptionBase) markKeyOnError(ctx context.Context, key *keypool.Key, err error) bool {
	cp, ok := intercept.AsCentralizedPool(i.cred)
	if !ok {
		return false
	}
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return cp.Pool.MarkKeyOnStatus(
		ctx, key, apiErr.Response, i.logger,
	)
}

// ResponseErrorFromKeyPool translates a *keypool.Error into
// a developer-facing ResponseError shaped for the Anthropic API.
func ResponseErrorFromKeyPool(keyPoolErr *keypool.Error) *ResponseError {
	if keyPoolErr == nil {
		return nil
	}
	switch keyPoolErr.Kind {
	case keypool.ErrorKindPermanent:
		return newResponseError(
			keyPoolErr.Error(),
			string(constant.ValueOf[constant.APIError]()),
			http.StatusBadGateway,
			keyPoolErr.RetryAfter,
		)
	case keypool.ErrorKindRateLimited:
		return newResponseError(
			keyPoolErr.Error(),
			string(constant.ValueOf[constant.RateLimitError]()),
			http.StatusTooManyRequests,
			keyPoolErr.RetryAfter,
		)
	default:
		// Fall back to a generic 502.
		return newResponseError(
			keyPoolErr.Error(),
			string(constant.ValueOf[constant.APIError]()),
			http.StatusBadGateway,
			keyPoolErr.RetryAfter,
		)
	}
}

func ResponseErrorFromAPIError(err error) *ResponseError {
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		return nil
	}

	msg := apierr.Error()
	errType := string(constant.ValueOf[constant.APIError]())

	var detail *anthropic.APIErrorObject
	if field, ok := apierr.JSON.ExtraFields["error"]; ok {
		_ = json.Unmarshal([]byte(field.Raw()), &detail)
	}
	if detail != nil {
		msg = detail.Message
		errType = string(detail.Type)
	}

	return newResponseError(msg, errType, apierr.StatusCode, keypool.ParseRetryAfter(apierr.Response))
}

var _ error = &ResponseError{}

type ResponseError struct {
	*anthropic.ErrorResponse

	StatusCode int           `json:"-"`
	RetryAfter time.Duration `json:"-"`
}

func newResponseError(msg, errType string, status int, retryAfter time.Duration) *ResponseError {
	return &ResponseError{
		ErrorResponse: &shared.ErrorResponse{
			Error: shared.ErrorObjectUnion{
				Message: msg,
				Type:    errType,
			},
			Type: constant.ValueOf[constant.Error](),
		},
		StatusCode: status,
		RetryAfter: retryAfter,
	}
}

func (e *ResponseError) Error() string {
	if e.ErrorResponse == nil {
		return ""
	}
	return e.ErrorResponse.Error.Message
}

// ToResponse marshals e into an *http.Response shaped for the
// Anthropic API.
func (e *ResponseError) ToResponse() *http.Response {
	body, err := json.Marshal(e)
	if err != nil {
		body = []byte(`{"type":"error","error":{"type":"error","message":"error marshaling upstream error"}}`)
	}
	return utils.NewJSONErrorResponse(e.StatusCode, e.RetryAfter, body)
}
