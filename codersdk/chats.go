package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

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
	ModelConfigID *uuid.UUID        `json:"model_config_id,omitempty" format:"uuid"`
	CreatedAt     time.Time         `json:"created_at" format:"date-time"`
	Role          string            `json:"role"`
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

// ChatMessagePartType represents a structured message part type.
type ChatMessagePartType string

const (
	ChatMessagePartTypeText       ChatMessagePartType = "text"
	ChatMessagePartTypeReasoning  ChatMessagePartType = "reasoning"
	ChatMessagePartTypeToolCall   ChatMessagePartType = "tool-call"
	ChatMessagePartTypeToolResult ChatMessagePartType = "tool-result"
	ChatMessagePartTypeSource     ChatMessagePartType = "source"
	ChatMessagePartTypeFile       ChatMessagePartType = "file"
)

// ChatMessagePart is a structured chunk of a chat message.
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
}

// ChatInputPartType represents an input part type for user chat input.
type ChatInputPartType string

const (
	ChatInputPartTypeText ChatInputPartType = "text"
)

// ChatInputPart is a single user input part for creating a chat.
type ChatInputPart struct {
	Type ChatInputPartType `json:"type"`
	Text string            `json:"text,omitempty"`
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

// ChatWithMessages is a chat along with its messages.
type ChatWithMessages struct {
	Chat           Chat                `json:"chat"`
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
	Include             []string         `json:"include,omitempty"`
	Instructions        *string          `json:"instructions,omitempty"`
	LogitBias           map[string]int64 `json:"logit_bias,omitempty"`
	LogProbs            *bool            `json:"log_probs,omitempty"`
	TopLogProbs         *int64           `json:"top_log_probs,omitempty"`
	MaxToolCalls        *int64           `json:"max_tool_calls,omitempty"`
	ParallelToolCalls   *bool            `json:"parallel_tool_calls,omitempty"`
	User                *string          `json:"user,omitempty"`
	ReasoningEffort     *string          `json:"reasoning_effort,omitempty"`
	ReasoningSummary    *string          `json:"reasoning_summary,omitempty"`
	MaxCompletionTokens *int64           `json:"max_completion_tokens,omitempty"`
	TextVerbosity       *string          `json:"text_verbosity,omitempty"`
	Prediction          map[string]any   `json:"prediction,omitempty"`
	Store               *bool            `json:"store,omitempty"`
	Metadata            map[string]any   `json:"metadata,omitempty"`
	PromptCacheKey      *string          `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier    *string          `json:"safety_identifier,omitempty"`
	ServiceTier         *string          `json:"service_tier,omitempty"`
	StructuredOutputs   *bool            `json:"structured_outputs,omitempty"`
	StrictJSONSchema    *bool            `json:"strict_json_schema,omitempty"`
}

// ChatModelAnthropicThinkingOptions configures Anthropic thinking budget.
type ChatModelAnthropicThinkingOptions struct {
	BudgetTokens *int64 `json:"budget_tokens,omitempty"`
}

// ChatModelAnthropicProviderOptions configures Anthropic provider behavior.
type ChatModelAnthropicProviderOptions struct {
	SendReasoning          *bool                              `json:"send_reasoning,omitempty"`
	Thinking               *ChatModelAnthropicThinkingOptions `json:"thinking,omitempty"`
	Effort                 *string                            `json:"effort,omitempty"`
	DisableParallelToolUse *bool                              `json:"disable_parallel_tool_use,omitempty"`
}

// ChatModelGoogleThinkingConfig configures Google thinking behavior.
type ChatModelGoogleThinkingConfig struct {
	ThinkingBudget  *int64 `json:"thinking_budget,omitempty"`
	IncludeThoughts *bool  `json:"include_thoughts,omitempty"`
}

// ChatModelGoogleSafetySetting configures Google safety filtering.
type ChatModelGoogleSafetySetting struct {
	Category  string `json:"category,omitempty"`
	Threshold string `json:"threshold,omitempty"`
}

// ChatModelGoogleProviderOptions configures Google provider behavior.
type ChatModelGoogleProviderOptions struct {
	ThinkingConfig *ChatModelGoogleThinkingConfig `json:"thinking_config,omitempty"`
	CachedContent  string                         `json:"cached_content,omitempty"`
	SafetySettings []ChatModelGoogleSafetySetting `json:"safety_settings,omitempty"`
	Threshold      string                         `json:"threshold,omitempty"`
}

// ChatModelOpenAICompatProviderOptions configures OpenAI-compatible behavior.
type ChatModelOpenAICompatProviderOptions struct {
	User            *string `json:"user,omitempty"`
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
}

// ChatModelOpenRouterReasoningOptions configures OpenRouter reasoning behavior.
type ChatModelOpenRouterReasoningOptions struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	Exclude   *bool   `json:"exclude,omitempty"`
	MaxTokens *int64  `json:"max_tokens,omitempty"`
	Effort    *string `json:"effort,omitempty"`
}

// ChatModelOpenRouterProvider configures OpenRouter routing preferences.
type ChatModelOpenRouterProvider struct {
	Order             []string `json:"order,omitempty"`
	AllowFallbacks    *bool    `json:"allow_fallbacks,omitempty"`
	RequireParameters *bool    `json:"require_parameters,omitempty"`
	DataCollection    *string  `json:"data_collection,omitempty"`
	Only              []string `json:"only,omitempty"`
	Ignore            []string `json:"ignore,omitempty"`
	Quantizations     []string `json:"quantizations,omitempty"`
	Sort              *string  `json:"sort,omitempty"`
}

// ChatModelOpenRouterProviderOptions configures OpenRouter provider behavior.
type ChatModelOpenRouterProviderOptions struct {
	Reasoning         *ChatModelOpenRouterReasoningOptions `json:"reasoning,omitempty"`
	ExtraBody         map[string]any                       `json:"extra_body,omitempty"`
	IncludeUsage      *bool                                `json:"include_usage,omitempty"`
	LogitBias         map[string]int64                     `json:"logit_bias,omitempty"`
	LogProbs          *bool                                `json:"log_probs,omitempty"`
	ParallelToolCalls *bool                                `json:"parallel_tool_calls,omitempty"`
	User              *string                              `json:"user,omitempty"`
	Provider          *ChatModelOpenRouterProvider         `json:"provider,omitempty"`
}

// ChatModelVercelReasoningOptions configures Vercel reasoning behavior.
type ChatModelVercelReasoningOptions struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	MaxTokens *int64  `json:"max_tokens,omitempty"`
	Effort    *string `json:"effort,omitempty"`
	Exclude   *bool   `json:"exclude,omitempty"`
}

// ChatModelVercelGatewayProviderOptions configures Vercel routing behavior.
type ChatModelVercelGatewayProviderOptions struct {
	Order  []string `json:"order,omitempty"`
	Models []string `json:"models,omitempty"`
}

// ChatModelVercelProviderOptions configures Vercel provider behavior.
type ChatModelVercelProviderOptions struct {
	Reasoning         *ChatModelVercelReasoningOptions       `json:"reasoning,omitempty"`
	ProviderOptions   *ChatModelVercelGatewayProviderOptions `json:"providerOptions,omitempty"`
	User              *string                                `json:"user,omitempty"`
	LogitBias         map[string]int64                       `json:"logit_bias,omitempty"`
	LogProbs          *bool                                  `json:"logprobs,omitempty"`
	TopLogProbs       *int64                                 `json:"top_logprobs,omitempty"`
	ParallelToolCalls *bool                                  `json:"parallel_tool_calls,omitempty"`
	ExtraBody         map[string]any                         `json:"extra_body,omitempty"`
}

// ChatModelCallConfig configures per-call model behavior defaults.
type ChatModelCallConfig struct {
	MaxOutputTokens  *int64                    `json:"max_output_tokens,omitempty"`
	Temperature      *float64                  `json:"temperature,omitempty"`
	TopP             *float64                  `json:"top_p,omitempty"`
	TopK             *int64                    `json:"top_k,omitempty"`
	PresencePenalty  *float64                  `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64                  `json:"frequency_penalty,omitempty"`
	ProviderOptions  *ChatModelProviderOptions `json:"provider_options,omitempty"`
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
	ChangesRequested bool       `json:"changes_requested"`
	Additions        int32      `json:"additions"`
	Deletions        int32      `json:"deletions"`
	ChangedFiles     int32      `json:"changed_files"`
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
	Role string          `json:"role,omitempty"`
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

// ListChats returns all chats for the authenticated user.
func (c *Client) ListChats(ctx context.Context) ([]Chat, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/chats", nil)
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

// StreamChat streams chat updates in real time.
//
// The returned channel includes initial snapshot events first, followed by
// live updates. Callers must close the returned io.Closer to release the
// websocket connection when done.
func (c *Client) StreamChat(ctx context.Context, chatID uuid.UUID) (<-chan ChatStreamEvent, io.Closer, error) {
	conn, err := c.Dial(
		ctx,
		fmt.Sprintf("/api/experimental/chats/%s/stream", chatID),
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
		return conn.Close(websocket.StatusNormalClosure, "")
	}), nil
}

// GetChat returns a chat by ID, including its messages.
func (c *Client) GetChat(ctx context.Context, chatID uuid.UUID) (ChatWithMessages, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/chats/%s", chatID), nil)
	if err != nil {
		return ChatWithMessages{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatWithMessages{}, ReadBodyAsError(res)
	}
	var chat ChatWithMessages
	return chat, json.NewDecoder(res.Body).Decode(&chat)
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
