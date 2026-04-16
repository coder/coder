package agentsdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

const chatRunnerEndpointBase = "/api/v2/workspaceagents/me/experimental/chat-runner"

// ChatRunnerRuntimeContextRequest requests the prompt-ready runtime context
// for a workspace-agent-authenticated chat runner session.
type ChatRunnerRuntimeContextRequest struct {
	ChatID uuid.UUID `json:"chat_id"`
}

// ChatRunnerMessage is a prompt-ready message in the chat runner wire format.
type ChatRunnerMessage struct {
	Role string `json:"role"`
	// Content stores structured parts when the message contains rich content.
	Content []codersdk.ChatMessagePart `json:"content,omitempty"`
	// Text stores a simple text-only message payload.
	Text string `json:"text,omitempty"`
}

// ChatRunnerToolDefinition describes a tool the agent can offer to the LLM.
type ChatRunnerToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	// ProviderConfig stores provider-native tool configuration without
	// including any credential-bearing fields.
	ProviderConfig json.RawMessage `json:"provider_config,omitempty"`
}

// ChatRunnerSkillMeta describes a skill available in the workspace.
type ChatRunnerSkillMeta struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
}

// ChatRunnerMCPTool describes an MCP tool discovered during runtime context
// resolution. The Name field carries the prefixed format
// (serverSlug__toolName) as returned by mcpclient.ConnectAll.
type ChatRunnerMCPTool struct {
	MCPServerConfigID uuid.UUID       `json:"mcp_server_config_id"`
	ToolName          string          `json:"tool_name"`
	Description       string          `json:"description,omitempty"`
	InputSchema       json.RawMessage `json:"input_schema,omitempty"`
	ServerDisplayName string          `json:"server_display_name"`
}

// ChatRunnerRuntimeContextResponse contains the model configuration,
// prompt-ready messages, and tool metadata needed to execute a chat runner
// step.
type ChatRunnerRuntimeContextResponse struct {
	ChatID       uuid.UUID `json:"chat_id"`
	ParentChatID uuid.UUID `json:"parent_chat_id,omitempty"`

	Provider string `json:"provider"`
	Model    string `json:"model"`

	// ProviderAPIKeys contains only LLM provider API keys that the agent uses
	// to call the model. MCP credentials must never appear here.
	ProviderAPIKeys map[string]string `json:"provider_api_keys"`
	// ProviderBaseURLs stores per-provider base URLs, such as custom OpenAI
	// compatible endpoints.
	ProviderBaseURLs map[string]string `json:"provider_base_urls,omitempty"`

	CallConfig codersdk.ChatModelCallConfig `json:"call_config"`

	ContextLimit int64 `json:"context_limit"`

	CompactionThresholdPercent int32 `json:"compaction_threshold_percent"`

	Messages []ChatRunnerMessage `json:"messages"`

	SystemInstruction string `json:"system_instruction,omitempty"`
	UserPrompt        string `json:"user_prompt,omitempty"`
	IsSubagent        bool   `json:"is_subagent"`

	BuiltinTools  []ChatRunnerToolDefinition `json:"builtin_tools,omitempty"`
	ProviderTools []ChatRunnerToolDefinition `json:"provider_tools,omitempty"`
	DynamicTools  []ChatRunnerToolDefinition `json:"dynamic_tools,omitempty"`

	Skills   []ChatRunnerSkillMeta `json:"skills,omitempty"`
	MCPTools []ChatRunnerMCPTool   `json:"mcp_tools,omitempty"`

	ModelConfigID uuid.UUID `json:"model_config_id"`

	LeaseEpoch int64 `json:"lease_epoch"`
}

// ChatRunnerUsage contains token usage metrics for a persisted chat runner
// step.
type ChatRunnerUsage struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	ReasoningTokens     int64 `json:"reasoning_tokens,omitempty"`
	CacheCreationTokens int64 `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int64 `json:"cache_read_tokens,omitempty"`
}

// ChatRunnerPersistStepRequest persists a completed assistant/tool-result step
// for the current lease owner.
type ChatRunnerPersistStepRequest struct {
	ChatID     uuid.UUID `json:"chat_id"`
	LeaseEpoch int64     `json:"lease_epoch"`

	AssistantParts []codersdk.ChatMessagePart `json:"assistant_parts,omitempty"`
	ToolResults    []codersdk.ChatMessagePart `json:"tool_results,omitempty"`

	Usage *ChatRunnerUsage `json:"usage,omitempty"`

	ContextLimit       *int64 `json:"context_limit,omitempty"`
	ProviderResponseID string `json:"provider_response_id,omitempty"`
	RuntimeMs          int64  `json:"runtime_ms,omitempty"`

	ToolNameToConfigID map[string]uuid.UUID `json:"tool_name_to_config_id,omitempty"`

	ModelConfigID uuid.UUID `json:"model_config_id"`
}

// ChatRunnerPersistStepResponse reports whether a completed step was persisted.
type ChatRunnerPersistStepResponse struct {
	OK bool `json:"ok"`
}

// ChatRunnerMCPToolCallRequest proxies an MCP tool invocation through coderd
// so credentials never leave the server.
type ChatRunnerMCPToolCallRequest struct {
	ChatID            uuid.UUID       `json:"chat_id"`
	LeaseEpoch        int64           `json:"lease_epoch"`
	MCPServerConfigID uuid.UUID       `json:"mcp_server_config_id"`
	ToolName          string          `json:"tool_name"`
	Args              json.RawMessage `json:"args"`
}

// ChatRunnerMCPToolCallResponse wraps the result of a proxied MCP tool call.
type ChatRunnerMCPToolCallResponse struct {
	Result  json.RawMessage `json:"result"`
	IsError bool            `json:"is_error"`
}

// ChatRunnerPublishStreamPartRequest publishes a transient stream part to chat
// UI subscribers for the current lease owner.
type ChatRunnerPublishStreamPartRequest struct {
	ChatID     uuid.UUID                `json:"chat_id"`
	LeaseEpoch int64                    `json:"lease_epoch"`
	Role       codersdk.ChatMessageRole `json:"role"`
	Part       codersdk.ChatMessagePart `json:"part"`
	// ToolNameToConfigID optionally annotates tool-related parts with MCP
	// server config IDs. It must not include any credential-bearing data.
	ToolNameToConfigID map[string]uuid.UUID `json:"tool_name_to_config_id,omitempty"`
}

// ChatRunnerPublishStreamPartResponse reports whether the transient stream part
// was published.
type ChatRunnerPublishStreamPartResponse struct {
	OK bool `json:"ok"`
}

// ChatRunnerPublishStreamPart is a single stream-part item within a batch
// publish request.
type ChatRunnerPublishStreamPart struct {
	Role               codersdk.ChatMessageRole `json:"role"`
	Part               codersdk.ChatMessagePart `json:"part"`
	ToolNameToConfigID map[string]uuid.UUID     `json:"tool_name_to_config_id,omitempty"`
}

// ChatRunnerPublishStreamPartsRequest publishes multiple stream parts in one
// round trip.
type ChatRunnerPublishStreamPartsRequest struct {
	ChatID     uuid.UUID                     `json:"chat_id"`
	LeaseEpoch int64                         `json:"lease_epoch"`
	Parts      []ChatRunnerPublishStreamPart `json:"parts"`
}

// ChatRunnerPublishStreamPartsResponse reports whether the batch was
// published.
type ChatRunnerPublishStreamPartsResponse struct {
	OK bool `json:"ok"`
}

// ChatRunnerReloadMessagesRequest reloads prompt-ready chat history for the
// current lease owner after retry or compaction.
type ChatRunnerReloadMessagesRequest struct {
	ChatID     uuid.UUID `json:"chat_id"`
	LeaseEpoch int64     `json:"lease_epoch"`
}

// ChatRunnerReloadMessagesResponse contains refreshed prompt-ready message
// history and instruction metadata.
type ChatRunnerReloadMessagesResponse struct {
	Messages          []ChatRunnerMessage   `json:"messages"`
	SystemInstruction string                `json:"system_instruction,omitempty"`
	UserPrompt        string                `json:"user_prompt,omitempty"`
	IsSubagent        bool                  `json:"is_subagent"`
	Skills            []ChatRunnerSkillMeta `json:"skills,omitempty"`
}

// ChatRunnerListTemplatesRequest requests a page of templates available to the
// current lease owner.
type ChatRunnerListTemplatesRequest struct {
	ChatID     uuid.UUID `json:"chat_id"`
	LeaseEpoch int64     `json:"lease_epoch"`
	Query      string    `json:"query,omitempty"`
	Page       int       `json:"page,omitempty"`
}

// ChatRunnerListTemplatesResponse contains a page of templates available to
// the current lease owner.
type ChatRunnerListTemplatesResponse struct {
	Templates  []ChatRunnerTemplate `json:"templates"`
	TotalCount int                  `json:"total_count"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
}

// ChatRunnerTemplate describes a template available to the chat runner.
type ChatRunnerTemplate struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name,omitempty"`
	Description string    `json:"description,omitempty"`
	Icon        string    `json:"icon,omitempty"`
}

// ChatRunnerReadTemplateRequest requests the details for a single template
// available to the current lease owner.
type ChatRunnerReadTemplateRequest struct {
	ChatID     uuid.UUID `json:"chat_id"`
	LeaseEpoch int64     `json:"lease_epoch"`
	TemplateID uuid.UUID `json:"template_id"`
}

// ChatRunnerReadTemplateResponse contains a template definition and its
// parameters.
type ChatRunnerReadTemplateResponse struct {
	Template   ChatRunnerTemplate            `json:"template"`
	Parameters []ChatRunnerTemplateParameter `json:"parameters"`
}

// ChatRunnerTemplateParameter describes an input parameter accepted by a
// template.
type ChatRunnerTemplateParameter struct {
	Name         string                              `json:"name"`
	DisplayName  string                              `json:"display_name,omitempty"`
	Description  string                              `json:"description,omitempty"`
	Type         string                              `json:"type"`
	DefaultValue string                              `json:"default_value,omitempty"`
	Required     bool                                `json:"required"`
	Mutable      bool                                `json:"mutable"`
	Options      []ChatRunnerTemplateParameterOption `json:"options,omitempty"`
}

// ChatRunnerTemplateParameterOption describes a single selectable value for a
// template parameter.
type ChatRunnerTemplateParameterOption struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value"`
}

// ChatRunnerRuntimeContext fetches the prompt-ready runtime context for a chat
// runner session.
func (c *Client) ChatRunnerRuntimeContext(ctx context.Context, req ChatRunnerRuntimeContextRequest) (ChatRunnerRuntimeContextResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, chatRunnerEndpointBase+"/runtime-context", req)
	if err != nil {
		return ChatRunnerRuntimeContextResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatRunnerRuntimeContextResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp ChatRunnerRuntimeContextResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ChatRunnerPersistStep persists a completed assistant/tool-result step for a
// chat runner session.
func (c *Client) ChatRunnerPersistStep(ctx context.Context, req ChatRunnerPersistStepRequest) (ChatRunnerPersistStepResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, chatRunnerEndpointBase+"/persist-step", req)
	if err != nil {
		return ChatRunnerPersistStepResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatRunnerPersistStepResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp ChatRunnerPersistStepResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ChatRunnerMCPToolCall proxies an MCP tool invocation through the coderd
// gateway. Credentials remain server-side; only the normalized result is
// returned.
func (c *Client) ChatRunnerMCPToolCall(ctx context.Context, req ChatRunnerMCPToolCallRequest) (ChatRunnerMCPToolCallResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, chatRunnerEndpointBase+"/mcp-tool-call", req)
	if err != nil {
		return ChatRunnerMCPToolCallResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatRunnerMCPToolCallResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp ChatRunnerMCPToolCallResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ChatRunnerPublishStreamPart publishes a transient stream part to chat UI
// subscribers.
func (c *Client) ChatRunnerPublishStreamPart(ctx context.Context, req ChatRunnerPublishStreamPartRequest) (ChatRunnerPublishStreamPartResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, chatRunnerEndpointBase+"/publish-stream-part", req)
	if err != nil {
		return ChatRunnerPublishStreamPartResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatRunnerPublishStreamPartResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp ChatRunnerPublishStreamPartResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ChatRunnerPublishStreamParts publishes transient stream parts to chat UI
// subscribers.
func (c *Client) ChatRunnerPublishStreamParts(ctx context.Context, req ChatRunnerPublishStreamPartsRequest) (ChatRunnerPublishStreamPartsResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, chatRunnerEndpointBase+"/publish-stream-parts", req)
	if err != nil {
		return ChatRunnerPublishStreamPartsResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatRunnerPublishStreamPartsResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp ChatRunnerPublishStreamPartsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ChatRunnerReloadMessages reloads prompt-ready message history after retry or
// compaction.
func (c *Client) ChatRunnerReloadMessages(ctx context.Context, req ChatRunnerReloadMessagesRequest) (ChatRunnerReloadMessagesResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, chatRunnerEndpointBase+"/reload-messages", req)
	if err != nil {
		return ChatRunnerReloadMessagesResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatRunnerReloadMessagesResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp ChatRunnerReloadMessagesResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ChatRunnerListTemplates lists available templates for the current lease
// owner.
func (c *Client) ChatRunnerListTemplates(ctx context.Context, req ChatRunnerListTemplatesRequest) (ChatRunnerListTemplatesResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, chatRunnerEndpointBase+"/list-templates", req)
	if err != nil {
		return ChatRunnerListTemplatesResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatRunnerListTemplatesResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp ChatRunnerListTemplatesResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ChatRunnerReadTemplate reads a template definition for the current lease
// owner.
func (c *Client) ChatRunnerReadTemplate(ctx context.Context, req ChatRunnerReadTemplateRequest) (ChatRunnerReadTemplateResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, chatRunnerEndpointBase+"/read-template", req)
	if err != nil {
		return ChatRunnerReadTemplateResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ChatRunnerReadTemplateResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp ChatRunnerReadTemplateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
