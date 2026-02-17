package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
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
	ID               uuid.UUID       `json:"id" format:"uuid"`
	OwnerID          uuid.UUID       `json:"owner_id" format:"uuid"`
	WorkspaceID      *uuid.UUID      `json:"workspace_id,omitempty" format:"uuid"`
	WorkspaceAgentID *uuid.UUID      `json:"workspace_agent_id,omitempty" format:"uuid"`
	Title            string          `json:"title"`
	Status           ChatStatus      `json:"status"`
	DiffStatus       *ChatDiffStatus `json:"diff_status,omitempty"`
	ModelConfig      json.RawMessage `json:"model_config,omitempty"`
	CreatedAt        time.Time       `json:"created_at" format:"date-time"`
	UpdatedAt        time.Time       `json:"updated_at" format:"date-time"`
}

// ChatMessage represents a single message in a chat.
type ChatMessage struct {
	ID         int64             `json:"id"`
	ChatID     uuid.UUID         `json:"chat_id" format:"uuid"`
	CreatedAt  time.Time         `json:"created_at" format:"date-time"`
	Role       string            `json:"role"`
	Content    json.RawMessage   `json:"content,omitempty"`
	Parts      []ChatMessagePart `json:"parts,omitempty"`
	ToolCallID *string           `json:"tool_call_id,omitempty"`
	Thinking   *string           `json:"thinking,omitempty"`
	Hidden     bool              `json:"hidden"`
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

// ChatToolResultMetadata exposes commonly used tool-result fields for rendering.
type ChatToolResultMetadata struct {
	Error            string `json:"error,omitempty"`
	Output           string `json:"output,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Content          string `json:"content,omitempty"`
	MimeType         string `json:"mime_type,omitempty"`
	Created          *bool  `json:"created,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	WorkspaceAgentID string `json:"workspace_agent_id,omitempty"`
	WorkspaceName    string `json:"workspace_name,omitempty"`
	WorkspaceURL     string `json:"workspace_url,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// ChatMessagePart is a structured chunk of a chat message.
type ChatMessagePart struct {
	Type        ChatMessagePartType     `json:"type"`
	Text        string                  `json:"text,omitempty"`
	Signature   string                  `json:"signature,omitempty"`
	ToolCallID  string                  `json:"tool_call_id,omitempty"`
	ToolName    string                  `json:"tool_name,omitempty"`
	Args        json.RawMessage         `json:"args,omitempty"`
	ArgsDelta   string                  `json:"args_delta,omitempty"`
	Result      json.RawMessage         `json:"result,omitempty"`
	ResultDelta string                  `json:"result_delta,omitempty"`
	IsError     bool                    `json:"is_error,omitempty"`
	ResultMeta  *ChatToolResultMetadata `json:"result_meta,omitempty"`
	SourceID    string                  `json:"source_id,omitempty"`
	URL         string                  `json:"url,omitempty"`
	Title       string                  `json:"title,omitempty"`
	MediaType   string                  `json:"media_type,omitempty"`
	Data        []byte                  `json:"data,omitempty"`
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

// ChatInput is the structured user input payload for chat creation.
type ChatInput struct {
	Parts []ChatInputPart `json:"parts"`
}

// CreateChatRequest is the request to create a new chat.
type CreateChatRequest struct {
	Input            *ChatInput      `json:"input,omitempty"`
	Message          string          `json:"message,omitempty"`
	SystemPrompt     string          `json:"system_prompt,omitempty"`
	WorkspaceID      *uuid.UUID      `json:"workspace_id,omitempty" format:"uuid"`
	WorkspaceAgentID *uuid.UUID      `json:"workspace_agent_id,omitempty" format:"uuid"`
	Model            string          `json:"model,omitempty"`
	ModelConfig      json.RawMessage `json:"model_config,omitempty"`
}

// UpdateChatRequest is the request to update a chat.
type UpdateChatRequest struct {
	Title string `json:"title"`
}

// CreateChatMessageRequest is the request to add a message to a chat.
type CreateChatMessageRequest struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCallID *string         `json:"tool_call_id,omitempty"`
	Thinking   *string         `json:"thinking,omitempty"`
}

// ChatWithMessages is a chat along with its messages.
type ChatWithMessages struct {
	Chat     Chat          `json:"chat"`
	Messages []ChatMessage `json:"messages"`
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

// ChatProviderConfig is an admin-managed provider configuration.
type ChatProviderConfig struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Provider    string    `json:"provider"`
	DisplayName string    `json:"display_name"`
	Enabled     bool      `json:"enabled"`
	HasAPIKey   bool      `json:"has_api_key"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

// CreateChatProviderConfigRequest creates a chat provider config.
type CreateChatProviderConfigRequest struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

// UpdateChatProviderConfigRequest updates a chat provider config.
type UpdateChatProviderConfigRequest struct {
	DisplayName string  `json:"display_name,omitempty"`
	APIKey      *string `json:"api_key,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

// ChatModelConfig is an admin-managed model configuration.
type ChatModelConfig struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	DisplayName string    `json:"display_name"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

// CreateChatModelConfigRequest creates a chat model config.
type CreateChatModelConfigRequest struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	DisplayName string `json:"display_name,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

// UpdateChatModelConfigRequest updates a chat model config.
type UpdateChatModelConfigRequest struct {
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
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
)

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

// ChatStreamEvent represents a real-time update for chat streaming.
type ChatStreamEvent struct {
	Type        ChatStreamEventType    `json:"type"`
	ChatID      uuid.UUID              `json:"chat_id" format:"uuid"`
	Message     *ChatMessage           `json:"message,omitempty"`
	MessagePart *ChatStreamMessagePart `json:"message_part,omitempty"`
	Status      *ChatStreamStatus      `json:"status,omitempty"`
	Error       *ChatStreamError       `json:"error,omitempty"`
}

// ListChats returns all chats for the authenticated user.
func (c *Client) ListChats(ctx context.Context) ([]Chat, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/chats", nil)
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
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/chats/models", nil)
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
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/chats/providers", nil)
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
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/chats/providers", req)
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
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/chats/providers/%s", providerID), req)
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
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/chats/providers/%s", providerID), nil)
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
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/chats/model-configs", nil)
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
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/chats/model-configs", req)
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
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/chats/model-configs/%s", modelConfigID), req)
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
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/chats/model-configs/%s", modelConfigID), nil)
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
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/chats", req)
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

// GetChat returns a chat by ID, including its messages.
func (c *Client) GetChat(ctx context.Context, chatID uuid.UUID) (ChatWithMessages, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s", chatID), nil)
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

// DeleteChat deletes a chat by ID.
func (c *Client) DeleteChat(ctx context.Context, chatID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/chats/%s", chatID), nil)
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
func (c *Client) CreateChatMessage(ctx context.Context, chatID uuid.UUID, req CreateChatMessageRequest) ([]ChatMessage, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/%s/messages", chatID), req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var messages []ChatMessage
	return messages, json.NewDecoder(res.Body).Decode(&messages)
}

// InterruptChat cancels an in-flight chat run and leaves it waiting.
func (c *Client) InterruptChat(ctx context.Context, chatID uuid.UUID) (Chat, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/%s/interrupt", chatID), nil)
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
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s/git-changes", chatID), nil)
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
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s/diff-status", chatID), nil)
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
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/%s/diff", chatID), nil)
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
