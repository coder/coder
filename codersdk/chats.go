package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// ChatStatus represents the status of a chat.
type ChatStatus string

const (
	ChatStatusWaiting   ChatStatus = "waiting"
	ChatStatusPending   ChatStatus = "pending"
	ChatStatusRunning   ChatStatus = "running"
	ChatStatusPaused    ChatStatus = "paused"
	ChatStatusCompleted ChatStatus = "completed"
	ChatStatusError     ChatStatus = "error"
)

// Chat represents a chat session with an AI agent.
type Chat struct {
	ID                uuid.UUID       `json:"id" format:"uuid"`
	OwnerID           uuid.UUID       `json:"owner_id" format:"uuid"`
	WorkspaceID       *uuid.UUID      `json:"workspace_id,omitempty" format:"uuid"`
	ParentChatID      *uuid.UUID      `json:"parent_chat_id,omitempty" format:"uuid"`
	RootChatID        *uuid.UUID      `json:"root_chat_id,omitempty" format:"uuid"`
	LastModelConfigID uuid.UUID       `json:"last_model_config_id" format:"uuid"`
	Title             string          `json:"title"`
	Status            ChatStatus      `json:"status"`
	LastError         *string         `json:"last_error"`
	DiffStatus        *ChatDiffStatus `json:"diff_status,omitempty"`
	CreatedAt         time.Time       `json:"created_at" format:"date-time"`
	UpdatedAt         time.Time       `json:"updated_at" format:"date-time"`
	Archived          bool            `json:"archived"`
}

// ChatMessage represents a single message in a chat.
type ChatMessage struct {
	ID            int64             `json:"id"`
	ChatID        uuid.UUID         `json:"chat_id" format:"uuid"`
	CreatedBy     *uuid.UUID        `json:"created_by,omitempty" format:"uuid"`
	ModelConfigID *uuid.UUID        `json:"model_config_id,omitempty" format:"uuid"`
	CreatedAt     time.Time         `json:"created_at" format:"date-time"`
	Role          ChatMessageRole   `json:"role"`
	Content       []ChatMessagePart `json:"content,omitempty"`
	Usage         *ChatMessageUsage `json:"usage,omitempty"`
}

// ChatMessageUsage contains token usage information for a chat message.
type ChatMessageUsage struct {
	InputTokens         *int64 `json:"input_tokens,omitempty"`
	OutputTokens        *int64 `json:"output_tokens,omitempty"`
	TotalTokens         *int64 `json:"total_tokens,omitempty"`
	ReasoningTokens     *int64 `json:"reasoning_tokens,omitempty"`
	CacheCreationTokens *int64 `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     *int64 `json:"cache_read_tokens,omitempty"`
	ContextLimit        *int64 `json:"context_limit,omitempty"`
}

// ChatMessageRole represents the role of a chat message sender.
type ChatMessageRole string

// ChatMessageRole enums.
const (
	ChatMessageRoleSystem    ChatMessageRole = "system"
	ChatMessageRoleUser      ChatMessageRole = "user"
	ChatMessageRoleAssistant ChatMessageRole = "assistant"
	ChatMessageRoleTool      ChatMessageRole = "tool"
)

// ChatMessagePartType represents a structured message part type.
type ChatMessagePartType string

const (
	ChatMessagePartTypeText          ChatMessagePartType = "text"
	ChatMessagePartTypeReasoning     ChatMessagePartType = "reasoning"
	ChatMessagePartTypeToolCall      ChatMessagePartType = "tool-call"
	ChatMessagePartTypeToolResult    ChatMessagePartType = "tool-result"
	ChatMessagePartTypeSource        ChatMessagePartType = "source"
	ChatMessagePartTypeFile          ChatMessagePartType = "file"
	ChatMessagePartTypeFileReference ChatMessagePartType = "file-reference"
)

// ChatMessagePart is a structured chunk of a chat message.
//
// WARNING: This type is both an API wire type and a database
// persistence format. Its JSON layout is stored in the
// chat_messages.content column. Field additions, renames, type
// changes, and omitempty behavior all affect backward-compatible
// deserialization of stored rows. Treat changes to this struct
// with the same care as a database migration.
type ChatMessagePart struct {
	Type        ChatMessagePartType `json:"type"`
	Text        string              `json:"text,omitempty"`
	Signature   string              `json:"signature,omitempty"`
	ToolCallID  string              `json:"tool_call_id,omitempty"`
	ToolName    string              `json:"tool_name,omitempty"`
	Args        json.RawMessage     `json:"args,omitempty"`
	ArgsDelta   string              `json:"args_delta,omitempty"`
	Result      json.RawMessage     `json:"result,omitempty"`
	ResultDelta string              `json:"result_delta,omitempty"`
	IsError     bool                `json:"is_error,omitempty"`
	SourceID    string              `json:"source_id,omitempty"`
	URL         string              `json:"url,omitempty"`
	Title       string              `json:"title,omitempty"`
	MediaType   string              `json:"media_type,omitempty"`
	Data        []byte              `json:"data,omitempty"`
	FileID      uuid.NullUUID       `json:"file_id,omitempty" format:"uuid"`
	// The following fields are only set when Type is
	// ChatInputPartTypeFileReference.
	FileName  string `json:"file_name,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	// The code content from the diff that was commented on.
	Content string `json:"content,omitempty"`
	// ProviderMetadata holds provider-specific response metadata
	// (e.g. Anthropic cache control hints) as raw JSON. Internal
	// only: stripped by db2sdk before API responses.
	ProviderMetadata json.RawMessage `json:"provider_metadata,omitempty" typescript:"-"`
	// ProviderExecuted indicates the tool call was executed by
	// the provider (e.g. Anthropic computer use).
	ProviderExecuted bool `json:"provider_executed,omitempty"`
}

// StripInternal removes internal-only fields that must not be
// sent to API clients. Call before publishing via REST or SSE.
//
// Note: ArgsDelta and ResultDelta are intentionally preserved.
// They are streaming-only fields consumed by the frontend via
// SSE message_part events (see processStepStream in chatloop).
func (p *ChatMessagePart) StripInternal() {
	p.ProviderMetadata = nil
	if p.FileID.Valid {
		p.Data = nil
	}
}

// ChatMessageText builds a text chat message part.
func ChatMessageText(text string) ChatMessagePart {
	return ChatMessagePart{Type: ChatMessagePartTypeText, Text: text}
}

// ChatMessageReasoning builds a reasoning chat message part.
func ChatMessageReasoning(text string) ChatMessagePart {
	return ChatMessagePart{Type: ChatMessagePartTypeReasoning, Text: text}
}

// ChatMessageToolCall builds a tool-call chat message part.
func ChatMessageToolCall(toolCallID, toolName string, args json.RawMessage) ChatMessagePart {
	return ChatMessagePart{
		Type:       ChatMessagePartTypeToolCall,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Args:       args,
	}
}

// ChatMessageToolResult builds a tool-result chat message part.
func ChatMessageToolResult(toolCallID, toolName string, result json.RawMessage, isError bool) ChatMessagePart {
	return ChatMessagePart{
		Type:       ChatMessagePartTypeToolResult,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		IsError:    isError,
	}
}

// ChatMessageFile builds a file chat message part.
func ChatMessageFile(fileID uuid.UUID, mediaType string) ChatMessagePart {
	return ChatMessagePart{
		Type:      ChatMessagePartTypeFile,
		FileID:    uuid.NullUUID{UUID: fileID, Valid: true},
		MediaType: mediaType,
	}
}

// ChatMessageFileReference builds a file-reference chat message part.
func ChatMessageFileReference(fileName string, startLine, endLine int, content string) ChatMessagePart {
	return ChatMessagePart{
		Type:      ChatMessagePartTypeFileReference,
		FileName:  fileName,
		StartLine: startLine,
		EndLine:   endLine,
		Content:   content,
	}
}

// ChatMessageSource builds a source chat message part.
func ChatMessageSource(sourceID, sourceURL, title string) ChatMessagePart {
	return ChatMessagePart{
		Type:     ChatMessagePartTypeSource,
		SourceID: sourceID,
		URL:      sourceURL,
		Title:    title,
	}
}

// ChatInputPartType represents an input part type for user chat input.
type ChatInputPartType string

const (
	ChatInputPartTypeText          ChatInputPartType = "text"
	ChatInputPartTypeFile          ChatInputPartType = "file"
	ChatInputPartTypeFileReference ChatInputPartType = "file-reference"
)

// ChatInputPart is a single user input part for creating a chat.
type ChatInputPart struct {
	Type   ChatInputPartType `json:"type"`
	Text   string            `json:"text,omitempty"`
	FileID uuid.UUID         `json:"file_id,omitempty" format:"uuid"`
	// The following fields are only set when Type is
	// ChatInputPartTypeFileReference.
	FileName  string `json:"file_name,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	// The code content from the diff that was commented on.
	Content string `json:"content,omitempty"`
}

// CreateChatRequest is the request to create a new chat.
type CreateChatRequest struct {
	Content       []ChatInputPart `json:"content"`
	WorkspaceID   *uuid.UUID      `json:"workspace_id,omitempty" format:"uuid"`
	ModelConfigID *uuid.UUID      `json:"model_config_id,omitempty" format:"uuid"`
}

// UpdateChatRequest is the request to update a chat.
type UpdateChatRequest struct {
	Title string `json:"title"`
}

// CreateChatMessageRequest is the request to add a message to a chat.
type CreateChatMessageRequest struct {
	Content       []ChatInputPart `json:"content"`
	ModelConfigID *uuid.UUID      `json:"model_config_id,omitempty" format:"uuid"`
}

// EditChatMessageRequest is the request to edit a user message in a chat.
type EditChatMessageRequest struct {
	Content []ChatInputPart `json:"content"`
}

// CreateChatMessageResponse is the response from adding a message to a chat.
type CreateChatMessageResponse struct {
	Message       *ChatMessage       `json:"message,omitempty"`
	QueuedMessage *ChatQueuedMessage `json:"queued_message,omitempty"`
	Queued        bool               `json:"queued"`
}

// UploadChatFileResponse is the response from uploading a chat file.
type UploadChatFileResponse struct {
	ID uuid.UUID `json:"id" format:"uuid"`
}

// ChatMessagesResponse contains the messages and queued messages for a chat.
type ChatMessagesResponse struct {
	Messages       []ChatMessage       `json:"messages"`
	QueuedMessages []ChatQueuedMessage `json:"queued_messages"`
}

// ChatModelProviderUnavailableReason explains why a provider cannot be used.
type ChatModelProviderUnavailableReason string

const (
	ChatModelProviderUnavailableMissingAPIKey ChatModelProviderUnavailableReason = "missing_api_key"
	ChatModelProviderUnavailableFetchFailed   ChatModelProviderUnavailableReason = "fetch_failed"
)

// ChatModel represents a model in the chat model catalog.
type ChatModel struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	DisplayName string `json:"display_name"`
}

// ChatModelProvider represents provider availability and model results.
type ChatModelProvider struct {
	Provider          string                             `json:"provider"`
	Available         bool                               `json:"available"`
	UnavailableReason ChatModelProviderUnavailableReason `json:"unavailable_reason,omitempty"`
	Models            []ChatModel                        `json:"models"`
}

// ChatModelsResponse is the catalog returned from chat model discovery.
type ChatModelsResponse struct {
	Providers []ChatModelProvider `json:"providers"`
}

// ChatSystemPromptResponse is the response for getting the chat system prompt.
type ChatSystemPromptResponse struct {
	SystemPrompt string `json:"system_prompt"`
}

// UpdateChatSystemPromptRequest is the request to update the chat system prompt.
type UpdateChatSystemPromptRequest struct {
	SystemPrompt string `json:"system_prompt"`
}

// UserChatCustomPromptResponse is the response for getting a user's
// custom chat prompt.
type UserChatCustomPromptResponse struct {
	CustomPrompt string `json:"custom_prompt"`
}

// UpdateUserChatCustomPromptRequest is the request to update a user's
// custom chat prompt.
type UpdateUserChatCustomPromptRequest struct {
	CustomPrompt string `json:"custom_prompt"`
}

// ChatProviderConfigSource describes how a provider entry is sourced.
type ChatProviderConfigSource string

const (
	ChatProviderConfigSourceDatabase  ChatProviderConfigSource = "database"
	ChatProviderConfigSourceEnvPreset ChatProviderConfigSource = "env_preset"
	ChatProviderConfigSourceSupported ChatProviderConfigSource = "supported"
)

// ChatProviderConfig is an admin-managed provider configuration.
type ChatProviderConfig struct {
	ID          uuid.UUID                `json:"id" format:"uuid"`
	Provider    string                   `json:"provider"`
	DisplayName string                   `json:"display_name"`
	Enabled     bool                     `json:"enabled"`
	HasAPIKey   bool                     `json:"has_api_key"`
	BaseURL     string                   `json:"base_url,omitempty"`
	Source      ChatProviderConfigSource `json:"source"`
	CreatedAt   time.Time                `json:"created_at,omitempty" format:"date-time"`
	UpdatedAt   time.Time                `json:"updated_at,omitempty" format:"date-time"`
}

// CreateChatProviderConfigRequest creates a chat provider config.
type CreateChatProviderConfigRequest struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

// UpdateChatProviderConfigRequest updates a chat provider config.
type UpdateChatProviderConfigRequest struct {
	DisplayName string  `json:"display_name,omitempty"`
	APIKey      *string `json:"api_key,omitempty"`
	BaseURL     *string `json:"base_url,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

// ChatModelConfig is an admin-managed model configuration.
type ChatModelConfig struct {
	ID                   uuid.UUID            `json:"id" format:"uuid"`
	Provider             string               `json:"provider"`
	Model                string               `json:"model"`
	DisplayName          string               `json:"display_name"`
	Enabled              bool                 `json:"enabled"`
	IsDefault            bool                 `json:"is_default"`
	ContextLimit         int64                `json:"context_limit"`
	CompressionThreshold int32                `json:"compression_threshold"`
	ModelConfig          *ChatModelCallConfig `json:"model_config,omitempty"`
	CreatedAt            time.Time            `json:"created_at" format:"date-time"`
	UpdatedAt            time.Time            `json:"updated_at" format:"date-time"`
}

// ChatModelProviderOptions contains typed provider-specific options.
//
// Note: Azure models use the `openai` options shape.
// Note: Bedrock models use the `anthropic` options shape.
type ChatModelProviderOptions struct {
	OpenAI       *ChatModelOpenAIProviderOptions       `json:"openai,omitempty"`
	Anthropic    *ChatModelAnthropicProviderOptions    `json:"anthropic,omitempty"`
	Google       *ChatModelGoogleProviderOptions       `json:"google,omitempty"`
	OpenAICompat *ChatModelOpenAICompatProviderOptions `json:"openaicompat,omitempty"`
	OpenRouter   *ChatModelOpenRouterProviderOptions   `json:"openrouter,omitempty"`
	Vercel       *ChatModelVercelProviderOptions       `json:"vercel,omitempty"`
}

// ChatModelOpenAIProviderOptions configures OpenAI provider behavior.
type ChatModelOpenAIProviderOptions struct {
	Include             []string         `json:"include,omitempty" description:"Model names to include in discovery" hidden:"true"`
	Instructions        *string          `json:"instructions,omitempty" description:"System-level instructions prepended to the conversation" hidden:"true"`
	LogitBias           map[string]int64 `json:"logit_bias,omitempty" description:"Token IDs mapped to bias values from -100 to 100" hidden:"true"`
	LogProbs            *bool            `json:"log_probs,omitempty" description:"Whether to return log probabilities of output tokens" hidden:"true"`
	TopLogProbs         *int64           `json:"top_log_probs,omitempty" description:"Number of most likely tokens to return log probabilities for" hidden:"true"`
	MaxToolCalls        *int64           `json:"max_tool_calls,omitempty" description:"Maximum number of tool calls per response"`
	ParallelToolCalls   *bool            `json:"parallel_tool_calls,omitempty" description:"Whether the model may make multiple tool calls in parallel"`
	User                *string          `json:"user,omitempty" description:"Unique identifier for the end user for abuse monitoring" hidden:"true"`
	ReasoningEffort     *string          `json:"reasoning_effort,omitempty" description:"Controls the level of reasoning effort" enum:"none,minimal,low,medium,high,xhigh"`
	ReasoningSummary    *string          `json:"reasoning_summary,omitempty" description:"Controls whether reasoning tokens are summarized in the response"`
	MaxCompletionTokens *int64           `json:"max_completion_tokens,omitempty" description:"Upper bound on tokens the model may generate"`
	TextVerbosity       *string          `json:"text_verbosity,omitempty" description:"Controls the verbosity of the text response" enum:"low,medium,high"`
	Prediction          map[string]any   `json:"prediction,omitempty" description:"Predicted output content to speed up responses" hidden:"true"`
	Store               *bool            `json:"store,omitempty" description:"Whether to store the output for model distillation or evals" hidden:"true"`
	Metadata            map[string]any   `json:"metadata,omitempty" description:"Arbitrary metadata to attach to the request" hidden:"true"`
	PromptCacheKey      *string          `json:"prompt_cache_key,omitempty" description:"Key for enabling cross-request prompt caching" hidden:"true"`
	SafetyIdentifier    *string          `json:"safety_identifier,omitempty" description:"Developer-specific safety identifier for the request" hidden:"true"`
	ServiceTier         *string          `json:"service_tier,omitempty" description:"Latency tier to use for processing the request"`
	StructuredOutputs   *bool            `json:"structured_outputs,omitempty" description:"Whether to enable structured JSON output mode" hidden:"true"`
	StrictJSONSchema    *bool            `json:"strict_json_schema,omitempty" description:"Whether to enforce strict adherence to the JSON schema" hidden:"true"`
	WebSearchEnabled    *bool            `json:"web_search_enabled,omitempty" description:"Enable OpenAI web search tool for grounding responses with real-time information"`
	SearchContextSize   *string          `json:"search_context_size,omitempty" description:"Amount of search context to use" enum:"low,medium,high"`
	AllowedDomains      []string         `json:"allowed_domains,omitempty" description:"Restrict web search to these domains"`
}

// ChatModelAnthropicThinkingOptions configures Anthropic thinking budget.
type ChatModelAnthropicThinkingOptions struct {
	BudgetTokens *int64 `json:"budget_tokens,omitempty" description:"Maximum number of tokens the model may use for thinking"`
}

// ChatModelAnthropicProviderOptions configures Anthropic provider behavior.
type ChatModelAnthropicProviderOptions struct {
	SendReasoning          *bool                              `json:"send_reasoning,omitempty" description:"Whether to include reasoning content in the response"`
	Thinking               *ChatModelAnthropicThinkingOptions `json:"thinking,omitempty" description:"Configuration for extended thinking"`
	Effort                 *string                            `json:"effort,omitempty" description:"Controls the level of reasoning effort" enum:"low,medium,high,max"`
	DisableParallelToolUse *bool                              `json:"disable_parallel_tool_use,omitempty" description:"Whether to disable parallel tool execution"`
	WebSearchEnabled       *bool                              `json:"web_search_enabled,omitempty" description:"Enable Anthropic web search tool for grounding responses with real-time information"`
	AllowedDomains         []string                           `json:"allowed_domains,omitempty" description:"Restrict web search to these domains (cannot be used with blocked_domains)"`
	BlockedDomains         []string                           `json:"blocked_domains,omitempty" description:"Block web search on these domains (cannot be used with allowed_domains)"`
}

// ChatModelGoogleThinkingConfig configures Google thinking behavior.
type ChatModelGoogleThinkingConfig struct {
	ThinkingBudget  *int64 `json:"thinking_budget,omitempty" description:"Maximum number of tokens the model may use for thinking"`
	IncludeThoughts *bool  `json:"include_thoughts,omitempty" description:"Whether to include thinking content in the response"`
}

// ChatModelGoogleSafetySetting configures Google safety filtering.
type ChatModelGoogleSafetySetting struct {
	Category  string `json:"category,omitempty" description:"The harm category to configure"`
	Threshold string `json:"threshold,omitempty" description:"The blocking threshold for the harm category"`
}

// ChatModelGoogleProviderOptions configures Google provider behavior.
type ChatModelGoogleProviderOptions struct {
	ThinkingConfig   *ChatModelGoogleThinkingConfig `json:"thinking_config,omitempty" description:"Configuration for extended thinking"`
	CachedContent    string                         `json:"cached_content,omitempty" description:"Resource name of a cached content object" hidden:"true"`
	SafetySettings   []ChatModelGoogleSafetySetting `json:"safety_settings,omitempty" description:"Safety filtering settings for harmful content categories" hidden:"true"`
	Threshold        string                         `json:"threshold,omitempty" hidden:"true"`
	WebSearchEnabled *bool                          `json:"web_search_enabled,omitempty" description:"Enable Google Search grounding for real-time information"`
}

// ChatModelOpenAICompatProviderOptions configures OpenAI-compatible behavior.
type ChatModelOpenAICompatProviderOptions struct {
	User            *string `json:"user,omitempty" description:"Unique identifier for the end user for abuse monitoring" hidden:"true"`
	ReasoningEffort *string `json:"reasoning_effort,omitempty" description:"Controls the level of reasoning effort" enum:"none,minimal,low,medium,high,xhigh"`
}

// ChatModelOpenRouterReasoningOptions configures OpenRouter reasoning behavior.
type ChatModelOpenRouterReasoningOptions struct {
	Enabled   *bool   `json:"enabled,omitempty" description:"Whether reasoning is enabled"`
	Exclude   *bool   `json:"exclude,omitempty" description:"Whether to exclude reasoning content from the response"`
	MaxTokens *int64  `json:"max_tokens,omitempty" description:"Maximum number of tokens for reasoning output"`
	Effort    *string `json:"effort,omitempty" description:"Controls the level of reasoning effort" enum:"low,medium,high"`
}

// ChatModelOpenRouterProvider configures OpenRouter routing preferences.
type ChatModelOpenRouterProvider struct {
	Order             []string `json:"order,omitempty" description:"Ordered list of preferred provider names"`
	AllowFallbacks    *bool    `json:"allow_fallbacks,omitempty" description:"Whether to allow fallback to other providers"`
	RequireParameters *bool    `json:"require_parameters,omitempty" description:"Whether to require all parameters to be supported by the provider"`
	DataCollection    *string  `json:"data_collection,omitempty" description:"Data collection policy preference"`
	Only              []string `json:"only,omitempty" description:"Restrict to only these provider names"`
	Ignore            []string `json:"ignore,omitempty" description:"Provider names to exclude from routing"`
	Quantizations     []string `json:"quantizations,omitempty" description:"Allowed model quantization levels"`
	Sort              *string  `json:"sort,omitempty" description:"Sort order for provider selection"`
}

// ChatModelOpenRouterProviderOptions configures OpenRouter provider behavior.
type ChatModelOpenRouterProviderOptions struct {
	Reasoning         *ChatModelOpenRouterReasoningOptions `json:"reasoning,omitempty" description:"Configuration for reasoning behavior"`
	ExtraBody         map[string]any                       `json:"extra_body,omitempty" description:"Additional fields to include in the request body" hidden:"true"`
	IncludeUsage      *bool                                `json:"include_usage,omitempty" description:"Whether to include token usage information in the response" hidden:"true"`
	LogitBias         map[string]int64                     `json:"logit_bias,omitempty" description:"Token IDs mapped to bias values from -100 to 100" hidden:"true"`
	LogProbs          *bool                                `json:"log_probs,omitempty" description:"Whether to return log probabilities of output tokens" hidden:"true"`
	ParallelToolCalls *bool                                `json:"parallel_tool_calls,omitempty" description:"Whether the model may make multiple tool calls in parallel"`
	User              *string                              `json:"user,omitempty" description:"Unique identifier for the end user for abuse monitoring" hidden:"true"`
	Provider          *ChatModelOpenRouterProvider         `json:"provider,omitempty" description:"Routing preferences for provider selection" hidden:"true"`
}

// ChatModelVercelReasoningOptions configures Vercel reasoning behavior.
type ChatModelVercelReasoningOptions struct {
	Enabled   *bool   `json:"enabled,omitempty" description:"Whether reasoning is enabled"`
	MaxTokens *int64  `json:"max_tokens,omitempty" description:"Maximum number of tokens for reasoning output"`
	Effort    *string `json:"effort,omitempty" description:"Controls the level of reasoning effort" enum:"none,minimal,low,medium,high,xhigh"`
	Exclude   *bool   `json:"exclude,omitempty" description:"Whether to exclude reasoning content from the response"`
}

// ChatModelVercelGatewayProviderOptions configures Vercel routing behavior.
type ChatModelVercelGatewayProviderOptions struct {
	Order  []string `json:"order,omitempty" description:"Ordered list of preferred provider names"`
	Models []string `json:"models,omitempty" description:"Model identifiers to route across"`
}

// ChatModelVercelProviderOptions configures Vercel provider behavior.
type ChatModelVercelProviderOptions struct {
	Reasoning         *ChatModelVercelReasoningOptions       `json:"reasoning,omitempty" description:"Configuration for reasoning behavior"`
	ProviderOptions   *ChatModelVercelGatewayProviderOptions `json:"providerOptions,omitempty" description:"Gateway routing options for provider selection" hidden:"true"`
	User              *string                                `json:"user,omitempty" description:"Unique identifier for the end user for abuse monitoring" hidden:"true"`
	LogitBias         map[string]int64                       `json:"logit_bias,omitempty" description:"Token IDs mapped to bias values from -100 to 100" hidden:"true"`
	LogProbs          *bool                                  `json:"logprobs,omitempty" description:"Whether to return log probabilities of output tokens" hidden:"true"`
	TopLogProbs       *int64                                 `json:"top_logprobs,omitempty" description:"Number of most likely tokens to return log probabilities for" hidden:"true"`
	ParallelToolCalls *bool                                  `json:"parallel_tool_calls,omitempty" description:"Whether the model may make multiple tool calls in parallel"`
	ExtraBody         map[string]any                         `json:"extra_body,omitempty" description:"Additional fields to include in the request body" hidden:"true"`
}

// ModelCostConfig stores pricing metadata for a chat model.
type ModelCostConfig struct {
	InputPricePerMillionTokens      *decimal.Decimal `json:"input_price_per_million_tokens,omitempty" description:"Input token price in USD per 1M tokens"`
	OutputPricePerMillionTokens     *decimal.Decimal `json:"output_price_per_million_tokens,omitempty" description:"Output token price in USD per 1M tokens"`
	CacheReadPricePerMillionTokens  *decimal.Decimal `json:"cache_read_price_per_million_tokens,omitempty" description:"Cache read token price in USD per 1M tokens"`
	CacheWritePricePerMillionTokens *decimal.Decimal `json:"cache_write_price_per_million_tokens,omitempty" description:"Cache write or cache creation token price in USD per 1M tokens"`
}

// ChatModelCallConfig configures per-call model behavior defaults.
type ChatModelCallConfig struct {
	MaxOutputTokens  *int64                    `json:"max_output_tokens,omitempty" description:"Upper bound on tokens the model may generate"`
	Temperature      *float64                  `json:"temperature,omitempty" description:"Sampling temperature between 0 and 2"`
	TopP             *float64                  `json:"top_p,omitempty" description:"Nucleus sampling probability cutoff"`
	TopK             *int64                    `json:"top_k,omitempty" description:"Number of highest-probability tokens to keep for sampling"`
	PresencePenalty  *float64                  `json:"presence_penalty,omitempty" description:"Penalty for tokens that have already appeared in the output"`
	FrequencyPenalty *float64                  `json:"frequency_penalty,omitempty" description:"Penalty for tokens based on their frequency in the output"`
	Cost             *ModelCostConfig          `json:"cost,omitempty" description:"Optional pricing metadata for this model"`
	ProviderOptions  *ChatModelProviderOptions `json:"provider_options,omitempty" description:"Provider-specific option overrides"`
}

// UnmarshalJSON accepts both the current nested cost object and the previous
// top-level pricing keys so legacy stored model_config JSON continues to load.
func (c *ChatModelCallConfig) UnmarshalJSON(data []byte) error {
	type chatModelCallConfigAlias ChatModelCallConfig
	aux := struct {
		*chatModelCallConfigAlias
		InputPricePerMillionTokens      *decimal.Decimal `json:"input_price_per_million_tokens,omitempty"`
		OutputPricePerMillionTokens     *decimal.Decimal `json:"output_price_per_million_tokens,omitempty"`
		CacheReadPricePerMillionTokens  *decimal.Decimal `json:"cache_read_price_per_million_tokens,omitempty"`
		CacheWritePricePerMillionTokens *decimal.Decimal `json:"cache_write_price_per_million_tokens,omitempty"`
	}{
		chatModelCallConfigAlias: (*chatModelCallConfigAlias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.InputPricePerMillionTokens == nil &&
		aux.OutputPricePerMillionTokens == nil &&
		aux.CacheReadPricePerMillionTokens == nil &&
		aux.CacheWritePricePerMillionTokens == nil {
		return nil
	}

	if c.Cost == nil {
		c.Cost = &ModelCostConfig{}
	}
	if c.Cost.InputPricePerMillionTokens == nil {
		c.Cost.InputPricePerMillionTokens = aux.InputPricePerMillionTokens
	}
	if c.Cost.OutputPricePerMillionTokens == nil {
		c.Cost.OutputPricePerMillionTokens = aux.OutputPricePerMillionTokens
	}
	if c.Cost.CacheReadPricePerMillionTokens == nil {
		c.Cost.CacheReadPricePerMillionTokens = aux.CacheReadPricePerMillionTokens
	}
	if c.Cost.CacheWritePricePerMillionTokens == nil {
		c.Cost.CacheWritePricePerMillionTokens = aux.CacheWritePricePerMillionTokens
	}
	return nil
}

// CreateChatModelConfigRequest creates a chat model config.
type CreateChatModelConfigRequest struct {
	Provider             string               `json:"provider"`
	Model                string               `json:"model"`
	DisplayName          string               `json:"display_name,omitempty"`
	Enabled              *bool                `json:"enabled,omitempty"`
	IsDefault            *bool                `json:"is_default,omitempty"`
	ContextLimit         *int64               `json:"context_limit,omitempty"`
	CompressionThreshold *int32               `json:"compression_threshold,omitempty"`
	ModelConfig          *ChatModelCallConfig `json:"model_config,omitempty"`
}

// UpdateChatModelConfigRequest updates a chat model config.
type UpdateChatModelConfigRequest struct {
	Provider             string               `json:"provider,omitempty"`
	Model                string               `json:"model,omitempty"`
	DisplayName          string               `json:"display_name,omitempty"`
	Enabled              *bool                `json:"enabled,omitempty"`
	IsDefault            *bool                `json:"is_default,omitempty"`
	ContextLimit         *int64               `json:"context_limit,omitempty"`
	CompressionThreshold *int32               `json:"compression_threshold,omitempty"`
	ModelConfig          *ChatModelCallConfig `json:"model_config,omitempty"`
}

// ChatGitChange represents a git file change detected during a chat session.
type ChatGitChange struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	ChatID      uuid.UUID `json:"chat_id" format:"uuid"`
	FilePath    string    `json:"file_path"`
	ChangeType  string    `json:"change_type"` // added, modified, deleted, renamed
	OldPath     *string   `json:"old_path,omitempty"`
	DiffSummary *string   `json:"diff_summary,omitempty"`
	DetectedAt  time.Time `json:"detected_at" format:"date-time"`
}

// ChatDiffStatus represents cached diff status for a chat. The URL
// may point to a pull request or a branch page depending on whether
// a PR has been opened.
type ChatDiffStatus struct {
	ChatID           uuid.UUID  `json:"chat_id" format:"uuid"`
	URL              *string    `json:"url,omitempty"`
	PullRequestState *string    `json:"pull_request_state,omitempty"`
	PullRequestTitle string     `json:"pull_request_title"`
	PullRequestDraft bool       `json:"pull_request_draft"`
	ChangesRequested bool       `json:"changes_requested"`
	Additions        int32      `json:"additions"`
	Deletions        int32      `json:"deletions"`
	ChangedFiles     int32      `json:"changed_files"`
	AuthorLogin      *string    `json:"author_login,omitempty"`
	AuthorAvatarURL  *string    `json:"author_avatar_url,omitempty"`
	BaseBranch       *string    `json:"base_branch,omitempty"`
	PRNumber         *int32     `json:"pr_number,omitempty"`
	Commits          *int32     `json:"commits,omitempty"`
	Approved         *bool      `json:"approved,omitempty"`
	ReviewerCount    *int32     `json:"reviewer_count,omitempty"`
	RefreshedAt      *time.Time `json:"refreshed_at,omitempty" format:"date-time"`
	StaleAt          *time.Time `json:"stale_at,omitempty" format:"date-time"`
}

// ChatDiffContents represents the resolved diff text for a chat.
type ChatDiffContents struct {
	ChatID         uuid.UUID `json:"chat_id" format:"uuid"`
	Provider       *string   `json:"provider,omitempty"`
	RemoteOrigin   *string   `json:"remote_origin,omitempty"`
	Branch         *string   `json:"branch,omitempty"`
	PullRequestURL *string   `json:"pull_request_url,omitempty"`
	Diff           string    `json:"diff,omitempty"`
}

// ChatStreamEventType represents the kind of chat stream update.
type ChatStreamEventType string

const (
	ChatStreamEventTypeMessagePart ChatStreamEventType = "message_part"
	ChatStreamEventTypeMessage     ChatStreamEventType = "message"
	ChatStreamEventTypeStatus      ChatStreamEventType = "status"
	ChatStreamEventTypeError       ChatStreamEventType = "error"
	ChatStreamEventTypeQueueUpdate ChatStreamEventType = "queue_update"
	ChatStreamEventTypeRetry       ChatStreamEventType = "retry"
)

// ChatQueuedMessage represents a queued message waiting to be processed.
type ChatQueuedMessage struct {
	ID        int64             `json:"id"`
	ChatID    uuid.UUID         `json:"chat_id" format:"uuid"`
	Content   []ChatMessagePart `json:"content"`
	CreatedAt time.Time         `json:"created_at" format:"date-time"`
}

// ChatStreamMessagePart is a streamed message part update.
type ChatStreamMessagePart struct {
	Role ChatMessageRole `json:"role,omitempty"`
	Part ChatMessagePart `json:"part"`
}

// ChatStreamStatus represents an updated chat status.
type ChatStreamStatus struct {
	Status ChatStatus `json:"status"`
}

// ChatStreamError represents an error event in the stream.
type ChatStreamError struct {
	Message string `json:"message"`
}

// ChatStreamRetry represents an auto-retry status event in the stream.
// Published when the server automatically retries a failed LLM call.
type ChatStreamRetry struct {
	// Attempt is the 1-indexed retry attempt number.
	Attempt int `json:"attempt"`
	// DelayMs is the backoff delay in milliseconds before the retry.
	DelayMs int64 `json:"delay_ms"`
	// Error is the error message from the failed attempt.
	Error string `json:"error"`
	// RetryingAt is the timestamp when the retry will be attempted.
	RetryingAt time.Time `json:"retrying_at" format:"date-time"`
}

// ChatStreamEvent represents a real-time update for chat streaming.
type ChatStreamEvent struct {
	Type           ChatStreamEventType    `json:"type"`
	ChatID         uuid.UUID              `json:"chat_id" format:"uuid"`
	Message        *ChatMessage           `json:"message,omitempty"`
	MessagePart    *ChatStreamMessagePart `json:"message_part,omitempty"`
	Status         *ChatStreamStatus      `json:"status,omitempty"`
	Error          *ChatStreamError       `json:"error,omitempty"`
	Retry          *ChatStreamRetry       `json:"retry,omitempty"`
	QueuedMessages []ChatQueuedMessage    `json:"queued_messages,omitempty"`
}

type chatStreamEnvelope struct {
	Type ServerSentEventType `json:"type"`
	Data json.RawMessage     `json:"data,omitempty"`
}

// ChatCostSummaryOptions are optional query parameters for GetChatCostSummary.
type ChatCostSummaryOptions struct {
	StartDate time.Time
	EndDate   time.Time
}

// ChatCostUsersOptions are optional query parameters for GetChatCostUsers.
type ChatCostUsersOptions struct {
	StartDate time.Time
	EndDate   time.Time
	Username  string
	Pagination
}

// ChatCostSummary is the response from the chat cost summary endpoint.
type ChatCostSummary struct {
	StartDate            time.Time                `json:"start_date" format:"date-time"`
	EndDate              time.Time                `json:"end_date" format:"date-time"`
	TotalCostMicros      int64                    `json:"total_cost_micros"`
	PricedMessageCount   int64                    `json:"priced_message_count"`
	UnpricedMessageCount int64                    `json:"unpriced_message_count"`
	TotalInputTokens     int64                    `json:"total_input_tokens"`
	TotalOutputTokens    int64                    `json:"total_output_tokens"`
	ByModel              []ChatCostModelBreakdown `json:"by_model"`
	ByChat               []ChatCostChatBreakdown  `json:"by_chat"`
}

// ChatCostModelBreakdown contains per-model cost aggregation.
type ChatCostModelBreakdown struct {
	ModelConfigID     uuid.UUID `json:"model_config_id" format:"uuid"`
	DisplayName       string    `json:"display_name"`
	Provider          string    `json:"provider"`
	Model             string    `json:"model"`
	TotalCostMicros   int64     `json:"total_cost_micros"`
	MessageCount      int64     `json:"message_count"`
	TotalInputTokens  int64     `json:"total_input_tokens"`
	TotalOutputTokens int64     `json:"total_output_tokens"`
}

// ChatCostChatBreakdown contains per-root-chat cost aggregation.
type ChatCostChatBreakdown struct {
	RootChatID        uuid.UUID `json:"root_chat_id" format:"uuid"`
	ChatTitle         string    `json:"chat_title"`
	TotalCostMicros   int64     `json:"total_cost_micros"`
	MessageCount      int64     `json:"message_count"`
	TotalInputTokens  int64     `json:"total_input_tokens"`
	TotalOutputTokens int64     `json:"total_output_tokens"`
}

// ChatCostUserRollup contains per-user cost aggregation for admin views.
type ChatCostUserRollup struct {
	UserID            uuid.UUID `json:"user_id" format:"uuid"`
	Username          string    `json:"username"`
	Name              string    `json:"name"`
	AvatarURL         string    `json:"avatar_url"`
	TotalCostMicros   int64     `json:"total_cost_micros"`
	MessageCount      int64     `json:"message_count"`
	ChatCount         int64     `json:"chat_count"`
	TotalInputTokens  int64     `json:"total_input_tokens"`
	TotalOutputTokens int64     `json:"total_output_tokens"`
}

// ChatCostUsersResponse is the response from the admin chat cost users endpoint.
type ChatCostUsersResponse struct {
	StartDate time.Time            `json:"start_date" format:"date-time"`
	EndDate   time.Time            `json:"end_date" format:"date-time"`
	Count     int64                `json:"count"`
	Users     []ChatCostUserRollup `json:"users"`
}

// ListChatsOptions are optional parameters for ListChats.
type ListChatsOptions struct {
	Query string
	Pagination
}

// ListChats returns all chats for the authenticated user.
func (c *Client) ListChats(ctx context.Context, opts *ListChatsOptions) ([]Chat, error) {
	var reqOpts []RequestOption
	if opts != nil {
		reqOpts = append(reqOpts, opts.Pagination.asRequestOption())
		if opts.Query != "" {
			reqOpts = append(reqOpts, func(r *http.Request) {
				q := r.URL.Query()
				q.Set("q", opts.Query)
				r.URL.RawQuery = q.Encode()
			})
		}
	}
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/chats", nil, reqOpts...)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var chats []Chat
	return chats, json.NewDecoder(res.Body).Decode(&chats)
}

// ListChatModels returns the available chat model catalog.
func (c *Client) ListChatModels(ctx context.Context) (ChatModelsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/chats/models", nil)
	if err != nil {
		return ChatModelsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatModelsResponse{}, ReadBodyAsError(res)
	}

	var catalog ChatModelsResponse
	return catalog, json.NewDecoder(res.Body).Decode(&catalog)
}

// ListChatProviders returns admin-managed chat provider configs.
func (c *Client) ListChatProviders(ctx context.Context) ([]ChatProviderConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/chats/providers", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var providers []ChatProviderConfig
	return providers, json.NewDecoder(res.Body).Decode(&providers)
}

// CreateChatProvider creates an admin-managed chat provider config.
func (c *Client) CreateChatProvider(ctx context.Context, req CreateChatProviderConfigRequest) (ChatProviderConfig, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/chats/providers", req)
	if err != nil {
		return ChatProviderConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return ChatProviderConfig{}, ReadBodyAsError(res)
	}

	var provider ChatProviderConfig
	return provider, json.NewDecoder(res.Body).Decode(&provider)
}

// UpdateChatProvider updates an admin-managed chat provider config.
func (c *Client) UpdateChatProvider(ctx context.Context, providerID uuid.UUID, req UpdateChatProviderConfigRequest) (ChatProviderConfig, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/experimental/chats/providers/%s", providerID), req)
	if err != nil {
		return ChatProviderConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatProviderConfig{}, ReadBodyAsError(res)
	}

	var provider ChatProviderConfig
	return provider, json.NewDecoder(res.Body).Decode(&provider)
}

// DeleteChatProvider deletes an admin-managed chat provider config.
func (c *Client) DeleteChatProvider(ctx context.Context, providerID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/chats/providers/%s", providerID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// ListChatModelConfigs returns admin-managed chat model configs.
func (c *Client) ListChatModelConfigs(ctx context.Context) ([]ChatModelConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/chats/model-configs", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var configs []ChatModelConfig
	return configs, json.NewDecoder(res.Body).Decode(&configs)
}

// CreateChatModelConfig creates an admin-managed chat model config.
func (c *Client) CreateChatModelConfig(ctx context.Context, req CreateChatModelConfigRequest) (ChatModelConfig, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/chats/model-configs", req)
	if err != nil {
		return ChatModelConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return ChatModelConfig{}, ReadBodyAsError(res)
	}

	var config ChatModelConfig
	return config, json.NewDecoder(res.Body).Decode(&config)
}

// UpdateChatModelConfig updates an admin-managed chat model config.
func (c *Client) UpdateChatModelConfig(ctx context.Context, modelConfigID uuid.UUID, req UpdateChatModelConfigRequest) (ChatModelConfig, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/experimental/chats/model-configs/%s", modelConfigID), req)
	if err != nil {
		return ChatModelConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatModelConfig{}, ReadBodyAsError(res)
	}

	var config ChatModelConfig
	return config, json.NewDecoder(res.Body).Decode(&config)
}

// DeleteChatModelConfig deletes an admin-managed chat model config.
func (c *Client) DeleteChatModelConfig(ctx context.Context, modelConfigID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/chats/model-configs/%s", modelConfigID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// GetChatCostSummary returns an aggregate cost summary for the specified
// user. Zero-valued StartDate or EndDate fields are omitted from the
// request, letting the server apply its own defaults (typically the last
// 30 days).
func (c *Client) GetChatCostSummary(ctx context.Context, user string, opts ChatCostSummaryOptions) (ChatCostSummary, error) {
	qp := url.Values{}
	if !opts.StartDate.IsZero() {
		qp.Set("start_date", opts.StartDate.Format(time.RFC3339))
	}
	if !opts.EndDate.IsZero() {
		qp.Set("end_date", opts.EndDate.Format(time.RFC3339))
	}
	reqURL := fmt.Sprintf("/api/experimental/chats/cost/%s/summary", user)
	if len(qp) > 0 {
		reqURL += "?" + qp.Encode()
	}
	res, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return ChatCostSummary{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatCostSummary{}, ReadBodyAsError(res)
	}
	var summary ChatCostSummary
	return summary, json.NewDecoder(res.Body).Decode(&summary)
}

// GetChatCostUsers returns a per-user cost rollup for the deployment
// (admin only). Zero-valued StartDate or EndDate fields are omitted from
// the request, letting the server apply its own defaults (typically the
// last 30 days).
func (c *Client) GetChatCostUsers(ctx context.Context, opts ChatCostUsersOptions) (ChatCostUsersResponse, error) {
	qp := url.Values{}
	if !opts.StartDate.IsZero() {
		qp.Set("start_date", opts.StartDate.Format(time.RFC3339))
	}
	if !opts.EndDate.IsZero() {
		qp.Set("end_date", opts.EndDate.Format(time.RFC3339))
	}
	if opts.Username != "" {
		qp.Set("username", opts.Username)
	}
	if opts.Limit > 0 {
		qp.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset > 0 {
		qp.Set("offset", strconv.Itoa(opts.Offset))
	}
	reqURL := "/api/experimental/chats/cost/users"
	if len(qp) > 0 {
		reqURL += "?" + qp.Encode()
	}
	res, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return ChatCostUsersResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatCostUsersResponse{}, ReadBodyAsError(res)
	}
	var resp ChatCostUsersResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// GetChatSystemPrompt returns the deployment-wide chat system prompt.
func (c *Client) GetChatSystemPrompt(ctx context.Context) (ChatSystemPromptResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/chats/config/system-prompt", nil)
	if err != nil {
		return ChatSystemPromptResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatSystemPromptResponse{}, ReadBodyAsError(res)
	}
	var resp ChatSystemPromptResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpdateChatSystemPrompt updates the deployment-wide chat system prompt.
func (c *Client) UpdateChatSystemPrompt(ctx context.Context, req UpdateChatSystemPromptRequest) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/experimental/chats/config/system-prompt", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// GetUserChatCustomPrompt fetches the user's custom chat prompt.
func (c *Client) GetUserChatCustomPrompt(ctx context.Context) (UserChatCustomPromptResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/chats/config/user-prompt", nil)
	if err != nil {
		return UserChatCustomPromptResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserChatCustomPromptResponse{}, ReadBodyAsError(res)
	}
	var resp UserChatCustomPromptResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpdateUserChatCustomPrompt updates the user's custom chat prompt.
func (c *Client) UpdateUserChatCustomPrompt(ctx context.Context, req UpdateUserChatCustomPromptRequest) (UserChatCustomPromptResponse, error) {
	res, err := c.Request(ctx, http.MethodPut, "/api/experimental/chats/config/user-prompt", req)
	if err != nil {
		return UserChatCustomPromptResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserChatCustomPromptResponse{}, ReadBodyAsError(res)
	}
	var resp UserChatCustomPromptResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// CreateChat creates a new chat.
func (c *Client) CreateChat(ctx context.Context, req CreateChatRequest) (Chat, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/chats", req)
	if err != nil {
		return Chat{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Chat{}, ReadBodyAsError(res)
	}
	var chat Chat
	return chat, json.NewDecoder(res.Body).Decode(&chat)
}

// StreamChatOptions are optional parameters for StreamChat.
type StreamChatOptions struct {
	// AfterID limits the initial snapshot to messages created
	// after the given ID. This is useful for relay connections
	// that only need live message_part events and can skip the
	// full message history.
	AfterID *int64
}

// StreamChat streams chat updates in real time.
//
// The returned channel includes initial snapshot events first, followed by
// live updates. Callers must close the returned io.Closer to release the
// websocket connection when done.
func (c *Client) StreamChat(ctx context.Context, chatID uuid.UUID, opts *StreamChatOptions) (<-chan ChatStreamEvent, io.Closer, error) {
	path := fmt.Sprintf("/api/experimental/chats/%s/stream", chatID)
	if opts != nil && opts.AfterID != nil {
		path += fmt.Sprintf("?after_id=%d", *opts.AfterID)
	}

	conn, err := c.Dial(
		ctx,
		path,
		&websocket.DialOptions{CompressionMode: websocket.CompressionDisabled},
	)
	if err != nil {
		return nil, nil, err
	}
	conn.SetReadLimit(1 << 22) // 4MiB

	streamCtx, streamCancel := context.WithCancel(ctx)
	events := make(chan ChatStreamEvent, 128)

	send := func(event ChatStreamEvent) bool {
		if event.ChatID == uuid.Nil {
			event.ChatID = chatID
		}
		select {
		case <-streamCtx.Done():
			return false
		case events <- event:
			return true
		}
	}

	go func() {
		defer close(events)
		defer streamCancel()
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "")
		}()

		for {
			var envelope chatStreamEnvelope
			if err := wsjson.Read(streamCtx, conn, &envelope); err != nil {
				if streamCtx.Err() != nil {
					return
				}
				switch websocket.CloseStatus(err) {
				case websocket.StatusNormalClosure, websocket.StatusGoingAway:
					return
				}
				_ = send(ChatStreamEvent{
					Type: ChatStreamEventTypeError,
					Error: &ChatStreamError{
						Message: fmt.Sprintf("read chat stream: %v", err),
					},
				})
				return
			}

			switch envelope.Type {
			case ServerSentEventTypePing:
				continue
			case ServerSentEventTypeData:
				var batch []ChatStreamEvent
				decodeErr := json.Unmarshal(envelope.Data, &batch)
				if decodeErr == nil {
					for _, streamedEvent := range batch {
						if !send(streamedEvent) {
							return
						}
					}
					continue
				}

				{
					_ = send(ChatStreamEvent{
						Type: ChatStreamEventTypeError,
						Error: &ChatStreamError{
							Message: fmt.Sprintf(
								"decode chat stream event batch: %v",
								decodeErr,
							),
						},
					})
					return
				}
			case ServerSentEventTypeError:
				message := "chat stream returned an error"
				if len(envelope.Data) > 0 {
					var response Response
					if err := json.Unmarshal(envelope.Data, &response); err == nil {
						message = formatChatStreamResponseError(response)
					} else {
						trimmed := strings.TrimSpace(string(envelope.Data))
						if trimmed != "" {
							message = trimmed
						}
					}
				}
				_ = send(ChatStreamEvent{
					Type: ChatStreamEventTypeError,
					Error: &ChatStreamError{
						Message: message,
					},
				})
				return
			default:
				_ = send(ChatStreamEvent{
					Type: ChatStreamEventTypeError,
					Error: &ChatStreamError{
						Message: fmt.Sprintf("unknown chat stream event type %q", envelope.Type),
					},
				})
				return
			}
		}
	}()

	return events, closeFunc(func() error {
		streamCancel()
		return nil
	}), nil
}

// GetChat returns a chat by ID.
func (c *Client) GetChat(ctx context.Context, chatID uuid.UUID) (Chat, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/chats/%s", chatID), nil)
	if err != nil {
		return Chat{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Chat{}, ReadBodyAsError(res)
	}
	var chat Chat
	return chat, json.NewDecoder(res.Body).Decode(&chat)
}

// GetChatMessages returns the messages and queued messages for a chat.
func (c *Client) GetChatMessages(ctx context.Context, chatID uuid.UUID) (ChatMessagesResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/chats/%s/messages", chatID), nil)
	if err != nil {
		return ChatMessagesResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatMessagesResponse{}, ReadBodyAsError(res)
	}
	var resp ChatMessagesResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) ArchiveChat(ctx context.Context, chatID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/chats/%s/archive", chatID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) UnarchiveChat(ctx context.Context, chatID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/chats/%s/unarchive", chatID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// CreateChatMessage adds a message to a chat.
func (c *Client) CreateChatMessage(ctx context.Context, chatID uuid.UUID, req CreateChatMessageRequest) (CreateChatMessageResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/chats/%s/messages", chatID), req)
	if err != nil {
		return CreateChatMessageResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return CreateChatMessageResponse{}, ReadBodyAsError(res)
	}
	var resp CreateChatMessageResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// EditChatMessage edits an existing user message in a chat and re-runs from there.
func (c *Client) EditChatMessage(
	ctx context.Context,
	chatID uuid.UUID,
	messageID int64,
	req EditChatMessageRequest,
) (ChatMessage, error) {
	res, err := c.Request(
		ctx,
		http.MethodPatch,
		fmt.Sprintf("/api/experimental/chats/%s/messages/%d", chatID, messageID),
		req,
	)
	if err != nil {
		return ChatMessage{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatMessage{}, ReadBodyAsError(res)
	}
	var message ChatMessage
	return message, json.NewDecoder(res.Body).Decode(&message)
}

// InterruptChat cancels an in-flight chat run and leaves it waiting.
func (c *Client) InterruptChat(ctx context.Context, chatID uuid.UUID) (Chat, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/chats/%s/interrupt", chatID), nil)
	if err != nil {
		return Chat{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Chat{}, ReadBodyAsError(res)
	}
	var chat Chat
	return chat, json.NewDecoder(res.Body).Decode(&chat)
}

// GetChatGitChanges returns git changes for a chat.
func (c *Client) GetChatGitChanges(ctx context.Context, chatID uuid.UUID) ([]ChatGitChange, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/chats/%s/git-changes", chatID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var changes []ChatGitChange
	return changes, json.NewDecoder(res.Body).Decode(&changes)
}

// GetChatDiffStatus returns cached GitHub pull request diff status for a chat.
func (c *Client) GetChatDiffStatus(ctx context.Context, chatID uuid.UUID) (ChatDiffStatus, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/chats/%s/diff-status", chatID), nil)
	if err != nil {
		return ChatDiffStatus{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatDiffStatus{}, ReadBodyAsError(res)
	}
	var status ChatDiffStatus
	return status, json.NewDecoder(res.Body).Decode(&status)
}

// GetChatDiffContents returns resolved diff contents for a chat.
func (c *Client) GetChatDiffContents(ctx context.Context, chatID uuid.UUID) (ChatDiffContents, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/chats/%s/diff", chatID), nil)
	if err != nil {
		return ChatDiffContents{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatDiffContents{}, ReadBodyAsError(res)
	}
	var diff ChatDiffContents
	return diff, json.NewDecoder(res.Body).Decode(&diff)
}

// UploadChatFile uploads a file for use in chat messages.
func (c *Client) UploadChatFile(ctx context.Context, organizationID uuid.UUID, contentType string, filename string, rd io.Reader) (UploadChatFileResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/chats/files?organization=%s", organizationID), rd, func(r *http.Request) {
		r.Header.Set("Content-Type", contentType)
		if filename != "" {
			r.Header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		}
	})
	if err != nil {
		return UploadChatFileResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return UploadChatFileResponse{}, ReadBodyAsError(res)
	}
	var resp UploadChatFileResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// GetChatFile retrieves a previously uploaded chat file by ID.
func (c *Client) GetChatFile(ctx context.Context, fileID uuid.UUID) ([]byte, string, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/chats/files/%s", fileID), nil)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", ReadBodyAsError(res)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	return data, res.Header.Get("Content-Type"), nil
}

func formatChatStreamResponseError(response Response) string {
	message := strings.TrimSpace(response.Message)
	detail := strings.TrimSpace(response.Detail)
	switch {
	case message == "" && detail == "":
		return "chat stream returned an error"
	case message == "":
		return detail
	case detail == "":
		return message
	default:
		return fmt.Sprintf("%s: %s", message, detail)
	}
}
