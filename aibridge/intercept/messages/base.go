package messages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibconfig "github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
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

type interceptionBase struct {
	id           uuid.UUID
	providerName string
	reqPayload   RequestPayload

	cfg        aibconfig.Anthropic
	bedrockCfg *aibconfig.AWSBedrock

	// clientHeaders are the original HTTP headers from the client request.
	clientHeaders  http.Header
	authHeaderName string

	tracer trace.Tracer
	logger slog.Logger

	recorder   recorder.Recorder
	mcpProxy   mcp.ServerProxier
	credential intercept.CredentialInfo
}

func (i *interceptionBase) ID() uuid.UUID {
	return i.id
}

func (i *interceptionBase) Credential() intercept.CredentialInfo {
	return i.credential
}

func (i *interceptionBase) Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier) {
	i.logger = logger
	i.recorder = rec
	i.mcpProxy = mcpProxy
}

func (i *interceptionBase) CorrelatingToolCallID() *string {
	return i.reqPayload.correlatingToolCallID()
}

func (i *interceptionBase) Model() string {
	if len(i.reqPayload) == 0 {
		return "coder-aibridge-unknown"
	}

	if i.bedrockCfg != nil {
		model := i.bedrockCfg.Model
		if i.isSmallFastModel() {
			model = i.bedrockCfg.SmallFastModel
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
		attribute.String(tracing.Provider, i.providerName),
		attribute.String(tracing.Model, i.Model()),
		attribute.Bool(tracing.Streaming, streaming),
		attribute.Bool(tracing.IsBedrock, i.bedrockCfg != nil),
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

func (i *interceptionBase) newMessagesService(ctx context.Context, opts ...option.RequestOption) (anthropic.MessageService, error) {
	// BYOK with access token uses Authorization: Bearer.
	// Otherwise use X-Api-Key (centralized or BYOK with personal API key).
	if i.cfg.BYOKBearerToken != "" {
		i.logger.Debug(ctx, "using byok access token auth",
			slog.F("bearer_hint", utils.MaskSecret(i.cfg.BYOKBearerToken)),
		)
		opts = append(opts, option.WithAuthToken(i.cfg.BYOKBearerToken))
	} else {
		i.logger.Debug(ctx, "using api key auth",
			slog.F("api_key_hint", utils.MaskSecret(i.cfg.Key)),
		)
		opts = append(opts, option.WithAPIKey(i.cfg.Key))
	}
	opts = append(opts, option.WithBaseURL(i.cfg.BaseURL))
	if i.cfg.MaxRetries != nil {
		opts = append(opts, option.WithMaxRetries(*i.cfg.MaxRetries))
	}

	// Add extra headers if configured.
	// Some providers require additional headers that are not added by the SDK.
	// TODO(ssncferreira): remove as part of https://github.com/coder/aibridge/issues/192
	for key, value := range i.cfg.ExtraHeaders {
		opts = append(opts, option.WithHeader(key, value))
	}

	// Forward client headers to upstream. This middleware runs after the SDK
	// has built the request, and replaces the outgoing headers with the sanitized
	// client headers plus provider auth.
	if i.clientHeaders != nil {
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			req.Header = intercept.BuildUpstreamHeaders(req.Header, i.clientHeaders, i.authHeaderName)
			return next(req)
		}))
	}

	// Add API dump middleware if configured
	if mw := apidump.NewBridgeMiddleware(i.cfg.APIDumpDir, i.providerName, i.Model(), i.id, i.logger, quartz.NewReal()); mw != nil {
		opts = append(opts, option.WithMiddleware(mw))
	}

	if i.bedrockCfg != nil {
		ctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()
		bedrockOpts, err := i.withAWSBedrockOptions(ctx, i.bedrockCfg)
		if err != nil {
			return anthropic.MessageService{}, err
		}
		opts = append(opts, bedrockOpts...)
		i.augmentRequestForBedrock()
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

// withAWSBedrockOptions returns request options for authenticating with AWS Bedrock.
//
// When both AccessKey and AccessKeySecret are set in the aibridge config, they are
// used directly as static credentials. Otherwise, the AWS SDK default credential chain
// resolves credentials (environment variables, shared config/credentials files, IAM
// roles, IRSA, SSO, IMDS, etc.).
func (*interceptionBase) withAWSBedrockOptions(ctx context.Context, cfg *aibconfig.AWSBedrock) ([]option.RequestOption, error) {
	if cfg == nil {
		return nil, xerrors.New("nil config given")
	}
	if cfg.Region == "" && cfg.BaseURL == "" {
		return nil, xerrors.New("region or base url required")
	}
	if cfg.Model == "" {
		return nil, xerrors.New("model required")
	}
	if cfg.SmallFastModel == "" {
		return nil, xerrors.New("small fast model required")
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}

	// Use static credentials when explicitly provided, otherwise fall back to the SDK default credential chain.
	switch {
	// Both set: use static credentials directly.
	case cfg.AccessKey != "" && cfg.AccessKeySecret != "":
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AccessKey,
				cfg.AccessKeySecret,
				"",
			),
		))
	// Only one set: misconfiguration.
	case cfg.AccessKey != "" || cfg.AccessKeySecret != "":
		return nil, xerrors.New("both access key and access key secret must be provided together")
	// Neither set: SDK default credential chain resolves credentials.
	default:
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, xerrors.Errorf("failed to load AWS Bedrock config: %w", err)
	}

	// Fail fast: ensure credentials can be resolved before making any requests.
	// awsCfg already carries the credentials provider, and the Bedrock middleware
	// will call Retrieve on it when signing each request.
	if _, err := awsCfg.Credentials.Retrieve(ctx); err != nil {
		return nil, xerrors.Errorf("no AWS credentials found: %w", err)
	}

	var out []option.RequestOption
	out = append(out, bedrock.WithConfig(awsCfg))

	// If a custom base URL is set, override the default endpoint constructed by the bedrock middleware.
	if cfg.BaseURL != "" {
		out = append(out, option.WithBaseURL(cfg.BaseURL))
	}

	return out, nil
}

// augmentRequestForBedrock will change the model used for the request since AWS Bedrock doesn't support
// Anthropics' model names. It also converts adaptive thinking to enabled with a budget for models that
// don't support adaptive thinking natively.
func (i *interceptionBase) augmentRequestForBedrock() {
	if i.bedrockCfg == nil {
		return
	}

	model := i.Model()
	updated, err := i.reqPayload.withModel(model)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to set model in request payload for Bedrock", slog.Error(err))
		return
	}
	i.reqPayload = updated

	if !bedrockModelSupportsAdaptiveThinking(model) {
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

	// Strip body fields that Bedrock does not accept.
	updated, err = i.reqPayload.removeUnsupportedBedrockFields(i.clientHeaders)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to remove unsupported fields for Bedrock", slog.Error(err))
		return
	}
	i.reqPayload = updated
}

// bedrockModelSupportsAdaptiveThinking returns true if the given Bedrock model ID
// supports the "adaptive" thinking type natively (i.e. Claude 4.6 models).
// See https://docs.aws.amazon.com/bedrock/latest/userguide/claude-messages-adaptive-thinking.html
func bedrockModelSupportsAdaptiveThinking(model string) bool {
	return strings.Contains(model, "anthropic.claude-opus-4-6") ||
		strings.Contains(model, "anthropic.claude-sonnet-4-6")
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
func (i *interceptionBase) writeUpstreamError(w http.ResponseWriter, antErr *responseError) {
	if antErr == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
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

func getErrorResponse(err error) *responseError {
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		return nil
	}

	msg := apierr.Error()
	typ := string(constant.ValueOf[constant.APIError]())

	var detail *anthropic.APIErrorObject
	if field, ok := apierr.JSON.ExtraFields["error"]; ok {
		_ = json.Unmarshal([]byte(field.Raw()), &detail)
	}
	if detail != nil {
		msg = detail.Message
		typ = string(detail.Type)
	}

	return &responseError{
		ErrorResponse: &anthropic.ErrorResponse{
			Error: anthropic.ErrorObjectUnion{
				Message: msg,
				Type:    typ,
			},
			Type: constant.ValueOf[constant.Error](),
		},
		StatusCode: apierr.StatusCode,
	}
}

var _ error = &responseError{}

type responseError struct {
	*anthropic.ErrorResponse

	StatusCode int `json:"-"`
}

func newErrorResponse(msg error) *responseError {
	return &responseError{
		ErrorResponse: &shared.ErrorResponse{
			Error: shared.ErrorObjectUnion{
				Message: msg.Error(),
				Type:    "error",
			},
		},
	}
}

func (a *responseError) Error() string {
	if a.ErrorResponse == nil {
		return ""
	}
	return a.ErrorResponse.Error.Message
}
